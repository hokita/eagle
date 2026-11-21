package app

import (
	"context"
	"log"
	"strings"
)

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
