package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mediaflow/internal/auth"
	"mediaflow/internal/config"
)

// Integration tests that test the complete upload flow with authentication
func TestUploadIntegration_WithAuth(t *testing.T) {
	// Setup configuration
	cfg := &config.Config{
		APIKey: "test-api-key",
	}
	
	storageConfig := &config.StorageConfig{
		Profiles: map[string]config.Profile{
			"avatar": {
				Kind:                 "image",
				AllowedMimes:         []string{"image/jpeg", "image/png"},
				SizeMaxBytes:         5 * 1024 * 1024,
				MultipartThresholdMB: 15,
				PartSizeMB:          8,
				TokenTTLSeconds:     900,
				PathTemplate:        "raw/{shard?}/{key_base}.{ext}",
				EnableSharding:      true,
			},
		},
	}

	// Create a mock upload service
	mockService := &MockUploadService{
		presignUploadFunc: func(ctx context.Context, req *PresignRequest, profile *config.Profile) (*PresignResponse, error) {
			return &PresignResponse{
				ObjectKey: "raw/ab/test-key.jpg",
				Upload: &UploadDetails{
					Single: &SingleUpload{
						Method:    "PUT",
						URL:       "https://test.s3.amazonaws.com/bucket/raw/ab/test-key.jpg",
						Headers:   map[string]string{"Content-Type": "image/jpeg"},
						ExpiresAt: time.Now().Add(15 * time.Minute),
					},
				},
			}, nil
		},
	}

	// Create handler with auth middleware
	handler := &TestHandler{
		uploadService: mockService,
		storageConfig: storageConfig,
		ctx:          context.Background(),
	}

	// Wrap with auth middleware
	authConfig := &auth.Config{APIKey: cfg.APIKey}
	middleware := auth.APIKeyMiddleware(authConfig)
	authenticatedHandler := middleware(http.HandlerFunc(handler.HandlePresign))

	tests := []struct {
		name           string
		authHeader     string
		apiKeyHeader   string
		requestBody    PresignRequest
		expectedStatus int
	}{
		{
			name:       "Valid Bearer token",
			authHeader: "Bearer test-api-key",
			requestBody: PresignRequest{
				KeyBase:   "test-key",
				Ext:       "jpg",
				Mime:      "image/jpeg",
				SizeBytes: 1024000,
				Kind:      "image",
				Profile:   "avatar",
				Multipart: "auto",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:         "Valid X-API-Key header",
			apiKeyHeader: "test-api-key",
			requestBody: PresignRequest{
				KeyBase:   "test-key",
				Ext:       "jpg",
				Mime:      "image/jpeg",
				SizeBytes: 1024000,
				Kind:      "image",
				Profile:   "avatar",
				Multipart: "auto",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:       "Invalid Bearer token",
			authHeader: "Bearer wrong-key",
			requestBody: PresignRequest{
				KeyBase:   "test-key",
				Ext:       "jpg",
				Mime:      "image/jpeg",
				SizeBytes: 1024000,
				Kind:      "image",
				Profile:   "avatar",
				Multipart: "auto",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "No authentication",
			requestBody: PresignRequest{
				KeyBase:   "test-key",
				Ext:       "jpg",
				Mime:      "image/jpeg",
				SizeBytes: 1024000,
				Kind:      "image",
				Profile:   "avatar",
				Multipart: "auto",
			},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/v1/uploads/presign", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			if tt.apiKeyHeader != "" {
				req.Header.Set("X-API-Key", tt.apiKeyHeader)
			}

			rr := httptest.NewRecorder()
			authenticatedHandler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}

			// For successful requests, verify response structure
			if tt.expectedStatus == http.StatusOK {
				var response PresignResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
					t.Errorf("Failed to parse response: %v", err)
				}

				if response.ObjectKey == "" {
					t.Errorf("Expected non-empty ObjectKey")
				}

				if response.Upload == nil {
					t.Errorf("Expected Upload details")
				}

				if response.Upload.Single == nil {
					t.Errorf("Expected Single upload details")
				}
			}

			// For error requests, verify error structure
			if tt.expectedStatus != http.StatusOK {
				var errorResp auth.ErrorResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &errorResp); err != nil {
					t.Errorf("Failed to parse error response: %v", err)
				}

				if errorResp.Code == "" {
					t.Errorf("Expected non-empty error code")
				}
			}
		})
	}
}

