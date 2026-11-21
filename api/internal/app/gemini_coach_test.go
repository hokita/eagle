package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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
	assertLowThinking(t, fake.gotConfig)
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
		"refined_messages":["I like dogs.","Shiba Inu, especially."],
		"naturalness_why_en":"w","naturalness_fix_en":"f",
		"phrases":[{"phrase":"in the future","meaning_en":"at some later time","example_en":"I want to live abroad in the future."}]
	}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}

	got, err := g.Summarize(context.Background(), promptQuestion, msgs("I like dogs.", "What kind?", "I like shiba-dog."), "将来は犬を飼いたい")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.RefinedMessages) != 2 || got.RefinedMessages[1] != "Shiba Inu, especially." {
		t.Fatalf("unexpected refinements: %q", got.RefinedMessages)
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
	if want := summaryOutputBudget(msgs("I like dogs.", "What kind?", "I like shiba-dog.")); fake.gotConfig.MaxOutputTokens != want {
		t.Fatalf("expected MaxOutputTokens=%d, got %d", want, fake.gotConfig.MaxOutputTokens)
	}
	assertLowThinking(t, fake.gotConfig)
}

func TestSummaryOutputBudgetScalesWithLearnerText(t *testing.T) {
	short := summaryOutputBudget(msgs("I like dogs."))
	if short < int32(summaryBaseOutputTokens+summaryTokensPerLearnerRune*len("I like dogs.")) {
		t.Fatalf("budget %d does not cover a short turn at the worst case", short)
	}

	ja := strings.Repeat("あ", maxDiscussionTurnLength)
	texts := make([]string, maxTranscriptMessages)
	for i := range texts {
		texts[i] = ja
	}
	largest := summaryOutputBudget(msgs(texts...))
	learnerRunes := ((maxTranscriptMessages + 1) / 2) * maxDiscussionTurnLength
	if largest < int32(summaryBaseOutputTokens+summaryTokensPerLearnerRune*learnerRunes) {
		t.Fatalf("budget %d is below the worst case for %d learner runes", largest, learnerRunes)
	}
	if largest > geminiMaxOutputTokens {
		t.Fatalf("budget %d exceeds the model's output ceiling %d", largest, geminiMaxOutputTokens)
	}

	withLongAITurn := summaryOutputBudget(msgs("I like dogs.", strings.Repeat("x", 500)))
	if withLongAITurn != short {
		t.Fatalf("coach turns must not count: got %d, want %d", withLongAITurn, short)
	}
}

func TestSummaryTimeoutScalesWithTheOutputBudget(t *testing.T) {
	small := summaryTimeout(summaryOutputBudget(msgs("I like dogs.")))
	if small < discussionTimeout {
		t.Fatalf("summary deadline %v is below the base discussion deadline %v", small, discussionTimeout)
	}
	turn := strings.Repeat("あ", 400)
	medium := summaryTimeout(summaryOutputBudget(msgs(turn, "why?", turn, "and?", turn)))
	if medium <= small {
		t.Fatalf("deadline must grow with the transcript: %v for a long session vs %v for a short one", medium, small)
	}
	if want := discussionTimeout + time.Duration(summaryOutputBudget(msgs(turn, "why?", turn, "and?", turn)))*time.Second/summaryTokensPerSecond; medium != want {
		t.Fatalf("deadline %v, want %v", medium, want)
	}
	largest := summaryTimeout(geminiMaxOutputTokens)
	if largest != maxSummaryTimeout {
		t.Fatalf("deadline for the largest budget must be capped at %v, got %v", maxSummaryTimeout, largest)
	}
	if maxSummaryTimeout >= 300*time.Second {
		t.Fatalf("summary deadline cap %v must stay under Cloud Run's 300s default request timeout", maxSummaryTimeout)
	}
}

