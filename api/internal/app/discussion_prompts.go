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

// buildSummaryPrompt produces the one analysis prompt a session runs, after
// the conversation and the Japanese reflection are both in. It asks for two
// things the learner reads back in English: a single natural rewrite of
// everything they said, and a few reusable phrases. Deliberately not a
// per-sentence mistake list — the rewrite carries the corrections implicitly,
// in the shape the learner would actually say.
func buildSummaryPrompt(q *DiscussionQuestion, transcript []DiscussionMessage, reflectionJA string) string {
	var b strings.Builder
	b.WriteString("You are an English tutor. A Japanese learner discussed a question in English, ")
	b.WriteString("then wrote in Japanese what else they wanted to say but could not express in English.\n\n")
	fmt.Fprintf(&b, "Discussion question: %s\n\n", q.QuestionEN)
	b.WriteString("English conversation:\n")
	b.WriteString(renderTranscript(transcript))
	fmt.Fprintf(&b, "\nWhat the learner also wanted to say (in Japanese):\n%s\n\n", reflectionJA)
	b.WriteString("Produce:\n")
	b.WriteString("1. natural_english: a single short paragraph that says everything the learner said ")
	b.WriteString("across the whole conversation, including the ideas they could only write in Japanese, ")
	b.WriteString("the way a native speaker would say it in casual conversation. Merge their separate ")
	b.WriteString("answers into connected sentences, keep their meaning and their opinions exactly, and ")
	b.WriteString("invent no new content. Keep it to at most 4 sentences.\n")
	b.WriteString("2. phrases: at most 4 phrases worth remembering, taken from either source — reusable ")
	b.WriteString("chunks that appear in natural_english, or a phrase that would let the learner ")
	b.WriteString("say an idea that stayed in the Japanese text. Prefer short chunks of common words ")
	b.WriteString("(\"in the future\", \"kind of a hassle\", \"I'd rather\") over single words — everyday spoken English ")
	b.WriteString("only, no idioms, no business or literary vocabulary, nothing the learner could not use ")
	b.WriteString("in a casual conversation today. For each: phrase (the chunk itself), meaning_en (a short ")
	b.WriteString("plain-English explanation), example_en (one natural example sentence using it).\n")
	b.WriteString("\nWrite every part of your response in English, including the explanations.\n")
	return b.String()
}
