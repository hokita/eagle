package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	mcpMaxPreviousWrongAnswers = 5
	mcpMaxLatestWrongAnswers   = 3
	mcpDefaultMistakesLimit    = 10
	mcpMaxMistakesLimit        = 50
)

var errMCPInternal = errors.New("Internal error")

func newMCPHandler(srv *Server, ownerUID string, limiter *RateLimiter) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return newMCPServer(srv, ownerUID, limiter)
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 64 << 10})
}

func newMCPServer(srv *Server, ownerUID string, limiter *RateLimiter) *mcp.Server {
	t := &mcpTools{srv: srv, uid: ownerUID, limiter: limiter}
	server := mcp.NewServer(&mcp.Implementation{Name: "eagle", Version: "1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_practice_sentence",
		Description: "Get a random Japanese sentence for the owner to translate into English — the same pick as the eagle app (skips mastered and reported sentences). The English answer is not included: ask the owner for their translation, then call check_answer with this id.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, t.getPracticeSentence)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "check_answer",
		Description: "Grade the owner's English translation of a sentence from get_practice_sentence and save it to their eagle history, exactly like answering in the app (wrong answers appear on the app's Mistakes page). Pass the owner's answer as they wrote it; don't correct it first.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: false},
	}, t.checkAnswer)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_mistakes",
		Description: "List the owner's past wrong answers in eagle, most recently missed sentence first, each with the correct English and their latest wrong attempts. Use it to coach the owner on recurring mistakes. Page with limit and offset.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, t.listMistakes)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "report_sentence",
		Description: "Report an eagle sentence as broken (wrong or unnatural Japanese or English) so it is no longer given for practice. The app has no undo, so confirm with the owner first.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPtr(true), IdempotentHint: true},
	}, t.reportSentence)
	return server
}

type mcpTools struct {
	srv     *Server
	uid     string
	limiter *RateLimiter
}

func (t *mcpTools) take() error {
	if ok, retryAfter := t.limiter.Take(); !ok {
		return rateLimitError(retryAfter)
	}
	return nil
}

func rateLimitError(retryAfter time.Duration) error {
	seconds := max(int(math.Ceil(retryAfter.Seconds())), 1)
	return fmt.Errorf("Rate limit reached: try again in %d seconds", seconds)
}

func internalError(tool string, err error) error {
	log.Printf("mcp %s: %v", tool, err)
	return errMCPInternal
}

func sentenceNotFound(id int) error {
	return fmt.Errorf("Sentence %d not found", id)
}

type mcpWrongAnswer struct {
	Answer string `json:"answer"`
	At     string `json:"at"`
}

func latestWrongAnswers(histories []AnswerHistory, n int) []mcpWrongAnswer {
	out := []mcpWrongAnswer{}
	for _, h := range histories[:min(n, len(histories))] {
		out = append(out, mcpWrongAnswer{Answer: h.IncorrectAnswer, At: h.CreatedAt})
	}
	return out
}

type getPracticeSentenceIn struct {
	Levels []int `json:"levels,omitempty" jsonschema:"optional difficulty levels to pick from, each 1–5; omit for any level"`
}

type getPracticeSentenceOut struct {
	ID             int    `json:"id"`
	Japanese       string `json:"japanese"`
	Level          int    `json:"level"`
	CorrectCount   int    `json:"correct_count"`
	IncorrectCount int    `json:"incorrect_count"`
}

func (t *mcpTools) getPracticeSentence(ctx context.Context, _ *mcp.CallToolRequest, in getPracticeSentenceIn) (*mcp.CallToolResult, getPracticeSentenceOut, error) {
	var zero getPracticeSentenceOut
	if err := t.take(); err != nil {
		return nil, zero, err
	}
	levels, ok := uniqueLevels(in.Levels)
	if !ok {
		return nil, zero, errors.New("levels must be between 1 and 5")
	}
	s, err := t.srv.repo.RandomCandidate(ctx, t.uid, levels)
	if errors.Is(err, ErrNoCandidate) {
		return nil, zero, errors.New("No sentences left to practice at these levels")
	}
	if err != nil {
		return nil, zero, internalError("get_practice_sentence", err)
	}
	return nil, getPracticeSentenceOut{
		ID:             s.ID,
		Japanese:       s.Japanese,
		Level:          s.Level,
		CorrectCount:   s.CorrectCount,
		IncorrectCount: s.IncorrectCount,
	}, nil
}

type checkAnswerIn struct {
	SentenceID int    `json:"sentence_id" jsonschema:"the id from get_practice_sentence"`
	Answer     string `json:"answer" jsonschema:"the owner's English translation, as written"`
}

