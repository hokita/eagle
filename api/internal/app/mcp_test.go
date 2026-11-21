package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpHarness struct {
	t       *testing.T
	session *mcp.ClientSession
}

func newMCPHarness(t *testing.T, repo *fakeRepo, limiter *RateLimiter) *mcpHarness {
	t.Helper()
	if limiter == nil {
		limiter = NewRateLimiter(1000, time.Hour, time.Now)
	}
	srv := NewServer(repo, &fakeExplainer{}, &fakeAnalyzer{})
	ts := httptest.NewServer(newMCPHandler(srv, "owner-uid", limiter))
	t.Cleanup(ts.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: ts.URL}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return &mcpHarness{t: t, session: session}
}

func (h *mcpHarness) callTool(name string, args map[string]any) *mcp.CallToolResult {
	h.t.Helper()
	res, err := h.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		h.t.Fatalf("call %s: %v", name, err)
	}
	return res
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatalf("result has no content")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *mcp.TextContent", res.Content[0])
	}
	return text.Text
}

func wantToolError(t *testing.T, res *mcp.CallToolResult, want string) {
	t.Helper()
	if !res.IsError {
		t.Fatalf("IsError = false, want tool error %q (content %q)", want, resultText(t, res))
	}
	if got := resultText(t, res); got != want {
		t.Fatalf("error text = %q, want %q", got, want)
	}
}

func decodeStructured(t *testing.T, res *mcp.CallToolResult, dst any) {
	t.Helper()
	if res.IsError {
		t.Fatalf("unexpected tool error: %q", resultText(t, res))
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("unmarshal structured content %s: %v", raw, err)
	}
}

func TestMCPListsExactlyFourTools(t *testing.T) {
	h := newMCPHarness(t, &fakeRepo{}, nil)
	res, err := h.session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	want := map[string]struct {
		description string
		annotations mcp.ToolAnnotations
	}{
		"get_practice_sentence": {
			"Get a random Japanese sentence for the owner to translate into English — the same pick as the eagle app (skips mastered and reported sentences). The English answer is not included: ask the owner for their translation, then call check_answer with this id.",
			mcp.ToolAnnotations{ReadOnlyHint: true},
		},
		"check_answer": {
			"Grade the owner's English translation of a sentence from get_practice_sentence and save it to their eagle history, exactly like answering in the app (wrong answers appear on the app's Mistakes page). Pass the owner's answer as they wrote it; don't correct it first.",
			mcp.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: false},
		},
		"list_mistakes": {
			"List the owner's past wrong answers in eagle, most recently missed sentence first, each with the correct English and their latest wrong attempts. Use it to coach the owner on recurring mistakes. Page with limit and offset.",
			mcp.ToolAnnotations{ReadOnlyHint: true},
		},
		"report_sentence": {
			"Report an eagle sentence as broken (wrong or unnatural Japanese or English) so it is no longer given for practice. The app has no undo, so confirm with the owner first.",
			mcp.ToolAnnotations{DestructiveHint: boolPtr(true), IdempotentHint: true},
		},
	}
	if len(res.Tools) != len(want) {
		t.Fatalf("got %d tools, want %d", len(res.Tools), len(want))
	}
	for _, tool := range res.Tools {
		w, ok := want[tool.Name]
		if !ok {
			t.Errorf("unexpected tool %q", tool.Name)
			continue
		}
		if tool.Description != w.description {
			t.Errorf("%s description = %q, want %q", tool.Name, tool.Description, w.description)
		}
		if tool.Annotations == nil {
			t.Errorf("%s has no annotations", tool.Name)
			continue
		}
		if !reflect.DeepEqual(*tool.Annotations, w.annotations) {
			t.Errorf("%s annotations = %+v, want %+v", tool.Name, *tool.Annotations, w.annotations)
		}
	}
}

