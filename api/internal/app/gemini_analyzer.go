package app

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

const maxInsightOutputTokens = 1024 + thinkingAllowanceTokens

type GeminiWeaknessAnalyzer struct {
	models contentGenerator
	model  string
}

func NewGeminiWeaknessAnalyzer(ctx context.Context, apiKey string) (*GeminiWeaknessAnalyzer, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create genai client: %w", err)
	}
	return &GeminiWeaknessAnalyzer{models: client.Models, model: geminiModel}, nil
}

func (g *GeminiWeaknessAnalyzer) Analyze(ctx context.Context, mistakes []MistakeSentence, language string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, explainTimeout)
	defer cancel()

	prompt := buildWeaknessPrompt(mistakes, language)
	contents := []*genai.Content{{Parts: []*genai.Part{{Text: prompt}}}}

	config := &genai.GenerateContentConfig{MaxOutputTokens: maxInsightOutputTokens, ThinkingConfig: lowThinking()}
	resp, err := g.models.GenerateContent(ctx, g.model, contents, config)
	if err != nil {
		return "", fmt.Errorf("gemini generate content: %w", err)
	}
	return resp.Text(), nil
}
