package app

import (
	"fmt"
	"strings"
	"testing"
)

func mistakesFixture() []MistakeSentence {
	return []MistakeSentence{
		{
			SentenceID:    1,
			Japanese:      "時間がありません。",
			CorrectAnswer: "I don't have time.",
			WrongAnswers: []AnswerHistory{
				{ID: 1, IncorrectAnswer: "I have no time.", CreatedAt: "2026-01-03T00:00:00Z"},
			},
		},
	}
}

func TestBuildWeaknessPromptIncludesMistakes(t *testing.T) {
	prompt := buildWeaknessPrompt(mistakesFixture(), "en")
	for _, want := range []string{"時間がありません。", "I don't have time.", "I have no time."} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to include %q", want)
		}
	}
}

func TestBuildWeaknessPromptFocusesOnPatternsAndIgnoresTypos(t *testing.T) {
	prompt := buildWeaknessPrompt(mistakesFixture(), "en")
	if !strings.Contains(prompt, "recurring") {
		t.Fatal("expected prompt to ask for recurring weaknesses")
	}
	if !strings.Contains(strings.ToLower(prompt), "typo") {
		t.Fatal("expected prompt to instruct ignoring typos")
	}
}

func TestBuildWeaknessPromptCapsWrongAnswersPerSentence(t *testing.T) {
	var wrongAnswers []AnswerHistory
	for i := 0; i < 20; i++ {
		wrongAnswers = append(wrongAnswers, AnswerHistory{IncorrectAnswer: "wrong answer"})
	}
	mistakes := []MistakeSentence{
		{SentenceID: 1, Japanese: "x", CorrectAnswer: "y", WrongAnswers: wrongAnswers},
	}
	prompt := buildWeaknessPrompt(mistakes, "en")
	if got := strings.Count(prompt, "Learner wrote:"); got != 5 {
		t.Fatalf("expected at most 5 wrong answers per sentence in the prompt, got %d", got)
	}
}

func TestBuildWeaknessPromptKeepsMostRecentWrongAnswersWhenCapping(t *testing.T) {
	var wrongAnswers []AnswerHistory
	for i := 0; i < 8; i++ {
		wrongAnswers = append(wrongAnswers, AnswerHistory{IncorrectAnswer: fmt.Sprintf("attempt-%d", i)})
	}
	mistakes := []MistakeSentence{
		{SentenceID: 1, Japanese: "x", CorrectAnswer: "y", WrongAnswers: wrongAnswers},
	}
	prompt := buildWeaknessPrompt(mistakes, "en")
	if !strings.Contains(prompt, "attempt-0") {
		t.Fatal("expected the most recent wrong answer (attempt-0, first in the already-sorted slice) to be kept")
	}
	if strings.Contains(prompt, "attempt-7") {
		t.Fatal("expected older wrong answers beyond the cap to be dropped")
	}
}

// TestBuildWeaknessPromptEnforcesTotalCharacterBudget is a regression test:
// maxInsightMistakes and maxWrongAnswersPerSentence bound the *shape* of the
// input (how many sentences, how many answers each), but not the size of
// individual fields. A learner with many sentences each carrying long
// wrong-answer text could still produce an enormous prompt. This asserts a
// hard ceiling on the prompt's total size regardless of per-field content.
func TestBuildWeaknessPromptEnforcesTotalCharacterBudget(t *testing.T) {
	longAnswer := strings.Repeat("a", 2000)
	var mistakes []MistakeSentence
	for i := 0; i < maxInsightMistakes; i++ {
		mistakes = append(mistakes, MistakeSentence{
			SentenceID:    i,
			Japanese:      "文",
			CorrectAnswer: "sentence",
			WrongAnswers: []AnswerHistory{
				{IncorrectAnswer: longAnswer}, {IncorrectAnswer: longAnswer},
				{IncorrectAnswer: longAnswer}, {IncorrectAnswer: longAnswer},
				{IncorrectAnswer: longAnswer},
			},
		})
	}
	prompt := buildWeaknessPrompt(mistakes, "en")
	if len(prompt) > maxPromptChars+1000 {
		t.Fatalf("expected prompt length to stay near the %d-char budget, got %d", maxPromptChars, len(prompt))
	}
	if strings.Count(prompt, "Learner wrote:") == maxInsightMistakes*maxWrongAnswersPerSentence {
		t.Fatal("expected the character budget to drop some mistakes when every field is maximally long")
	}
	if !strings.HasSuffix(prompt, "in English.") {
		t.Fatal("expected the trailing language instruction to survive even when mistakes are truncated")
	}
}

func TestBuildWeaknessPromptRequestsMarkdownFormatting(t *testing.T) {
	prompt := buildWeaknessPrompt(mistakesFixture(), "en")
	if !strings.Contains(strings.ToLower(prompt), "markdown") {
		t.Fatal("expected prompt to explicitly ask for a Markdown-formatted response")
	}
}

func TestBuildWeaknessPromptWritesInRequestedLanguage(t *testing.T) {
	en := buildWeaknessPrompt(mistakesFixture(), "en")
	if !strings.HasSuffix(en, "in English.") {
		t.Fatalf("expected English instruction suffix, got: %q", en)
	}
	ja := buildWeaknessPrompt(mistakesFixture(), "ja")
	if !strings.HasSuffix(ja, "in Japanese.") {
		t.Fatalf("expected Japanese instruction suffix, got: %q", ja)
	}
}