func TestMCPGetPracticeSentence(t *testing.T) {
	repo := &fakeRepo{random: &Sentence{ID: 7, Japanese: "犬が好きです。", English: "I like dogs.", Level: 3, CorrectCount: 2, IncorrectCount: 1}}
	h := newMCPHarness(t, repo, nil)

	res := h.callTool("get_practice_sentence", map[string]any{"levels": []int{3, 1, 3}})
	var out map[string]any
	decodeStructured(t, res, &out)
	if len(repo.randomUIDs) != 1 || repo.randomUIDs[0] != "owner-uid" {
		t.Errorf("uids = %v, want [owner-uid]", repo.randomUIDs)
	}
	if len(repo.randomLevelCalls) != 1 || !reflect.DeepEqual(repo.randomLevelCalls[0], []int{3, 1}) {
		t.Errorf("levels = %v, want [[3 1]]", repo.randomLevelCalls)
	}
	want := map[string]any{"id": 7.0, "japanese": "犬が好きです。", "level": 3.0, "correct_count": 2.0, "incorrect_count": 1.0}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("structured = %v, want %v", out, want)
	}
	if _, ok := out["english"]; ok {
		t.Errorf("structured output has an english key")
	}
	if strings.Contains(resultText(t, res), "I like dogs.") {
		t.Errorf("text content leaks the English answer: %q", resultText(t, res))
	}

	wantToolError(t, h.callTool("get_practice_sentence", map[string]any{"levels": []int{6}}), "levels must be between 1 and 5")
	wantToolError(t, h.callTool("get_practice_sentence", map[string]any{"levels": []int{0}}), "levels must be between 1 and 5")

	repo.randomErr = ErrNoCandidate
	wantToolError(t, h.callTool("get_practice_sentence", nil), "No sentences left to practice at these levels")
}

type mcpCheckOut struct {
	IsCorrect            bool   `json:"is_correct"`
	YourAnswer           string `json:"your_answer"`
	CorrectAnswer        string `json:"correct_answer"`
	PreviousWrongAnswers []struct {
		Answer string `json:"answer"`
		At     string `json:"at"`
	} `json:"previous_wrong_answers"`
}

func TestMCPCheckAnswer(t *testing.T) {
	var histories []AnswerHistory
	for i := 7; i >= 1; i-- {
		histories = append(histories, AnswerHistory{ID: int64(i), IncorrectAnswer: fmt.Sprintf("wrong %d", i), CreatedAt: fmt.Sprintf("2026-11-0%dT00:00:00Z", i)})
	}
	repo := &fakeRepo{correct: "I like dogs.", histories: histories}
	h := newMCPHarness(t, repo, nil)

	var out mcpCheckOut
	decodeStructured(t, h.callTool("check_answer", map[string]any{"sentence_id": 7, "answer": " I love dogs "}), &out)
	if out.IsCorrect {
		t.Errorf("is_correct = true, want false")
	}
	if out.YourAnswer != " I love dogs " || out.CorrectAnswer != "I like dogs." {
		t.Errorf("out = %+v", out)
	}
	if len(out.PreviousWrongAnswers) != 5 {
		t.Fatalf("previous_wrong_answers len = %d, want 5", len(out.PreviousWrongAnswers))
	}
	if out.PreviousWrongAnswers[0].Answer != "wrong 7" || out.PreviousWrongAnswers[0].At != "2026-11-07T00:00:00Z" || out.PreviousWrongAnswers[4].Answer != "wrong 3" {
		t.Errorf("previous_wrong_answers = %+v", out.PreviousWrongAnswers)
	}
	wantRec := []recordedAnswer{{"owner-uid", 7, false, " I love dogs "}}
	if !reflect.DeepEqual(repo.recorded, wantRec) {
		t.Errorf("recorded = %+v, want %+v", repo.recorded, wantRec)
	}

	out = mcpCheckOut{}
	decodeStructured(t, h.callTool("check_answer", map[string]any{"sentence_id": 7, "answer": "i like dogs"}), &out)
	if !out.IsCorrect {
		t.Errorf("is_correct = false, want true")
	}
}

func TestMCPCheckAnswerNoHistories(t *testing.T) {
	h := newMCPHarness(t, &fakeRepo{correct: "I like dogs."}, nil)
	res := h.callTool("check_answer", map[string]any{"sentence_id": 7, "answer": "cats"})
	var out map[string]any
	decodeStructured(t, res, &out)
	if got, ok := out["previous_wrong_answers"].([]any); !ok || len(got) != 0 {
		t.Errorf("previous_wrong_answers = %#v, want []", out["previous_wrong_answers"])
	}
}

