package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"
)

const (
	discussionTimeout = 30 * time.Second

	// Output bounds per call — the input side is bounded by transcript
	// validation; these keep the response side predictable too.
	maxCoachReplyOutputTokens   = 256
	maxCoachSummaryOutputTokens = 1536
)

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
		"natural_english": {Type: genai.TypeString},
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
	Required: []string{"natural_english", "phrases"},
}

// GeminiCoach implements DiscussionCoach using the Gemini API, reusing the
// same client configuration and model as GeminiExplainer.
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
	return &GeminiCoach{models: client.Models, model: geminiExplainModel}, nil
}

func (g *GeminiCoach) generate(ctx context.Context, prompt string, config *genai.GenerateContentConfig) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, discussionTimeout)
	defer cancel()
	contents := []*genai.Content{{Parts: []*genai.Part{{Text: prompt}}}}
	resp, err := g.models.GenerateContent(ctx, g.model, contents, config)
	if err != nil {
		return "", fmt.Errorf("gemini generate content: %w", err)
	}
	return resp.Text(), nil
}

func (g *GeminiCoach) Reply(ctx context.Context, q *DiscussionQuestion, transcript []DiscussionMessage) (*CoachReply, error) {
	text, err := g.generate(ctx, buildDiscussionReplyPrompt(q, transcript), &genai.GenerateContentConfig{
		MaxOutputTokens:  maxCoachReplyOutputTokens,
		ResponseMIMEType: "application/json",
		ResponseSchema:   coachReplySchema,
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
	text, err := g.generate(ctx, buildSummaryPrompt(q, transcript, reflectionJA), &genai.GenerateContentConfig{
		MaxOutputTokens:  maxCoachSummaryOutputTokens,
		ResponseMIMEType: "application/json",
		ResponseSchema:   summarySchema,
	})
	if err != nil {
		return nil, err
	}
	var summary Summary
	if err := json.Unmarshal([]byte(text), &summary); err != nil {
		return nil, fmt.Errorf("parse summary: %w", err)
	}
	// The rewrite is the screen — without it there is nothing to show, and
	// the session has no other step left to fall back on.
	if strings.TrimSpace(summary.NaturalEnglish) == "" {
		return nil, fmt.Errorf("summary produced an empty rewrite")
	}
	// Keep only well-formed phrases and truncate extras. The response schema
	// only constrains field types, so a record can legally arrive with a
	// blank gloss or example, which would render as an empty slot. Ending up
	// with none is a valid outcome rather than an error: a learner who
	// already said everything naturally has nothing worth picking up.
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
