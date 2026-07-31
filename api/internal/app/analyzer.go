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

// buildWeaknessPrompt is a pure function (kept separate from the Gemini
// client so it is unit-testable without network access) that renders the
// learner's mistakes into an analysis prompt. It reuses validExplainLanguages
// semantics: "ja" produces a Japanese analysis, anything else English.
func buildWeaknessPrompt(mistakes []MistakeSentence, language string) string {
	var b strings.Builder
	b.WriteString("You are an English tutor analyzing the mistakes a Japanese learner has made ")
	b.WriteString("while translating Japanese sentences into English.\n\n")
	b.WriteString("Here are the sentences the learner has gotten wrong, each with the reference ")
	b.WriteString("English translation and the learner's incorrect attempts:\n\n")
	for _, m := range mistakes {
		b.WriteString(fmt.Sprintf("Japanese: %s\n", m.Japanese))
		b.WriteString(fmt.Sprintf("Reference English: %s\n", m.CorrectAnswer))
		for _, w := range m.WrongAnswers {
			b.WriteString(fmt.Sprintf("Learner wrote: %s\n", w.IncorrectAnswer))
		}
		b.WriteString("\n")
	}
	b.WriteString("Identify the learner's main recurring weaknesses across these mistakes — patterns ")
	b.WriteString("such as verb tense, subject-verb agreement, articles, plurals, prepositions, word ")
	b.WriteString("order, vocabulary choice, or register.\n")
	b.WriteString("Ignore one-off typos, spelling slips, and isolated misunderstandings that do not ")
	b.WriteString("represent a repeated pattern.\n")
	b.WriteString("Respond with a 1-2 sentence summary, then a short bulleted list of the top weakness ")
	b.WriteString("areas, each with a brief, actionable tip.\n")
	if language == "ja" {
		b.WriteString("Write your analysis in Japanese.")
	} else {
		b.WriteString("Write your analysis in English.")
	}
	return b.String()
}
