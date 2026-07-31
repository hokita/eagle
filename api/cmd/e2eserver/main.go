package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/firestore"

	"github.com/hokita/eagle/internal/app"
)

// stubExplanation is asserted verbatim by e2e/tests/incorrect-explain.spec.ts.
const stubExplanation = "This is a stub explanation for e2e tests."

// stubExplainer avoids real Gemini calls in e2e tests, keeping every CI run
// free, fast, and deterministic while still exercising the full HTTP/auth
// wiring around the Explain endpoint.
type stubExplainer struct{}

func (stubExplainer) Explain(_ context.Context, _, _, _, _ string) (string, error) {
	return stubExplanation, nil
}

// stubInsight is returned by stubAnalyzer so e2e runs never call real Gemini.
const stubInsight = "This is a stub weakness insight for e2e tests."

type stubAnalyzer struct{}

func (stubAnalyzer) Analyze(_ context.Context, _ []app.MistakeSentence, _ string) (string, error) {
	return stubInsight, nil
}

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

	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("failed to create Firestore client: %v", err)
	}
	defer client.Close()

	verifier, err := app.NewFirebaseVerifier(ctx, projectID)
	if err != nil {
		log.Fatalf("failed to create auth verifier: %v", err)
	}

	srv := app.NewServer(app.NewFirestoreRepo(client), stubExplainer{}, stubAnalyzer{})

	frontendURL := os.Getenv("FRONTEND_URL")
	mux := app.NewMux(srv, verifier, allowedEmails, frontendURL)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("e2e server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
