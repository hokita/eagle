package app

import (
	"strings"
	"testing"
)

func msgs(texts ...string) []DiscussionMessage {
	out := make([]DiscussionMessage, len(texts))
	for i, t := range texts {
		role := "user"
		if i%2 == 1 {
			role = "ai"
		}
		out[i] = DiscussionMessage{Role: role, Text: t}
	}
	return out
}

func TestValidateTranscriptOK(t *testing.T) {
	if err := validateTranscript(msgs("I think companies.", "Why?", "Because they pollute more.")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateTranscriptEmpty(t *testing.T) {
	if err := validateTranscript(nil); err == nil {
		t.Fatal("expected error for empty transcript")
	}
}

func TestValidateTranscriptTooLong(t *testing.T) {
	texts := make([]string, maxTranscriptMessages+1)
	for i := range texts {
		texts[i] = "x"
	}
	if err := validateTranscript(msgs(texts...)); err == nil {
		t.Fatal("expected error for transcript over the cap")
	}
}

func TestValidateTranscriptRolesMustAlternateStartingWithUser(t *testing.T) {
	bad := []DiscussionMessage{{Role: "ai", Text: "Why?"}}
	if err := validateTranscript(bad); err == nil {
		t.Fatal("expected error when first message is not from the user")
	}
	bad = []DiscussionMessage{{Role: "user", Text: "a"}, {Role: "user", Text: "b"}}
	if err := validateTranscript(bad); err == nil {
		t.Fatal("expected error when roles do not alternate")
	}
}

func TestValidateTranscriptRejectsBlankAndOversizedMessages(t *testing.T) {
	if err := validateTranscript(msgs("   ")); err == nil {
		t.Fatal("expected error for whitespace-only message")
	}
	if err := validateTranscript(msgs(strings.Repeat("a", maxDiscussionTurnLength+1))); err == nil {
		t.Fatal("expected error for oversized message")
	}
}

func TestCountAITurns(t *testing.T) {
	if got := countAITurns(msgs("a", "b", "c", "d", "e")); got != 2 {
		t.Fatalf("expected 2 AI turns, got %d", got)
	}
	if got := countAITurns(msgs("a")); got != 0 {
		t.Fatalf("expected 0 AI turns, got %d", got)
	}
}
