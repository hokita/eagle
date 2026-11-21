package app

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/genai"
)

const (
	geminiModel    = "gemini-3.8-flash"
	explainTimeout = 20 * time.Second

	thinkingAllowanceTokens = 1024

	maxExplainOutputTokens = 512 + thinkingAllowanceTokens

	maxFollowUpOutputTokens = 512 + thinkingAllowanceTokens
)

type contentGenerator interface {
	GenerateContent(ctx context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error)
}

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

	config := &genai.GenerateContentConfig{MaxOutputTokens: maxOutputTokens, ThinkingConfig: lowThinking()}
	resp, err := g.models.GenerateContent(ctx, g.model, contents, config)
	if err != nil {
		return "", fmt.Errorf("gemini generate content: %w", err)
	}
	return resp.Text(), nil
}

func lowThinking() *genai.ThinkingConfig {
	return &genai.ThinkingConfig{ThinkingLevel: genai.ThinkingLevelLow}
}
