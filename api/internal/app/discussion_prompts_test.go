package app

import (
	"strings"
	"testing"
)

var promptQuestion = &DiscussionQuestion{
	ID:           1,
	QuestionEN:   "Who should take more responsibility for environmental problems?",
	Topic:        "environment",
	Level:        3,
	TargetSkills: []string{"giving opinions", "giving reasons"},
}

func TestBuildDiscussionReplyPromptIncludesQuestionAndTranscript(t *testing.T) {
	got := buildDiscussionReplyPrompt(promptQuestion, msgs("I think companies.", "Why do you think so?", "Because they pollute more."))
	for _, want := range []string{
		promptQuestion.QuestionEN,
		"giving opinions, giving reasons",
		"Learner: I think companies.",
		"You: Why do you think so?",
		"Learner: Because they pollute more.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildDiscussionReplyPromptForbidsCorrection(t *testing.T) {
	got := buildDiscussionReplyPrompt(promptQuestion, msgs("I think companies.", "Why?", "Because."))
	for _, want := range []string{
		"Never correct the learner's grammar",
		"never answer the question for them",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

// The number of follow-ups is decided by the server (discussionFollowUps),
// not by the model, so the prompt must never offer it a way to end the
// conversation — it always asks one more question.
func TestBuildDiscussionReplyPromptNeverLetsTheModelEndTheConversation(t *testing.T) {
	got := buildDiscussionReplyPrompt(promptQuestion, msgs("I think companies.", "Why?", "Because."))
	for _, forbidden := range []string{
		"You have asked",
		"done",
		"closing",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("prompt must not mention %q:\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, `"message"`) {
		t.Fatalf("prompt missing the message-only response shape:\n%s", got)
	}
}

func TestBuildGapAnalysisPromptIncludesReflectionAndRules(t *testing.T) {
	got := buildGapAnalysisPrompt(promptQuestion, msgs("I think companies."), "制度を変える必要がある。")
	for _, want := range []string{
		promptQuestion.QuestionEN,
		"Learner: I think companies.",
		"制度を変える必要がある。",
		"ideas and intentions, not literal wording",
		"2 to 4",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

// Learners found the taught expressions too hard: the old prompt pitched
// them above the learner's level. They must now be everyday spoken phrases.
func TestBuildGapAnalysisPromptAsksForEverydayExpressions(t *testing.T) {
	got := buildGapAnalysisPrompt(promptQuestion, msgs("I think companies."), "制度を変える必要がある。")
	if !strings.Contains(got, "everyday spoken English") {
		t.Fatalf("prompt missing the everyday-phrase instruction:\n%s", got)
	}
	for _, forbidden := range []string{"slightly above", "level 3 of 5"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("prompt must not pitch above the learner's level (%q):\n%s", forbidden, got)
		}
	}
}

func TestBuildGapAnalysisPromptAsksForCorrections(t *testing.T) {
	got := buildGapAnalysisPrompt(promptQuestion, msgs("I think companies."), "制度を変える必要がある。")
	for _, want := range []string{
		"corrections",
		"at most 3",
		"exactly as the learner wrote it",
		"never invent a mistake",
		"empty list",
		"note_ja",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildRetryReviewPromptIncludesAnswersAndExpressions(t *testing.T) {
	got := buildRetryReviewPrompt(promptQuestion, "I think companies.",
		"Companies should take responsibility for their impact.",
		[]Expression{{Phrase: "take responsibility for"}, {Phrase: "make systemic changes"}})
	for _, want := range []string{
		"First answer: I think companies.",
		"New answer: Companies should take responsibility for their impact.",
		"- take responsibility for",
		"- make systemic changes",
		"Do not rewrite",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildRetryReviewPromptWithoutExpressionsSkipsUsageFeedback(t *testing.T) {
	got := buildRetryReviewPrompt(promptQuestion, "I think companies.",
		"I still think companies, because they pollute more.", nil)
	for _, forbidden := range []string{
		"Expressions taught",
		"taught expressions",
		"studied a few new expressions",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("prompt for a no-expressions session must not contain %q:\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, "what improved compared with the first answer") {
		t.Fatalf("prompt missing the before/after comparison request:\n%s", got)
	}
}
