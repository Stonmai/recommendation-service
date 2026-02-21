package repository

import (
	"context"
	"fmt"

	"github.com/actuallystonmai/recommendation-service/internal/domain"
)

func (r *Repository) GetUnwatchedContent(ctx context.Context, userID int64, limit int) ([]domain.Content, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT c.id, c.title, c.genre, c.popularity_score, c.created_at
		FROM content c
		LEFT JOIN user_watch_history uwh
    		ON uwh.content_id = c.id AND uwh.user_id = $1
    	WHERE uwh.content_id IS NULL
     	ORDER BY c.popularity_score DESC
     	LIMIT $2`, userID, limit,
	)
	
	if err != nil {
		return nil, fmt.Errorf("query unwatched content for user %d: %w", userID, err)
	}
	defer rows.Close()
	
	var items []domain.Content
	for rows.Next() {
		var c domain.Content
		err := rows.Scan(&c.ID, &c.Title, &c.Genre, &c.PopularityScore, &c.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan content: %w", err)
		}
		items = append(items, c)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate over content: %w", err)
	}
	return items, nil
}