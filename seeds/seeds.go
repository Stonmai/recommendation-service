package seeds

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Setup(ctx context.Context, pool *pgxpool.Pool) error {
	rng := rand.New(rand.NewSource(42))

	// Truncate existing data before insert
	log.Println("[seed] truncating existing data")
	if _, err := pool.Exec(ctx, `
		TRUNCATE user_watch_history, content, users RESTART IDENTITY CASCADE
	`); err != nil {
		return fmt.Errorf("truncate: %w", err)
	}

	log.Println("[seed] inserting users")
	if err := seedUsers(ctx, pool, rng, 20); err != nil {
		return fmt.Errorf("seed users: %w", err)
	}

	log.Println("[seed] inserting content")
	if err := seedContent(ctx, pool, rng, 50); err != nil {
		return fmt.Errorf("seed content: %w", err)
	}

	log.Println("[seed] inserting watch history")
	if err := seedWatchHistory(ctx, pool, rng, 200); err != nil {
		return fmt.Errorf("seed watch history: %w", err)
	}

	log.Println("[seed] seeding complete")
	return nil
}

func seedUsers(ctx context.Context, pool *pgxpool.Pool, rng *rand.Rand, n int) error {
	countries := []string{"US", "GB", "CA", "AU", "DE", "FR", "JP", "BR"}
	subscriptionTypes := []string{"free", "basic", "premium"}
	subscriptionWeights := []float64{0.5, 0.3, 0.2}

	rows := []string{}
	args := []any{}

	for i := range n {
		age := rng.Intn(48) + 18
		country := countries[rng.Intn(len(countries))]
		subscription := weightedChoice(rng, subscriptionTypes, subscriptionWeights)
		createdAt := time.Now().AddDate(0, 0, -rng.Intn(365))

		base := i * 4
		rows = append(rows, fmt.Sprintf("($%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4))

		args = append(args, age, country, subscription, createdAt)
	}
	
	if len(rows) == 0 {
		return nil
	}
	
	query := "INSERT INTO users (age, country, subscription_type, created_at) VALUES " + strings.Join(rows, ", ")

	_, err := pool.Exec(ctx, query, args...)
	return err
}

func seedContent(ctx context.Context, pool *pgxpool.Pool, rng *rand.Rand, n int) error {
	genres := []string{"action", "drama", "comedy", "thriller", "sci-fi"}
	titles := map[string][]string{
		"action": {
			"Die Hard", "Mad Max: Fury Road", "John Wick", "The Dark Knight",
			"Gladiator", "Top Gun: Maverick", "The Raid", "Mission: Impossible",
			"Casino Royale", "The Avengers",
		},
		"drama": {
			"The Shawshank Redemption", "Forrest Gump", "The Godfather",
			"Schindler's List", "A Beautiful Mind", "12 Angry Men",
			"Parasite", "Moonlight", "Whiplash", "The Green Mile",
		},
		"comedy": {
			"Superbad", "The Hangover", "Bridesmaids", "Step Brothers",
			"Anchorman", "Mean Girls", "Borat", "Hot Fuzz",
			"Groundhog Day", "The Grand Budapest Hotel",
		},
		"thriller": {
			"Se7en", "Gone Girl", "Zodiac", "Prisoners",
			"Sicario", "No Country for Old Men", "Nightcrawler",
			"Shutter Island", "The Silence of the Lambs", "Oldboy",
		},
		"sci-fi": {
			"Blade Runner 2049", "Interstellar", "The Matrix", "Arrival",
			"Dune", "Ex Machina", "Alien", "Inception",
			"Edge of Tomorrow", "2001: A Space Odyssey",
		},
	}
	
	rows := []string{}
	args := []any{}

	for i := range n {
		genre := genres[i%len(genres)]
		titleList := titles[genre]
		title := titleList[i%len(titleList)]

		if i >= len(genres) {
			title = fmt.Sprintf("%s %d", title, i/len(genres)+1)
		}

		popularity := powerLawScore(rng)
		createdAt := time.Now().AddDate(0, 0, -rng.Intn(730))

		base := len(args)
		rows = append(rows, fmt.Sprintf("($%d, $%d, $%d, $%d)", base+1, base+2, base+3, base+4))
		args = append(args, title, genre, popularity, createdAt)
	}

	if len(rows) == 0 {
		return nil
	}

	query := "INSERT INTO content (title, genre, popularity_score, created_at) VALUES " +
		strings.Join(rows, ", ")

	_, err := pool.Exec(ctx, query, args...)
	return err
}

func seedWatchHistory(ctx context.Context, pool *pgxpool.Pool, rng *rand.Rand, n int) error {
	seen := make(map[[2]int64]bool)

	rows := []string{}
	args := []any{}

	for range n {
		userID := int64(math.Ceil(math.Pow(rng.Float64(), 1.5) * 20))
		userID = max(1, min(userID, 20))

		contentID := int64(math.Ceil(math.Pow(rng.Float64(), 1.3) * 50))
		contentID = max(1, min(contentID, 50))

		key := [2]int64{userID, contentID}
		if seen[key] {
			continue
		}
		seen[key] = true

		watchedAt := time.Now().AddDate(0, 0, -rng.Intn(180))

		base := len(args)
		rows = append(rows, fmt.Sprintf("($%d, $%d, $%d)", base+1, base+2, base+3))
		args = append(args, userID, contentID, watchedAt)
	}

	if len(rows) == 0 {
		return nil
	}

	query := "INSERT INTO user_watch_history (user_id, content_id, watched_at) VALUES " +
		strings.Join(rows, ", ")

	_, err := pool.Exec(ctx, query, args...)
	return err
}


func powerLawScore(rng *rand.Rand) float64 {
	u := rng.Float64()
	if u == 0 {
		u = 0.001
	}
	raw := math.Pow(u, 2.0)
	if raw < 0.01 {
		raw = 0.01
	}
	return math.Round(raw*100) / 100
}

func weightedChoice(rng *rand.Rand, choices []string, weights []float64) string {
	total := 0.0
	for _, w := range weights {
		total += w
	}
	r := rng.Float64() * total
	cumulative := 0.0
	for i, w := range weights {
		cumulative += w
		if r <= cumulative {
			return choices[i]
		}
	}
	return choices[len(choices)-1]
}