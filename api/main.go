package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

type Sentence struct {
	ID        int    `json:"id"`
	Japanese  string `json:"japanese"`
	English   string `json:"english"`
	Page      string `json:"page"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type AnswerHistory struct {
	ID              int    `json:"id"`
	IncorrectAnswer string `json:"incorrect_answer"`
	CreatedAt       string `json:"created_at"`
}

type CheckAnswerRequest struct {
	SentenceID int    `json:"sentence_id"`
	UserAnswer string `json:"user_answer"`
}

type CheckAnswerResponse struct {
	IsCorrect     bool            `json:"is_correct"`
	CorrectAnswer string          `json:"correct_answer"`
	Histories     []AnswerHistory `json:"histories"`
}

var mockSentences = []Sentence{
	{
		ID:        1,
		Japanese:  "時間がありません。",
		English:   "I don't have time.",
		Page:      "12",
		CreatedAt: "2024-06-28T10:00:00Z",
		UpdatedAt: "2024-06-28T10:00:00Z",
	},
	{
		ID:        2,
		Japanese:  "今日は暑いです。",
		English:   "It's hot today.",
		Page:      "15",
		CreatedAt: "2024-06-28T10:05:00Z",
		UpdatedAt: "2024-06-28T10:05:00Z",
	},
	{
		ID:        3,
		Japanese:  "明日は雨が降るでしょう。",
		English:   "It will rain tomorrow.",
		Page:      "23",
		CreatedAt: "2024-06-28T10:10:00Z",
		UpdatedAt: "2024-06-28T10:10:00Z",
	},
}

var mockHistories = map[int][]AnswerHistory{
	1: {
		{ID: 1001, IncorrectAnswer: "I have no time.", CreatedAt: "2024-06-27T15:21:30Z"},
		{ID: 1020, IncorrectAnswer: "There is no time.", CreatedAt: "2024-06-28T09:55:12Z"},
		{ID: 1050, IncorrectAnswer: "I don't have times.", CreatedAt: "2024-06-28T10:08:41Z"},
	},
	2: {
		{ID: 2001, IncorrectAnswer: "Today is hot.", CreatedAt: "2024-06-27T16:30:00Z"},
	},
}

func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func getRandomSentence(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rand.Seed(time.Now().UnixNano())
	randomIndex := rand.Intn(len(mockSentences))
	sentence := mockSentences[randomIndex]

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sentence)
}

func checkAnswer(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CheckAnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var targetSentence *Sentence
	for _, sentence := range mockSentences {
		if sentence.ID == req.SentenceID {
			targetSentence = &sentence
			break
		}
	}

	if targetSentence == nil {
		http.Error(w, "Sentence not found", http.StatusNotFound)
		return
	}

	isCorrect := strings.TrimSpace(strings.ToLower(req.UserAnswer)) == strings.TrimSpace(strings.ToLower(targetSentence.English))

	histories := mockHistories[req.SentenceID]
	if histories == nil {
		histories = []AnswerHistory{}
	}

	response := CheckAnswerResponse{
		IsCorrect:     isCorrect,
		CorrectAnswer: targetSentence.English,
		Histories:     histories,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func main() {
	http.HandleFunc("/api/sentence/random", getRandomSentence)
	http.HandleFunc("/api/answer/check", checkAnswer)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	port = ":" + port
	fmt.Printf("Server starting on port %s\n", port)
	log.Fatal(http.ListenAndServe(port, nil))
}