package main

import "testing"

func validRow() row {
	return row{
		ID: 1, Topic: "environment", Level: 3,
		QuestionEN:   "Who should take more responsibility for environmental problems?",
		TargetSkills: []string{"giving opinions"},
		IsActive:     1,
	}
}

func TestToFirestoreFieldsOK(t *testing.T) {
	fields, err := toFirestoreFields(validRow(), "2026-08-23T00:00:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fields["question_en"] != validRow().QuestionEN || fields["is_active"] != true ||
		fields["level"] != 3 || fields["topic"] != "environment" ||
		fields["created_at"] != "2026-08-23T00:00:00Z" {
		t.Fatalf("unexpected fields: %+v", fields)
	}
	skills, ok := fields["target_skills"].([]string)
	if !ok || len(skills) != 1 {
		t.Fatalf("unexpected target_skills: %+v", fields["target_skills"])
	}
}

func TestToFirestoreFieldsRejectsBadRows(t *testing.T) {
	bad := validRow()
	bad.Level = 6
	if _, err := toFirestoreFields(bad, "now"); err == nil {
		t.Fatal("expected error for level out of range")
	}
	bad = validRow()
	bad.QuestionEN = "  "
	if _, err := toFirestoreFields(bad, "now"); err == nil {
		t.Fatal("expected error for blank question")
	}
	bad = validRow()
	bad.Topic = ""
	if _, err := toFirestoreFields(bad, "now"); err == nil {
		t.Fatal("expected error for blank topic")
	}
	bad = validRow()
	bad.TargetSkills = nil
	if _, err := toFirestoreFields(bad, "now"); err == nil {
		t.Fatal("expected error for missing target_skills")
	}
}
