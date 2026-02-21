package handler

import "github.com/actuallystonmai/recommendation-service/internal/domain"

type RecommendationResponse struct {
	UserID          int64                        `json:"user_id"`
	Recommendations []domain.ScoredRecommendation `json:"recommendations"`
	Metadata        domain.RecommendationMeta     `json:"metadata"`
}

type BatchResponse struct {
	Page       int                      `json:"page"`
	Limit      int                      `json:"limit"`
	TotalUsers int                      `json:"total_users"`
	Results    []domain.BatchUserResult  `json:"results"`
	Summary    domain.BatchSummary       `json:"summary"`
	Metadata   domain.BatchMeta          `json:"metadata"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
