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
// the conversation and the Japanese reflection are both in. It asks for
// three things the learner reads back in English: a single natural rewrite
// of everything they said, an explanation of why their own wording sounded
// unnatural and what to do about it, and a few reusable phrases drawn first
// from the gap the reflection exposes — the ideas the learner had but could
// not reach in English are the point of the mode, so wording they already
// managed only fills what the gap leaves over. The
// explanation is deliberately pitched at the pattern level rather than as a
// per-sentence list — the rewrite already shows the shape they should have
// used, so what is left to add is the habit behind the difference.
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
	b.WriteString("2. naturalness_why_en: why the learner's English sounded unnatural to a native ear. ")
	b.WriteString("Describe the patterns across their whole conversation — over-formal or textbook word ")
	b.WriteString("choice, the same opener repeated every turn, phrasings carried over from Japanese ")
	b.WriteString("word order or particles — not a sentence-by-sentence list. Name at most two patterns ")
	b.WriteString("and quote the learner's own words in passing to show each one. Keep it to at most 3 sentences.\n")
	b.WriteString("3. naturalness_fix_en: what the learner should do differently next time, concretely ")
	b.WriteString("enough to act on — what to say instead of the habits you named. Keep it to at most 3 sentences.\n")
	b.WriteString("Judge only what the learner actually wrote, and never invent problems they did not have. ")
	b.WriteString("If their English already sounded natural, say that it already sounded natural in ")
	b.WriteString("naturalness_why_en and name the one thing worth polishing next in naturalness_fix_en — ")
	b.WriteString("never leave either field empty.\n")
	b.WriteString("4. phrases: at most 4 phrases worth remembering. Start from what the learner ")
	b.WriteString("could not say: for each idea that stayed in the Japanese text, give the phrase a ")
	b.WriteString("native speaker would use to say it. Only once those are covered may you add ")
	b.WriteString("reusable chunks from natural_english that replace clumsy wording the learner ")
	b.WriteString("actually used. If the Japanese text holds fewer than four such ideas, return ")
	b.WriteString("fewer phrases — never pad the list. Every phrase must pass this test: ")
	b.WriteString("would a friend say it to you in ordinary conversation today? Prefer the plainest ")
	b.WriteString("wording that carries the idea (\"kind of a hassle\" over \"somewhat burdensome\"), and ")
	b.WriteString("short chunks of common words (\"in the future\", \"I'd rather\") over single words. ")
	b.WriteString("Nothing the learner would have to reach for. When unsure whether a phrase is ")
	b.WriteString("common enough, leave it out. For each: phrase (the chunk itself), meaning_en (a short ")
	b.WriteString("plain-English explanation), example_en (one natural example sentence using it).\n")
	b.WriteString("\nWrite every part of your response in English, including the explanations.\n")
	return b.String()
}
