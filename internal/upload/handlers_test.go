package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mediaflow/internal/config"
)

// Create a test handler that uses an interface for the upload service
type TestHandler struct {
	uploadService UploadService
	storageConfig *config.StorageConfig
	ctx           context.Context
}

func (h *TestHandler) HandlePresign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, ErrBadRequest, "Method not allowed", "")
		return
	}

	// Parse request body
	var req PresignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "Invalid request body", "")
		return
	}

	// Validate required fields
	if req.KeyBase == "" {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "key_base is required", "")
		return
	}
	if req.Ext == "" {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "ext is required", "")
		return
	}
	if req.Mime == "" {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "mime is required", "")
		return
	}
	if req.SizeBytes <= 0 {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "size_bytes must be greater than 0", "")
		return
	}
	if req.Kind == "" {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "kind is required", "")
		return
	}
	if req.Profile == "" {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "profile is required", "")
		return
	}

	// Get profile configuration
	profile := h.storageConfig.GetProfile(req.Profile)
	if profile == nil {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, fmt.Sprintf("No configuration for profile: %s", req.Profile), "Configure profile in your storage config")
		return
	}

	// Validate kind matches profile
	if profile.Kind != req.Kind {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, fmt.Sprintf("Kind mismatch: expected %s, got %s", profile.Kind, req.Kind), "")
		return
	}

	// Construct base URL from request
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)
	
	// Generate presigned upload
	presignResp, err := h.uploadService.PresignUpload(h.ctx, &req, profile, baseURL)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "mime type not allowed:") {
			h.writeError(w, http.StatusBadRequest, ErrMimeNotAllowed, err.Error(), "Check allowed_mimes in upload configuration")
			return
		}
		if strings.Contains(errStr, "file size exceeds maximum:") {
			h.writeError(w, http.StatusBadRequest, ErrSizeTooLarge, err.Error(), "Reduce file size or check size_max_bytes in configuration")
			return
		}
		h.writeError(w, http.StatusInternalServerError, ErrBadRequest, fmt.Sprintf("Failed to generate presigned upload: %v", err), "")
		return
	}

	// Return presigned response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(presignResp)
}

func (h *TestHandler) HandleCompleteMultipart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeError(w, http.StatusMethodNotAllowed, ErrBadRequest, "Method not allowed", "")
		return
	}

	// Extract object_key and upload_id from URL path
	path := strings.TrimPrefix(r.URL.Path, "/v1/uploads/")
	parts := strings.Split(path, "/complete/")
	if len(parts) != 2 {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "Invalid URL format", "Expected /v1/uploads/{object_key}/complete/{upload_id}")
		return
	}
	
	objectKey := parts[0]
	uploadID := parts[1]

	// Parse request body
	var req CompleteMultipartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "Invalid request body", "")
		return
	}

	// Validate required fields
	if len(req.Parts) == 0 {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "parts is required and cannot be empty", "")
		return
	}

	// Complete the multipart upload
	err := h.uploadService.CompleteMultipartUpload(h.ctx, objectKey, uploadID, &req)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, ErrBadRequest, fmt.Sprintf("Failed to complete multipart upload: %v", err), "")
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]string{"status": "completed", "object_key": objectKey}
	_ = json.NewEncoder(w).Encode(response)
}

func (h *TestHandler) HandleAbortMultipart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.writeError(w, http.StatusMethodNotAllowed, ErrBadRequest, "Method not allowed", "")
		return
	}

	// Extract object_key and upload_id from URL path
	path := strings.TrimPrefix(r.URL.Path, "/v1/uploads/")
	parts := strings.Split(path, "/abort/")
	if len(parts) != 2 {
		h.writeError(w, http.StatusBadRequest, ErrBadRequest, "Invalid URL format", "Expected /v1/uploads/{object_key}/abort/{upload_id}")
		return
	}
	
	objectKey := parts[0]
	uploadID := parts[1]

	// Abort the multipart upload
	err := h.uploadService.AbortMultipartUpload(h.ctx, objectKey, uploadID)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, ErrBadRequest, fmt.Sprintf("Failed to abort multipart upload: %v", err), "")
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]string{"status": "aborted", "upload_id": uploadID}
	_ = json.NewEncoder(w).Encode(response)
}

