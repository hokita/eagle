package app

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// maxDiscussionRequestBytes bounds every discussion request body so an
	// authenticated caller can't exhaust memory or inflate Gemini request
	// size. Sized from the complete endpoint's multibyte worst case — the
	// field limits below are rune counts (matching the frontend textareas'
	// character-based maxLength), so a maximal valid session is 12 messages
	// x 2,000 runes + a 4,000-rune reflection + a 2,000-rune retry answer =
	// 30,000 runes, up to 4 UTF-8 bytes each (~120 KiB) plus JSON escaping
	// overhead and the coach-bounded analysis arrays. A body cap below that
	// would reject a session whose every field passed its own validation.
	maxDiscussionRequestBytes = 192 * 1024
	// maxDiscussionTurnLength bounds a single transcript message, in runes —
	// the same unit the browser textareas' maxLength approximates, so text
	// the client accepts is never rejected server-side for its length.
	maxDiscussionTurnLength = 2000
	// maxReflectionLength bounds the Japanese reflection text, in runes.
	maxReflectionLength = 4000
	// maxTranscriptMessages: initial answer + 5 follow-ups + 5 replies = 11,
	// plus the AI's closing line when the conversation ends = 12.
	maxTranscriptMessages = 12
	// maxAIFollowUps is the server-side hard cap on AI turns — the reply
	// handler returns done without calling Gemini once it is reached.
	maxAIFollowUps = 5
	// maxDiscussionSessionList caps the history list response.
	maxDiscussionSessionList = 50
	// maxSessionExpressions caps the expressions list accepted by the
	// complete endpoint and enforced by the coach's gap analysis.
	maxSessionExpressions = 4
	// maxSessionIdeas caps the expressed/missing idea lists accepted by the
	// complete endpoint and enforced by the coach's gap analysis.
	maxSessionIdeas = 20
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
