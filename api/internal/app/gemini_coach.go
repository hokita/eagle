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
	maxCoachReplyOutputTokens = 256
	// The summary echoes roughly all of the learner's own text back as
	// refinements, so its budget is sized from the largest transcript that
	// validation accepts: maxTranscriptMessages / 2 learner turns of
	// maxDiscussionTurnLength runes, 12,000 characters, is about 4,000
	// tokens of English even at a dense 3 characters per token, before the
	// explanation and phrases. 8,192 leaves that comfortable headroom; a
	// tighter cap would truncate the JSON of a long but valid session and
	// fail every retry of its completion.
	maxCoachSummaryOutputTokens = 8192
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