func (h *TestHandler) writeError(w http.ResponseWriter, statusCode int, code, message, hint string) {
	errorResp := ErrorResponse{
		Code:    code,
		Message: message,
		Hint:    hint,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(errorResp)
}

// UploadService interface for dependency injection
type UploadService interface {
	PresignUpload(ctx context.Context, req *PresignRequest, profile *config.Profile, baseURL string) (*PresignResponse, error)
	CompleteMultipartUpload(ctx context.Context, objectKey, uploadID string, req *CompleteMultipartRequest) error
	AbortMultipartUpload(ctx context.Context, objectKey, uploadID string) error
}

// MockUploadService implements the upload service interface for testing
type MockUploadService struct {
	presignUploadFunc           func(ctx context.Context, req *PresignRequest, profile *config.Profile, baseURL string) (*PresignResponse, error)
	completeMultipartUploadFunc func(ctx context.Context, objectKey, uploadID string, req *CompleteMultipartRequest) error
	abortMultipartUploadFunc    func(ctx context.Context, objectKey, uploadID string) error
}

func (m *MockUploadService) PresignUpload(ctx context.Context, req *PresignRequest, profile *config.Profile, baseURL string) (*PresignResponse, error) {
	if m.presignUploadFunc != nil {
		return m.presignUploadFunc(ctx, req, profile, baseURL)
	}

	// Default mock response
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
}

func (m *MockUploadService) CompleteMultipartUpload(ctx context.Context, objectKey, uploadID string, req *CompleteMultipartRequest) error {
	if m.completeMultipartUploadFunc != nil {
		return m.completeMultipartUploadFunc(ctx, objectKey, uploadID, req)
	}
	return nil
}

func (m *MockUploadService) AbortMultipartUpload(ctx context.Context, objectKey, uploadID string) error {
	if m.abortMultipartUploadFunc != nil {
		return m.abortMultipartUploadFunc(ctx, objectKey, uploadID)
	}
	return nil
}

func TestHandler_HandlePresign_Success(t *testing.T) {
	// Setup
	mockService := &MockUploadService{}
	storageConfig := &config.StorageConfig{
		Profiles: map[string]config.Profile{
			"avatar": {
				Kind:                 "image",
				AllowedMimes:         []string{"image/jpeg", "image/png"},
				SizeMaxBytes:         5 * 1024 * 1024,
				MultipartThresholdMB: 15,
				PartSizeMB:           8,
				TokenTTLSeconds:      900,
				StoragePath:         "originals/{shard?}/{key_base}.{ext}",
				EnableSharding:       true,
			},
		},
	}

	handler := &TestHandler{
		uploadService: mockService,
		storageConfig: storageConfig,
		ctx:           context.Background(),
	}

	// Create request
	requestBody := PresignRequest{
		KeyBase:   "test-key",
		Ext:       "jpg",
		Mime:      "image/jpeg",
		SizeBytes: 1024000,
		Kind:      "image",
		Profile:   "avatar",
		Multipart: "auto",
	}

	body, _ := json.Marshal(requestBody)
	req := httptest.NewRequest("POST", "/v1/uploads/presign", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	// Execute
	rr := httptest.NewRecorder()
	handler.HandlePresign(rr, req)

	// Assert
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

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
}

func TestHandler_HandlePresign_ValidationErrors(t *testing.T) {
	// Setup
	storageConfig := &config.StorageConfig{
		Profiles: map[string]config.Profile{
			"avatar": {
				Kind:                 "image",
				AllowedMimes:         []string{"image/jpeg", "image/png"},
				SizeMaxBytes:         5 * 1024 * 1024,
				MultipartThresholdMB: 15,
				PartSizeMB:           8,
				TokenTTLSeconds:      900,
				StoragePath:         "originals/{shard?}/{key_base}.{ext}",
				EnableSharding:       true,
			},
		},
	}

	handler := &TestHandler{
		uploadService: &MockUploadService{},
		storageConfig: storageConfig,
		ctx:           context.Background(),
	}

	tests := []struct {
		name           string
		method         string
		requestBody    interface{}
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "Invalid method",
			method:         "GET",
			requestBody:    map[string]interface{}{},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedCode:   ErrBadRequest,
		},
		{
			name:   "Missing key_base",
			method: "POST",
			requestBody: map[string]interface{}{
				"ext":        "jpg",
				"mime":       "image/jpeg",
				"size_bytes": 1024000,
				"kind":       "image",
				"profile":    "avatar",
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   ErrBadRequest,
		},
		{
			name:   "Missing ext",
			method: "POST",
			requestBody: map[string]interface{}{
				"key_base":   "test-key",
				"mime":       "image/jpeg",
				"size_bytes": 1024000,
				"kind":       "image",
				"profile":    "avatar",
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   ErrBadRequest,
		},
		{
			name:   "Missing mime",
			method: "POST",
			requestBody: map[string]interface{}{
				"key_base":   "test-key",
				"ext":        "jpg",
				"size_bytes": 1024000,
				"kind":       "image",
				"profile":    "avatar",
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   ErrBadRequest,
		},
		{
			name:   "Invalid size_bytes",
			method: "POST",
			requestBody: map[string]interface{}{
				"key_base":   "test-key",
				"ext":        "jpg",
				"mime":       "image/jpeg",
				"size_bytes": 0,
				"kind":       "image",
				"profile":    "avatar",
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   ErrBadRequest,
		},
		{
			name:   "Missing kind",
			method: "POST",
			requestBody: map[string]interface{}{
				"key_base":   "test-key",
				"ext":        "jpg",
				"mime":       "image/jpeg",
				"size_bytes": 1024000,
				"profile":    "avatar",
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   ErrBadRequest,
		},
		{
			name:   "Missing profile",
			method: "POST",
			requestBody: map[string]interface{}{
				"key_base":   "test-key",
				"ext":        "jpg",
				"mime":       "image/jpeg",
				"size_bytes": 1024000,
				"kind":       "image",
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   ErrBadRequest,
		},
		{
			name:   "Invalid profile",
			method: "POST",
			requestBody: map[string]interface{}{
				"key_base":   "test-key",
				"ext":        "jpg",
				"mime":       "image/jpeg",
				"size_bytes": 1024000,
				"kind":       "image",
				"profile":    "nonexistent",
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   ErrBadRequest,
		},
		{
			name:   "Kind mismatch",
			method: "POST",
			requestBody: map[string]interface{}{
				"key_base":   "test-key",
				"ext":        "jpg",
				"mime":       "image/jpeg",
				"size_bytes": 1024000,
				"kind":       "video", // Profile is configured for "image"
				"profile":    "avatar",
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   ErrBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.requestBody != nil {
				body, _ = json.Marshal(tt.requestBody)
			}

			req := httptest.NewRequest(tt.method, "/v1/uploads/presign", bytes.NewReader(body))
			if tt.method == "POST" {
				req.Header.Set("Content-Type", "application/json")
			}

			rr := httptest.NewRecorder()
			handler.HandlePresign(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}

			var errorResp ErrorResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &errorResp); err != nil {
				t.Errorf("Failed to parse error response: %v", err)
			}

			if errorResp.Code != tt.expectedCode {
				t.Errorf("Expected error code '%s', got '%s'", tt.expectedCode, errorResp.Code)
			}
		})
	}
}

func TestHandler_HandlePresign_ServiceErrors(t *testing.T) {
	// Setup
	storageConfig := &config.StorageConfig{
		Profiles: map[string]config.Profile{
			"avatar": {
				Kind:                 "image",
				AllowedMimes:         []string{"image/jpeg", "image/png"},
				SizeMaxBytes:         5 * 1024 * 1024,
				MultipartThresholdMB: 15,
				PartSizeMB:           8,
				TokenTTLSeconds:      900,
				StoragePath:         "originals/{shard?}/{key_base}.{ext}",
				EnableSharding:       true,
			},
		},
	}

	tests := []struct {
		name           string
		serviceError   error
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "MIME not allowed",
			serviceError:   fmt.Errorf("mime type not allowed: text/plain"),
			expectedStatus: http.StatusBadRequest,
			expectedCode:   ErrMimeNotAllowed,
		},
		{
			name:           "File too large",
			serviceError:   fmt.Errorf("file size exceeds maximum: 10485760 > 5242880"),
			expectedStatus: http.StatusBadRequest,
			expectedCode:   ErrSizeTooLarge,
		},
		{
			name:           "Generic service error",
			serviceError:   fmt.Errorf("some other error"),
			expectedStatus: http.StatusInternalServerError,
			expectedCode:   ErrBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &MockUploadService{
				presignUploadFunc: func(ctx context.Context, req *PresignRequest, profile *config.Profile, baseURL string) (*PresignResponse, error) {
					return nil, tt.serviceError
				},
			}

			handler := &TestHandler{
				uploadService: mockService,
				storageConfig: storageConfig,
				ctx:           context.Background(),
			}

			requestBody := PresignRequest{
				KeyBase:   "test-key",
				Ext:       "jpg",
				Mime:      "image/jpeg",
				SizeBytes: 1024000,
				Kind:      "image",
				Profile:   "avatar",
				Multipart: "auto",
			}

			body, _ := json.Marshal(requestBody)
			req := httptest.NewRequest("POST", "/v1/uploads/presign", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			handler.HandlePresign(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}

			var errorResp ErrorResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &errorResp); err != nil {
				t.Errorf("Failed to parse error response: %v", err)
			}

			if errorResp.Code != tt.expectedCode {
				t.Errorf("Expected error code '%s', got '%s'", tt.expectedCode, errorResp.Code)
			}
		})
	}
}

func TestHandler_HandlePresign_InvalidJSON(t *testing.T) {
	storageConfig := &config.StorageConfig{
		Profiles: map[string]config.Profile{
			"avatar": {
				Kind: "image",
			},
		},
	}

	handler := &TestHandler{
		uploadService: &MockUploadService{},
		storageConfig: storageConfig,
		ctx:           context.Background(),
	}

	req := httptest.NewRequest("POST", "/v1/uploads/presign", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.HandlePresign(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	var errorResp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &errorResp); err != nil {
		t.Errorf("Failed to parse error response: %v", err)
	}

	if errorResp.Code != ErrBadRequest {
		t.Errorf("Expected error code '%s', got '%s'", ErrBadRequest, errorResp.Code)
	}
}

func TestHandler_writeError(t *testing.T) {
	handler := &TestHandler{}

	rr := httptest.NewRecorder()
	handler.writeError(rr, http.StatusBadRequest, ErrBadRequest, "Test error", "Test hint")

	// Check status
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	// Check Content-Type
	contentType := rr.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}

	// Check response body
	var errorResp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &errorResp); err != nil {
		t.Errorf("Failed to parse error response: %v", err)
	}

	if errorResp.Code != ErrBadRequest {
		t.Errorf("Expected code '%s', got '%s'", ErrBadRequest, errorResp.Code)
	}

	if errorResp.Message != "Test error" {
		t.Errorf("Expected message 'Test error', got '%s'", errorResp.Message)
	}

	if errorResp.Hint != "Test hint" {
		t.Errorf("Expected hint 'Test hint', got '%s'", errorResp.Hint)
	}
}

func TestHandler_HandleCompleteMultipart_Success(t *testing.T) {
	called := false
	var capturedObjectKey, capturedUploadID string
	var capturedRequest *CompleteMultipartRequest

	mockService := &MockUploadService{
		completeMultipartUploadFunc: func(ctx context.Context, objectKey, uploadID string, req *CompleteMultipartRequest) error {
			called = true
			capturedObjectKey = objectKey
			capturedUploadID = uploadID
			capturedRequest = req
			return nil
		},
	}

	handler := &TestHandler{
		uploadService: mockService,
		ctx:           context.Background(),
	}

	requestBody := CompleteMultipartRequest{
		Parts: []CompletedPart{
			{PartNumber: 1, ETag: "etag1"},
			{PartNumber: 2, ETag: "etag2"},
		},
	}

	body, _ := json.Marshal(requestBody)
	req := httptest.NewRequest("POST", "/v1/uploads/raw/test-object-key.jpg/complete/test-upload-id", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.HandleCompleteMultipart(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	if !called {
		t.Errorf("Expected service method to be called")
	}

	if capturedObjectKey != "raw/test-object-key.jpg" {
		t.Errorf("Expected object key 'raw/test-object-key.jpg', got '%s'", capturedObjectKey)
	}

	if capturedUploadID != "test-upload-id" {
		t.Errorf("Expected upload ID 'test-upload-id', got '%s'", capturedUploadID)
	}

	if len(capturedRequest.Parts) != 2 {
		t.Errorf("Expected 2 parts, got %d", len(capturedRequest.Parts))
	}

	var response map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Errorf("Failed to parse response: %v", err)
	}

	if response["status"] != "completed" {
		t.Errorf("Expected status 'completed', got '%s'", response["status"])
	}

	if response["object_key"] != "raw/test-object-key.jpg" {
		t.Errorf("Expected object_key 'raw/test-object-key.jpg', got '%s'", response["object_key"])
	}
}

func TestHandler_HandleCompleteMultipart_ValidationErrors(t *testing.T) {
	handler := &TestHandler{
		uploadService: &MockUploadService{},
		ctx:           context.Background(),
	}

	tests := []struct {
		name           string
		method         string
		url            string
		requestBody    interface{}
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "Invalid method",
			method:         "GET",
			url:            "/v1/uploads/raw/test-object-key.jpg/complete/test-upload-id",
			requestBody:    map[string]interface{}{},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedCode:   ErrBadRequest,
		},
		{
			name:           "Invalid URL format",
			method:         "POST",
			url:            "/v1/uploads/invalid-url",
			requestBody:    map[string]interface{}{},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   ErrBadRequest,
		},
		{
			name:   "Empty parts",
			method: "POST",
			url:    "/v1/uploads/raw/test-object-key.jpg/complete/test-upload-id",
			requestBody: map[string]interface{}{
				"parts": []interface{}{},
			},
			expectedStatus: http.StatusBadRequest,
			expectedCode:   ErrBadRequest,
		},
		{
			name:           "Invalid JSON",
			method:         "POST",
			url:            "/v1/uploads/raw/test-object-key.jpg/complete/test-upload-id",
			requestBody:    "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   ErrBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.requestBody != nil {
				if s, ok := tt.requestBody.(string); ok && s == "invalid json" {
					body = []byte(s)
				} else {
					body, _ = json.Marshal(tt.requestBody)
				}
			}

			req := httptest.NewRequest(tt.method, tt.url, bytes.NewReader(body))
			if tt.method == "POST" {
				req.Header.Set("Content-Type", "application/json")
			}

			rr := httptest.NewRecorder()
			handler.HandleCompleteMultipart(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}

			var errorResp ErrorResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &errorResp); err != nil {
				t.Errorf("Failed to parse error response: %v", err)
			}

			if errorResp.Code != tt.expectedCode {
				t.Errorf("Expected error code '%s', got '%s'", tt.expectedCode, errorResp.Code)
			}
		})
	}
}

func TestHandler_HandleAbortMultipart_Success(t *testing.T) {
	called := false
	var capturedObjectKey, capturedUploadID string

	mockService := &MockUploadService{
		abortMultipartUploadFunc: func(ctx context.Context, objectKey, uploadID string) error {
			called = true
			capturedObjectKey = objectKey
			capturedUploadID = uploadID
			return nil
		},
	}

	handler := &TestHandler{
		uploadService: mockService,
		ctx:           context.Background(),
	}

	req := httptest.NewRequest("DELETE", "/v1/uploads/raw/test-object-key.jpg/abort/test-upload-id", nil)

	rr := httptest.NewRecorder()
	handler.HandleAbortMultipart(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}

	if !called {
		t.Errorf("Expected service method to be called")
	}

	if capturedObjectKey != "raw/test-object-key.jpg" {
		t.Errorf("Expected object key 'raw/test-object-key.jpg', got '%s'", capturedObjectKey)
	}

	if capturedUploadID != "test-upload-id" {
		t.Errorf("Expected upload ID 'test-upload-id', got '%s'", capturedUploadID)
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

func TestHandler_HandleAbortMultipart_ValidationErrors(t *testing.T) {
	handler := &TestHandler{
		uploadService: &MockUploadService{},
		ctx:           context.Background(),
	}

	tests := []struct {
		name           string
		method         string
		url            string
		expectedStatus int
		expectedCode   string
	}{
		{
			name:           "Invalid method",
			method:         "POST",
			url:            "/v1/uploads/raw/test-object-key.jpg/abort/test-upload-id",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedCode:   ErrBadRequest,
		},
		{
			name:           "Invalid URL format",
			method:         "DELETE",
			url:            "/v1/uploads/invalid-url",
			expectedStatus: http.StatusBadRequest,
			expectedCode:   ErrBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.url, nil)

			rr := httptest.NewRecorder()
			handler.HandleAbortMultipart(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d. Body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}

			var errorResp ErrorResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &errorResp); err != nil {
				t.Errorf("Failed to parse error response: %v", err)
			}

			if errorResp.Code != tt.expectedCode {
				t.Errorf("Expected error code '%s', got '%s'", tt.expectedCode, errorResp.Code)
			}
		})
	}
}

