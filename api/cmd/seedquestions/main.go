package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
)

type row struct {
	ID           int      `json:"id"`
	Topic        string   `json:"topic"`
	Level        int      `json:"level"`
	QuestionEN   string   `json:"question_en"`
	TargetSkills []string `json:"target_skills"`
	IsActive     int      `json:"is_active"`
}

// toFirestoreFields builds the Firestore write payload for one NDJSON row,
// validating the fields the app depends on.
func toFirestoreFields(rw row, now string) (map[string]interface{}, error) {
	if rw.Level < 1 || rw.Level > 5 {
		return nil, fmt.Errorf("question %d: level must be 1-5, got %d", rw.ID, rw.Level)
	}
	if strings.TrimSpace(rw.QuestionEN) == "" {
		return nil, fmt.Errorf("question %d: question_en is blank", rw.ID)
	}
	if strings.TrimSpace(rw.Topic) == "" {
		return nil, fmt.Errorf("question %d: topic is blank", rw.ID)
	}
	if len(rw.TargetSkills) == 0 {
		return nil, fmt.Errorf("question %d: target_skills is empty", rw.ID)
	}
	return map[string]interface{}{
		"question_en":   rw.QuestionEN,
		"topic":         rw.Topic,
		"level":         rw.Level,
		"target_skills": rw.TargetSkills,
		"is_active":     rw.IsActive != 0,
		"created_at":    now,
		"updated_at":    now,
	}, nil
}

func main() {
	// The command must run from api/ (the module root), so the default
	// resolves the committed bank at the repository-level docs/ directory.
	path := flag.String("file", "../docs/discussion_questions_seed.ndjson", "path to NDJSON file")
	flag.Parse()

	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		log.Fatal("GOOGLE_CLOUD_PROJECT is required")
	}

	ctx := context.Background()
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("firestore client: %v", err)
	}
	defer client.Close()

	f, err := os.Open(*path)
	if err != nil {
		log.Fatalf("open %s: %v", *path, err)
	}
	defer f.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	count := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rw row
		if err := json.Unmarshal(line, &rw); err != nil {
			log.Fatalf("parse line: %v", err)
		}
		fields, err := toFirestoreFields(rw, now)
		if err != nil {
			log.Fatalf("invalid row: %v", err)
		}
		if _, err := client.Collection("discussion_questions").Doc(strconv.Itoa(rw.ID)).Set(ctx, fields); err != nil {
			log.Fatalf("write question %d: %v", rw.ID, err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("scan: %v", err)
	}
	fmt.Printf("seeded %d discussion questions\n", count)
}
