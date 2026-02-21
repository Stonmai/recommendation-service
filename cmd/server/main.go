package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/actuallystonmai/recommendation-service/internal/config"
	"github.com/actuallystonmai/recommendation-service/seeds"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config %v", err)
	}
	
	ctx := context.Background()
	
	// ------------ PostgreSQL ---------------
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to parse database config %v", err)
	}
	poolConfig.MaxConns = int32(cfg.DBPoolSize)
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("failed to connect to database %v", err)
	}
	defer pool.Close()

	if err := waitForDB(ctx, pool); err != nil {
		log.Fatalf("database not ready: %v", err)
	}
	log.Println("connected to PostgreSQL")
	
	// ------------ Run Migrations ---------------
	// for migrate-down using CLI command
	if len(os.Args) > 1 && os.Args[1] == "migrate-down" {
		if err := migrateDown(ctx, pool); err != nil {
			log.Fatalf("failed to migrate down %v", err)
		}
		log.Println("migrations dropped")
		return
	}

	if err := migrateUp(ctx, pool); err != nil {
		log.Fatalf("failed to migrate up %v", err)
	}
	
	// ------------ Setup Seed Data ---------------
	if err := checkSeed(ctx, pool); err != nil {
		log.Fatalf("failed to check seed %v", err)
	}
	
	
	// ---------------- Server --------------------	
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func waitForDB(ctx context.Context, pool *pgxpool.Pool) error {
	for i := 0; i < 30; i++ {
		if err := pool.Ping(ctx); err == nil {
			return nil
		}
		log.Printf("waiting for database... (%d/30)", i+1)
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("database connection timeout after 30s")
}

func migrateDown(ctx context.Context, pool *pgxpool.Pool) error {
	sql, err := os.ReadFile("migrations/create_tables.down.sql")
	if err != nil {
		return fmt.Errorf("read migration file: %w", err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		return fmt.Errorf("execute migration: %w", err)
	}
	log.Println("migrations dropped successfully")
	return nil
}

func migrateUp(ctx context.Context, pool *pgxpool.Pool) error {
	sql, err := os.ReadFile("migrations/create_tables.up.sql")
	if err != nil {
		return fmt.Errorf("read migration file: %w", err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		return fmt.Errorf("execute migration: %w", err)
	}
	log.Println("migrations applied successfully")
	return nil
}

func checkSeed(ctx context.Context, pool *pgxpool.Pool) error {
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return fmt.Errorf("check users count: %w", err)
	}
	if count > 0 {
		log.Printf("database already seeded (%d users), skipping", count)
		return nil
	}
	return seeds.Setup(ctx, pool)
}