package handler

import (
	"net/http"
)

// HealthHandler handles health and readiness check endpoints.
type HealthHandler struct{}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler() HealthHandler {
	return HealthHandler{}
}

// Health responds with a simple health check indicating the service is alive.
// GET /health
func (h HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := map[string]string{
		"status": "healthy",
	}

	JSON(w, http.StatusOK, response)
}

// Ready responds with readiness status, indicating whether the service is ready to handle requests.
// GET /ready
func (h HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := map[string]string{
		"status": "ready",
	}

	JSON(w, http.StatusOK, response)
}
