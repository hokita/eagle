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

func TestGeminiCoachAnalyzeGapTruncatesIdeasToTwenty(t *testing.T) {
	ideas := make([]string, 25)
	for i := range ideas {
		ideas[i] = fmt.Sprintf(`"idea %d"`, i)
	}
	ideasJSON := "[" + strings.Join(ideas, ",") + "]"
	fake := &fakeContentGenerator{resp: textResp(fmt.Sprintf(`{
		"expressed_ideas":%s,"missing_ideas":%s,
		"expressions":[{"phrase":"a","meaning_ja":"あ","example_en":"A."}]}`, ideasJSON, ideasJSON))}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.AnalyzeGap(context.Background(), promptQuestion, msgs("a"), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.ExpressedIdeas) != 20 {
		t.Fatalf("expected 20 expressed_ideas after truncation, got %d", len(got.ExpressedIdeas))
	}
	if len(got.MissingIdeas) != 20 {
		t.Fatalf("expected 20 missing_ideas after truncation, got %d", len(got.MissingIdeas))
	}
}

func TestGeminiCoachAnalyzeGapDiscardsIncompleteExpressions(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{"expressed_ideas":[],"missing_ideas":[],"expressions":[
		{"phrase":"take responsibility for","meaning_ja":"","example_en":"Companies should take responsibility for pollution."},
		{"phrase":"have a greater impact on","meaning_ja":"〜により大きな影響を与える","example_en":"  "},
		{"phrase":"make systemic changes","meaning_ja":"制度的な変更を行う","example_en":"Governments can make systemic changes."}
	]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.AnalyzeGap(context.Background(), promptQuestion, msgs("a"), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Expressions) != 1 || got.Expressions[0].Phrase != "make systemic changes" {
		t.Fatalf("expected only the complete expression to survive, got %+v", got.Expressions)
	}
}

func TestGeminiCoachAnalyzeGapRejectsNoValidExpressions(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{"expressed_ideas":[],"missing_ideas":[],"expressions":[{"phrase":"  ","meaning_ja":"","example_en":""}]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	if _, err := g.AnalyzeGap(context.Background(), promptQuestion, msgs("a"), "x"); err == nil {
		t.Fatal("expected error when no valid expression remains")
	}
}

func TestGeminiCoachAnalyzeGapParsesCorrections(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{
		"expressed_ideas":[],"missing_ideas":[],
		"expressions":[{"phrase":"a","meaning_ja":"あ","example_en":"A."}],
		"corrections":[
			{"original":"I am agree with you.","better":"I agree with you.","note_ja":"agree は動詞なので be 動詞は不要です。"}
		]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.AnalyzeGap(context.Background(), promptQuestion, msgs("I am agree with you."), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Corrections) != 1 {
		t.Fatalf("expected 1 correction, got %+v", got.Corrections)
	}
	c := got.Corrections[0]
	if c.Original != "I am agree with you." || c.Better != "I agree with you." || c.NoteJA == "" {
		t.Fatalf("unexpected correction: %+v", c)
	}
}

func TestGeminiCoachAnalyzeGapDiscardsIncompleteCorrections(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{
		"expressed_ideas":[],"missing_ideas":[],
		"expressions":[{"phrase":"a","meaning_ja":"あ","example_en":"A."}],
		"corrections":[
			{"original":"","better":"I agree with you.","note_ja":"x"},
			{"original":"I am agree.","better":"  ","note_ja":"x"},
			{"original":"He go there.","better":"He goes there.","note_ja":""},
			{"original":"I very like it.","better":"I really like it.","note_ja":"very は動詞を修飾できません。"}
		]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.AnalyzeGap(context.Background(), promptQuestion, msgs("I very like it."), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Corrections) != 1 || got.Corrections[0].Original != "I very like it." {
		t.Fatalf("expected only the complete correction to survive, got %+v", got.Corrections)
	}
}

// The prompt asks for a verbatim quote of the learner's own sentence, but
// the response schema cannot enforce that: a hallucinated sentence, or one
// lifted from the coach's own follow-up, would otherwise be shown back to
// the learner as a mistake they never made — and saved to history as one.
func TestGeminiCoachAnalyzeGapDropsCorrectionsTheLearnerNeverWrote(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{
		"expressed_ideas":[],"missing_ideas":[],
		"expressions":[{"phrase":"a","meaning_ja":"あ","example_en":"A."}],
		"corrections":[
			{"original":"Why do you think so?","better":"Why do you think that?","note_ja":"x"},
			{"original":"I never said this sentence.","better":"I did not say this.","note_ja":"x"},
			{"original":"I am agree with you.","better":"I agree with you.","note_ja":"agree は動詞です。"}
		]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.AnalyzeGap(context.Background(), promptQuestion,
		msgs("I am agree with you.", "Why do you think so?", "Because they pollute more."), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Corrections) != 1 || got.Corrections[0].Original != "I am agree with you." {
		t.Fatalf("expected only the learner's own sentence to survive, got %+v", got.Corrections)
	}
}

// Grounding must not be so strict that it throws away good corrections: a
// quote that differs only in case or spacing is still the learner's sentence.
func TestGeminiCoachAnalyzeGapKeepsCorrectionsQuotedLoosely(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{
		"expressed_ideas":[],"missing_ideas":[],
		"expressions":[{"phrase":"a","meaning_ja":"あ","example_en":"A."}],
		"corrections":[
			{"original":"i am  AGREE   with you.","better":"I agree with you.","note_ja":"agree は動詞です。"}
		]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.AnalyzeGap(context.Background(), promptQuestion, msgs("I am agree with you."), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Corrections) != 1 {
		t.Fatalf("expected the loosely quoted correction to survive, got %+v", got.Corrections)
	}
}

func TestGeminiCoachAnalyzeGapTruncatesCorrections(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{
		"expressed_ideas":[],"missing_ideas":[],
		"expressions":[{"phrase":"a","meaning_ja":"あ","example_en":"A."}],
		"corrections":[
			{"original":"1","better":"one","note_ja":"x"},
			{"original":"2","better":"two","note_ja":"x"},
			{"original":"3","better":"three","note_ja":"x"},
			{"original":"4","better":"four","note_ja":"x"}
		]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.AnalyzeGap(context.Background(), promptQuestion, msgs("1 2 3 4"), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Corrections) != maxSessionCorrections {
		t.Fatalf("expected %d corrections after truncation, got %d", maxSessionCorrections, len(got.Corrections))
	}
}

// A conversation with no mistakes is a success, not a failure: the analysis
// must survive an absent corrections list and serialize as [] rather than
// null, which the study screen renders as an empty card.
func TestGeminiCoachAnalyzeGapAllowsNoCorrections(t *testing.T) {
	fake := &fakeContentGenerator{resp: textResp(`{
		"expressed_ideas":[],"missing_ideas":[],
		"expressions":[{"phrase":"a","meaning_ja":"あ","example_en":"A."}]}`)}
	g := &GeminiCoach{models: fake, model: "gemini-test"}
	got, err := g.AnalyzeGap(context.Background(), promptQuestion, msgs("a"), "x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Corrections == nil {
		t.Fatal("expected an empty non-nil corrections slice")
	}
	if len(got.Corrections) != 0 {
		t.Fatalf("expected no corrections, got %+v", got.Corrections)
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
