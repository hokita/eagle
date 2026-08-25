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

func TestBuildSummaryPromptIncludesConversationAndReflection(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs.", "What kind?", "I like shiba-dog."), "今は猫を飼っているが将来は犬を飼いたい")
	for _, want := range []string{
		promptQuestion.QuestionEN,
		"Learner: I like dogs.",
		"You: What kind?",
		"Learner: I like shiba-dog.",
		"今は猫を飼っているが将来は犬を飼いたい",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

// The rewrite is one passage covering the whole conversation, not a
// per-sentence correction list: the learner's turns are merged with the
// ideas from their Japanese reflection into something they could have said.
func TestBuildSummaryPromptAsksForOneNaturalPassage(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs."), "犬が好き")
	for _, want := range []string{
		"natural_english",
		"single short paragraph",
		"everything the learner said",
		"including the ideas they could only write in Japanese",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"corrections", "original", "note_ja"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("prompt must not ask for per-sentence corrections (%q):\n%s", forbidden, got)
		}
	}
}

// Phrases may come from either source: a reusable chunk of the rewrite, or
// an expression that covers an idea which stayed in the Japanese text.
func TestBuildSummaryPromptAsksForPhrasesFromBothSources(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs."), "犬が好き")
	for _, want := range []string{
		"phrases",
		"at most 4",
		"chunks that appear in natural_english",
		"say an idea that stayed in the Japanese text",
		"everyday spoken English",
		"meaning_en",
		"example_en",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

// The reflection is the only Japanese in the session; every question and
// explanation the learner reads back is English.
func TestBuildSummaryPromptKeepsExplanationsInEnglish(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs."), "犬が好き")
	if !strings.Contains(got, "Write every part of your response in English") {
		t.Fatalf("prompt missing the English-only instruction:\n%s", got)
	}
	if strings.Contains(got, "Japanese gloss") {
		t.Fatalf("prompt must not ask for a Japanese gloss:\n%s", got)
	}
}
