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
	"time"

	"cloud.google.com/go/firestore"
)

type row struct {
	ID         int         `json:"id"`
	Japanese   string      `json:"japanese"`
	English    string      `json:"english"`
	Page       json.Number `json:"page"`
	IsReported int         `json:"is_reported"`
}

func main() {
	path := flag.String("file", "docs/sentences_export.ndjson", "path to NDJSON export")
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
		_, err := client.Collection("sentences").Doc(strconv.Itoa(rw.ID)).Set(ctx, map[string]interface{}{
			"japanese":    rw.Japanese,
			"english":     rw.English,
			"page":        rw.Page.String(),
			"is_reported": rw.IsReported != 0,
			"created_at":  now,
			"updated_at":  now,
		})
		if err != nil {
			log.Fatalf("write sentence %d: %v", rw.ID, err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("scan: %v", err)
	}
	fmt.Printf("seeded %d sentences\n", count)
}
