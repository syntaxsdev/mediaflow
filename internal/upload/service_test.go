package upload

import (
	"context"
	"testing"
	"time"

	"mediaflow/internal/config"
)

// MockS3Client implements S3Client interface for testing
type MockS3Client struct {
	createMultipartUploadFunc func(ctx context.Context, key string, headers map[string]string) (string, error)
	presignPutObjectFunc      func(ctx context.Context, key string, expires time.Duration, headers map[string]string) (string, error)
	presignUploadPartFunc     func(ctx context.Context, key, uploadID string, partNumber int32, expires time.Duration) (string, error)
}

func (m *MockS3Client) CreateMultipartUpload(ctx context.Context, key string, headers map[string]string) (string, error) {
	if m.createMultipartUploadFunc != nil {
		return m.createMultipartUploadFunc(ctx, key, headers)
	}
	return "test-upload-id", nil
}

func (m *MockS3Client) PresignPutObject(ctx context.Context, key string, expires time.Duration, headers map[string]string) (string, error) {
	if m.presignPutObjectFunc != nil {
		return m.presignPutObjectFunc(ctx, key, expires, headers)
	}
	return "https://test.s3.amazonaws.com/bucket/" + key, nil
}

func (m *MockS3Client) PresignUploadPart(ctx context.Context, key, uploadID string, partNumber int32, expires time.Duration) (string, error) {
	if m.presignUploadPartFunc != nil {
		return m.presignUploadPartFunc(ctx, key, uploadID, partNumber, expires)
	}
	return "https://test.s3.amazonaws.com/bucket/" + key + "?partNumber=" + string(rune(partNumber)), nil
}

func TestGenerateShard(t *testing.T) {
	tests := []struct {
		keyBase  string
		expected string
	}{
		{"test-key-1", "1a"},
		{"test-key-2", "0d"},
		{"different-key", "af"},
		{"", "da"}, // SHA1 of empty string
	}

	for _, tt := range tests {
		t.Run(tt.keyBase, func(t *testing.T) {
			result := GenerateShard(tt.keyBase)
			if result != tt.expected {
				t.Errorf("GenerateShard(%s) = %s, expected %s", tt.keyBase, result, tt.expected)
			}
			// Verify it's always 2 hex characters
			if len(result) != 2 {
				t.Errorf("Expected 2 characters, got %d", len(result))
			}
		})
	}
}

func TestService_isMimeAllowed(t *testing.T) {
	service := &Service{}

	tests := []struct {
		name         string
		mime         string
		allowedMimes []string
		expected     bool
	}{
		{
			name:         "Allowed MIME type",
			mime:         "image/jpeg",
			allowedMimes: []string{"image/jpeg", "image/png"},
			expected:     true,
		},
		{
			name:         "Not allowed MIME type",
			mime:         "text/plain",
			allowedMimes: []string{"image/jpeg", "image/png"},
			expected:     false,
		},
		{
			name:         "Empty allowed list",
			mime:         "image/jpeg",
			allowedMimes: []string{},
			expected:     false,
		},
		{
			name:         "Case sensitive",
			mime:         "IMAGE/JPEG",
			allowedMimes: []string{"image/jpeg"},
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.isMimeAllowed(tt.mime, tt.allowedMimes)
			if result != tt.expected {
				t.Errorf("isMimeAllowed(%s, %v) = %t, expected %t", tt.mime, tt.allowedMimes, result, tt.expected)
			}
		})
	}
}

func TestService_determineStrategy(t *testing.T) {
	service := &Service{}

	tests := []struct {
		name        string
		multipart   string
		sizeBytes   int64
		thresholdMB int64
		expected    string
	}{
		{
			name:        "Force multipart",
			multipart:   "force",
			sizeBytes:   1000000,
			thresholdMB: 15,
			expected:    "multipart",
		},
		{
			name:        "Force single",
			multipart:   "off",
			sizeBytes:   50000000,
			thresholdMB: 15,
			expected:    "single",
		},
		{
			name:        "Auto - below threshold",
			multipart:   "auto",
			sizeBytes:   10 * 1024 * 1024, // 10MB
			thresholdMB: 15,
			expected:    "single",
		},
		{
			name:        "Auto - above threshold",
			multipart:   "auto",
			sizeBytes:   20 * 1024 * 1024, // 20MB
			thresholdMB: 15,
			expected:    "multipart",
		},
		{
			name:        "Empty multipart (defaults to auto)",
			multipart:   "",
			sizeBytes:   20 * 1024 * 1024, // 20MB
			thresholdMB: 15,
			expected:    "multipart",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.determineStrategy(tt.multipart, tt.sizeBytes, tt.thresholdMB)
			if result != tt.expected {
				t.Errorf("determineStrategy(%s, %d, %d) = %s, expected %s", tt.multipart, tt.sizeBytes, tt.thresholdMB, result, tt.expected)
			}
		})
	}
}

