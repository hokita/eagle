package main

import (
	"context"
	"fmt"
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

// stubCoach avoids real Gemini calls in e2e tests. Deterministic: a numbered
// follow-up per turn (the server decides when the conversation ends), fixed
// analysis, fixed retry feedback. The literals are asserted verbatim by
// e2e/tests/discussion.spec.ts.
type stubCoach struct{}

func (stubCoach) Reply(_ context.Context, _ *app.DiscussionQuestion, transcript []app.DiscussionMessage) (*app.CoachReply, error) {
	aiTurns := 0
	for _, m := range transcript {
		if m.Role == "ai" {
			aiTurns++
		}
	}
	return &app.CoachReply{Message: fmt.Sprintf("Stub follow-up %d: can you tell me more?", aiTurns+1)}, nil
}

func (stubCoach) AnalyzeGap(_ context.Context, _ *app.DiscussionQuestion, _ []app.DiscussionMessage, _ string) (*app.GapAnalysis, error) {
	return &app.GapAnalysis{
		ExpressedIdeas: []string{"You said companies are responsible."},
		MissingIdeas:   []string{"Systemic change is more effective than individual action."},
		Expressions: []app.Expression{
			{Phrase: "take responsibility for", MeaningJA: "〜に責任を持つ", ExampleEN: "Companies should take responsibility for their impact."},
			{Phrase: "make systemic changes", MeaningJA: "制度的な変更を行う", ExampleEN: "Governments can make systemic changes."},
		},
		Corrections: []app.Correction{
			// Quotes the answer discussion.spec.ts actually types: the real
			// coach drops corrections that are not grounded in a learner turn.
			{Original: "I think companies.", Better: "I think companies are responsible.", NoteJA: "文が途中で終わっています。"},
		},
	}, nil
}

func (stubCoach) ReviewRetry(_ context.Context, _ *app.DiscussionQuestion, _, _ string, _ []app.Expression) (string, error) {
	return "This is a stub retry feedback for e2e tests.", nil
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

	repo := app.NewFirestoreRepo(client)
	srv := app.NewServer(repo, stubExplainer{}, stubAnalyzer{}).WithDiscussion(repo, stubCoach{})

	frontendURL := os.Getenv("FRONTEND_URL")
	mux := app.NewMux(srv, verifier, allowedEmails, frontendURL)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("e2e server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
