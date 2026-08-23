package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/firestore"

	"github.com/hokita/eagle/internal/app"
)

func main() {
	ctx := context.Background()

	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		log.Fatal("GOOGLE_CLOUD_PROJECT is required")
	}

	allowedEmails := app.ParseAllowedEmails(os.Getenv("ALLOWED_EMAILS"))
	if len(allowedEmails) == 0 {
		log.Fatal("ALLOWED_EMAILS is required")
	}

	geminiAPIKey := os.Getenv("GEMINI_API_KEY")
	if geminiAPIKey == "" {
		log.Fatal("GEMINI_API_KEY is required")
	}

	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("failed to create Firestore client: %v", err)
	}
	defer client.Close()

	verifier, err := app.NewFirebaseVerifier(ctx, projectID)
	if err != nil {
		log.Fatalf("failed to create auth verifier: %v", err)
	}

	explainer, err := app.NewGeminiExplainer(ctx, geminiAPIKey)
	if err != nil {
		log.Fatalf("failed to create Gemini explainer: %v", err)
	}

	analyzer, err := app.NewGeminiWeaknessAnalyzer(ctx, geminiAPIKey)
	if err != nil {
		log.Fatalf("failed to create Gemini weakness analyzer: %v", err)
	}

	coach, err := app.NewGeminiCoach(ctx, geminiAPIKey)
	if err != nil {
		log.Fatalf("failed to create Gemini coach: %v", err)
	}

	repo := app.NewFirestoreRepo(client)
	srv := app.NewServer(repo, explainer, analyzer).WithDiscussion(repo, coach)

	frontendURL := os.Getenv("FRONTEND_URL")
	mux := app.NewMux(srv, verifier, allowedEmails, frontendURL)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
