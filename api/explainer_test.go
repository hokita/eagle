package main

import (
	"strings"
	"testing"
)

func TestBuildExplainPromptIncludesInputs(t *testing.T) {
	prompt := buildExplainPrompt("時間がありません。", "I don't have time.", "I have no time.")
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
	prompt := buildExplainPrompt("日本語", "reference", "answer")
	if !strings.Contains(prompt, "only one valid way") {
		t.Fatal("expected prompt to instruct that the reference is not the only correct answer")
	}
}
