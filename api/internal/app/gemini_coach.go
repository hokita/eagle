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
	maxCoachAnalyzeOutputTokens = 1024
	maxCoachReviewOutputTokens  = 512
)

var coachReplySchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"done":    {Type: genai.TypeBoolean},
		"message": {Type: genai.TypeString},
	},
	Required: []string{"done", "message"},
}

var gapAnalysisSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"expressed_ideas": {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
		"missing_ideas":   {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
		"expressions": {Type: genai.TypeArray, Items: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"phrase":     {Type: genai.TypeString},
				"meaning_ja": {Type: genai.TypeString},
				"example_en": {Type: genai.TypeString},
			},
			Required: []string{"phrase", "meaning_ja", "example_en"},
		}},
	},
	Required: []string{"expressed_ideas", "missing_ideas", "expressions"},
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

func (g *GeminiCoach) AnalyzeGap(ctx context.Context, q *DiscussionQuestion, transcript []DiscussionMessage, reflectionJA string) (*GapAnalysis, error) {
	text, err := g.generate(ctx, buildGapAnalysisPrompt(q, transcript, reflectionJA), &genai.GenerateContentConfig{
		MaxOutputTokens:  maxCoachAnalyzeOutputTokens,
		ResponseMIMEType: "application/json",
		ResponseSchema:   gapAnalysisSchema,
	})
	if err != nil {
		return nil, err
	}
	var analysis GapAnalysis
	if err := json.Unmarshal([]byte(text), &analysis); err != nil {
		return nil, fmt.Errorf("parse gap analysis: %w", err)
	}
	// Keep only well-formed expressions, then enforce the 2-4 range as
	// best we can: truncate extras, error when nothing usable remains.
	valid := analysis.Expressions[:0]
	for _, e := range analysis.Expressions {
		if strings.TrimSpace(e.Phrase) == "" {
			continue
		}
		valid = append(valid, e)
	}
	if len(valid) == 0 {
		return nil, fmt.Errorf("gap analysis produced no usable expressions")
	}
	if len(valid) > maxSessionExpressions {
		valid = valid[:maxSessionExpressions]
	}
	analysis.Expressions = valid
	if analysis.ExpressedIdeas == nil {
		analysis.ExpressedIdeas = []string{}
	}
	if analysis.MissingIdeas == nil {
		analysis.MissingIdeas = []string{}
	}
	// The complete endpoint rejects idea lists longer than maxSessionIdeas —
	// truncate here so a verbose model response can't brick the flow.
	if len(analysis.ExpressedIdeas) > maxSessionIdeas {
		analysis.ExpressedIdeas = analysis.ExpressedIdeas[:maxSessionIdeas]
	}
	if len(analysis.MissingIdeas) > maxSessionIdeas {
		analysis.MissingIdeas = analysis.MissingIdeas[:maxSessionIdeas]
	}
	return &analysis, nil
}

func (g *GeminiCoach) ReviewRetry(ctx context.Context, q *DiscussionQuestion, firstAnswer, retryAnswer string, expressions []Expression) (string, error) {
	text, err := g.generate(ctx, buildRetryReviewPrompt(q, firstAnswer, retryAnswer, expressions), &genai.GenerateContentConfig{
		MaxOutputTokens: maxCoachReviewOutputTokens,
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("retry review produced empty feedback")
	}
	return text, nil
}
