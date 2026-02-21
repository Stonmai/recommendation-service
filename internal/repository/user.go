package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/actuallystonmai/recommendation-service/internal/domain"
	"github.com/jackc/pgx/v5"
)

// Get single user
func (r *Repository) GetUserByID(ctx context.Context, userID int64) (*domain.User, error) {
	user := &domain.User{}

	err := r.pool.QueryRow(ctx,
		`SELECT id, age, country, subscription_type, created_at
		 FROM users WHERE id = $1`,
		userID,
	).Scan(&user.ID, &user.Age, &user.Country, &user.SubscriptionType, &user.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("query user id=%d: %w", userID, err)
	}

	return user, nil
}

// Get user ids for page
func (r *Repository) GetUserIDsPaginated(ctx context.Context, page, limit int) ([]int64, error) {
	offset := (page - 1) * limit
	rows, err := r.pool.Query(ctx,
		`SELECT id FROM users ORDER BY id LIMIT $1 OFFSET $2`, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("query user ids for page %d: %w", page, err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan user id: %w", err)
		}
		ids = append(ids, id)
	}
	
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user ids: %w", err)
	}
	return ids, nil
}

// Count total users
func (r *Repository) CountUsers(ctx context.Context) (int, error) {
	var total int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM users`,
	).Scan(&total)

	if err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return total, nil
}