package handler

import (
	"net/http"
	"strconv"
)

// GET /recommendations/batch
func (h *Handler) GetBatchRecommendations(w http.ResponseWriter, r *http.Request) {
	// Parse and validate page
	page := 1
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		parsed, err := strconv.Atoi(pageStr)
		if err != nil || parsed < 1 || parsed > 10000  {
			writeError(w, http.StatusBadRequest, "invalid_parameter", "Invalid page parameter")
			return
		}
		page = parsed
	}

	// Parse and validate limit
	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "invalid_parameter", "Invalid limit parameter")
			return
		}
		limit = parsed
	}
	
	// Call service
	result, err := h.service.GetBatchRecommendations(r.Context(), page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred")
		return
	}

	writeJSON(w, http.StatusOK, result)
}