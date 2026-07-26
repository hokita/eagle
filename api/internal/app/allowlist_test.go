package app

import "testing"

func TestParseAllowedEmailsSplitsOnComma(t *testing.T) {
	got := ParseAllowedEmails("a@example.com,b@example.com")
	want := []string{"a@example.com", "b@example.com"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestParseAllowedEmailsTrimsWhitespace(t *testing.T) {
	got := ParseAllowedEmails(" a@example.com , b@example.com ")
	want := []string{"a@example.com", "b@example.com"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestParseAllowedEmailsDropsEmptyEntries(t *testing.T) {
	got := ParseAllowedEmails("a@example.com,,b@example.com,")
	want := []string{"a@example.com", "b@example.com"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestParseAllowedEmailsEmptyStringReturnsEmpty(t *testing.T) {
	got := ParseAllowedEmails("")
	if len(got) != 0 {
		t.Fatalf("expected empty slice, got %v", got)
	}
}

func TestParseAllowedEmailsSingleEmail(t *testing.T) {
	got := ParseAllowedEmails("solo@example.com")
	if len(got) != 1 || got[0] != "solo@example.com" {
		t.Fatalf("expected [solo@example.com], got %v", got)
	}
}
