package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/actuallystonmai/recommendation-service/internal/domain"
	"github.com/go-chi/chi/v5"
)

// GET /users/{userID}/recommendations
func (h *Handler) GetRecommendations(w http.ResponseWriter, r *http.Request) {
	// Parse and validate user_id
	userIDStr := chi.URLParam(r, "userID")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_parameter", "Invalid user_id parameter")
		return
	}

	// Parse and validate limit
	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed < 1 || parsed > 50 {
			writeError(w, http.StatusBadRequest, "invalid_parameter", "Invalid limit parameter")
			return
		}
		limit = parsed
	}

	result, err := h.service.GetRecommendations(r.Context(), userID, limit)
	if err != nil {
		// User not found
		if errors.Is(err, domain.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user_not_found",
				fmt.Sprintf("User with ID %d does not exist", userID))
			return
		}
		// Model inference failure
		if errors.Is(err, domain.ErrModelUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "model_unavailable",
				"Recommendation model is temporarily unavailable")
			return
		}
		// Request timeout
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			writeError(w, http.StatusServiceUnavailable, "request_timeout",
				"Request timed out, please try again")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	resp := RecommendationResponse{
		UserID:          userID,
		Recommendations: result.Recommendations,
		Metadata: domain.RecommendationMeta{
			CacheHit:    result.CacheHit,
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			TotalCount:  len(result.Recommendations),
		},
	}

	writeJSON(w, http.StatusOK, resp)
}
