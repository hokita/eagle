package app

import (
	"context"
	"fmt"
	"strings"
)

// WeaknessAnalyzer produces a natural-language summary of a learner's
// recurring weaknesses from the set of sentences they have gotten wrong.
type WeaknessAnalyzer interface {
	Analyze(ctx context.Context, mistakes []MistakeSentence, language string) (string, error)
}

// maxWrongAnswersPerSentence bounds how many of a sentence's most recent
// wrong answers are included in the weakness-analysis prompt, so a single
// sentence missed many times doesn't blow up prompt size on its own.
const maxWrongAnswersPerSentence = 5

// maxPromptChars bounds the total size of the mistakes section of the
// weakness-analysis prompt. maxInsightMistakes and maxWrongAnswersPerSentence
// bound the *shape* of the input (how many sentences, how many answers
// each), but not the length of individual fields — a learner's stored
// answers can each be up to maxUserAnswerLength long, so a full 50 sentences
// x 5 answers could still be very large. This is the final backstop: whole
// mistake blocks are dropped, oldest (least recent) first, once adding the
// next one would exceed the budget, so the request sent to Gemini has a
// predictable upper bound regardless of per-field content.
const maxPromptChars = 20000

// buildWeaknessPrompt is a pure function (kept separate from the Gemini
// client so it is unit-testable without network access) that renders the
// learner's mistakes into an analysis prompt. It reuses validExplainLanguages
// semantics: "ja" produces a Japanese analysis, anything else English.
func buildWeaknessPrompt(mistakes []MistakeSentence, language string) string {
	var header strings.Builder
	header.WriteString("You are an English tutor analyzing the mistakes a Japanese learner has made ")
	header.WriteString("while translating Japanese sentences into English.\n\n")
	header.WriteString("Here are the sentences the learner has gotten wrong, each with the reference ")
	header.WriteString("English translation and the learner's incorrect attempts:\n\n")

	var body strings.Builder
	for _, m := range mistakes {
		var block strings.Builder
		block.WriteString(fmt.Sprintf("Japanese: %s\n", m.Japanese))
		block.WriteString(fmt.Sprintf("Reference English: %s\n", m.CorrectAnswer))
		wrongAnswers := m.WrongAnswers
		if len(wrongAnswers) > maxWrongAnswersPerSentence {
			wrongAnswers = wrongAnswers[:maxWrongAnswersPerSentence]
		}
		for _, w := range wrongAnswers {
			block.WriteString(fmt.Sprintf("Learner wrote: %s\n", w.IncorrectAnswer))
		}
		block.WriteString("\n")

		if body.Len()+block.Len() > maxPromptChars {
			break
		}
		body.WriteString(block.String())
	}

	var footer strings.Builder
	footer.WriteString("Identify the learner's main recurring weaknesses across these mistakes — patterns ")
	footer.WriteString("such as verb tense, subject-verb agreement, articles, plurals, prepositions, word ")
	footer.WriteString("order, vocabulary choice, or register.\n")
	footer.WriteString("Ignore one-off typos, spelling slips, and isolated misunderstandings that do not ")
	footer.WriteString("represent a repeated pattern.\n")
	footer.WriteString("Respond with a 1-2 sentence summary, then a short bulleted list of the top weakness ")
	footer.WriteString("areas, each with a brief, actionable tip.\n")
	footer.WriteString("Format the response as Markdown (use \"- \" for bullets and \"**text**\" for emphasis).\n")
	if language == "ja" {
		footer.WriteString("Write your analysis in Japanese.")
	} else {
		footer.WriteString("Write your analysis in English.")
	}

	return header.String() + body.String() + footer.String()
}
