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

// The two phrase sources are ranked, not interchangeable. The point of the
// mode is the gap — the idea the learner had but could not get into English
// — so phrases start from the Japanese reflection, and chunks of the rewrite
// only fill slots the gap left over. A model free to choose "either source"
// can spend all four slots on wording the learner already managed.
func TestBuildSummaryPromptTakesPhrasesFromTheGapFirst(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs."), "犬が好き")
	for _, want := range []string{
		"phrases",
		"at most 4",
		"Start from what the learner could not say",
		"each idea that stayed in the Japanese text",
		"Only once those are covered",
		"never pad the list",
		"meaning_en",
		"example_en",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
	// Matched on the full old phrasing, not on "either source" alone — that
	// substring also lives inside "neither source", which the ranked version
	// legitimately uses.
	if strings.Contains(got, "taken from either source") {
		t.Fatalf("prompt must rank the phrase sources, not offer them as equals:\n%s", got)
	}
}

// "Never pad" is about the absence of phrases worth teaching, not about how
// many ideas the reflection happened to hold. Tying it to the gap count
// cancels the fallback the sentence before it grants: three gap phrases plus
// one worthwhile rewrite chunk is a legitimate four.
func TestBuildSummaryPromptLetsRewriteChunksFillRemainingSlots(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs."), "\u72ac\u304c\u597d\u304d")
	if !strings.Contains(got, "If neither source offers four phrases worth remembering, return fewer") {
		t.Fatalf("prompt must tie \"never pad\" to candidate worth, not to the gap count:\n%s", got)
	}
	if strings.Contains(got, "fewer than four such ideas") {
		t.Fatalf("prompt must not cap the list at the number of ideas in the reflection:\n%s", got)
	}
}

// "Everyday" is stated as a test the model can apply, not as a list of
// prohibitions that leaves it guessing what is left. Reaching for a fancier
// phrase is worse than returning fewer, so unsure means drop it.
func TestBuildSummaryPromptKeepsPhrasesToEverydaySpeech(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs."), "犬が好き")
	for _, want := range []string{
		"would a friend say it to you in ordinary conversation today",
		"Prefer the plainest wording that carries the idea",
		"leave it out",
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

// The explanation section is one overall diagnosis, not a per-sentence
// list: the patterns that made the learner's English sound unnatural, and
// what to do differently next time.
func TestBuildSummaryPromptAsksWhyItSoundedUnnaturalAndHowToFixIt(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs."), "犬が好き")
	for _, want := range []string{
		"naturalness_why_en",
		"naturalness_fix_en",
		"patterns",
		"at most 3 sentences",
		"quote the learner's own words",
		"invent problems",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

// A learner whose English already sounded natural still reads a section —
// the model says so and names what to polish next, rather than going blank.
func TestBuildSummaryPromptCoversTheAlreadyNaturalCase(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs."), "犬が好き")
	for _, want := range []string{
		"already sounded natural",
		"never leave either field empty",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}
