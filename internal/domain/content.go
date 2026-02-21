package domain

import "time"

type Content struct {
	ID              int64     `json:"id"`
	Title           string    `json:"title"`
	Genre           string    `json:"genre"`
	PopularityScore float64   `json:"popularity_score"`
	CreatedAt       time.Time `json:"created_at"`
}