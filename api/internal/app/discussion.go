package app

import (
	"context"
	"fmt"
	"strings"
)

const (
	// maxDiscussionRequestBytes bounds transcript-bearing request bodies so an
	// authenticated caller can't exhaust memory or inflate Gemini request size.
	maxDiscussionRequestBytes = 32 * 1024
	// maxDiscussionTurnLength bounds a single transcript message.
	maxDiscussionTurnLength = 2000
	// maxReflectionLength bounds the Japanese reflection text.
	maxReflectionLength = 4000
	// maxTranscriptMessages: initial answer + 5 follow-ups + 5 replies = 11,
	// plus the AI's closing line when the conversation ends = 12.
	maxTranscriptMessages = 12
	// maxAIFollowUps is the server-side hard cap on AI turns — the reply
	// handler returns done without calling Gemini once it is reached.
	maxAIFollowUps = 5
	// maxDiscussionSessionList caps the history list response.
	maxDiscussionSessionList = 50
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

type Expression struct {
	Phrase    string `json:"phrase"`
	MeaningJA string `json:"meaning_ja"`
	ExampleEN string `json:"example_en"`
}

type GapAnalysis struct {
	ExpressedIdeas []string     `json:"expressed_ideas"`
	MissingIdeas   []string     `json:"missing_ideas"`
	Expressions    []Expression `json:"expressions"`
}

type CoachReply struct {
	Done    bool   `json:"done"`
	Message string `json:"message"`
}

type DiscussionSession struct {
	ID             string              `json:"id"`
	QuestionID     int                 `json:"question_id"`
	QuestionEN     string              `json:"question_en"`
	Topic          string              `json:"topic"`
	Transcript     []DiscussionMessage `json:"transcript"`
	ReflectionJA   string              `json:"reflection_ja"`
	ExpressedIdeas []string            `json:"expressed_ideas"`
	MissingIdeas   []string            `json:"missing_ideas"`
	Expressions    []Expression        `json:"expressions"`
	FirstAnswer    string              `json:"first_answer"`
	RetryAnswer    string              `json:"retry_answer"`
	RetryFeedback  string              `json:"retry_feedback"`
	CreatedAt      string              `json:"created_at"`
}

type DiscussionSessionSummary struct {
	ID         string `json:"id"`
	QuestionEN string `json:"question_en"`
	Topic      string `json:"topic"`
	CreatedAt  string `json:"created_at"`
}

// DiscussionRepository is the data-access seam for discussion practice.
type DiscussionRepository interface {
	// RandomQuestion returns a random active question, ErrNoCandidate when
	// the bank is empty.
	RandomQuestion(ctx context.Context) (*DiscussionQuestion, error)
	// GetQuestion returns ErrNotFound for a missing id.
	GetQuestion(ctx context.Context, id int) (*DiscussionQuestion, error)
	// SaveSession writes a completed session and returns its new id.
	SaveSession(ctx context.Context, uid string, s *DiscussionSession) (string, error)
	ListSessions(ctx context.Context, uid string, limit int) ([]DiscussionSessionSummary, error)
	// GetSession returns ErrNotFound for a missing id.
	GetSession(ctx context.Context, uid, id string) (*DiscussionSession, error)
}

// DiscussionCoach is the LLM seam for the three AI steps of a session.
type DiscussionCoach interface {
	Reply(ctx context.Context, q *DiscussionQuestion, transcript []DiscussionMessage) (*CoachReply, error)
	AnalyzeGap(ctx context.Context, q *DiscussionQuestion, transcript []DiscussionMessage, reflectionJA string) (*GapAnalysis, error)
	ReviewRetry(ctx context.Context, q *DiscussionQuestion, firstAnswer, retryAnswer string, expressions []Expression) (string, error)
}

// validateTranscript enforces the transcript shape shared by every
// discussion endpoint: non-empty, capped length, roles alternating starting
// with "user", every message non-blank and within the per-turn cap.
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
		if len(m.Text) > maxDiscussionTurnLength {
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