func TestMCPCheckAnswerUnknownSentence(t *testing.T) {
	repo := &fakeRepo{correct: "x"}
	h := newMCPHarness(t, repo, nil)
	wantToolError(t, h.callTool("check_answer", map[string]any{"sentence_id": 0, "answer": "x"}), "Sentence 0 not found")
	wantToolError(t, h.callTool("check_answer", map[string]any{"sentence_id": -1, "answer": "x"}), "Sentence -1 not found")
	repo.correctErr = ErrNotFound
	wantToolError(t, h.callTool("check_answer", map[string]any{"sentence_id": 424242, "answer": "x"}), "Sentence 424242 not found")
	if len(repo.recorded) != 0 {
		t.Errorf("recorded = %+v, want nothing", repo.recorded)
	}
}

func TestMCPCheckAnswerMultibyte(t *testing.T) {
	repo := &fakeRepo{correct: "It's fine."}
	h := newMCPHarness(t, repo, nil)

	var out mcpCheckOut
	decodeStructured(t, h.callTool("check_answer", map[string]any{"sentence_id": 1, "answer": "it’s fine"}), &out)
	if !out.IsCorrect {
		t.Errorf("curly apostrophe graded wrong")
	}

	out = mcpCheckOut{}
	decodeStructured(t, h.callTool("check_answer", map[string]any{"sentence_id": 1, "answer": "大丈夫です"}), &out)
	if out.IsCorrect {
		t.Errorf("japanese answer graded correct")
	}
	if last := repo.recorded[len(repo.recorded)-1]; last.answer != "大丈夫です" {
		t.Errorf("recorded answer = %q, want 大丈夫です", last.answer)
	}

	decodeStructured(t, h.callTool("check_answer", map[string]any{"sentence_id": 1, "answer": strings.Repeat("あ", 666)}), &out)
	before := len(repo.recorded)
	wantToolError(t, h.callTool("check_answer", map[string]any{"sentence_id": 1, "answer": strings.Repeat("あ", 667)}), "answer must be 1–2000 bytes")
	wantToolError(t, h.callTool("check_answer", map[string]any{"sentence_id": 1, "answer": "   "}), "answer must be 1–2000 bytes")
	if len(repo.recorded) != before {
		t.Errorf("rejected answers were recorded")
	}
}

type mcpMistakesOut struct {
	Total    int  `json:"total"`
	Offset   int  `json:"offset"`
	HasMore  bool `json:"has_more"`
	Mistakes []struct {
		SentenceID         int    `json:"sentence_id"`
		Japanese           string `json:"japanese"`
		CorrectAnswer      string `json:"correct_answer"`
		WrongAnswerCount   int    `json:"wrong_answer_count"`
		LatestWrongAnswers []struct {
			Answer string `json:"answer"`
			At     string `json:"at"`
		} `json:"latest_wrong_answers"`
	} `json:"mistakes"`
}

