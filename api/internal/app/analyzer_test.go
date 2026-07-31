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
