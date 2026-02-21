package domain

import "time"

type WatchHistoryItem struct {
	ContentID int64     `json:"content_id"`
	Genre     string    `json:"genre"`
	WatchedAt time.Time `json:"watched_at"`
}