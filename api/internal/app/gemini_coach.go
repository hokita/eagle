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

	// Output bounds per call — the input side is bounded by transcript
	// validation; these keep the response side predictable too.
	maxCoachReplyOutputTokens = 256

	// The summary echoes roughly all of the learner's own text back as
	// refinements, so its output budget is derived from the transcript it
	// is given (see summaryOutputBudget) rather than fixed: a cap sized for
	// a typical session truncates the JSON of a long but valid one, and its
	// completion then fails on every retry.
	//
	// summaryBaseOutputTokens covers everything that does not scale with
	// the transcript — the explanation, up to four phrases, JSON structure.
	summaryBaseOutputTokens = 1024
	// summaryTokensPerLearnerRune is the worst case for one rune of learner
	// text echoed back. Validation bounds turns in runes and accepts any
	// Unicode, so English's ~0.3 tokens per character is no bound; a rune
	// outside the tokenizer's vocabulary falls back to one token per UTF-8
	// byte, at most 4.
	summaryTokensPerLearnerRune = 4
	// geminiMaxOutputTokens is the model's own output ceiling, which the
	// derived budget must stay under.
	geminiMaxOutputTokens = 65536

	// The summary's deadline scales with its output budget for the same
	// reason the budget scales with the transcript: a fixed 30 seconds fits
	// a typical session's few hundred tokens of refinements but not the
	// tens of thousands the largest valid transcript may need, and a
	// deadline that cannot be met fails every retry of a valid completion.
	//
	// summaryTokensPerSecond is a deliberately low generation rate — the
	// model streams several times faster — so the allowance per token is
	// generous. maxSummaryTimeout keeps the longest deadline under Cloud
	// Run's 300-second default request timeout, which would otherwise cut
	// the request off first with nothing to tell the client; the extreme
	// end of the input space is bounded there rather than served.
	summaryTokensPerSecond = 50
	maxSummaryTimeout      = 240 * time.Second
)

// summaryOutputBudget is the MaxOutputTokens for a summary of transcript:
// the worst-case token count of echoing every learner turn, plus a fixed
// allowance for the rest. The largest transcript validation accepts
// (maxTranscriptMessages/2 turns of maxDiscussionTurnLength runes) stays
// under geminiMaxOutputTokens, so the clamp is a guard, not the usual path.
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

// summaryTimeout is the deadline for a summary call allowed budget output
// tokens: the base discussion deadline plus time to generate the budget at
// summaryTokensPerSecond, capped at maxSummaryTimeout.
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
	})
	if err != nil {
		return nil, err
	}
	var summary Summary
	if err := json.Unmarshal([]byte(text), &summary); err != nil {
		return nil, fmt.Errorf("parse summary: %w", err)
	}
	// The refinements are read zipped against the transcript's learner
	// turns, so the list has to line up with them exactly: one too few and
	// the last turn silently goes unrefined, one too many and every turn
	// after the slip shows someone else's sentence. Either is the model
	// failing to follow the prompt, which numbers the turns for it, and the
	// session has no other step left to fall back on — so it is an error,
	// and the client offers to try the summary again.
	if want, got := countLearnerTurns(transcript), len(summary.RefinedMessages); got != want {
		return nil, fmt.Errorf("summary refined %d messages for %d learner turns", got, want)
	}
	for i, m := range summary.RefinedMessages {
		if strings.TrimSpace(m) == "" {
			return nil, fmt.Errorf("summary produced an empty refinement for learner turn %d", i+1)
		}
	}
	// Both halves of the explanation are required for the same reason: the
	// prompt has an answer even for a learner who sounded natural, so a
	// blank field is the model failing to follow it, not an empty result.
	if strings.TrimSpace(summary.NaturalnessWhyEN) == "" || strings.TrimSpace(summary.NaturalnessFixEN) == "" {
		return nil, fmt.Errorf("summary produced an incomplete naturalness explanation")
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
