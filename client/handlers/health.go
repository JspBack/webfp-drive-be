package handlers

import "net/http"

// Health godoc
// @Summary      Check service health
// @Description  Returns success when the HTTP service is responding.
// @Tags         health
// @Success      200  "Service is healthy"
// @Router       /api/health [get]
func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