func TestGeminiCoachSummarizeAppliesTheDerivedDeadline(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{"refined_messages":["ok"],"naturalness_why_en":"w","naturalness_fix_en":"f","phrases":[]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	transcript := msgs(strings.Repeat("a", 1500))
	before := time.Now()
	if _, err := g.Summarize(context.Background(), promptQuestion, transcript, "x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := summaryTimeout(summaryOutputBudget(transcript))
	if want <= discussionTimeout {
		t.Fatalf("test transcript must need more than the base deadline, got %v", want)
	}
	got := fake.gotDeadline.Sub(before)
	if fake.gotDeadline.IsZero() || got > want+time.Second || got < want-5*time.Second {
		t.Fatalf("model call deadline was %v away, want about %v", got, want)
	}
}

func TestGeminiCoachSummarizeRejectsMalformedJSON(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp("not json")}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.Summarize(context.Background(), promptQuestion, msgs("a"), "x"); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestGeminiCoachSummarizeRejectsMisalignedRefinements(t *testing.T) {
	for name, body := range map[string]string{
		"too few":  `{"refined_messages":["one"],"naturalness_why_en":"w","naturalness_fix_en":"f","phrases":[]}`,
		"too many": `{"refined_messages":["one","two","three"],"naturalness_why_en":"w","naturalness_fix_en":"f","phrases":[]}`,
		"missing":  `{"naturalness_why_en":"w","naturalness_fix_en":"f","phrases":[]}`,
		"blank":    `{"refined_messages":["one","   "],"naturalness_why_en":"w","naturalness_fix_en":"f","phrases":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			fake := &fakeContentGenerator{resp: textResp(body)}
			g := &GeminiCoach{models: fake, model: "gemini-test"}
			if _, err := g.Summarize(context.Background(), promptQuestion, msgs("a", "why?", "b"), "x"); err == nil {
				t.Fatal("expected error for refinements that do not match the learner turns")
			}
		})
	}
}

func TestGeminiCoachSummarizeTruncatesToFourPhrases(t *testing.T) {
	var items []string
	for i := 0; i < 7; i++ {
		items = append(items, fmt.Sprintf(`{"phrase":"p%d","meaning_en":"m","example_en":"e"}`, i))
	}
	fake := &fakeContentGenerator{resp: textResp(fmt.Sprintf(
		`{"refined_messages":["ok"],"naturalness_why_en":"w","naturalness_fix_en":"f","phrases":[%s]}`, strings.Join(items, ",")))}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.Summarize(context.Background(), promptQuestion, msgs("a"), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Phrases) != maxSessionPhrases {
		t.Fatalf("expected %d phrases, got %d", maxSessionPhrases, len(got.Phrases))
	}
}

func TestGeminiCoachSummarizeDropsIncompletePhrases(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(
		`{"refined_messages":["ok"],"naturalness_why_en":"w","naturalness_fix_en":"f","phrases":[{"phrase":"  ","meaning_en":"","example_en":""},{"phrase":"in the future","meaning_en":"later","example_en":"See you in the future."}]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.Summarize(context.Background(), promptQuestion, msgs("a"), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Phrases) != 1 || got.Phrases[0].Phrase != "in the future" {
		t.Fatalf("expected the malformed phrase to be dropped, got %+v", got.Phrases)
	}
}

func TestGeminiCoachSummarizeAllowsNoPhrases(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{"refined_messages":["ok"],"naturalness_why_en":"w","naturalness_fix_en":"f"}`)}
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

func TestGeminiCoachSummarizeParsesNaturalnessExplanation(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{
		"refined_messages":["I like dogs."],
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
		"blank why":    `{"refined_messages":["ok"],"naturalness_why_en":"  ","naturalness_fix_en":"f","phrases":[]}`,
		"blank fix":    `{"refined_messages":["ok"],"naturalness_why_en":"w","naturalness_fix_en":"","phrases":[]}`,
		"both missing": `{"refined_messages":["ok"],"phrases":[]}`,
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
