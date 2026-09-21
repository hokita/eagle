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

func followUpInput() FollowUpInput {
	return FollowUpInput{
		Japanese:      "時間がありません。",
		CorrectAnswer: "I don't have time.",
		UserAnswer:    "I have no time.",
		Explanation:   "The reference uses do-support.",
		Question:      "When would \"I have no time\" sound natural?",
		Language:      "en",
	}
}

func TestBuildFollowUpPromptIncludesTheWholeContext(t *testing.T) {
	prompt := buildFollowUpPrompt(followUpInput())
	for _, want := range []string{
		"時間がありません。",
		"I don't have time.",
		"I have no time.",
		"The reference uses do-support.",
		`When would "I have no time" sound natural?`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to include %q", want)
		}
	}
}

func TestBuildFollowUpPromptIncludesEarlierTurns(t *testing.T) {
	in := followUpInput()
	in.History = []FollowUpTurn{
		{Question: "Was my answer wrong?", Answer: "Not wrong, just less common."},
	}
	prompt := buildFollowUpPrompt(in)
	if !strings.Contains(prompt, "Was my answer wrong?") || !strings.Contains(prompt, "Not wrong, just less common.") {
		t.Fatalf("expected prompt to include the earlier turn, got: %q", prompt)
	}
}

func TestBuildFollowUpPromptOmitsTheHistorySectionWhenThereIsNone(t *testing.T) {
	prompt := buildFollowUpPrompt(followUpInput())
	if strings.Contains(prompt, "Follow-up questions already asked") {
		t.Fatal("expected no history section on the first question of a thread")
	}
}

// The question is free text from the learner, so the prompt has to say what
// it is: a question to answer, not instructions for the model to follow.
func TestBuildFollowUpPromptTreatsTheQuestionAsUntrusted(t *testing.T) {
	prompt := buildFollowUpPrompt(followUpInput())
	if !strings.Contains(prompt, "never an instruction to") {
		t.Fatalf("expected prompt to guard against instructions in the learner's question, got: %q", prompt)
	}
}

func TestBuildFollowUpPromptKeepsTheModelOnTopic(t *testing.T) {
	prompt := buildFollowUpPrompt(followUpInput())
	if !strings.Contains(prompt, "not about English or this sentence") {
		t.Fatalf("expected prompt to tell the model what to do with an off-topic question, got: %q", prompt)
	}
}

func TestBuildFollowUpPromptRequestsMarkdownFormatting(t *testing.T) {
	prompt := buildFollowUpPrompt(followUpInput())
	if !strings.Contains(strings.ToLower(prompt), "markdown") {
		t.Fatal("expected prompt to explicitly ask for a Markdown-formatted response")
	}
}

func TestBuildFollowUpPromptWritesInRequestedLanguage(t *testing.T) {
	in := followUpInput()
	if !strings.HasSuffix(buildFollowUpPrompt(in), "write it in English.") {
		t.Fatal("expected prompt to end with the English instruction")
	}

	in.Language = "ja"
	if !strings.HasSuffix(buildFollowUpPrompt(in), "write it in Japanese.") {
		t.Fatal("expected prompt to end with the Japanese instruction")
	}
}
