package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/firestore"
)

func main() {
	ctx := context.Background()

	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		log.Fatal("GOOGLE_CLOUD_PROJECT is required")
	}

	allowedEmail := os.Getenv("ALLOWED_EMAIL")
	if allowedEmail == "" {
		log.Fatal("ALLOWED_EMAIL is required")
	}

	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("failed to create Firestore client: %v", err)
	}
	defer client.Close()

	verifier, err := NewFirebaseVerifier(ctx, projectID)
	if err != nil {
		log.Fatalf("failed to create auth verifier: %v", err)
	}

	srv := NewServer(NewFirestoreRepo(client))

	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return requireAuth(verifier, allowedEmail, h)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/sentence/random", auth(srv.getRandomSentence))
	mux.HandleFunc("/api/answer/check", auth(srv.checkAnswer))
	mux.HandleFunc("/api/sentence/report", auth(srv.reportSentence))
	mux.HandleFunc("/api/liveness", livenessHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
