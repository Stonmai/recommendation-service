package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/actuallystonmai/recommendation-service/internal/domain"
)

func TestScore(t *testing.T) {
	client := NewClient()

	input := ScoreInput{
		User: &domain.User{
			ID:               1,
			Age:              25,
			Country:          "US",
			SubscriptionType: "premium",
		},
		WatchHistory: []domain.WatchHistoryItem{
			{ContentID: 1, Genre: "action", WatchedAt: time.Now()},
			{ContentID: 2, Genre: "action", WatchedAt: time.Now()},
			{ContentID: 3, Genre: "action", WatchedAt: time.Now()},
			{ContentID: 4, Genre: "drama", WatchedAt: time.Now()},
		},
		Candidates: []domain.Content{
			{ID: 10, Title: "Action Movie", Genre: "action", PopularityScore: 0.9, CreatedAt: time.Now()},
			{ID: 11, Title: "Drama Movie", Genre: "drama", PopularityScore: 0.5, CreatedAt: time.Now()},
			{ID: 12, Title: "Comedy Movie", Genre: "comedy", PopularityScore: 0.3, CreatedAt: time.Now()},
		},
		Limit: 2,
	}

	results, err := client.Score(input)
	if err != nil {
		// 1.5% random failure -> retry
		results, err = client.Score(input)
		if err != nil {
			t.Fatalf("Score failed twice: %v", err)
		}
	}

	// Check result limit
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// Check sorted descending
	if results[0].Score < results[1].Score {
		t.Errorf("results not sorted: %f < %f", results[0].Score, results[1].Score)
	}

	// Check highest
	if results[0].Genre != "action" {
		t.Errorf("expected action first, got %s", results[0].Genre)
	}

	// Print results
	for _, r := range results {
		fmt.Printf("  %s (%s) → score: %.3f\n", r.Title, r.Genre, r.Score)
	}
}

func TestGenrePreferences(t *testing.T) {
	history := []domain.WatchHistoryItem{
		{Genre: "action"},
		{Genre: "action"},
		{Genre: "action"},
		{Genre: "drama"},
	}

	prefs := calculateGenrePreferenceWeights(history)

	// action: 3/4 = 0.75
	if prefs["action"] != 0.75 {
		t.Errorf("expected action=0.75, got %f", prefs["action"])
	}

	// drama: 1/4 = 0.25
	if prefs["drama"] != 0.25 {
		t.Errorf("expected drama=0.25, got %f", prefs["drama"])
	}

	// comedy: not in map
	if _, exists := prefs["comedy"]; exists {
		t.Error("comedy should not be in preferences")
	}
}

func TestEmptyWatchHistory(t *testing.T) {
	prefs := calculateGenrePreferenceWeights([]domain.WatchHistoryItem{})

	if len(prefs) != 0 {
		t.Errorf("expected empty prefs, got %v", prefs)
	}
}

func TestRecencyFactor(t *testing.T) {
	now := time.Now()

	// Content today -> factor close to 1.0
	recent := calculateRecencyFactor(now, now)
	if recent < 0.99 {
		t.Errorf("expected ~1.0 for today, got %f", recent)
	}

	// Content 1 year -> factor close to 0.5
	oneYearAgo := now.AddDate(-1, 0, 0)
	old := calculateRecencyFactor(oneYearAgo, now)
	if old < 0.45 || old > 0.55 {
		t.Errorf("expected ~0.5 for 1 year, got %f", old)
	}

	fmt.Printf("  Today: %.3f\n", recent)
	fmt.Printf("  1 year ago: %.3f\n", old)
}

func TestModelInferenceError(t *testing.T) {
	err := &ModelInferenceError{Msg: "model inference failed"}

	if !IsModelInferenceError(err) {
		t.Error("should detect ModelInferenceError")
	}

	if IsModelInferenceError(fmt.Errorf("random error")) {
		t.Error("should not detect regular error as ModelInferenceError")
	}
}