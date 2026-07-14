package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithCORSSetsWildcardWhenUnset(t *testing.T) {
	h := withCORS("", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil))
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("expected wildcard origin, got %q", got)
	}
}

func TestWithCORSSetsConfiguredOrigin(t *testing.T) {
	h := withCORS("https://eagle.example.com", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil))
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://eagle.example.com" {
		t.Fatalf("expected configured origin, got %q", got)
	}
}

func TestWithCORSHandlesPreflightWithoutCallingNext(t *testing.T) {
	called := false
	h := withCORS("", func(w http.ResponseWriter, r *http.Request) { called = true })
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodOptions, "/api/sentence/random", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if called {
		t.Fatal("next handler should not be called for an OPTIONS preflight")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Authorization" {
		t.Fatalf("expected Allow-Headers to include Authorization, got %q", got)
	}
}

func TestWithCORSPassesThroughNonPreflight(t *testing.T) {
	called := false
	h := withCORS("", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/api/sentence/random", nil))
	if !called {
		t.Fatal("expected the wrapped handler to be called for a non-preflight request")
	}
}
