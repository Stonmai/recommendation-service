package model

import (
	"errors"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/actuallystonmai/recommendation-service/internal/domain"
)

type Client struct {}

func NewClient() *Client {
	return &Client{}
}

type ModelInferenceError struct {
	Msg string
}

func (e *ModelInferenceError) Error() string {
	return e.Msg
}

type ScoreInput struct {
	User *domain.User
	WatchHistory []domain.WatchHistoryItem
	Candidates []domain.Content
	Limit int
}

func IsModelInferenceError(err error) bool {
	var target *ModelInferenceError
	return errors.As(err, &target)
}

func (c *Client) Score(input ScoreInput) ([]domain.ScoredRecommendation, error) {
	//  Set model latency: 30-50ms
	delay := time.Duration(30+rand.Intn(21)) * time.Millisecond
	time.Sleep(delay)

	// Set random fail: 1.5% rate
	if rand.Float64() < 0.015 {
		return nil, &ModelInferenceError{Msg: "model inference failed"}
	}

	// Calculate preference
	genrePreferences := calculateGenrePreferenceWeights(input.WatchHistory)

	// Score each candidate
	now := time.Now()
	scored := make([]domain.ScoredRecommendation, 0, len(input.Candidates))

	for _, content := range input.Candidates {
		score := computeFinalScore(content, genrePreferences, now)
		scored = append(scored, domain.ScoredRecommendation{
			ContentID:       content.ID,
			Title:           content.Title,
			Genre:           content.Genre,
			PopularityScore: content.PopularityScore,
			Score:           math.Round(score*1000) / 1000, // 3 decimal places
		})
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	// Take top N
	if len(scored) > input.Limit {
		scored = scored[:input.Limit]
	}

	return scored, nil
}

func calculateGenrePreferenceWeights(history []domain.WatchHistoryItem) map[string]float64 {
	genreCounts := make(map[string]int)
	for _, item := range history {
		genreCounts[item.Genre]++
	}

	total := float64(len(history))
	prefs := make(map[string]float64, len(genreCounts))

	if total == 0 {
		return prefs
	}
	
	for genre, count := range genreCounts {
		prefs[genre] = float64(count) / total
	}

	return prefs
}


func calculateRecencyFactor(createdAt, now time.Time) float64 {
	daysSinceCreation := now.Sub(createdAt).Hours() / 24.0
	return 1.0 / (1.0 + daysSinceCreation/365.0)
}

func computeFinalScore(content domain.Content, genrePrefs map[string]float64, now time.Time) float64 {
	popularityComponent := content.PopularityScore * 0.4

	genrePref, ok := genrePrefs[content.Genre]
	if !ok {
		genrePref = 0.1
	}
	genreBoost := genrePref * 0.35
	
	// Recency component
	recencyFactor := calculateRecencyFactor(content.CreatedAt, now)
	recencyComponent := recencyFactor * 0.15

	randomNoise := (rand.Float64()*0.1 - 0.05) * 0.1

	return popularityComponent + genreBoost + recencyComponent + randomNoise
}