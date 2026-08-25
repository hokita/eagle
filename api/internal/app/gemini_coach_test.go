package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	fake := &fakeContentGenerator{resp: textResp(`{"message":"Why do you think so?"}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}

	got, err := g.Reply(context.Background(), promptQuestion, msgs("I think companies."))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Message != "Why do you think so?" {
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
	fake := &fakeContentGenerator{resp: textResp(`{"message":"  "}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.Reply(context.Background(), promptQuestion, msgs("a")); err == nil {
		t.Fatal("expected error for blank follow-up message")
	}
}

func TestGeminiCoachSummarizeParsesAndValidates(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{
		"natural_english":"I like dogs, especially Shiba Inu. I have a cat now, but I want a dog in the future.",
		"naturalness_why_en":"w","naturalness_fix_en":"f",
		"phrases":[{"phrase":"in the future","meaning_en":"at some later time","example_en":"I want to live abroad in the future."}]
	}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}

	got, err := g.Summarize(context.Background(), promptQuestion, msgs("I like dogs."), "将来は犬を飼いたい")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got.NaturalEnglish, "Shiba Inu") {
		t.Fatalf("unexpected rewrite: %q", got.NaturalEnglish)
	}
	if len(got.Phrases) != 1 || got.Phrases[0].Phrase != "in the future" {
		t.Fatalf("unexpected phrases: %+v", got.Phrases)
	}
	if fake.gotConfig == nil || fake.gotConfig.ResponseMIMEType != "application/json" {
		t.Fatalf("expected JSON response config, got %+v", fake.gotConfig)
	}
	if fake.gotConfig.ResponseSchema == nil {
		t.Fatal("expected a response schema")
	}
}

func TestGeminiCoachSummarizeRejectsMalformedJSON(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp("not json")}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.Summarize(context.Background(), promptQuestion, msgs("a"), "x"); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

// The rewrite is the whole point of the screen — unlike the phrase list,
// there is nothing to show without it, so a blank one is a failure.
func TestGeminiCoachSummarizeRejectsBlankRewrite(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{"natural_english":"   ","phrases":[]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.Summarize(context.Background(), promptQuestion, msgs("a"), "x"); err == nil {
		t.Fatal("expected error for a blank rewrite")
	}
}

func TestGeminiCoachSummarizeTruncatesToFourPhrases(t *testing.T) {
	var items []string
	for i := 0; i < 7; i++ {
		items = append(items, fmt.Sprintf(`{"phrase":"p%d","meaning_en":"m","example_en":"e"}`, i))
	}
	fake := &fakeContentGenerator{resp: textResp(fmt.Sprintf(
		`{"natural_english":"ok","naturalness_why_en":"w","naturalness_fix_en":"f","phrases":[%s]}`, strings.Join(items, ",")))}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.Summarize(context.Background(), promptQuestion, msgs("a"), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Phrases) != maxSessionPhrases {
		t.Fatalf("expected %d phrases, got %d", maxSessionPhrases, len(got.Phrases))
	}
}

// The response schema constrains field types, not emptiness, so a phrase can
// legally arrive with a blank gloss and would render as an empty slot.
func TestGeminiCoachSummarizeDropsIncompletePhrases(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(
		`{"natural_english":"ok","naturalness_why_en":"w","naturalness_fix_en":"f","phrases":[{"phrase":"  ","meaning_en":"","example_en":""},{"phrase":"in the future","meaning_en":"later","example_en":"See you in the future."}]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.Summarize(context.Background(), promptQuestion, msgs("a"), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Phrases) != 1 || got.Phrases[0].Phrase != "in the future" {
		t.Fatalf("expected the malformed phrase to be dropped, got %+v", got.Phrases)
	}
}

// A learner who already said everything naturally has nothing to pick up;
// that is a valid summary, not an error, and must serialize as [] not null.
func TestGeminiCoachSummarizeAllowsNoPhrases(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{"natural_english":"ok","naturalness_why_en":"w","naturalness_fix_en":"f"}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.Summarize(context.Background(), promptQuestion, msgs("a"), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Phrases == nil || len(got.Phrases) != 0 {
		t.Fatalf("expected an empty non-nil phrase list, got %+v", got.Phrases)
	}
}

func TestGeminiCoachPropagatesError(t *testing.T) {
	fake := &fakeContentGenerator{err: errors.New("network error")}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.Reply(context.Background(), promptQuestion, msgs("a")); err == nil {
		t.Fatal("expected error")
	}
}

// Both halves of the explanation are required: the card the learner reads
// pairs why their English sounded unnatural with what to do about it, so
// half a card is not a summary worth saving.
func TestGeminiCoachSummarizeParsesNaturalnessExplanation(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{
		"natural_english":"I like dogs.",
		"naturalness_why_en":"You opened every turn with \"I think that\".",
		"naturalness_fix_en":"Drop \"that\" and vary the opener.",
		"phrases":[]
	}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}

	got, err := g.Summarize(context.Background(), promptQuestion, msgs("I like dogs."), "犬が好き")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(got.NaturalnessWhyEN, "I think that") {
		t.Fatalf("unexpected why: %q", got.NaturalnessWhyEN)
	}
	if !strings.Contains(got.NaturalnessFixEN, "vary the opener") {
		t.Fatalf("unexpected fix: %q", got.NaturalnessFixEN)
	}
}

func TestGeminiCoachSummarizeRejectsBlankNaturalness(t *testing.T) {
	for name, body := range map[string]string{
		"blank why":    `{"natural_english":"ok","naturalness_why_en":"  ","naturalness_fix_en":"f","phrases":[]}`,
		"blank fix":    `{"natural_english":"ok","naturalness_why_en":"w","naturalness_fix_en":"","phrases":[]}`,
		"both missing": `{"natural_english":"ok","phrases":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeContentGenerator{resp: textResp(body)}
			g := &GeminiCoach{models: fake, model: "gemini-test"}
			if _, err := g.Summarize(context.Background(), promptQuestion, msgs("a"), "x"); err == nil {
				t.Fatal("expected error for a blank explanation")
			}
		})
	}
}
