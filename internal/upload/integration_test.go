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
	"mediaflow/internal/s3"
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
				StoragePath:        "originals/{shard?}/{key_base}.{ext}",
				EnableSharding:      true,
			},
		},
	}

	// Create a mock upload service
	mockService := &MockUploadService{
		presignUploadFunc: func(ctx context.Context, req *PresignRequest, profile *config.Profile, baseURL string) (*PresignResponse, error) {
			return &PresignResponse{
				ObjectKey: "originals/ab/test-key.jpg",
				Upload: &UploadDetails{
					Single: &SingleUpload{
						Method:    "PUT",
						URL:       "https://test.s3.amazonaws.com/bucket/originals/ab/test-key.jpg",
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
				StoragePath:        "originals/{shard?}/{key_base}.{ext}",
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
				if !strings.Contains(response.ObjectKey, "originals/") {
					t.Errorf("Expected object key to contain 'originals/', got: %s", response.ObjectKey)
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
				StoragePath:        "originals/{shard?}/{key_base}.{ext}",
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

	// Verify complete and abort URLs are present
	if response.Upload.Multipart.Complete == nil {
		t.Errorf("Expected complete URL to be populated")
	} else {
		if response.Upload.Multipart.Complete.Method != "POST" {
			t.Errorf("Expected complete method to be POST, got %s", response.Upload.Multipart.Complete.Method)
		}
		if !strings.Contains(response.Upload.Multipart.Complete.URL, "/complete/") {
			t.Errorf("Complete URL should contain '/complete/', got: %s", response.Upload.Multipart.Complete.URL)
		}
	}

	if response.Upload.Multipart.Abort == nil {
		t.Errorf("Expected abort URL to be populated")
	} else {
		if response.Upload.Multipart.Abort.Method != "DELETE" {
			t.Errorf("Expected abort method to be DELETE, got %s", response.Upload.Multipart.Abort.Method)
		}
		if !strings.Contains(response.Upload.Multipart.Abort.URL, "/abort/") {
			t.Errorf("Abort URL should contain '/abort/', got: %s", response.Upload.Multipart.Abort.URL)
		}
	}
}

func TestUploadIntegration_CompleteMultipartFlow(t *testing.T) {
	// Setup configuration
	cfg := &config.Config{
		APIKey: "test-api-key",
	}
	
	storageConfig := &config.StorageConfig{
		Profiles: map[string]config.Profile{
			"video": {
				Kind:                 "video",
				AllowedMimes:         []string{"video/mp4"},
				SizeMaxBytes:         100 * 1024 * 1024,
				MultipartThresholdMB: 15,
				PartSizeMB:          8,
				TokenTTLSeconds:     900,
				StoragePath:        "originals/{key_base}.{ext}",
				EnableSharding:      false,
			},
		},
	}

	// Create mock S3 client
	mockS3 := &MockS3Client{
		createMultipartUploadFunc: func(ctx context.Context, key string, headers map[string]string) (string, error) {
			return "test-upload-id", nil
		},
		presignUploadPartFunc: func(ctx context.Context, key, uploadID string, partNumber int32, expires time.Duration) (string, error) {
			return "https://test.s3.amazonaws.com/bucket/" + key + "?partNumber=" + string(rune(partNumber+'0')), nil
		},
		completeMultipartUploadFunc: func(ctx context.Context, key, uploadID string, parts []s3.PartInfo) error {
			// Verify the expected parameters
			if key != "originals/test-video.mp4" {
				t.Errorf("Expected key 'originals/test-video.mp4', got '%s'", key)
			}
			if uploadID != "test-upload-id" {
				t.Errorf("Expected upload ID 'test-upload-id', got '%s'", uploadID)
			}
			if len(parts) != 2 {
				t.Errorf("Expected 2 parts, got %d", len(parts))
			}
			return nil
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
	
	// Test complete multipart upload
	requestBody := CompleteMultipartRequest{
		Parts: []CompletedPart{
			{PartNumber: 1, ETag: "etag1"},
			{PartNumber: 2, ETag: "etag2"},
		},
	}

	body, _ := json.Marshal(requestBody)
	req := httptest.NewRequest("POST", "/v1/uploads/originals/test-video.mp4/complete/test-upload-id", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-api-key")

	authenticatedHandler := middleware(http.HandlerFunc(handler.HandleCompleteMultipart))
	rr := httptest.NewRecorder()
	authenticatedHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var response map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}

	if response["status"] != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", response["status"])
	}

	if response["object_key"] != "originals/test-video.mp4" {
		t.Errorf("Expected object_key 'originals/test-video.mp4', got '%s'", response["object_key"])
	}
}

func TestUploadIntegration_AbortMultipartFlow(t *testing.T) {
	// Setup configuration
	cfg := &config.Config{
		APIKey: "test-api-key",
	}

	// Create mock S3 client
	mockS3 := &MockS3Client{
		abortMultipartUploadFunc: func(ctx context.Context, key, uploadID string) error {
			// Verify the expected parameters
			if key != "originals/test-video.mp4" {
				t.Errorf("Expected key 'originals/test-video.mp4', got '%s'", key)
			}
			if uploadID != "test-upload-id" {
				t.Errorf("Expected upload ID 'test-upload-id', got '%s'", uploadID)
			}
			return nil
		},
	}
	
	realService := NewService(mockS3, &config.Config{S3Bucket: "test-bucket"})

	handler := &Handler{
		uploadService: realService,
		storageConfig: &config.StorageConfig{},
		ctx:          context.Background(),
	}

	// Wrap with auth middleware
	authConfig := &auth.Config{APIKey: cfg.APIKey}
	middleware := auth.APIKeyMiddleware(authConfig)
	
	req := httptest.NewRequest("DELETE", "/v1/uploads/originals/test-video.mp4/abort/test-upload-id", nil)
	req.Header.Set("Authorization", "Bearer test-api-key")

	authenticatedHandler := middleware(http.HandlerFunc(handler.HandleAbortMultipart))
	rr := httptest.NewRecorder()
	authenticatedHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	var response map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}

	if response["status"] != "aborted" {
		t.Errorf("Expected status 'aborted', got '%s'", response["status"])
	}

	if response["upload_id"] != "test-upload-id" {
		t.Errorf("Expected upload_id 'test-upload-id', got '%s'", response["upload_id"])
	}
}

func TestUploadIntegration_CompleteMultipartAuth(t *testing.T) {
	// Test authentication for complete endpoint
	cfg := &config.Config{
		APIKey: "test-api-key",
	}

	mockS3 := &MockS3Client{}
	realService := NewService(mockS3, &config.Config{S3Bucket: "test-bucket"})

	handler := &Handler{
		uploadService: realService,
		storageConfig: &config.StorageConfig{},
		ctx:          context.Background(),
	}

	authConfig := &auth.Config{APIKey: cfg.APIKey}
	middleware := auth.APIKeyMiddleware(authConfig)
	
	requestBody := CompleteMultipartRequest{
		Parts: []CompletedPart{{PartNumber: 1, ETag: "etag1"}},
	}

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
	}{
		{
			name:           "Valid auth",
			authHeader:     "Bearer test-api-key",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid auth",
			authHeader:     "Bearer wrong-key",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "No auth",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(requestBody)
			req := httptest.NewRequest("POST", "/v1/uploads/originals/test-video.mp4/complete/test-upload-id", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			authenticatedHandler := middleware(http.HandlerFunc(handler.HandleCompleteMultipart))
			rr := httptest.NewRecorder()
			authenticatedHandler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}
		})
	}
}