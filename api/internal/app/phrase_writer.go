package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const phraseExampleCount = 3

type PhraseSource struct {
	Phrase   string
	Japanese string
	English  string
}

type PhraseContent struct {
	Meaning  string   `json:"meaning"`
	Examples []string `json:"examples"`
}

type PhraseWriter interface {
	Write(ctx context.Context, src PhraseSource) (*PhraseContent, error)
}

func buildPhrasePrompt(src PhraseSource) string {
	var b strings.Builder
	b.WriteString("You are an English tutor helping a Japanese speaker build their vocabulary.\n\n")
	b.WriteString("The learner saved this English phrase to remember:\n")
	fmt.Fprintf(&b, "Phrase: %s\n\n", src.Phrase)
	b.WriteString("They came across it while translating this sentence. Use it only to tell which sense of the phrase ")
	b.WriteString("they meant; do not mention it or reuse it.\n")
	fmt.Fprintf(&b, "Japanese: %s\n", src.Japanese)
	fmt.Fprintf(&b, "English: %s\n\n", src.English)
	b.WriteString("Write, in English only:\n")
	b.WriteString("- \"meaning\": a short, simple definition of the phrase, under 15 words, in plain English a learner ")
	b.WriteString("understands. No Japanese, no example, and do not repeat the phrase itself.\n")
	fmt.Fprintf(&b, "- \"examples\": exactly %d natural example sentences a native speaker would actually say in ", phraseExampleCount)
	b.WriteString("conversation, each from a different everyday situation (for example work, home, travel, friends). ")
	b.WriteString("Keep each one short (under 15 words) and use the phrase exactly as written, without changing its ")
	b.WriteString("form. No Japanese and no translations.\n")
	return b.String()
}

func cleanPhraseContent(phrase string, content *PhraseContent) (*PhraseContent, error) {
	if content == nil {
		return nil, errors.New("phrase writer returned nothing")
	}
	meaning := strings.Join(strings.Fields(content.Meaning), " ")
	if meaning == "" {
		return nil, errors.New("phrase writer returned an empty meaning")
	}
	want := phraseKey(phrase)
	examples := make([]string, 0, phraseExampleCount)
	for _, ex := range content.Examples {
		ex = strings.Join(strings.Fields(ex), " ")
		if ex == "" || !strings.Contains(phraseKey(ex), want) {
			continue
		}
		examples = append(examples, ex)
		if len(examples) == phraseExampleCount {
			break
		}
	}
	if len(examples) == 0 {
		return nil, fmt.Errorf("phrase writer returned no examples using %q", phrase)
	}
	return &PhraseContent{Meaning: meaning, Examples: examples}, nil
}

func phraseKey(s string) string {
	s = answerPunctuation.Replace(s)
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}
