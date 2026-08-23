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

func TestBuildDiscussionReplyPromptForbidsCorrectionAndTracksFollowUps(t *testing.T) {
	got := buildDiscussionReplyPrompt(promptQuestion, msgs("I think companies.", "Why?", "Because."))
	for _, want := range []string{
		"Never correct the learner's grammar",
		"never answer the question for them",
		"You have asked 1 follow-up question(s) so far",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
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
		"level 3 of 5",
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