func TestService_buildRequiredHeaders(t *testing.T) {
	service := &Service{}

	tests := []struct {
		name     string
		mime     string
		expected map[string]string
	}{
		{
			name: "Image MIME type",
			mime: "image/jpeg",
			expected: map[string]string{
				"Content-Type": "image/jpeg",
			},
		},
		{
			name: "Video MIME type",
			mime: "video/mp4",
			expected: map[string]string{
				"Content-Type": "video/mp4",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.buildRequiredHeaders(tt.mime)
			
			for key, expectedValue := range tt.expected {
				if result[key] != expectedValue {
					t.Errorf("Expected header %s = %s, got %s", key, expectedValue, result[key])
				}
			}
		})
	}
}

func TestService_buildObjectKey(t *testing.T) {
	service := &Service{}

	tests := []struct {
		name     string
		template string
		keyBase  string
		ext      string
		shard    string
		expected string
	}{
		{
			name:     "With shard",
			template: "raw/{shard?}/{key_base}.{ext}",
			keyBase:  "test-key",
			ext:      "jpg",
			shard:    "ab",
			expected: "raw/ab/test-key.jpg",
		},
		{
			name:     "Without shard",
			template: "raw/{shard?}/{key_base}.{ext}",
			keyBase:  "test-key",
			ext:      "jpg",
			shard:    "",
			expected: "raw/test-key.jpg",
		},
		{
			name:     "Simple template",
			template: "{key_base}.{ext}",
			keyBase:  "test-key",
			ext:      "mp4",
			shard:    "",
			expected: "test-key.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.buildObjectKey(tt.template, tt.keyBase, tt.ext, tt.shard)
			if result != tt.expected {
				t.Errorf("buildObjectKey(%s, %s, %s, %s) = %s, expected %s", tt.template, tt.keyBase, tt.ext, tt.shard, result, tt.expected)
			}
		})
	}
}

func TestService_PresignUpload_Validation(t *testing.T) {
	mockS3 := &MockS3Client{}
	cfg := &config.Config{S3Bucket: "test-bucket"}
	service := NewService(mockS3, cfg)

	profile := &config.Profile{
		Kind:                 "image",
		AllowedMimes:         []string{"image/jpeg", "image/png"},
		SizeMaxBytes:         5 * 1024 * 1024, // 5MB
		MultipartThresholdMB: 15,
		PartSizeMB:          8,
		TokenTTLSeconds:     900,
		PathTemplate:        "raw/{shard?}/{key_base}.{ext}",
		EnableSharding:      true,
	}

	tests := []struct {
		name        string
		request     *PresignRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid request",
			request: &PresignRequest{
				KeyBase:   "test-key",
				Ext:       "jpg",
				Mime:      "image/jpeg",
				SizeBytes: 1024000,
				Kind:      "image",
				Profile:   "avatar",
				Multipart: "auto",
			},
			expectError: false,
		},
		{
			name: "Invalid MIME type",
			request: &PresignRequest{
				KeyBase:   "test-key",
				Ext:       "txt",
				Mime:      "text/plain",
				SizeBytes: 1024000,
				Kind:      "image",
				Profile:   "avatar",
				Multipart: "auto",
			},
			expectError: true,
			errorMsg:    "mime type not allowed: text/plain",
		},
		{
			name: "File too large",
			request: &PresignRequest{
				KeyBase:   "test-key",
				Ext:       "jpg",
				Mime:      "image/jpeg",
				SizeBytes: 10 * 1024 * 1024, // 10MB > 5MB limit
				Kind:      "image",
				Profile:   "avatar",
				Multipart: "auto",
			},
			expectError: true,
			errorMsg:    "file size exceeds maximum: 10485760 > 5242880",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := service.PresignUpload(ctx, tt.request, profile)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if err.Error() != tt.errorMsg {
					t.Errorf("Expected error '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result == nil {
					t.Errorf("Expected result but got nil")
				} else {
					// Validate response structure
					if result.ObjectKey == "" {
						t.Errorf("Expected non-empty ObjectKey")
					}
					if result.Upload == nil {
						t.Errorf("Expected Upload details")
					}
				}
			}
		})
	}
}

