package app

import (
	"context"
	"log"
	"strings"
)

// checkAndRecord grades an answer against the stored sentence and records the
// attempt. It is shared by the REST handler and the MCP tool so both grade and
// persist identically. Histories holds the wrong answers from before this
// attempt, so they are read before recording. A RecordAnswer failure is logged
// rather than returned: the learner still gets their result.
func (s *Server) checkAndRecord(ctx context.Context, uid string, sentenceID int, answer string) (*CheckAnswerResponse, error) {
	correct, err := s.repo.CorrectAnswer(ctx, sentenceID)
	if err != nil {
		return nil, err
	}
	histories, err := s.repo.ListIncorrectHistories(ctx, uid, sentenceID)
	if err != nil {
		return nil, err
	}
	isCorrect := strings.EqualFold(normalizeAnswer(answer), normalizeAnswer(correct))
	recorded := ""
	if !isCorrect {
		recorded = answer
	}
	if err := s.repo.RecordAnswer(ctx, uid, sentenceID, isCorrect, recorded); err != nil {
		log.Printf("record answer error: %v", err)
	}
	return &CheckAnswerResponse{
		IsCorrect:     isCorrect,
		CorrectAnswer: correct,
		Histories:     histories,
	}, nil
}
