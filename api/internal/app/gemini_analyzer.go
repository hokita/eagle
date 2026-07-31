package app

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

// GeminiWeaknessAnalyzer implements WeaknessAnalyzer using the Gemini API,
// reusing the same client configuration, model, and timeout as
// GeminiExplainer.
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
	return &GeminiWeaknessAnalyzer{models: client.Models, model: geminiExplainModel}, nil
}

func (g *GeminiWeaknessAnalyzer) Analyze(ctx context.Context, mistakes []MistakeSentence, language string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, explainTimeout)
	defer cancel()

	prompt := buildWeaknessPrompt(mistakes, language)
	contents := []*genai.Content{{Parts: []*genai.Part{{Text: prompt}}}}

	resp, err := g.models.GenerateContent(ctx, g.model, contents, nil)
	if err != nil {
		return "", fmt.Errorf("gemini generate content: %w", err)
	}
	return resp.Text(), nil
}
