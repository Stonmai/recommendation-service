package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port int
	DatabaseURL string
	RedisURL string
	DBPoolSize int
}

// Load configuration from env
func Load() (*Config, error) {
	port := getEnvInt("PORT", 8080)
	dbURL := getEnv("DATABASE_URL", "postgresql://admin:password@localhost:5432/recommendations?sslmode=disable")
	redisURL := getEnv("REDIS_URL", "redis://localhost:6379")
	dbPoolSize := getEnvInt("DB_POOL_SIZE", 20)
	
	return &Config {
		Port: port,
		DatabaseURL: dbURL,
		RedisURL: redisURL,
		DBPoolSize: dbPoolSize,
	}, nil
}

func (c *Config) Addr() string {
	return fmt.Sprintf(":%d", c.Port)
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}