func TestHandler_HandleCompleteMultipart_ServiceError(t *testing.T) {
	mockService := &MockUploadService{
		completeMultipartUploadFunc: func(ctx context.Context, objectKey, uploadID string, req *CompleteMultipartRequest) error {
			return fmt.Errorf("service error")
		},
	}

	handler := &TestHandler{
		uploadService: mockService,
		ctx:           context.Background(),
	}

	requestBody := CompleteMultipartRequest{
		Parts: []CompletedPart{{PartNumber: 1, ETag: "etag1"}},
	}

	body, _ := json.Marshal(requestBody)
	req := httptest.NewRequest("POST", "/v1/uploads/raw/test-object-key.jpg/complete/test-upload-id", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.HandleCompleteMultipart(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}

	var errorResp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &errorResp); err != nil {
		t.Errorf("Failed to parse error response: %v", err)
	}

	if errorResp.Code != ErrBadRequest {
		t.Errorf("Expected error code '%s', got '%s'", ErrBadRequest, errorResp.Code)
	}
}

func TestHandler_HandleAbortMultipart_ServiceError(t *testing.T) {
	mockService := &MockUploadService{
		abortMultipartUploadFunc: func(ctx context.Context, objectKey, uploadID string) error {
			return fmt.Errorf("service error")
		},
	}

	handler := &TestHandler{
		uploadService: mockService,
		ctx:           context.Background(),
	}

	req := httptest.NewRequest("DELETE", "/v1/uploads/raw/test-object-key.jpg/abort/test-upload-id", nil)

	rr := httptest.NewRecorder()
	handler.HandleAbortMultipart(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}

	var errorResp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &errorResp); err != nil {
		t.Errorf("Failed to parse error response: %v", err)
	}

	if errorResp.Code != ErrBadRequest {
		t.Errorf("Expected error code '%s', got '%s'", ErrBadRequest, errorResp.Code)
	}
}
