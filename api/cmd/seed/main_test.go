package main

import (
	"encoding/json"
	"testing"
)

func TestToFirestoreFieldsValidLevel(t *testing.T) {
	rw := row{ID: 1, Japanese: "あ", English: "a", Page: json.Number("1"), Level: 3}
	fields, err := toFirestoreFields(rw, "2026-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fields["japanese"] != "あ" || fields["english"] != "a" || fields["page"] != "1" {
		t.Fatalf("unexpected fields: %+v", fields)
	}
	if fields["level"] != 3 {
		t.Fatalf("expected level 3, got %v", fields["level"])
	}
	if fields["is_reported"] != false {
		t.Fatalf("expected is_reported false, got %v", fields["is_reported"])
	}
	if fields["created_at"] != "2026-01-01T00:00:00Z" || fields["updated_at"] != "2026-01-01T00:00:00Z" {
		t.Fatalf("unexpected timestamps: %+v", fields)
	}
}

func TestToFirestoreFieldsRejectsOutOfRangeLevel(t *testing.T) {
	for _, lvl := range []int{0, -1, 6, 100} {
		rw := row{ID: 1, Japanese: "あ", English: "a", Page: json.Number("1"), Level: lvl}
		if _, err := toFirestoreFields(rw, "2026-01-01T00:00:00Z"); err == nil {
			t.Fatalf("expected error for level %d", lvl)
		}
	}
}
