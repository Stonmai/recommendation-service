package handler

import "github.com/actuallystonmai/recommendation-service/internal/domain"

type RecommendationResponse struct {
	UserID          int64                        `json:"user_id"`
	Recommendations []domain.ScoredRecommendation `json:"recommendations"`
	Metadata        domain.RecommendationMeta     `json:"metadata"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