type checkAnswerOut struct {
	IsCorrect            bool             `json:"is_correct"`
	YourAnswer           string           `json:"your_answer"`
	CorrectAnswer        string           `json:"correct_answer"`
	PreviousWrongAnswers []mcpWrongAnswer `json:"previous_wrong_answers"`
}

func (t *mcpTools) checkAnswer(ctx context.Context, _ *mcp.CallToolRequest, in checkAnswerIn) (*mcp.CallToolResult, checkAnswerOut, error) {
	var zero checkAnswerOut
	if err := t.take(); err != nil {
		return nil, zero, err
	}
	if in.SentenceID <= 0 {
		return nil, zero, sentenceNotFound(in.SentenceID)
	}
	if strings.TrimSpace(in.Answer) == "" || len(in.Answer) > maxUserAnswerLength {
		return nil, zero, errors.New("answer must be 1–2000 bytes")
	}
	res, err := t.srv.checkAndRecord(ctx, t.uid, in.SentenceID, in.Answer)
	if errors.Is(err, ErrNotFound) {
		return nil, zero, sentenceNotFound(in.SentenceID)
	}
	if err != nil {
		return nil, zero, internalError("check_answer", err)
	}
	return nil, checkAnswerOut{
		IsCorrect:            res.IsCorrect,
		YourAnswer:           in.Answer,
		CorrectAnswer:        res.CorrectAnswer,
		PreviousWrongAnswers: latestWrongAnswers(res.Histories, mcpMaxPreviousWrongAnswers),
	}, nil
}

type listMistakesIn struct {
	Limit  int `json:"limit,omitempty" jsonschema:"sentences per page, 1–50; default 10"`
	Offset int `json:"offset,omitempty" jsonschema:"sentences to skip; default 0"`
}

type mcpMistake struct {
	SentenceID         int              `json:"sentence_id"`
	Japanese           string           `json:"japanese"`
	CorrectAnswer      string           `json:"correct_answer"`
	WrongAnswerCount   int              `json:"wrong_answer_count"`
	LatestWrongAnswers []mcpWrongAnswer `json:"latest_wrong_answers"`
}

type listMistakesOut struct {
	Total    int          `json:"total"`
	Offset   int          `json:"offset"`
	HasMore  bool         `json:"has_more"`
	Mistakes []mcpMistake `json:"mistakes"`
}

func (t *mcpTools) listMistakes(ctx context.Context, _ *mcp.CallToolRequest, in listMistakesIn) (*mcp.CallToolResult, listMistakesOut, error) {
	var zero listMistakesOut
	if err := t.take(); err != nil {
		return nil, zero, err
	}
	limit := in.Limit
	if limit == 0 {
		limit = mcpDefaultMistakesLimit
	}
	if limit < 1 || limit > mcpMaxMistakesLimit {
		return nil, zero, errors.New("limit must be between 1 and 50")
	}
	if in.Offset < 0 {
		return nil, zero, errors.New("offset must be 0 or more")
	}
	mistakes, err := t.srv.repo.ListMistakes(ctx, t.uid)
	if err != nil {
		return nil, zero, internalError("list_mistakes", err)
	}
	start := min(in.Offset, len(mistakes))
	end := min(start+limit, len(mistakes))
	out := listMistakesOut{
		Total:    len(mistakes),
		Offset:   in.Offset,
		HasMore:  end < len(mistakes),
		Mistakes: []mcpMistake{},
	}
	for _, m := range mistakes[start:end] {
		out.Mistakes = append(out.Mistakes, mcpMistake{
			SentenceID:         m.SentenceID,
			Japanese:           m.Japanese,
			CorrectAnswer:      m.CorrectAnswer,
			WrongAnswerCount:   len(m.WrongAnswers),
			LatestWrongAnswers: latestWrongAnswers(m.WrongAnswers, mcpMaxLatestWrongAnswers),
		})
	}
	return nil, out, nil
}

type reportSentenceIn struct {
	SentenceID int `json:"sentence_id" jsonschema:"the id of the sentence to report"`
}

func (t *mcpTools) reportSentence(ctx context.Context, _ *mcp.CallToolRequest, in reportSentenceIn) (*mcp.CallToolResult, any, error) {
	if err := t.take(); err != nil {
		return nil, nil, err
	}
	if in.SentenceID <= 0 {
		return nil, nil, sentenceNotFound(in.SentenceID)
	}
	err := t.srv.repo.Report(ctx, in.SentenceID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil, sentenceNotFound(in.SentenceID)
	}
	if err != nil {
		return nil, nil, internalError("report_sentence", err)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{
		Text: fmt.Sprintf("Reported sentence %d; it won't be given for practice again.", in.SentenceID),
	}}}, nil, nil
}

func boolPtr(b bool) *bool { return &b }
