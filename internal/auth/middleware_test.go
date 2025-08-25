package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIKeyMiddleware(t *testing.T) {
	// Create a test handler that returns "OK" if auth passes
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	tests := []struct {
		name           string
		apiKey         string
		authHeader     string
		apiKeyHeader   string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "No API key configured - should pass",
			apiKey:         "",
			authHeader:     "",
			apiKeyHeader:   "",
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
		},
		{
			name:           "Valid Bearer token",
			apiKey:         "test-secret-key",
			authHeader:     "Bearer test-secret-key",
			apiKeyHeader:   "",
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
		},
		{
			name:           "Valid X-API-Key header",
			apiKey:         "test-secret-key",
			authHeader:     "",
			apiKeyHeader:   "test-secret-key",
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
		},
		{
			name:           "Invalid Bearer token",
			apiKey:         "test-secret-key",
			authHeader:     "Bearer wrong-key",
			apiKeyHeader:   "",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"code":"unauthorized","message":"Invalid or missing API key","hint":"Provide API key via Authorization: Bearer <key> or X-API-Key: <key>"}`,
		},
		{
			name:           "Invalid X-API-Key",
			apiKey:         "test-secret-key",
			authHeader:     "",
			apiKeyHeader:   "wrong-key",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"code":"unauthorized","message":"Invalid or missing API key","hint":"Provide API key via Authorization: Bearer <key> or X-API-Key: <key>"}`,
		},
		{
			name:           "No auth headers provided",
			apiKey:         "test-secret-key",
			authHeader:     "",
			apiKeyHeader:   "",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"code":"unauthorized","message":"Invalid or missing API key","hint":"Provide API key via Authorization: Bearer <key> or X-API-Key: <key>"}`,
		},
		{
			name:           "Malformed Bearer token",
			apiKey:         "test-secret-key",
			authHeader:     "InvalidFormat test-secret-key",
			apiKeyHeader:   "",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"code":"unauthorized","message":"Invalid or missing API key","hint":"Provide API key via Authorization: Bearer <key> or X-API-Key: <key>"}`,
		},
		{
			name:           "Empty Bearer token",
			apiKey:         "test-secret-key",
			authHeader:     "Bearer ",
			apiKeyHeader:   "",
			expectedStatus: http.StatusUnauthorized,
			expectedBody:   `{"code":"unauthorized","message":"Invalid or missing API key","hint":"Provide API key via Authorization: Bearer <key> or X-API-Key: <key>"}`,
		},
		{
			name:           "Both headers provided - Bearer wins",
			apiKey:         "test-secret-key",
			authHeader:     "Bearer test-secret-key",
			apiKeyHeader:   "wrong-key",
			expectedStatus: http.StatusOK,
			expectedBody:   "OK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			config := &Config{APIKey: tt.apiKey}
			middleware := APIKeyMiddleware(config)
			handler := middleware(testHandler)

			// Create request
			req := httptest.NewRequest("POST", "/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.apiKeyHeader != "" {
				req.Header.Set("X-API-Key", tt.apiKeyHeader)
			}

			// Execute
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			// Assert status
			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			// Assert body
			if tt.expectedStatus == http.StatusUnauthorized {
				// For error responses, check JSON structure
				var errorResp ErrorResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &errorResp); err != nil {
					t.Errorf("Failed to parse error response: %v", err)
				}
				if errorResp.Code != "unauthorized" {
					t.Errorf("Expected error code 'unauthorized', got '%s'", errorResp.Code)
				}
				if errorResp.Message != "Invalid or missing API key" {
					t.Errorf("Expected error message 'Invalid or missing API key', got '%s'", errorResp.Message)
				}
			} else {
				// For success responses
				if rr.Body.String() != tt.expectedBody {
					t.Errorf("Expected body '%s', got '%s'", tt.expectedBody, rr.Body.String())
				}
			}
		})
	}
}

func TestAPIKeyMiddleware_ContentType(t *testing.T) {
	config := &Config{APIKey: "test-key"}
	middleware := APIKeyMiddleware(config)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})
	handler := middleware(testHandler)

	req := httptest.NewRequest("POST", "/test", nil)
	// No auth headers - should fail
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Check Content-Type header is set correctly
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

func TestWriteUnauthorized(t *testing.T) {
	rr := httptest.NewRecorder()
	writeUnauthorized(rr)

	// Check status
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	// Check Content-Type
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}

	// Check response body structure
	var errorResp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &errorResp); err != nil {
		t.Errorf("Failed to parse error response: %v", err)
	}

	if errorResp.Code != "unauthorized" {
		t.Errorf("Expected code 'unauthorized', got '%s'", errorResp.Code)
	}

	if errorResp.Message == "" {
		t.Error("Expected non-empty message")
	}

	if errorResp.Hint == "" {
		t.Error("Expected non-empty hint")
	}
}