func TestMCPListMistakesPaging(t *testing.T) {
	var mistakes []MistakeSentence
	for i := 1; i <= 120; i++ {
		var wrong []AnswerHistory
		for j := 5; j >= 1; j-- {
			wrong = append(wrong, AnswerHistory{IncorrectAnswer: fmt.Sprintf("s%d w%d", i, j), CreatedAt: fmt.Sprintf("2026-11-0%dT00:00:00Z", j)})
		}
		mistakes = append(mistakes, MistakeSentence{SentenceID: i, Japanese: fmt.Sprintf("j%d", i), CorrectAnswer: fmt.Sprintf("e%d", i), WrongAnswers: wrong})
	}
	h := newMCPHarness(t, &fakeRepo{mistakes: mistakes}, nil)

	var out mcpMistakesOut
	decodeStructured(t, h.callTool("list_mistakes", nil), &out)
	if len(out.Mistakes) != 10 || out.Total != 120 || !out.HasMore || out.Offset != 0 {
		t.Fatalf("default page: len %d total %d has_more %v offset %d", len(out.Mistakes), out.Total, out.HasMore, out.Offset)
	}
	first := out.Mistakes[0]
	if first.SentenceID != 1 || first.Japanese != "j1" || first.CorrectAnswer != "e1" || first.WrongAnswerCount != 5 || len(first.LatestWrongAnswers) != 3 {
		t.Errorf("first = %+v", first)
	}
	if first.LatestWrongAnswers[0].Answer != "s1 w5" || first.LatestWrongAnswers[0].At != "2026-11-05T00:00:00Z" {
		t.Errorf("latest wrong answers = %+v", first.LatestWrongAnswers)
	}

	out = mcpMistakesOut{}
	decodeStructured(t, h.callTool("list_mistakes", map[string]any{"limit": 50, "offset": 100}), &out)
	if len(out.Mistakes) != 20 || out.HasMore || out.Offset != 100 || out.Mistakes[0].SentenceID != 101 {
		t.Errorf("last page: len %d has_more %v offset %d", len(out.Mistakes), out.HasMore, out.Offset)
	}

	out = mcpMistakesOut{}
	decodeStructured(t, h.callTool("list_mistakes", map[string]any{"offset": 120}), &out)
	if len(out.Mistakes) != 0 || out.HasMore || out.Total != 120 {
		t.Errorf("past end: len %d has_more %v total %d", len(out.Mistakes), out.HasMore, out.Total)
	}

	wantToolError(t, h.callTool("list_mistakes", map[string]any{"limit": 51}), "limit must be between 1 and 50")
	wantToolError(t, h.callTool("list_mistakes", map[string]any{"limit": -1}), "limit must be between 1 and 50")
	wantToolError(t, h.callTool("list_mistakes", map[string]any{"offset": -1}), "offset must be 0 or more")
}

func TestMCPReportSentence(t *testing.T) {
	repo := &fakeRepo{}
	h := newMCPHarness(t, repo, nil)
	res := h.callTool("report_sentence", map[string]any{"sentence_id": 9})
	if res.IsError {
		t.Fatalf("unexpected tool error: %q", resultText(t, res))
	}
	if got := resultText(t, res); got != "Reported sentence 9; it won't be given for practice again." {
		t.Errorf("text = %q", got)
	}
	if !reflect.DeepEqual(repo.reported, []int{9}) {
		t.Errorf("reported = %v, want [9]", repo.reported)
	}

	wantToolError(t, h.callTool("report_sentence", map[string]any{"sentence_id": 0}), "Sentence 0 not found")
	repo.reportErr = ErrNotFound
	wantToolError(t, h.callTool("report_sentence", map[string]any{"sentence_id": 424242}), "Sentence 424242 not found")
}

func TestMCPRateLimited(t *testing.T) {
	limiter := NewRateLimiter(1, time.Minute, time.Now)
	h := newMCPHarness(t, &fakeRepo{random: &Sentence{ID: 1}}, limiter)

	if res := h.callTool("get_practice_sentence", nil); res.IsError {
		t.Fatalf("first call errored: %q", resultText(t, res))
	}
	res := h.callTool("get_practice_sentence", nil)
	if !res.IsError || !strings.HasPrefix(resultText(t, res), "Rate limit reached: try again in") {
		t.Fatalf("second call = %v %q, want rate limit error", res.IsError, resultText(t, res))
	}
	if _, err := h.session.ListTools(context.Background(), nil); err != nil {
		t.Errorf("list tools while limited: %v", err)
	}
}

func TestMCPRateLimitMessage(t *testing.T) {
	for _, tc := range []struct {
		retry time.Duration
		want  string
	}{
		{0, "Rate limit reached: try again in 1 seconds"},
		{300 * time.Millisecond, "Rate limit reached: try again in 1 seconds"},
		{59*time.Second + time.Millisecond, "Rate limit reached: try again in 60 seconds"},
	} {
		if got := rateLimitError(tc.retry).Error(); got != tc.want {
			t.Errorf("rateLimitError(%v) = %q, want %q", tc.retry, got, tc.want)
		}
	}
}

func TestMCPInternalErrorHidden(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	h := newMCPHarness(t, &fakeRepo{mistakesErr: errors.New("rpc error: secret detail")}, nil)
	wantToolError(t, h.callTool("list_mistakes", nil), "Internal error")
	if !strings.Contains(logs.String(), "mcp list_mistakes: rpc error: secret detail") {
		t.Errorf("log = %q, want the underlying error logged", logs.String())
	}
}
