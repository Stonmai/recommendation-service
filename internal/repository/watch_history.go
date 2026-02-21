package repository

import (
	"context"
	"fmt"

	"github.com/actuallystonmai/recommendation-service/internal/domain"
)

func (r *Repository) GetUserWatchHistoryWithGenres(ctx context.Context, userID int64, limit int) ([]domain.WatchHistoryItem, error) {
	row, err := r.pool.Query(ctx,
		`SELECT c.id, c.genre, uwh.watched_at
		FROM user_watch_history uwh
		JOIN content c ON uwh.content_id = c.id
		WHERE uwh.user_id = $1
		ORDER BY uwh.watched_at DESC
		LIMIT $2`,
		userID, limit,
	)
	
	if err != nil {
		return nil, fmt.Errorf("get watch history for user %d: %w", userID, err)
	}
	
	defer row.Close()
	
	var items []domain.WatchHistoryItem
	for row.Next() {
		var item domain.WatchHistoryItem
		if err := row.Scan(&item.ContentID, &item.Genre, &item.WatchedAt); err != nil {
			return nil, fmt.Errorf("scan watch history item: %w", err)
		}
		items = append(items, item)
	}
	
	if err := row.Err(); err != nil {
		return nil, fmt.Errorf("iterate over watch history items: %w", err)
	}
	
	return items, nil
}