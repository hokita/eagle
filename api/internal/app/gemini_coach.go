package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/genai"
)

const (
	discussionTimeout = 30 * time.Second

	maxCoachReplyOutputTokens = 256 + thinkingAllowanceTokens

	summaryBaseOutputTokens     = 1024 + thinkingAllowanceTokens
	summaryTokensPerLearnerRune = 4
	geminiMaxOutputTokens       = 65536

	summaryTokensPerSecond = 50
	maxSummaryTimeout      = 240 * time.Second
)

func summaryOutputBudget(transcript []DiscussionMessage) int32 {
	runes := 0
	for _, m := range transcript {
		if m.Role == "user" {
			runes += utf8.RuneCountInString(m.Text)
		}
	}
	budget := summaryBaseOutputTokens + summaryTokensPerLearnerRune*runes
	if budget > geminiMaxOutputTokens {
		budget = geminiMaxOutputTokens
	}
	return int32(budget)
}

func summaryTimeout(budget int32) time.Duration {
	timeout := discussionTimeout + time.Duration(budget)*time.Second/summaryTokensPerSecond
	if timeout > maxSummaryTimeout {
		timeout = maxSummaryTimeout
	}
	return timeout
}

var coachReplySchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"message": {Type: genai.TypeString},
	},
	Required: []string{"message"},
}

var summarySchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"refined_messages":   {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
		"naturalness_why_en": {Type: genai.TypeString},
		"naturalness_fix_en": {Type: genai.TypeString},
		"phrases": {Type: genai.TypeArray, Items: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"phrase":     {Type: genai.TypeString},
				"meaning_en": {Type: genai.TypeString},
				"example_en": {Type: genai.TypeString},
			},
			Required: []string{"phrase", "meaning_en", "example_en"},
		}},
	},
	Required: []string{"refined_messages", "naturalness_why_en", "naturalness_fix_en", "phrases"},
}

type GeminiCoach struct {
	models contentGenerator
	model  string
}

func NewGeminiCoach(ctx context.Context, apiKey string) (*GeminiCoach, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create genai client: %w", err)
	}
	return &GeminiCoach{models: client.Models, model: geminiModel}, nil
}

func (g *GeminiCoach) generate(ctx context.Context, timeout time.Duration, prompt string, config *genai.GenerateContentConfig) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	contents := []*genai.Content{{Parts: []*genai.Part{{Text: prompt}}}}
	resp, err := g.models.GenerateContent(ctx, g.model, contents, config)
	if err != nil {
		return "", fmt.Errorf("gemini generate content: %w", err)
	}
	return resp.Text(), nil
}

func (g *GeminiCoach) Reply(ctx context.Context, q *DiscussionQuestion, transcript []DiscussionMessage) (*CoachReply, error) {
	text, err := g.generate(ctx, discussionTimeout, buildDiscussionReplyPrompt(q, transcript), &genai.GenerateContentConfig{
		MaxOutputTokens:  maxCoachReplyOutputTokens,
		ResponseMIMEType: "application/json",
		ResponseSchema:   coachReplySchema,
		ThinkingConfig:   lowThinking(),
	})
	if err != nil {
		return nil, err
	}
	var reply CoachReply
	if err := json.Unmarshal([]byte(text), &reply); err != nil {
		return nil, fmt.Errorf("parse coach reply: %w", err)
	}
	if strings.TrimSpace(reply.Message) == "" {
		return nil, fmt.Errorf("coach reply has an empty message")
	}
	return &reply, nil
}

func (g *GeminiCoach) Summarize(ctx context.Context, q *DiscussionQuestion, transcript []DiscussionMessage, reflectionJA string) (*Summary, error) {
	budget := summaryOutputBudget(transcript)
	text, err := g.generate(ctx, summaryTimeout(budget), buildSummaryPrompt(q, transcript, reflectionJA), &genai.GenerateContentConfig{
		MaxOutputTokens:  budget,
		ResponseMIMEType: "application/json",
		ResponseSchema:   summarySchema,
		ThinkingConfig:   lowThinking(),
	})
	if err != nil {
		return nil, err
	}
	var summary Summary
	if err := json.Unmarshal([]byte(text), &summary); err != nil {
		return nil, fmt.Errorf("parse summary: %w", err)
	}
	if want, got := countLearnerTurns(transcript), len(summary.RefinedMessages); got != want {
		return nil, fmt.Errorf("summary refined %d messages for %d learner turns", got, want)
	}
	for i, m := range summary.RefinedMessages {
		if strings.TrimSpace(m) == "" {
			return nil, fmt.Errorf("summary produced an empty refinement for learner turn %d", i+1)
		}
	}
	if strings.TrimSpace(summary.NaturalnessWhyEN) == "" || strings.TrimSpace(summary.NaturalnessFixEN) == "" {
		return nil, fmt.Errorf("summary produced an incomplete naturalness explanation")
	}
	valid := make([]Phrase, 0, len(summary.Phrases))
	for _, ph := range summary.Phrases {
		if strings.TrimSpace(ph.Phrase) == "" ||
			strings.TrimSpace(ph.MeaningEN) == "" ||
			strings.TrimSpace(ph.ExampleEN) == "" {
			continue
		}
		valid = append(valid, ph)
	}
	if len(valid) > maxSessionPhrases {
		valid = valid[:maxSessionPhrases]
	}
	summary.Phrases = valid
	return &summary, nil
}
