package app

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxDiscussionRequestBytes = 192 * 1024
	maxDiscussionTurnLength   = 2000
	maxReflectionLength       = 4000
	maxTranscriptMessages     = 12
	discussionFollowUps       = 2
	maxDiscussionSessionList  = 50
	maxSessionPhrases         = 4
)

type DiscussionQuestion struct {
	ID           int      `json:"id"`
	QuestionEN   string   `json:"question_en"`
	Topic        string   `json:"topic"`
	Level        int      `json:"level"`
	TargetSkills []string `json:"target_skills"`
}

type DiscussionMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type Phrase struct {
	Phrase    string `json:"phrase"`
	MeaningEN string `json:"meaning_en"`
	ExampleEN string `json:"example_en"`
}

type Summary struct {
	RefinedMessages  []string `json:"refined_messages"`
	NaturalnessWhyEN string   `json:"naturalness_why_en"`
	NaturalnessFixEN string   `json:"naturalness_fix_en"`
	Phrases          []Phrase `json:"phrases"`
}

type CoachReply struct {
	Message string `json:"message"`
}

type DiscussionSession struct {
	ID               string              `json:"id"`
	QuestionID       int                 `json:"question_id"`
	QuestionEN       string              `json:"question_en"`
	Topic            string              `json:"topic"`
	Transcript       []DiscussionMessage `json:"transcript"`
	ReflectionJA     string              `json:"reflection_ja"`
	RefinedMessages  []string            `json:"refined_messages"`
	NaturalnessWhyEN string              `json:"naturalness_why_en"`
	NaturalnessFixEN string              `json:"naturalness_fix_en"`
	Phrases          []Phrase            `json:"phrases"`
	CreatedAt        string              `json:"created_at"`
}

type DiscussionSessionSummary struct {
	ID         string `json:"id"`
	QuestionEN string `json:"question_en"`
	Topic      string `json:"topic"`
	CreatedAt  string `json:"created_at"`
}

type DiscussionRepository interface {
	RandomQuestion(ctx context.Context) (*DiscussionQuestion, error)
	GetQuestion(ctx context.Context, id int) (*DiscussionQuestion, error)
	SaveSession(ctx context.Context, uid string, s *DiscussionSession) (string, error)
	ListSessions(ctx context.Context, uid string, limit int) ([]DiscussionSessionSummary, error)
	GetSession(ctx context.Context, uid, id string) (*DiscussionSession, error)
}

type DiscussionCoach interface {
	Reply(ctx context.Context, q *DiscussionQuestion, transcript []DiscussionMessage) (*CoachReply, error)
	Summarize(ctx context.Context, q *DiscussionQuestion, transcript []DiscussionMessage, reflectionJA string) (*Summary, error)
}

func validateTranscript(transcript []DiscussionMessage) error {
	if len(transcript) == 0 {
		return fmt.Errorf("transcript is empty")
	}
	if len(transcript) > maxTranscriptMessages {
		return fmt.Errorf("transcript exceeds %d messages", maxTranscriptMessages)
	}
	for i, m := range transcript {
		want := "user"
		if i%2 == 1 {
			want = "ai"
		}
		if m.Role != want {
			return fmt.Errorf("message %d: expected role %q, got %q", i, want, m.Role)
		}
		if strings.TrimSpace(m.Text) == "" {
			return fmt.Errorf("message %d: text is blank", i)
		}
		if utf8.RuneCountInString(m.Text) > maxDiscussionTurnLength {
			return fmt.Errorf("message %d: text exceeds %d characters", i, maxDiscussionTurnLength)
		}
	}
	return nil
}

func countAITurns(transcript []DiscussionMessage) int {
	n := 0
	for _, m := range transcript {
		if m.Role == "ai" {
			n++
		}
	}
	return n
}

func countLearnerTurns(transcript []DiscussionMessage) int {
	n := 0
	for _, m := range transcript {
		if m.Role == "user" {
			n++
		}
	}
	return n
}
