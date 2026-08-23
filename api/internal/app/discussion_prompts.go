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
// network) producing the follow-up-question prompt. The JSON shape of the
// answer is enforced by the response schema in GeminiCoach; this prompt
// carries the semantics of done/message and the learning-philosophy rules.
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
	fmt.Fprintf(&b, "- You have asked %d follow-up question(s) so far. ", countAITurns(transcript))
	b.WriteString("Once the learner has answered at least 2 follow-up questions and the conversation feels ")
	b.WriteString("complete, set \"done\" to true and make \"message\" a one-sentence friendly closing ")
	b.WriteString("comment instead of a question.\n")
	b.WriteString("\nRespond as JSON with fields \"done\" (boolean) and \"message\" (string).\n")
	return b.String()
}

// buildGapAnalysisPrompt compares the ideas of the English conversation with
// the Japanese reflection and asks for 2-4 reusable expressions.
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
	b.WriteString("3. expressions: 2 to 4 natural spoken-English expressions that would let the learner say ")
	b.WriteString("the missing ideas. Prefer reusable chunks (\"take responsibility for\") over single words. ")
	fmt.Fprintf(&b, "Pitch them slightly above the learner's current level — this question is level %d of 5. ", q.Level)
	b.WriteString("For each: phrase (the chunk itself), meaning_ja (a short Japanese gloss), ")
	b.WriteString("example_en (one natural example sentence using it).\n")
	return b.String()
}

// buildRetryReviewPrompt produces encouraging usage feedback on the retry —
// never a rewrite or correction list.
func buildRetryReviewPrompt(q *DiscussionQuestion, firstAnswer, retryAnswer string, expressions []Expression) string {
	var b strings.Builder
	b.WriteString("You are an English tutor. A learner answered a discussion question, studied a few new ")
	b.WriteString("expressions, and then answered the same question again.\n\n")
	fmt.Fprintf(&b, "Question: %s\n", q.QuestionEN)
	fmt.Fprintf(&b, "First answer: %s\n", firstAnswer)
	b.WriteString("Expressions taught:\n")
	for _, e := range expressions {
		fmt.Fprintf(&b, "- %s\n", e.Phrase)
	}
	fmt.Fprintf(&b, "New answer: %s\n\n", retryAnswer)
	b.WriteString("Write 2-3 friendly sentences in English: say which of the taught expressions the learner ")
	b.WriteString("actually used, and what improved compared with the first answer. ")
	b.WriteString("Do not rewrite their answer, do not list grammar mistakes, do not suggest further corrections. ")
	b.WriteString("Respond with plain text only.\n")
	return b.String()
}
