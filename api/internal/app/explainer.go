package app

import (
	"context"
	"fmt"
	"strings"
)

// Explainer generates a natural-language explanation comparing a learner's
// English translation to a reference translation of a Japanese sentence.
type Explainer interface {
	Explain(ctx context.Context, japanese, correctAnswer, userAnswer, language string) (string, error)
}

// validExplainLanguages is the allow-list of languages an explanation can be
// written in. Shared by request validation (handlers.go) and prompt
// building (buildExplainPrompt) as a single source of truth.
var validExplainLanguages = map[string]bool{
	"en": true,
	"ja": true,
}

func buildExplainPrompt(japanese, correctAnswer, userAnswer, language string) string {
	var b strings.Builder
	b.WriteString("You are an English tutor helping a Japanese speaker learn English translation.\n\n")
	b.WriteString(fmt.Sprintf("Japanese sentence: %s\n", japanese))
	b.WriteString(fmt.Sprintf("Reference English translation: %s\n", correctAnswer))
	b.WriteString(fmt.Sprintf("Learner's English translation: %s\n\n", userAnswer))
	b.WriteString("The reference translation is only one valid way to translate the sentence, not the only correct answer. ")
	b.WriteString("Judge the learner's translation on its own merits: is it natural, grammatically correct English that ")
	b.WriteString("conveys the same meaning as the Japanese sentence?\n\n")
	b.WriteString("If the learner's translation is acceptable, say so clearly and explain any difference in nuance, ")
	b.WriteString("formality, or phrasing compared to the reference — do not imply it was wrong just because it differs.\n")
	b.WriteString("If the learner's translation has a real grammar, vocabulary, or meaning error, explain what is wrong ")
	b.WriteString("and why the reference translation is more correct.\n\n")
	if language == "ja" {
		b.WriteString("Keep the explanation concise (2-4 sentences) and write it in Japanese.")
	} else {
		b.WriteString("Keep the explanation concise (2-4 sentences) and write it in English.")
	}
	return b.String()
}
