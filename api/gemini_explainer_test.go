package main

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
}

func (f *fakeContentGenerator) GenerateContent(_ context.Context, model string, contents []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	f.gotModel = model
	f.gotContents = contents
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

	got, err := g.Explain(context.Background(), "japanese", "correct", "user")
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

func TestGeminiExplainerExplainPropagatesError(t *testing.T) {
	fake := &fakeContentGenerator{err: errors.New("network error")}
	g := &GeminiExplainer{models: fake, model: "gemini-2.5-flash"}

	_, err := g.Explain(context.Background(), "japanese", "correct", "user")
	if err == nil {
		t.Fatal("expected error")
	}
}
