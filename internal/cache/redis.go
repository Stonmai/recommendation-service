package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/actuallystonmai/recommendation-service/internal/domain"
	"github.com/redis/go-redis/v9"
)

const defaultTTL = 10 * time.Minute

type Cache struct {
	client *redis.Client
}

func NewCache(client *redis.Client) *Cache {
	return &Cache{client: client}
}

func buildKey(userID int64, limit int) string {
	return fmt.Sprintf("rec:user:%d:limit:%d", userID, limit)
}

// Get recommendations from cache
func (c *Cache) Get(ctx context.Context, userID int64, limit int) ([]domain.ScoredRecommendation, error) {
	key := buildKey(userID, limit)
	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	
	if err != nil {
		return nil, fmt.Errorf("failed to get recommendations from cache: %w", err)
	}
	
	var recs []domain.ScoredRecommendation
	if err := json.Unmarshal([]byte(val), &recs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recommendations %s: %w", key, err)
	}
	
	return recs, nil
}

// Store recommendations in cache
func (c *Cache) Set(ctx context.Context, userID int64, limit int, recs []domain.ScoredRecommendation) error {
	key := buildKey(userID, limit)
	val, err := json.Marshal(recs)
	if err != nil {
		return fmt.Errorf("failed to marshal recommendations: %w", err)
	}
	
	if err := c.client.Set(ctx, key, val, defaultTTL).Err(); err != nil {
		return fmt.Errorf("failed to set recommendations in cache: %w", err)
	}
	
	return nil
}

// Clear user cache: used when watch history changes
func (c *Cache) ClearUserCache(ctx context.Context, userID int64) error {
	pattern := fmt.Sprintf("rec:user:%d:limit:*", userID)
	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
			return fmt.Errorf("cache delete %s: %w", iter.Val(), err)
		}
	}
	return iter.Err()
}

// Ping connectivity
func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}