func TestUploadIntegration_ValidationFlow(t *testing.T) {
	// Setup configuration for validation tests
	cfg := &config.Config{
		APIKey: "test-api-key",
	}
	
	storageConfig := &config.StorageConfig{
		Profiles: map[string]config.Profile{
			"avatar": {
				Kind:                 "image",
				AllowedMimes:         []string{"image/jpeg", "image/png"},
				SizeMaxBytes:         1024 * 1024, // 1MB limit for testing
				MultipartThresholdMB: 15,
				PartSizeMB:          8,
				TokenTTLSeconds:     900,
				PathTemplate:        "raw/{shard?}/{key_base}.{ext}",
				EnableSharding:      true,
			},
		},
	}

	// Create real service with mock S3 client for more realistic testing
	mockS3 := &MockS3Client{
		presignPutObjectFunc: func(ctx context.Context, key string, expires time.Duration, headers map[string]string) (string, error) {
			return "https://test.s3.amazonaws.com/bucket/" + key, nil
		},
	}
	
	realService := NewService(mockS3, &config.Config{S3Bucket: "test-bucket"})

	handler := &Handler{
		uploadService: realService,
		storageConfig: storageConfig,
		ctx:          context.Background(),
	}

	// Wrap with auth middleware
	authConfig := &auth.Config{APIKey: cfg.APIKey}
	middleware := auth.APIKeyMiddleware(authConfig)
	authenticatedHandler := middleware(http.HandlerFunc(handler.HandlePresign))

	tests := []struct {
		name           string
		requestBody    PresignRequest
		expectedStatus int
		expectedError  string
	}{
		{
			name: "Valid request",
			requestBody: PresignRequest{
				KeyBase:   "test-key",
				Ext:       "jpg",
				Mime:      "image/jpeg",
				SizeBytes: 500000, // 500KB - within limit
				Kind:      "image",
				Profile:   "avatar",
				Multipart: "auto",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "File too large",
			requestBody: PresignRequest{
				KeyBase:   "test-key",
				Ext:       "jpg",
				Mime:      "image/jpeg",
				SizeBytes: 2 * 1024 * 1024, // 2MB - exceeds 1MB limit
				Kind:      "image",
				Profile:   "avatar",
				Multipart: "auto",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "size_too_large",
		},
		{
			name: "Invalid MIME type",
			requestBody: PresignRequest{
				KeyBase:   "test-key",
				Ext:       "txt",
				Mime:      "text/plain",
				SizeBytes: 500000,
				Kind:      "image",
				Profile:   "avatar",
				Multipart: "auto",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "mime_not_allowed",
		},
		{
			name: "Invalid profile",
			requestBody: PresignRequest{
				KeyBase:   "test-key",
				Ext:       "jpg",
				Mime:      "image/jpeg",
				SizeBytes: 500000,
				Kind:      "image",
				Profile:   "nonexistent",
				Multipart: "auto",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "bad_request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest("POST", "/v1/uploads/presign", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-api-key")

			rr := httptest.NewRecorder()
			authenticatedHandler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}

			if tt.expectedStatus == http.StatusOK {
				var response PresignResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
					t.Errorf("Failed to parse response: %v", err)
				}

				// Verify response has required fields
				if response.ObjectKey == "" {
					t.Errorf("Expected non-empty ObjectKey")
				}

				if response.Upload == nil || response.Upload.Single == nil {
					t.Errorf("Expected Single upload details")
				}

				// Verify object key follows template pattern
				if !strings.Contains(response.ObjectKey, "raw/") {
					t.Errorf("Expected object key to contain 'raw/', got: %s", response.ObjectKey)
				}

				if !strings.Contains(response.ObjectKey, ".jpg") {
					t.Errorf("Expected object key to contain extension, got: %s", response.ObjectKey)
				}
			} else {
				var errorResp ErrorResponse
				if err := json.Unmarshal(rr.Body.Bytes(), &errorResp); err != nil {
					t.Errorf("Failed to parse error response: %v", err)
				}

				if errorResp.Code != tt.expectedError {
					t.Errorf("Expected error code '%s', got '%s'", tt.expectedError, errorResp.Code)
				}
			}
		})
	}
}

