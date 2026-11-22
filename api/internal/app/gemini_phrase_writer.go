package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/genai"
)

const (
	phraseWriterTimeout         = 15 * time.Second
	maxPhraseWriterOutputTokens = 1024 + thinkingAllowanceTokens
)

var phraseContentSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"meaning":  {Type: genai.TypeString},
		"examples": {Type: genai.TypeArray, Items: &genai.Schema{Type: genai.TypeString}},
	},
	Required: []string{"meaning", "examples"},
}

type GeminiPhraseWriter struct {
	models contentGenerator
	model  string
}

func NewGeminiPhraseWriter(ctx context.Context, apiKey string) (*GeminiPhraseWriter, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("create genai client: %w", err)
	}
	return &GeminiPhraseWriter{models: client.Models, model: geminiModel}, nil
}

func (g *GeminiPhraseWriter) Write(ctx context.Context, src PhraseSource) (*PhraseContent, error) {
	ctx, cancel := context.WithTimeout(ctx, phraseWriterTimeout)
	defer cancel()
	contents := []*genai.Content{{Parts: []*genai.Part{{Text: buildPhrasePrompt(src)}}}}
	resp, err := g.models.GenerateContent(ctx, g.model, contents, &genai.GenerateContentConfig{
		MaxOutputTokens:  maxPhraseWriterOutputTokens,
		ResponseMIMEType: "application/json",
		ResponseSchema:   phraseContentSchema,
		ThinkingConfig:   lowThinking(),
	})
	if err != nil {
		return nil, fmt.Errorf("gemini generate content: %w", err)
	}
	var content PhraseContent
	if err := json.Unmarshal([]byte(resp.Text()), &content); err != nil {
		return nil, fmt.Errorf("parse phrase content (finish reason %q): %w", finishReason(resp), err)
	}
	return &content, nil
}

func finishReason(resp *genai.GenerateContentResponse) genai.FinishReason {
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0] == nil {
		return ""
	}
	return resp.Candidates[0].FinishReason
}