func TestService_PresignUpload_SingleStrategy(t *testing.T) {
	mockS3 := &MockS3Client{
		presignPutObjectFunc: func(ctx context.Context, key string, expires time.Duration, headers map[string]string) (string, error) {
			return "https://test.s3.amazonaws.com/bucket/" + key, nil
		},
	}
	cfg := &config.Config{S3Bucket: "test-bucket"}
	service := NewService(mockS3, cfg)

	profile := &config.Profile{
		Kind:                 "image",
		AllowedMimes:         []string{"image/jpeg"},
		SizeMaxBytes:         5 * 1024 * 1024,
		MultipartThresholdMB: 15,
		PartSizeMB:          8,
		TokenTTLSeconds:     900,
		PathTemplate:        "raw/{key_base}.{ext}",
		EnableSharding:      false,
	}

	request := &PresignRequest{
		KeyBase:   "test-key",
		Ext:       "jpg",
		Mime:      "image/jpeg",
		SizeBytes: 1024000, // 1MB - below threshold
		Kind:      "image",
		Profile:   "avatar",
		Multipart: "auto",
	}

	ctx := context.Background()
	result, err := service.PresignUpload(ctx, request, profile)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result.Upload.Single == nil {
		t.Errorf("Expected single upload details")
	}

	if result.Upload.Multipart != nil {
		t.Errorf("Expected no multipart upload details")
	}

	if result.Upload.Single.Method != "PUT" {
		t.Errorf("Expected PUT method, got %s", result.Upload.Single.Method)
	}

	if result.ObjectKey != "raw/test-key.jpg" {
		t.Errorf("Expected object key 'raw/test-key.jpg', got '%s'", result.ObjectKey)
	}
}

func TestService_PresignUpload_MultipartStrategy(t *testing.T) {
	mockS3 := &MockS3Client{
		createMultipartUploadFunc: func(ctx context.Context, key string, headers map[string]string) (string, error) {
			return "test-upload-id", nil
		},
		presignUploadPartFunc: func(ctx context.Context, key, uploadID string, partNumber int32, expires time.Duration) (string, error) {
			return "https://test.s3.amazonaws.com/bucket/" + key + "?partNumber=" + string(rune(partNumber+'0')), nil
		},
	}
	cfg := &config.Config{S3Bucket: "test-bucket"}
	service := NewService(mockS3, cfg)

	profile := &config.Profile{
		Kind:                 "video",
		AllowedMimes:         []string{"video/mp4"},
		SizeMaxBytes:         100 * 1024 * 1024,
		MultipartThresholdMB: 15,
		PartSizeMB:          8,
		TokenTTLSeconds:     900,
		PathTemplate:        "raw/{key_base}.{ext}",
		EnableSharding:      false,
	}

	request := &PresignRequest{
		KeyBase:   "test-video",
		Ext:       "mp4",
		Mime:      "video/mp4",
		SizeBytes: 50 * 1024 * 1024, // 50MB - above threshold
		Kind:      "video",
		Profile:   "video",
		Multipart: "auto",
	}

	ctx := context.Background()
	result, err := service.PresignUpload(ctx, request, profile)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if result.Upload.Multipart == nil {
		t.Errorf("Expected multipart upload details")
	}

	if result.Upload.Single != nil {
		t.Errorf("Expected no single upload details")
	}

	if result.Upload.Multipart.UploadID != "test-upload-id" {
		t.Errorf("Expected upload ID 'test-upload-id', got '%s'", result.Upload.Multipart.UploadID)
	}

	if len(result.Upload.Multipart.Parts) == 0 {
		t.Errorf("Expected part URLs to be generated")
	}

	// Check that part numbers are sequential
	for i, part := range result.Upload.Multipart.Parts {
		expectedPartNumber := i + 1
		if part.PartNumber != expectedPartNumber {
			t.Errorf("Expected part number %d, got %d", expectedPartNumber, part.PartNumber)
		}
		if part.Method != "PUT" {
			t.Errorf("Expected PUT method for part, got %s", part.Method)
		}
	}
}