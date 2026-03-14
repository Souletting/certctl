package handler

import (
	"encoding/json"
	"net/http"
)

// PagedResponse represents a paginated API response.
type PagedResponse struct {
	Data    interface{} `json:"data"`
	Total   int64       `json:"total"`
	Page    int         `json:"page"`
	PerPage int         `json:"per_page"`
}

// ErrorResponse represents a standard error response.
type ErrorResponse struct {
	Error     string `json:"error"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// JSON writes a JSON response with the given status code and data.
func JSON(w http.ResponseWriter, status int, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(data)
}

// Error writes a JSON error response with the given status code and message.
func Error(w http.ResponseWriter, status int, message string) error {
	errResp := ErrorResponse{
		Error:   http.StatusText(status),
		Message: message,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(errResp)
}

// ErrorWithRequestID writes a JSON error response including a request ID.
func ErrorWithRequestID(w http.ResponseWriter, status int, message, requestID string) error {
	errResp := ErrorResponse{
		Error:     http.StatusText(status),
		Message:   message,
		RequestID: requestID,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(errResp)
}
