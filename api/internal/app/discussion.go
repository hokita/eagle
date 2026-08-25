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
	// character-based maxLength), so a maximal valid request is 12 messages
	// x 2,000 runes + a 4,000-rune reflection = 28,000 runes, up to 4 UTF-8
	// bytes each (~112 KiB) plus JSON escaping overhead. A body cap below
	// that would reject a session whose every field passed its own
	// validation.
	maxDiscussionRequestBytes = 192 * 1024
	// maxDiscussionTurnLength bounds a single transcript message, in runes —
	// the same unit the browser textareas' maxLength approximates, so text
	// the client accepts is never rejected server-side for its length.
	maxDiscussionTurnLength = 2000
	// maxReflectionLength bounds the Japanese reflection text, in runes.
	maxReflectionLength = 4000
	// maxTranscriptMessages: the current flow produces at most
	// 1 + 2*discussionFollowUps = 5 messages. The cap is left at the older,
	// looser 12 so sessions recorded under the previous variable-length flow
	// still validate on their way into history.
	maxTranscriptMessages = 12
	// discussionFollowUps is how many follow-up questions every session
	// gets. It is server arithmetic, not a model decision: the reply handler
	// returns done — without calling Gemini — once that many AI turns are in
	// the transcript, and the coach prompt has no way to end the
	// conversation early.
	discussionFollowUps = 2
	// maxDiscussionSessionList caps the history list response.
	maxDiscussionSessionList = 50
	// maxSessionPhrases caps the phrase list the coach's summary may teach.
	// An empty list is valid — a learner who already said everything
	// naturally has nothing worth picking up.
	maxSessionPhrases = 4
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

// Phrase is one chunk worth remembering, explained in English — the
// reflection is the only Japanese a session contains.
type Phrase struct {
	Phrase    string `json:"phrase"`
	MeaningEN string `json:"meaning_en"`
	ExampleEN string `json:"example_en"`
}

// Summary is what a finished session gives back: one natural rewrite of
// everything the learner said, and a few phrases to keep. NaturalEnglish is
// required; Phrases may legitimately be empty.
type Summary struct {
	NaturalEnglish string   `json:"natural_english"`
	Phrases        []Phrase `json:"phrases"`
}

// CoachReply is what the model produces for a follow-up turn: a question and
// nothing else. Whether the conversation continues is the server's call, so
// there is deliberately no "done" field for the model to set.
type CoachReply struct {
	Message string `json:"message"`
}

type DiscussionSession struct {
	ID             string              `json:"id"`
	QuestionID     int                 `json:"question_id"`
	QuestionEN     string              `json:"question_en"`
	Topic          string              `json:"topic"`
	Transcript     []DiscussionMessage `json:"transcript"`
	ReflectionJA   string              `json:"reflection_ja"`
	NaturalEnglish string              `json:"natural_english"`
	Phrases        []Phrase            `json:"phrases"`
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

// DiscussionCoach is the LLM seam for the two AI steps of a session: the
// follow-up questions, and the closing summary.
type DiscussionCoach interface {
	Reply(ctx context.Context, q *DiscussionQuestion, transcript []DiscussionMessage) (*CoachReply, error)
	Summarize(ctx context.Context, q *DiscussionQuestion, transcript []DiscussionMessage, reflectionJA string) (*Summary, error)
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
