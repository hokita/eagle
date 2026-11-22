package app

import (
	"context"
	"fmt"
	"strings"
)

type WeaknessAnalyzer interface {
	Analyze(ctx context.Context, mistakes []MistakeSentence, language string) (string, error)
}

const maxWrongAnswersPerSentence = 5

const maxPromptChars = 20000

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
