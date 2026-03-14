package middleware

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// RequestIDKey is the context key for storing request IDs.
type RequestIDKey struct{}

// UserKey is the context key for storing authenticated user information.
type UserKey struct{}

// RequestID middleware generates a unique request ID and adds it to the request context and response headers.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := uuid.New().String()
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), RequestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Logging middleware logs request details including method, path, status, and duration.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		requestID := getRequestID(r.Context())
		log.Printf("[%s] %s %s %d %v", requestID, r.Method, r.URL.Path, wrapped.statusCode, duration)
	})
}

// Recovery middleware recovers from panics and returns a 500 error.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				requestID := getRequestID(r.Context())
				log.Printf("[%s] PANIC: %v", requestID, err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Auth middleware is a placeholder that checks the Authorization header and extracts user information.
// In production, this would validate tokens, verify signatures, etc.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// For now, allow requests without auth (placeholder)
			// In production, enforce auth on protected routes
			next.ServeHTTP(w, r)
			return
		}

		// Simple stub: just extract user ID from Bearer token (format: "Bearer <user_id>")
		// This is NOT secure and for development only
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			userID := authHeader[7:]
			ctx := context.WithValue(r.Context(), UserKey{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		http.Error(w, "Invalid Authorization header", http.StatusUnauthorized)
	})
}

// ContentType middleware sets the Content-Type header to application/json.
func ContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}

// CORS middleware adds CORS headers to allow cross-origin requests.
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// GetRequestID extracts the request ID from context.
func GetRequestID(ctx context.Context) string {
	return getRequestID(ctx)
}

// getRequestID is an internal helper to extract request ID from context.
func getRequestID(ctx context.Context) string {
	id, ok := ctx.Value(RequestIDKey{}).(string)
	if !ok {
		return "unknown"
	}
	return id
}

// GetUser extracts the authenticated user from context.
func GetUser(ctx context.Context) (string, bool) {
	user, ok := ctx.Value(UserKey{}).(string)
	return user, ok
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Chain chains multiple middleware functions.
func Chain(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}
