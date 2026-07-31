package app

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/genai"
)

func TestGeminiWeaknessAnalyzerReturnsText(t *testing.T) {
	fake := &fakeContentGenerator{
		resp: &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{Content: &genai.Content{Parts: []*genai.Part{{Text: "insight text"}}}},
			},
		},
	}
	g := &GeminiWeaknessAnalyzer{models: fake, model: "gemini-test"}

	got, err := g.Analyze(context.Background(),
		[]MistakeSentence{{Japanese: "x", CorrectAnswer: "y", WrongAnswers: []AnswerHistory{{IncorrectAnswer: "z"}}}},
		"en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "insight text" {
		t.Fatalf("unexpected insight: %q", got)
	}
	if fake.gotModel != "gemini-test" {
		t.Fatalf("unexpected model: %q", fake.gotModel)
	}
	if len(fake.gotContents) != 1 || len(fake.gotContents[0].Parts) != 1 || fake.gotContents[0].Parts[0].Text == "" {
		t.Fatalf("expected prompt text to be sent, got: %+v", fake.gotContents)
	}
}

func TestGeminiWeaknessAnalyzerPropagatesError(t *testing.T) {
	fake := &fakeContentGenerator{err: errors.New("network error")}
	g := &GeminiWeaknessAnalyzer{models: fake, model: "gemini-test"}

	_, err := g.Analyze(context.Background(), []MistakeSentence{{Japanese: "x"}}, "en")
	if err == nil {
		t.Fatal("expected error")
	}
}