func TestUploadIntegration_MultipartStrategy(t *testing.T) {
	// Setup configuration for multipart testing
	cfg := &config.Config{
		APIKey: "test-api-key",
	}
	
	storageConfig := &config.StorageConfig{
		Profiles: map[string]config.Profile{
			"video": {
				Kind:                 "video",
				AllowedMimes:         []string{"video/mp4"},
				SizeMaxBytes:         100 * 1024 * 1024, // 100MB
				MultipartThresholdMB: 15,               // 15MB threshold
				PartSizeMB:          8,                  // 8MB parts
				TokenTTLSeconds:     900,
				PathTemplate:        "raw/{shard?}/{key_base}.{ext}",
				EnableSharding:      true,
			},
		},
	}

	// Create mock S3 client for multipart testing
	mockS3 := &MockS3Client{
		createMultipartUploadFunc: func(ctx context.Context, key string, headers map[string]string) (string, error) {
			return "test-upload-id", nil
		},
		presignUploadPartFunc: func(ctx context.Context, key, uploadID string, partNumber int32, expires time.Duration) (string, error) {
			return "https://test.s3.amazonaws.com/bucket/" + key + "?partNumber=" + string(rune(partNumber+'0')), nil
		},
	}
	
	realService := NewService(mockS3, &config.Config{S3Bucket: "test-bucket"})

	handler := &Handler{
		uploadService: realService,
		storageConfig: storageConfig,
		ctx:          context.Background(),
	}

	// Wrap with auth middleware
	authConfig := &auth.Config{APIKey: cfg.APIKey}
	middleware := auth.APIKeyMiddleware(authConfig)
	authenticatedHandler := middleware(http.HandlerFunc(handler.HandlePresign))

	// Test multipart upload for large file
	requestBody := PresignRequest{
		KeyBase:   "large-video",
		Ext:       "mp4",
		Mime:      "video/mp4",
		SizeBytes: 50 * 1024 * 1024, // 50MB - above 15MB threshold
		Kind:      "video",
		Profile:   "video",
		Multipart: "auto",
	}

	body, _ := json.Marshal(requestBody)
	req := httptest.NewRequest("POST", "/v1/uploads/presign", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-api-key")

	rr := httptest.NewRecorder()
	authenticatedHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var response PresignResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}

	// Verify multipart response
	if response.Upload.Multipart == nil {
		t.Errorf("Expected multipart upload details")
	}

	if response.Upload.Single != nil {
		t.Errorf("Expected no single upload details for large file")
	}

	if response.Upload.Multipart.UploadID != "test-upload-id" {
		t.Errorf("Expected upload ID 'test-upload-id', got '%s'", response.Upload.Multipart.UploadID)
	}

	if len(response.Upload.Multipart.Parts) == 0 {
		t.Errorf("Expected part URLs to be generated")
	}

	// Verify part numbers are sequential
	for i, part := range response.Upload.Multipart.Parts {
		expectedPartNumber := i + 1
		if part.PartNumber != expectedPartNumber {
			t.Errorf("Expected part number %d, got %d", expectedPartNumber, part.PartNumber)
		}
		if part.Method != "PUT" {
			t.Errorf("Expected PUT method for part, got %s", part.Method)
		}
		if part.URL == "" {
			t.Errorf("Expected non-empty URL for part %d", part.PartNumber)
		}
	}
}