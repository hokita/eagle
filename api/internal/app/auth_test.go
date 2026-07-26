package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeVerifier struct {
	uid   string
	email string
	err   error
}

func (f fakeVerifier) Verify(_ context.Context, _ string) (string, string, error) {
	return f.uid, f.email, f.err
}

const testAllowedEmail = "test@example.com"

func TestRequireAuthRejectsMissingHeader(t *testing.T) {
	h := requireAuth(fakeVerifier{uid: "u1", email: testAllowedEmail}, []string{testAllowedEmail}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuthRejectsInvalidToken(t *testing.T) {
	h := requireAuth(fakeVerifier{err: errors.New("bad token")}, []string{testAllowedEmail}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil)
	req.Header.Set("Authorization", "Bearer sometoken")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuthPassesUID(t *testing.T) {
	var gotUID string
	h := requireAuth(fakeVerifier{uid: "user-123", email: testAllowedEmail}, []string{testAllowedEmail}, func(w http.ResponseWriter, r *http.Request) {
		gotUID, _ = uidFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil)
	req.Header.Set("Authorization", "Bearer validtoken")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotUID != "user-123" {
		t.Fatalf("expected uid user-123, got %q", gotUID)
	}
}

func TestRequireAuthRejectsDisallowedEmail(t *testing.T) {
	h := requireAuth(fakeVerifier{uid: "u1", email: "someone-else@gmail.com"}, []string{testAllowedEmail}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil)
	req.Header.Set("Authorization", "Bearer validtoken")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 (matches corgi's authMiddleware, avoids leaking why auth failed), got %d", rec.Code)
	}
}

func TestRequireAuthRejectsEmptyEmail(t *testing.T) {
	h := requireAuth(fakeVerifier{uid: "u1", email: ""}, []string{testAllowedEmail}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil)
	req.Header.Set("Authorization", "Bearer validtoken")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestRequireAuthAllowsSecondEmailInList(t *testing.T) {
	h := requireAuth(fakeVerifier{uid: "u2", email: "second@example.com"}, []string{testAllowedEmail, "second@example.com"}, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil)
	req.Header.Set("Authorization", "Bearer validtoken")
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
