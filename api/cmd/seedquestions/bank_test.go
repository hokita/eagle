package main

import (
	"bufio"
	"encoding/json"
	"os"
	"testing"
)

// wantBankSize is the number of questions the committed bank must hold, so
// the discussion mode has enough variety that a user rarely repeats one.
const wantBankSize = 100

// readBank parses the committed NDJSON bank the seeder loads by default.
func readBank(t *testing.T) []row {
	t.Helper()
	f, err := os.Open("../../../docs/discussion_questions_seed.ndjson")
	if err != nil {
		t.Fatalf("open bank: %v", err)
	}
	defer f.Close()

	var rows []row
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var rw row
		if err := json.Unmarshal(scanner.Bytes(), &rw); err != nil {
			t.Fatalf("parse line %d: %v", len(rows)+1, err)
		}
		rows = append(rows, rw)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan bank: %v", err)
	}
	return rows
}

func TestBankIsSeedable(t *testing.T) {
	rows := readBank(t)
	if len(rows) != wantBankSize {
		t.Fatalf("bank holds %d questions, want %d", len(rows), wantBankSize)
	}

	ids := make(map[int]bool, len(rows))
	questions := make(map[string]bool, len(rows))
	for _, rw := range rows {
		if _, err := toFirestoreFields(rw, "2026-08-24T00:00:00Z"); err != nil {
			t.Errorf("row would fail to seed: %v", err)
		}
		if ids[rw.ID] {
			t.Errorf("duplicate id %d", rw.ID)
		}
		ids[rw.ID] = true
		if questions[rw.QuestionEN] {
			t.Errorf("duplicate question %q", rw.QuestionEN)
		}
		questions[rw.QuestionEN] = true
		if rw.IsActive != 1 {
			t.Errorf("question %d is inactive; the bank should ship only active questions", rw.ID)
		}
	}
	for id := 1; id <= wantBankSize; id++ {
		if !ids[id] {
			t.Errorf("missing id %d; ids must be contiguous 1-%d", id, wantBankSize)
		}
	}
}
