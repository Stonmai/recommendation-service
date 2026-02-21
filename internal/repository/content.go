package repository

import (
	"context"
	"fmt"

	"github.com/actuallystonmai/recommendation-service/internal/domain"
)

func (r *Repository) GetUnwatchedContent(ctx context.Context, userID int64, limit int) ([]domain.Content, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, title, genre, popularity_score, created_at
		 FROM content
		 WHERE id NOT IN (
			 SELECT content_id FROM user_watch_history WHERE user_id = $1
		 )
		 ORDER BY popularity_score DESC
		 LIMIT $2`, userID, limit,
	)
	
	if err != nil {
		return nil, fmt.Errorf("failed to query unwatched content for user %d: %w", userID, err)
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
	return items, nil
}