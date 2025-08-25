package auth

import (
	"encoding/json"
	"net/http"
	"strings"
)

type Config struct {
	APIKey string
}

type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// APIKeyMiddleware validates API key authentication
func APIKeyMiddleware(config *Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth if no API key configured (for development)
			if config.APIKey == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Check Authorization header (Bearer token)
			authHeader := r.Header.Get("Authorization")
			if authHeader != "" {
				if strings.HasPrefix(authHeader, "Bearer ") {
					token := strings.TrimPrefix(authHeader, "Bearer ")
					if token == config.APIKey {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			// Check X-API-Key header
			apiKeyHeader := r.Header.Get("X-API-Key")
			if apiKeyHeader == config.APIKey {
				next.ServeHTTP(w, r)
				return
			}

			// No valid authentication found
			writeUnauthorized(w)
		})
	}
}

func writeUnauthorized(w http.ResponseWriter) {
	errorResp := ErrorResponse{
		Code:    "unauthorized",
		Message: "Invalid or missing API key",
		Hint:    "Provide API key via Authorization: Bearer <key> or X-API-Key: <key>",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(errorResp)
}