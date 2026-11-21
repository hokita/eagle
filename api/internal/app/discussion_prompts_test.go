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

func TestBuildSummaryPromptAsksForOneRefinementPerLearnerTurn(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs.", "What kind?", "I like shiba-dog."), "犬が好き")
	for _, want := range []string{
		"refined_messages",
		"for each of the learner's turns, in order",
		"Refine each turn on its own",
		"Never merge turns",
		"The conversation has 2 learner turns, so refined_messages must contain exactly 2 strings",
		"If a turn already sounds natural, return it unchanged",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"natural_english", "single short paragraph", "corrections", "note_ja"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("prompt must not ask for a merged passage or a correction list (%q):\n%s", forbidden, got)
		}
	}
}

func TestBuildSummaryPromptKeepsRefinementsToWhatEachTurnSaid(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs."), "犬が好き")
	for _, want := range []string{
		"keep its meaning and the learner's opinions exactly",
		"invent no new content",
		"about as long as the learner's own turn",
		"never move an idea from one turn into another",
		"The ideas they could only write in Japanese belong in phrases, not here",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildSummaryPromptAsksForSpokenRegisterInTheRefinements(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs."), "犬が好き")
	for _, want := range []string{
		"say it out loud to a friend",
		"as speech rather than as writing",
		"softeners people actually say",
		"Prefer the plainest wording that carries the idea",
		"more literary or formal word",
		"would a friend say it to you in ordinary conversation today",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
	for _, want := range []string{
		"only where the learner's own words already carry it",
		"never on an opinion they stated flatly",
		"never let one add a fact, a timeframe, or a degree of conviction the learner did not express",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
}

func TestBuildSummaryPromptTakesPhrasesFromTheGapFirst(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs."), "犬が好き")
	for _, want := range []string{
		"phrases",
		"at most 4",
		"Start from what the learner could not say",
		"ideas that stayed in the Japanese text",
		"Only once those are covered",
		"never pad the list",
		"meaning_en",
		"example_en",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "taken from either source") {
		t.Fatalf("prompt must rank the phrase sources, not offer them as equals:\n%s", got)
	}
}

func TestBuildSummaryPromptPrioritizesGapIdeasWithinTheCap(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs."), "\u72ac\u304c\u597d\u304d")
	for _, want := range []string{
		"which would help them most",
		"up to the four-phrase cap",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "for each idea that stayed in the Japanese text") {
		t.Fatalf("prompt must not demand a phrase for every idea; the cap is 4:\n%s", got)
	}
}

func TestBuildSummaryPromptLetsRewriteChunksFillRemainingSlots(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs."), "\u72ac\u304c\u597d\u304d")
	if !strings.Contains(got, "If neither source offers four phrases worth remembering, return fewer") {
		t.Fatalf("prompt must tie \"never pad\" to candidate worth, not to the gap count:\n%s", got)
	}
	if strings.Contains(got, "fewer than four such ideas") {
		t.Fatalf("prompt must not cap the list at the number of ideas in the reflection:\n%s", got)
	}
}

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

func TestBuildSummaryPromptKeepsExplanationsInEnglish(t *testing.T) {
	got := buildSummaryPrompt(promptQuestion, msgs("I like dogs."), "犬が好き")
	if !strings.Contains(got, "Write every part of your response in English") {
		t.Fatalf("prompt missing the English-only instruction:\n%s", got)
	}
	if strings.Contains(got, "Japanese gloss") {
		t.Fatalf("prompt must not ask for a Japanese gloss:\n%s", got)
	}
}

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
