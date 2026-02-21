package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/actuallystonmai/recommendation-service/internal/cache"
	"github.com/actuallystonmai/recommendation-service/internal/domain"
	"github.com/actuallystonmai/recommendation-service/internal/model"
	"github.com/actuallystonmai/recommendation-service/internal/repository"
)

const (
	defaultLimit        = 10
	maxLimit            = 50
	watchHistoryLimit   = 50
	candidatePoolSize   = 100
	batchConcurrency    = 10
	batchRecLimit       = 10
)

type BatchStatus string

const (
	StatusSuccess BatchStatus = "success"
	StatusFailed  BatchStatus = "failed"
)

type Service struct {
	repo *repository.Repository
	cache *cache.Cache
	modelClient *model.Client
}

func NewService(repo *repository.Repository, cache *cache.Cache, modelClient *model.Client) *Service {
	return &Service{
		repo: repo,
		cache: cache,
		modelClient: modelClient,
	}
}

func (s *Service) GetRecommendations(ctx context.Context, userID int64, limit int) (*domain.RecommendationResult, error) {
	if limit <= 0 {
		limit = defaultLimit
	} else if limit > maxLimit {
		limit = maxLimit
	}
	
	// Check Cache
	cached, found, err := s.cache.Get(ctx, userID, limit)
	if err != nil {
		log.Printf("[service] cache get error for user %d: %v", userID, err)
	}
	
	// Use recommendations from cache if available
	if found {
		return &domain.RecommendationResult {
			Recommendations: cached,
			CacheHit: true,
		}, nil
	}
	
	// Cache miss -> generate recommendations
	recs, err := s.generateRecommendations(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	
	// Store recommendations in cache
	if cacheErr := s.cache.Set(ctx, userID, limit, recs); cacheErr != nil {
		log.Printf("[service] cache set error for user %d: %v", userID, cacheErr)
	}
	
	return &domain.RecommendationResult{
		Recommendations: recs,
		CacheHit: false,
	}, nil
}

func (s *Service) generateRecommendations(ctx context.Context, userID int64, limit int) ([]domain.ScoredRecommendation, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("fetch user: %w", err)
	}

	watchHistory, err := s.repo.GetUserWatchHistoryWithGenres(ctx, userID, watchHistoryLimit)
	if err != nil {
		return nil, fmt.Errorf("fetch watch history: %w", err)
	}

	candidates, err := s.repo.GetUnwatchedContent(ctx, userID, candidatePoolSize)
	if err != nil {
		return nil, fmt.Errorf("fetch candidates: %w", err)
	}

	scored, err := s.modelClient.Score(model.ScoreInput{
		User:         user,
		WatchHistory: watchHistory,
		Candidates:   candidates,
		Limit:        limit,
	})
	if err != nil {
		return nil, err
	}

	return scored, nil
}

func (s *Service) GetBatchRecommendations(ctx context.Context, page, limit int) (*domain.BatchResponse, error) {
	start := time.Now()

	// Fetch paginated user IDs
	userIDs, err := s.repo.GetUserIDsPaginated(ctx, page, limit)
	if err != nil {
		return nil, fmt.Errorf("fetch user ids: %w", err)
	}
	
	// Fetch total user
	totalUsers, err := s.repo.CountUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("count user: %w", err)
	}

	// Process users concurrently with bounded worker pool
	results := make([]domain.BatchUserResult, len(userIDs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, batchConcurrency) // semaphore

	for i, userID := range userIDs {
		wg.Add(1)
		go func(idx int, uid int64) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			result := s.processUserForBatch(ctx, uid)
			results[idx] = result
		}(i, userID)
	}
	wg.Wait()

	// summary
	successCount := 0
	failedCount := 0
	for _, r := range results {
		if r.Status == domain.StatusSuccess {
			successCount++
		} else {
			failedCount++
		}
	}

	elapsed := time.Since(start).Milliseconds()

	return &domain.BatchResponse{
		Page:       page,
		Limit:      limit,
		TotalUsers: totalUsers,
		Results:    results,
		Summary: domain.BatchSummary{
			SuccessCount:     successCount,
			FailedCount:      failedCount,
			ProcessingTimeMs: elapsed,
		},
		Metadata: domain.BatchMeta{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}, nil
}

// Generates recommendations for a singl user, capturing errors.
func (s *Service) processUserForBatch(ctx context.Context, userID int64) domain.BatchUserResult {
	result, err := s.GetRecommendations(ctx, userID, batchRecLimit)
	if err != nil {
		log.Printf("[service] batch: failed for user %d: %v", userID, err)
		code, msg := categorizeError(err)
		return domain.BatchUserResult{
			UserID:  userID,
			Status:  domain.StatusFailed,
			Error:   code,
			Message: msg,
		}
	}

	return domain.BatchUserResult{
		UserID:          userID,
		Recommendations: result.Recommendations,
		Status:          domain.StatusSuccess,
	}
}

// Add watch history for a user and clear user's cache
func (s *Service) AddWatchHistory(ctx context.Context, userID, contentID int64) error {
    if err := s.repo.AddWatchHistory(ctx, userID, contentID); err != nil {
        return err
    }
    if err := s.cache.ClearUserCache(ctx, userID); err != nil {
        log.Printf("[service] cache invalidation error for user %d: %v", userID, err)
    }
    return nil
}

// Handle response error
func categorizeError(err error) (string, string) {
	if errors.Is(err, domain.ErrUserNotFound) {
		return "user_not_found", "user not found"
	}
	if model.IsModelInferenceError(err) {
		return "model_inference_error", "recommendation model failed to generate a response"
	}
	return "internal_error", "an unexpected error occurred"
}