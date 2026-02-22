package domain

import "errors"

type BatchStatus string

const (
	StatusSuccess BatchStatus = "success"
	StatusFailed  BatchStatus = "failed"
)

var ErrUserNotFound     = errors.New("user not found")
var ErrModelUnavailable = errors.New("recommendation model unavailable")
var ErrRequestTimeout   = errors.New("request timed out")

type ScoredRecommendation struct {
	ContentID       int64   `json:"content_id"`
	Title           string  `json:"title"`
	Genre           string  `json:"genre"`
	PopularityScore float64 `json:"popularity_score"`
	Score           float64 `json:"score"`
}

type RecommendationMeta struct {
	CacheHit    bool   `json:"cache_hit"`
	GeneratedAt string `json:"generated_at"`
	TotalCount  int    `json:"total_count"`
}

type RecommendationResult struct {
	Recommendations []ScoredRecommendation
	CacheHit        bool
}

type BatchUserResult struct {
	UserID          int64                  `json:"user_id"`
	Recommendations []ScoredRecommendation `json:"recommendations,omitempty"`
	Status          BatchStatus            `json:"status"`
	Error           string                 `json:"error,omitempty"`
	Message         string                 `json:"message,omitempty"`
}

type BatchSummary struct {
	SuccessCount     int   `json:"success_count"`
	FailedCount      int   `json:"failed_count"`
	ProcessingTimeMs int64 `json:"processing_time_ms"`
}

type BatchMeta struct {
	GeneratedAt string `json:"generated_at"`
}

type BatchResponse struct {
	Page       int               `json:"page"`
	Limit      int               `json:"limit"`
	TotalUsers int               `json:"total_users"`
	Results    []BatchUserResult `json:"results"`
	Summary    BatchSummary      `json:"summary"`
	Metadata   BatchMeta         `json:"metadata"`
}
