package app

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/genai"
)

// fakeContentGenerator substitutes for *genai.Models in tests, so no test
// makes a real network call to Gemini.
type fakeContentGenerator struct {
	resp *genai.GenerateContentResponse
	err  error

	gotModel    string
	gotContents []*genai.Content
	gotConfig   *genai.GenerateContentConfig
}

func (f *fakeContentGenerator) GenerateContent(_ context.Context, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	f.gotModel = model
	f.gotContents = contents
	f.gotConfig = config
	return f.resp, f.err
}

func TestGeminiExplainerExplainReturnsText(t *testing.T) {
	fake := &fakeContentGenerator{
		resp: &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{Content: &genai.Content{Parts: []*genai.Part{{Text: "explanation text"}}}},
			},
		},
	}
	g := &GeminiExplainer{models: fake, model: "gemini-2.5-flash"}

	got, err := g.Explain(context.Background(), "japanese", "correct", "user", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "explanation text" {
		t.Fatalf("unexpected explanation: %q", got)
	}
	if fake.gotModel != "gemini-2.5-flash" {
		t.Fatalf("unexpected model: %q", fake.gotModel)
	}
	if len(fake.gotContents) != 1 || len(fake.gotContents[0].Parts) != 1 {
		t.Fatalf("unexpected contents: %+v", fake.gotContents)
	}
	if fake.gotContents[0].Parts[0].Text == "" {
		t.Fatal("expected prompt text to be sent")
	}
}

func TestGeminiExplainerCapsMaxOutputTokens(t *testing.T) {
	fake := &fakeContentGenerator{
		resp: &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{Content: &genai.Content{Parts: []*genai.Part{{Text: "explanation text"}}}},
			},
		},
	}
	g := &GeminiExplainer{models: fake, model: "gemini-2.5-flash"}

	if _, err := g.Explain(context.Background(), "japanese", "correct", "user", "en"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fake.gotConfig == nil {
		t.Fatal("expected a GenerateContentConfig to be passed, got nil")
	}
	if fake.gotConfig.MaxOutputTokens != maxExplainOutputTokens {
		t.Fatalf("expected MaxOutputTokens=%d, got %d", maxExplainOutputTokens, fake.gotConfig.MaxOutputTokens)
	}
}

func TestGeminiExplainerExplainPropagatesError(t *testing.T) {
	fake := &fakeContentGenerator{err: errors.New("network error")}
	g := &GeminiExplainer{models: fake, model: "gemini-2.5-flash"}

	_, err := g.Explain(context.Background(), "japanese", "correct", "user", "en")
	if err == nil {
		t.Fatal("expected error")
	}
}
