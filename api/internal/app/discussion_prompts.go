package app

import (
	"fmt"
	"strings"
)

// renderTranscript renders the conversation with "Learner:"/"You:" labels —
// "You" is the coach, so the model reads its own past turns correctly.
func renderTranscript(transcript []DiscussionMessage) string {
	var b strings.Builder
	for _, m := range transcript {
		label := "Learner"
		if m.Role == "ai" {
			label = "You"
		}
		fmt.Fprintf(&b, "%s: %s\n", label, m.Text)
	}
	return b.String()
}

// buildDiscussionReplyPrompt is a pure function (unit-testable without
// network) producing the follow-up-question prompt. How many follow-ups a
// session gets is the server's decision (discussionFollowUps), so this
// prompt never offers the model a way to end the conversation — it always
// asks exactly one more question.
func buildDiscussionReplyPrompt(q *DiscussionQuestion, transcript []DiscussionMessage) string {
	var b strings.Builder
	b.WriteString("You are a friendly English conversation partner helping a Japanese learner ")
	b.WriteString("practice expressing their own opinions in English.\n\n")
	fmt.Fprintf(&b, "Discussion question: %s\n", q.QuestionEN)
	fmt.Fprintf(&b, "Practice goals for this question: %s\n\n", strings.Join(q.TargetSkills, ", "))
	b.WriteString("Conversation so far:\n")
	b.WriteString(renderTranscript(transcript))
	b.WriteString("\nRules:\n")
	b.WriteString("- Ask exactly ONE short follow-up question that draws more of the learner's own ")
	b.WriteString("thinking out — why they think so, a concrete example, the opposite view, or what they would do.\n")
	b.WriteString("- Never correct the learner's grammar or vocabulary, never rewrite their sentences, ")
	b.WriteString("and never answer the question for them or suggest ideas.\n")
	b.WriteString("- Never ask the learner to use Japanese.\n")
	b.WriteString("- Keep your message to at most 2 short sentences of natural spoken English.\n")
	b.WriteString("\nRespond as JSON with the field \"message\" (string) holding your question.\n")
	return b.String()
}

// buildGapAnalysisPrompt compares the ideas of the English conversation with
// the Japanese reflection, asks for 2-4 everyday expressions to close the
// gap, and for corrections of the mistakes the learner actually made — the
// only place in a session where their English is corrected at all.
func buildGapAnalysisPrompt(q *DiscussionQuestion, transcript []DiscussionMessage, reflectionJA string) string {
	var b strings.Builder
	b.WriteString("You are an English tutor. A Japanese learner discussed a question in English, ")
	b.WriteString("then wrote in Japanese what else they wanted to say but could not express in English.\n\n")
	fmt.Fprintf(&b, "Discussion question: %s\n\n", q.QuestionEN)
	b.WriteString("English conversation:\n")
	b.WriteString(renderTranscript(transcript))
	fmt.Fprintf(&b, "\nWhat the learner also wanted to say (in Japanese):\n%s\n\n", reflectionJA)
	b.WriteString("Compare the ideas and intentions, not literal wording. Produce:\n")
	b.WriteString("1. expressed_ideas: the main ideas the learner successfully communicated in English ")
	b.WriteString("(short English sentences, at most 5).\n")
	b.WriteString("2. missing_ideas: ideas present in the Japanese text that never appeared in the English ")
	b.WriteString("conversation (short English sentences, at most 5).\n")
	b.WriteString("3. expressions: at most 4 everyday spoken English phrases that would let the learner say ")
	b.WriteString("the missing ideas. Prefer short reusable chunks of common words (\"kind of a hassle\", ")
	b.WriteString("\"I'd rather\") over single words — no idioms, no business or literary vocabulary, ")
	b.WriteString("nothing the learner could not use in a casual conversation today. ")
	b.WriteString("For each: phrase (the chunk itself), meaning_ja (a short Japanese gloss), ")
	b.WriteString("example_en (one natural example sentence using it). Return an ")
	b.WriteString("empty list when the learner expressed everything they wanted to say — never invent ")
	b.WriteString("something to teach.\n")
	b.WriteString("4. corrections: at most 3 sentences from the learner's English that contain a grammar ")
	b.WriteString("mistake or an unnatural word choice. Quote the sentence exactly as the learner wrote it ")
	b.WriteString("in \"original\", give a natural spoken rewrite keeping their meaning in \"better\", and one ")
	b.WriteString("short Japanese sentence explaining the fix in \"note_ja\". Flag only real mistakes — ")
	b.WriteString("never invent a mistake and never flag a matter of style. Return an empty list when the ")
	b.WriteString("learner's English was already fine.\n")
	return b.String()
}

// buildRetryReviewPrompt produces encouraging usage feedback on the retry —
// never a rewrite or correction list; mistakes are handled by the gap
// analysis instead. A session can still reach completion with no taught
// expressions (the complete endpoint accepts an empty list), and the prompt
// must not claim any were studied, or the model invents usage feedback about
// nothing.
func buildRetryReviewPrompt(q *DiscussionQuestion, firstAnswer, retryAnswer string, expressions []Expression) string {
	var b strings.Builder
	b.WriteString("You are an English tutor. A learner answered a discussion question")
	if len(expressions) > 0 {
		b.WriteString(", studied a few new expressions,")
	}
	b.WriteString(" and then answered the same question again.\n\n")
	fmt.Fprintf(&b, "Question: %s\n", q.QuestionEN)
	fmt.Fprintf(&b, "First answer: %s\n", firstAnswer)
	if len(expressions) > 0 {
		b.WriteString("Expressions taught:\n")
		for _, e := range expressions {
			fmt.Fprintf(&b, "- %s\n", e.Phrase)
		}
	}
	fmt.Fprintf(&b, "New answer: %s\n\n", retryAnswer)
	if len(expressions) > 0 {
		b.WriteString("Write 2-3 friendly sentences in English: say which of the taught expressions the learner ")
		b.WriteString("actually used, and what improved compared with the first answer. ")
	} else {
		b.WriteString("Write 2-3 friendly sentences in English about what improved compared with the first answer. ")
	}
	b.WriteString("Do not rewrite their answer, do not list grammar mistakes, do not suggest further corrections. ")
	b.WriteString("Respond with plain text only.\n")
	return b.String()
}
