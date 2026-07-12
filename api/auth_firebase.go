package main

import (
	"context"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
)

type firebaseVerifier struct {
	client *auth.Client
}

// NewFirebaseVerifier builds a TokenVerifier backed by the Firebase Admin SDK.
// Honors FIREBASE_AUTH_EMULATOR_HOST for local development.
func NewFirebaseVerifier(ctx context.Context, projectID string) (*firebaseVerifier, error) {
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, err
	}
	return &firebaseVerifier{client: client}, nil
}

func (v *firebaseVerifier) Verify(ctx context.Context, idToken string) (string, string, error) {
	token, err := v.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return "", "", err
	}
	email, _ := token.Claims["email"].(string)
	return token.UID, email, nil
}
