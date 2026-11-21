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

const stubExplanation = "This is a stub explanation for e2e tests."

type stubExplainer struct{}

func (stubExplainer) Explain(_ context.Context, _, _, _, _ string) (string, error) {
	return stubExplanation, nil
}

func (stubExplainer) AnswerFollowUp(_ context.Context, in app.FollowUpInput) (string, error) {
	return fmt.Sprintf("Stub answer to: %s", in.Question), nil
}

const stubInsight = "This is a stub weakness insight for e2e tests."

type stubAnalyzer struct{}

func (stubAnalyzer) Analyze(_ context.Context, _ []app.MistakeSentence, _ string) (string, error) {
	return stubInsight, nil
}

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

func (stubCoach) Summarize(_ context.Context, _ *app.DiscussionQuestion, transcript []app.DiscussionMessage, _ string) (*app.Summary, error) {
	var refined []string
	for _, m := range transcript {
		if m.Role == "user" {
			refined = append(refined, fmt.Sprintf("Stub refinement %d: %s", len(refined)+1, m.Text))
		}
	}
	return &app.Summary{
		RefinedMessages:  refined,
		NaturalnessWhyEN: "Stub explanation: you opened every turn with \"I think that\", which reads as written English rather than speech.",
		NaturalnessFixEN: "Stub advice: drop \"that\" after \"I think\", and swap textbook words for everyday ones.",
		Phrases: []app.Phrase{
			{Phrase: "take responsibility for", MeaningEN: "to accept that something is your job or your fault", ExampleEN: "Companies should take responsibility for their impact."},
			{Phrase: "in the future", MeaningEN: "at some time later than now", ExampleEN: "I want to live abroad in the future."},
		},
	}, nil
}

type stubPhraseWriter struct{}

func (stubPhraseWriter) Write(_ context.Context, src app.PhraseSource) (*app.PhraseContent, error) {
	return &app.PhraseContent{
		Meaning: fmt.Sprintf("Stub meaning of %s", src.Phrase),
		Examples: []string{
			fmt.Sprintf("Stub example 1 with %s.", src.Phrase),
			fmt.Sprintf("Stub example 2 with %s.", src.Phrase),
			fmt.Sprintf("Stub example 3 with %s.", src.Phrase),
		},
	}, nil
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
	srv := app.NewServer(repo, stubExplainer{}, stubAnalyzer{}).
		WithDiscussion(repo, stubCoach{}).
		WithPhrases(repo, stubPhraseWriter{})

	frontendURL := os.Getenv("FRONTEND_URL")
	mcpCfg := app.MCPConfigFromEnv()
	mcpCfg.LogStatus()
	mux := app.NewMux(srv, verifier, allowedEmails, frontendURL, mcpCfg)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("e2e server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
