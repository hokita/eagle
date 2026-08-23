package app

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/genai"
)

func textResp(text string) *genai.GenerateContentResponse {
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{Content: &genai.Content{Parts: []*genai.Part{{Text: text}}}},
		},
	}
}

func TestGeminiCoachReplyParsesJSON(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{"done":false,"message":"Why do you think so?"}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}

	got, err := g.Reply(context.Background(), promptQuestion, msgs("I think companies."))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Done || got.Message != "Why do you think so?" {
		t.Fatalf("unexpected reply: %+v", got)
	}
	if fake.gotConfig == nil || fake.gotConfig.ResponseMIMEType != "application/json" {
		t.Fatalf("expected JSON response config, got %+v", fake.gotConfig)
	}
	if fake.gotConfig.ResponseSchema == nil {
		t.Fatal("expected a response schema")
	}
	if fake.gotConfig.MaxOutputTokens != maxCoachReplyOutputTokens {
		t.Fatalf("expected MaxOutputTokens=%d, got %d", maxCoachReplyOutputTokens, fake.gotConfig.MaxOutputTokens)
	}
}

func TestGeminiCoachReplyRejectsMalformedJSON(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp("not json")}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.Reply(context.Background(), promptQuestion, msgs("a")); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestGeminiCoachReplyRejectsBlankMessage(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{"done":false,"message":"  "}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.Reply(context.Background(), promptQuestion, msgs("a")); err == nil {
		t.Fatal("expected error for blank follow-up message")
	}
}

func TestGeminiCoachAnalyzeGapParsesAndValidates(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{
		"expressed_ideas":["Companies are responsible."],
		"missing_ideas":["Systemic change is needed."],
		"expressions":[
			{"phrase":"take responsibility for","meaning_ja":"〜に責任を持つ","example_en":"Companies should take responsibility for pollution."},
			{"phrase":"make systemic changes","meaning_ja":"制度的な変更を行う","example_en":"Governments can make systemic changes."}
		]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}

	got, err := g.AnalyzeGap(context.Background(), promptQuestion, msgs("I think companies."), "制度を変えるべき。")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Expressions) != 2 || got.Expressions[0].Phrase != "take responsibility for" {
		t.Fatalf("unexpected analysis: %+v", got)
	}
}

func TestGeminiCoachAnalyzeGapTruncatesToFourExpressions(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{
		"expressed_ideas":[],"missing_ideas":[],
		"expressions":[
			{"phrase":"a","meaning_ja":"あ","example_en":"A."},
			{"phrase":"b","meaning_ja":"い","example_en":"B."},
			{"phrase":"c","meaning_ja":"う","example_en":"C."},
			{"phrase":"d","meaning_ja":"え","example_en":"D."},
			{"phrase":"e","meaning_ja":"お","example_en":"E."}
		]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.AnalyzeGap(context.Background(), promptQuestion, msgs("a"), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Expressions) != 4 {
		t.Fatalf("expected 4 expressions after truncation, got %d", len(got.Expressions))
	}
}

func TestGeminiCoachAnalyzeGapRejectsNoValidExpressions(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{"expressed_ideas":[],"missing_ideas":[],"expressions":[{"phrase":"  ","meaning_ja":"","example_en":""}]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.AnalyzeGap(context.Background(), promptQuestion, msgs("a"), "x"); err == nil {
		t.Fatal("expected error when no valid expression remains")
	}
}

func TestGeminiCoachReviewRetryReturnsText(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp("Nice use of the new expressions!")}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.ReviewRetry(context.Background(), promptQuestion, "first", "second", []Expression{{Phrase: "p"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Nice use of the new expressions!" {
		t.Fatalf("unexpected feedback: %q", got)
	}
}

func TestGeminiCoachReviewRetryRejectsEmpty(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp("   ")}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.ReviewRetry(context.Background(), promptQuestion, "a", "b", nil); err == nil {
		t.Fatal("expected error for empty feedback")
	}
}

func TestGeminiCoachPropagatesError(t *testing.T) {
	fake := &fakeContentGenerator{err: errors.New("network error")}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.Reply(context.Background(), promptQuestion, msgs("a")); err == nil {
		t.Fatal("expected error")
	}
}
