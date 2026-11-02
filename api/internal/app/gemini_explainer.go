package app

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/genai"
)

const (
	// geminiModel is the model every AI feature uses: explanations,
	// follow-ups, the weakness insight, the discussion coach and saved
	// phrases. They all write short tutoring text under a deadline, which
	// suits a Flash model; a flagship reasoning model would be slower and
	// cost more without making a short definition or reply better.
	geminiModel    = "gemini-3.8-flash"
	explainTimeout = 20 * time.Second

	// maxExplainOutputTokens bounds the size of the explanation response
	// itself. buildExplainPrompt asks for a concise 2-4 sentence answer, but
	// nothing stops the model from ignoring that instruction — mirrors the
	// same safeguard on the weakness-insight path (maxInsightOutputTokens).
	maxExplainOutputTokens = 512

	// maxFollowUpOutputTokens bounds a follow-up answer the same way, and for
	// the same reason: buildFollowUpPrompt asks for 2-4 sentences, but nothing
	// makes the model keep to it.
	maxFollowUpOutputTokens = 512
)

// contentGenerator is the seam between GeminiExplainer and the genai SDK, so
// tests can substitute a fake instead of making real network calls.
// *genai.Models satisfies this interface structurally.
type contentGenerator interface {
	GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
}

// GeminiExplainer implements Explainer using the Gemini API.
type GeminiExplainer struct {
	models contentGenerator
	model  string
}

func NewGeminiExplainer(ctx context.Context, apiKey string) (*GeminiExplainer, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create genai client: %w", err)
	}
	return &GeminiExplainer{models: client.Models, model: geminiModel}, nil
}

func (g *GeminiExplainer) Explain(ctx context.Context, japanese, correctAnswer, userAnswer, language string) (string, error) {
	prompt := buildExplainPrompt(japanese, correctAnswer, userAnswer, language)
	return g.generate(ctx, prompt, maxExplainOutputTokens)
}

func (g *GeminiExplainer) AnswerFollowUp(ctx context.Context, in FollowUpInput) (string, error) {
	return g.generate(ctx, buildFollowUpPrompt(in), maxFollowUpOutputTokens)
}

func (g *GeminiExplainer) generate(ctx context.Context, prompt string, maxOutputTokens int32) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, explainTimeout)
	defer cancel()

	contents := []*genai.Content{{Parts: []*genai.Part{{Text: prompt}}}}

	config := &genai.GenerateContentConfig{MaxOutputTokens: maxOutputTokens}
	resp, err := g.models.GenerateContent(ctx, g.model, contents, config)
	if err != nil {
		return "", fmt.Errorf("gemini generate content: %w", err)
	}
	return resp.Text(), nil
}
