package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/genai"
)

func textResponse(text string) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{Content: &genai.Content{Parts: []*genai.Part{{Text: text}}}}},
	}
}

func TestGeminiPhraseWriterParsesJSON(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResponse(`{"meaning":"as little as possible","examples":["Keep it to a minimum."]}`)}
	g := &GeminiPhraseWriter{models: fake, model: "test-model"}

	got, err := g.Write(context.Background(), PhraseSource{Phrase: "to a minimum"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Meaning != "as little as possible" || len(got.Examples) != 1 {
		t.Fatalf("unexpected content: %+v", got)
	}
	if fake.gotModel != "test-model" || fake.gotConfig.ResponseMIMEType != "application/json" ||
		fake.gotConfig.ResponseSchema != phraseContentSchema || fake.gotConfig.MaxOutputTokens != maxPhraseWriterOutputTokens {
		t.Fatalf("unexpected call: model=%q config=%+v", fake.gotModel, fake.gotConfig)
	}
	if fake.gotConfig.ThinkingConfig == nil || fake.gotConfig.ThinkingConfig.ThinkingLevel != genai.ThinkingLevelLow {
		t.Fatalf("expected low thinking, got %+v", fake.gotConfig.ThinkingConfig)
	}
	if fake.gotDeadline.IsZero() {
		t.Fatal("expected the call to carry a deadline")
	}
}

func TestGeminiPhraseWriterReportsFinishReason(t *testing.T) {
	fake := &fakeContentGenerator{resp: &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content:      &genai.Content{Parts: []*genai.Part{{Text: `{"meaning":"to sho`}}},
			FinishReason: genai.FinishReasonMaxTokens,
		}},
	}}
	g := &GeminiPhraseWriter{models: fake, model: "m"}
	_, err := g.Write(context.Background(), PhraseSource{Phrase: "x"})
	if err == nil || !strings.Contains(err.Error(), "MAX_TOKENS") {
		t.Fatalf("expected the error to name the finish reason, got %v", err)
	}
}

func TestGeminiPhraseWriterErrors(t *testing.T) {
	cases := map[string]*fakeContentGenerator{
		"api error": {err: errors.New("boom")},
		"bad json":  {resp: textResponse("not json")},
	}
	for name, fake := range cases {
		t.Run(name, func(t *testing.T) {
			g := &GeminiPhraseWriter{models: fake, model: "m"}
			if _, err := g.Write(context.Background(), PhraseSource{Phrase: "x"}); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
