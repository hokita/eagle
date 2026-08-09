package app

import (
	"strings"
	"testing"
)

func TestBuildExplainPromptIncludesInputs(t *testing.T) {
	prompt := buildExplainPrompt("時間がありません。", "I don't have time.", "I have no time.", "en")
	if !strings.Contains(prompt, "時間がありません。") {
		t.Fatal("expected prompt to include the Japanese sentence")
	}
	if !strings.Contains(prompt, "I don't have time.") {
		t.Fatal("expected prompt to include the reference answer")
	}
	if !strings.Contains(prompt, "I have no time.") {
		t.Fatal("expected prompt to include the learner's answer")
	}
}

func TestBuildExplainPromptInstructsJudgingOnMerits(t *testing.T) {
	prompt := buildExplainPrompt("日本語", "reference", "answer", "en")
	if !strings.Contains(prompt, "only one valid way") {
		t.Fatal("expected prompt to instruct that the reference is not the only correct answer")
	}
}

func TestBuildExplainPromptRequestsMarkdownFormatting(t *testing.T) {
	prompt := buildExplainPrompt("日本語", "reference", "answer", "en")
	if !strings.Contains(strings.ToLower(prompt), "markdown") {
		t.Fatal("expected prompt to explicitly ask for a Markdown-formatted response")
	}
}

func TestBuildExplainPromptInstructsPlainTextQuoting(t *testing.T) {
	prompt := buildExplainPrompt("日本語", "reference", "answer", "en")
	if !strings.Contains(prompt, "without adding markdown formatting") {
		t.Fatal("expected prompt to instruct the model not to add markdown emphasis when quoting the learner's own wording")
	}
}

func TestBuildExplainPromptWritesInRequestedLanguage(t *testing.T) {
	enPrompt := buildExplainPrompt("日本語", "reference", "answer", "en")
	if !strings.HasSuffix(enPrompt, "write it in English.") {
		t.Fatalf("expected prompt to end with the English instruction, got: %q", enPrompt)
	}

	jaPrompt := buildExplainPrompt("日本語", "reference", "answer", "ja")
	if !strings.HasSuffix(jaPrompt, "write it in Japanese.") {
		t.Fatalf("expected prompt to end with the Japanese instruction, got: %q", jaPrompt)
	}
}
