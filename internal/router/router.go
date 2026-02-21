package router

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/actuallystonmai/recommendation-service/internal/handler"
)

func Setup(h *handler.Handler) http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Routes
	r.Get("/users/{userID}/recommendations", h.GetRecommendations)
	r.Get("/recommendations/batch", h.GetBatchRecommendations)
	r.Get("/health", healthCheck)

	return r
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}
