package app

import (
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
