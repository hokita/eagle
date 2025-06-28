package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	testPort = "8081"
	baseURL  = "http://localhost:" + testPort
)

var serverCmd *exec.Cmd

func TestMain(m *testing.M) {
	if err := startServer(); err != nil {
		fmt.Printf("Failed to start server: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	stopServer()
	os.Exit(code)
}

func startServer() error {
	serverCmd = exec.Command("go", "run", "main.go")
	serverCmd.Env = append(os.Environ(), "PORT="+testPort)

	if err := serverCmd.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	if err := waitForServer(); err != nil {
		serverCmd.Process.Kill()
		return fmt.Errorf("server did not start properly: %w", err)
	}

	return nil
}

func stopServer() {
	if serverCmd != nil && serverCmd.Process != nil {
		serverCmd.Process.Kill()
		serverCmd.Wait()
	}
}

func waitForServer() error {
	maxRetries := 30
	for i := 0; i < maxRetries; i++ {
		resp, err := http.Get(baseURL + "/api/sentence/random")
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("server did not respond after %d retries", maxRetries)
}

func TestRandomSentenceResponseBody(t *testing.T) {
	resp, err := http.Get(baseURL + "/api/sentence/random")
	if err != nil {
		t.Fatalf("failed to get random sentence: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	t.Logf("Response Status: %d", resp.StatusCode)
	t.Logf("Response Headers: %v", resp.Header)
	t.Logf("Response Body: %s", string(body))

	var sentence Sentence
	if err := json.Unmarshal(body, &sentence); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	t.Logf("Parsed Sentence: %+v", sentence)

	if sentence.ID == 0 {
		t.Error("sentence ID should not be zero")
	}
	if sentence.Japanese == "" {
		t.Error("Japanese text should not be empty")
	}
	if sentence.English == "" {
		t.Error("English text should not be empty")
	}
	if sentence.Page == "" {
		t.Error("Page should not be empty")
	}
	if sentence.CreatedAt == "" {
		t.Error("CreatedAt should not be empty")
	}
	if sentence.UpdatedAt == "" {
		t.Error("UpdatedAt should not be empty")
	}
}

func TestCheckAnswerResponseBody(t *testing.T) {
	tests := []struct {
		name          string
		sentenceID    int
		userAnswer    string
		expectCorrect bool
	}{
		{
			name:          "Correct answer for sentence 1",
			sentenceID:    1,
			userAnswer:    "I don't have time.",
			expectCorrect: true,
		},
		{
			name:          "Incorrect answer for sentence 1",
			sentenceID:    1,
			userAnswer:    "I have no time.",
			expectCorrect: false,
		},
		{
			name:          "Correct answer for sentence 2",
			sentenceID:    2,
			userAnswer:    "It's hot today.",
			expectCorrect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := CheckAnswerRequest{
				SentenceID: tt.sentenceID,
				UserAnswer: tt.userAnswer,
			}

			jsonData, _ := json.Marshal(reqBody)
			t.Logf("Request Body: %s", string(jsonData))

			resp, err := http.Post(baseURL+"/api/answer/check", "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				t.Fatalf("failed to check answer: %v", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("failed to read response body: %v", err)
			}

			t.Logf("Response Status: %d", resp.StatusCode)
			t.Logf("Response Headers: %v", resp.Header)
			t.Logf("Response Body: %s", string(body))

			var response CheckAnswerResponse
			if err := json.Unmarshal(body, &response); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			t.Logf("Parsed Response: %+v", response)

			if response.IsCorrect != tt.expectCorrect {
				t.Errorf("expected IsCorrect %v, got %v", tt.expectCorrect, response.IsCorrect)
			}

			if response.CorrectAnswer == "" {
				t.Error("CorrectAnswer should not be empty")
			}

			if response.Histories == nil {
				t.Error("Histories should not be nil")
			}

			t.Logf("Histories count: %d", len(response.Histories))
			for i, history := range response.Histories {
				t.Logf("History %d: ID=%d, Answer='%s', CreatedAt='%s'",
					i, history.ID, history.IncorrectAnswer, history.CreatedAt)
			}
		})
	}
}

func TestInvalidRequestResponseBodies(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		endpoint       string
		body           string
		contentType    string
		expectedStatus int
	}{
		{
			name:           "Invalid JSON to check answer",
			method:         "POST",
			endpoint:       "/api/answer/check",
			body:           `{"invalid": json}`,
			contentType:    "application/json",
			expectedStatus: 400,
		},
		{
			name:           "Non-existent sentence ID",
			method:         "POST",
			endpoint:       "/api/answer/check",
			body:           `{"sentence_id": 999, "user_answer": "test"}`,
			contentType:    "application/json",
			expectedStatus: 404,
		},
		{
			name:           "POST to random sentence endpoint",
			method:         "POST",
			endpoint:       "/api/sentence/random",
			body:           "",
			contentType:    "",
			expectedStatus: 405,
		},
		{
			name:           "GET to check answer endpoint",
			method:         "GET",
			endpoint:       "/api/answer/check",
			body:           "",
			contentType:    "",
			expectedStatus: 405,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reqBody io.Reader
			if tt.body != "" {
				reqBody = strings.NewReader(tt.body)
			}

			req, err := http.NewRequest(tt.method, baseURL+tt.endpoint, reqBody)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("failed to make request: %v", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("failed to read response body: %v", err)
			}

			t.Logf("Request: %s %s", tt.method, tt.endpoint)
			t.Logf("Request Body: %s", tt.body)
			t.Logf("Response Status: %d", resp.StatusCode)
			t.Logf("Response Headers: %v", resp.Header)
			t.Logf("Response Body: %s", string(body))

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}
		})
	}
}

func TestCORSResponseHeaders(t *testing.T) {
	endpoints := []string{
		"/api/sentence/random",
		"/api/answer/check",
	}

	for _, endpoint := range endpoints {
		t.Run("OPTIONS "+endpoint, func(t *testing.T) {
			req, err := http.NewRequest("OPTIONS", baseURL+endpoint, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("failed to make request: %v", err)
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("failed to read response body: %v", err)
			}

			t.Logf("OPTIONS %s", endpoint)
			t.Logf("Response Status: %d", resp.StatusCode)
			t.Logf("Response Headers: %v", resp.Header)
			t.Logf("Response Body: %s", string(body))

			expectedHeaders := map[string]string{
				"Access-Control-Allow-Origin":  "*",
				"Access-Control-Allow-Methods": "GET, POST, OPTIONS",
				"Access-Control-Allow-Headers": "Content-Type",
			}

			for header, expectedValue := range expectedHeaders {
				actualValue := resp.Header.Get(header)
				if actualValue != expectedValue {
					t.Errorf("expected %s header '%s', got '%s'", header, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestMultipleRandomSentenceResponses(t *testing.T) {
	sentenceMap := make(map[int]Sentence)

	for i := 0; i < 5; i++ {
		resp, err := http.Get(baseURL + "/api/sentence/random")
		if err != nil {
			t.Fatalf("failed to get random sentence on attempt %d: %v", i+1, err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatalf("failed to read response body on attempt %d: %v", i+1, err)
		}

		var sentence Sentence
		if err := json.Unmarshal(body, &sentence); err != nil {
			t.Fatalf("failed to unmarshal response on attempt %d: %v", i+1, err)
		}

		sentenceMap[sentence.ID] = sentence

		t.Logf("Attempt %d - Response Body: %s", i+1, string(body))
		t.Logf("Attempt %d - Parsed: ID=%d, Japanese='%s', English='%s'",
			i+1, sentence.ID, sentence.Japanese, sentence.English)
	}

	t.Logf("Total unique sentences received: %d", len(sentenceMap))

	for id, sentence := range sentenceMap {
		t.Logf("Sentence %d: %+v", id, sentence)
	}
}
