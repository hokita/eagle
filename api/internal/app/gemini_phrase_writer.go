package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/genai"
)

const (
	// phraseWriterTimeout is shorter than the explanation's because saving a
	// phrase waits on it: past this, the phrase is saved without a meaning.
	phraseWriterTimeout = 15 * time.Second
	// maxPhraseWriterOutputTokens bounds the reply: a one-line meaning and
	// three short sentences fit in a few hundred tokens, plus room for
	// thinking (thinkingAllowanceTokens).
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

// GeminiPhraseWriter implements PhraseWriter using the Gemini API, with the
// same model as the other AI features (geminiModel).
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
		// The finish reason tells a cut-off reply (MAX_TOKENS) apart from a
		// blocked or malformed one, which the logged error otherwise hides.
		return nil, fmt.Errorf("parse phrase content (finish reason %q): %w", finishReason(resp), err)
	}
	return &content, nil
}

// finishReason is why the model stopped writing its first candidate, or ""
// when the response has none.
func finishReason(resp *genai.GenerateContentResponse) genai.FinishReason {
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0] == nil {
		return ""
	}
	return resp.Candidates[0].FinishReason
}
