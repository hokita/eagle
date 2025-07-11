package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

type Sentence struct {
	ID             int    `json:"id"`
	Japanese       string `json:"japanese"`
	English        string `json:"english"`
	Page           string `json:"page"`
	IsReported     bool   `json:"-"`
	CorrectCount   int    `json:"correct_count"`
	IncorrectCount int    `json:"incorrect_count"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
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

type ReportSentenceRequest struct {
	SentenceID int `json:"sentence_id"`
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

var db *sql.DB

func initDB() {
	if os.Getenv("ENV") != "production" {
		if err := godotenv.Load(); err != nil {
			log.Println("Warning: could not load .env file:", err)
		}
	}

	dbUser := os.Getenv("DB_USER")
	dbName := os.Getenv("DB_NAME")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbEndpoint := os.Getenv("DB_ENDPOINT")

	if dbUser == "" || dbName == "" || dbPassword == "" || dbEndpoint == "" {
		log.Fatal("Database configuration missing. Please set DB_USER, DB_NAME, DB_PASSWORD, and DB_ENDPOINT.")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s", dbUser, dbPassword, dbEndpoint, dbName)

	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal("Failed to open database connection:", err)
	}

	if err = db.Ping(); err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	fmt.Println("Successfully connected to MySQL database")
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

	query := `
        SELECT
            s.id, s.japanese, s.english, s.page, s.is_reported, s.created_at, s.updated_at,
            SUM(CASE WHEN ah.is_correct = 1 THEN 1 ELSE 0 END) as correct_count,
            SUM(CASE WHEN ah.is_correct = 0 THEN 1 ELSE 0 END) as incorrect_count
        FROM sentences s
        LEFT JOIN answer_histories ah ON s.id = ah.sentence_id
		WHERE s.is_reported = false
        GROUP BY s.id
        HAVING correct_count - incorrect_count < 2
    `
	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Database query error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var sentences []Sentence
	for rows.Next() {
		var sentence Sentence
		err := rows.Scan(
			&sentence.ID, &sentence.Japanese, &sentence.English, &sentence.Page, &sentence.IsReported, &sentence.CreatedAt, &sentence.UpdatedAt,
			&sentence.CorrectCount, &sentence.IncorrectCount,
		)
		if err != nil {
			log.Printf("Database scan error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		sentences = append(sentences, sentence)
	}

	if len(sentences) == 0 {
		http.Error(w, "No sentences found", http.StatusNotFound)
		return
	}

	rand.Seed(time.Now().UnixNano())
	randomIndex := rand.Intn(len(sentences))
	selectedSentence := sentences[randomIndex]

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(selectedSentence)
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

	query := `
		SELECT
			s.english,
			COALESCE(ah.id, 0) as history_id,
			COALESCE(ah.incorrect_answer, '') as incorrect_answer,
			COALESCE(ah.created_at, '') as history_created_at
		FROM sentences s
		LEFT JOIN answer_histories ah ON s.id = ah.sentence_id AND ah.is_correct = false
		WHERE s.id = ?
		ORDER BY ah.created_at DESC
	`

	rows, err := db.Query(query, req.SentenceID)
	if err != nil {
		log.Printf("Database error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var correctAnswer string
	histories := make([]AnswerHistory, 0)
	sentenceFound := false

	for rows.Next() {
		var historyID int
		var incorrectAnswer, historyCreatedAt string

		err := rows.Scan(&correctAnswer, &historyID, &incorrectAnswer, &historyCreatedAt)
		if err != nil {
			log.Printf("Failed to scan row: %v", err)
			continue
		}

		sentenceFound = true

		if historyID > 0 {
			histories = append(histories, AnswerHistory{
				ID:              historyID,
				IncorrectAnswer: incorrectAnswer,
				CreatedAt:       historyCreatedAt,
			})
		}
	}

	if !sentenceFound {
		http.Error(w, "Sentence not found", http.StatusNotFound)
		return
	}

	isCorrect := strings.TrimSpace(strings.ToLower(req.UserAnswer)) == strings.TrimSpace(strings.ToLower(correctAnswer))

	incorrectAnswer := ""
	if !isCorrect {
		incorrectAnswer = req.UserAnswer
	}

	insertQuery := "INSERT INTO answer_histories (sentence_id, is_correct, incorrect_answer) VALUES (?, ?, ?)"
	_, err = db.Exec(insertQuery, req.SentenceID, isCorrect, incorrectAnswer)
	if err != nil {
		log.Printf("Failed to insert answer history: %v", err)
	}

	response := CheckAnswerResponse{
		IsCorrect:     isCorrect,
		CorrectAnswer: correctAnswer,
		Histories:     histories,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func reportSentence(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ReportSentenceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	query := "UPDATE sentences SET is_reported = true WHERE id = ?"
	_, err := db.Exec(query, req.SentenceID)
	if err != nil {
		log.Printf("Failed to update sentence: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	if err := db.Ping(); err != nil {
		http.Error(w, "Database not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

func livenessHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

func main() {
	initDB()
	defer db.Close()

	http.HandleFunc("/api/sentence/random", getRandomSentence)
	http.HandleFunc("/api/answer/check", checkAnswer)
	http.HandleFunc("/api/sentence/report", reportSentence)
	http.HandleFunc("/api/readiness", readinessHandler)
	http.HandleFunc("/api/liveness", livenessHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	port = ":" + port
	fmt.Printf("Server starting on port %s\n", port)
	log.Fatal(http.ListenAndServe(port, nil))
}