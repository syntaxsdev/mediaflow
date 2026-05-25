package upload

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"mediaflow/internal/config"
)

func newDownloadTestHandler() (*Handler, *MockS3Client) {
	mockS3 := &MockS3Client{}
	cfg := &config.Config{S3Bucket: "test-bucket"}
	svc := NewService(mockS3, cfg)
	storageConfig := &config.StorageConfig{
		Profiles: map[string]config.Profile{
			"download": {
				Kind:            "file",
				StoragePath:     "private/downloads/{key_base}",
				TokenTTLSeconds: 900,
				EnableSharding:  false,
			},
			"photo": {
				Kind:        "image",
				StoragePath: "originals/photos/{key_base}",
			},
		},
	}
	h := NewHandler(context.Background(), svc, storageConfig)
	return h, mockS3
}

func TestHandler_HandleDownloadPresign(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		query          url.Values
		headObjectErr  error
		expectedStatus int
		checkBody      func(t *testing.T, body []byte)
	}{
		{
			name:   "happy path returns 200 with object_key url expires_at",
			method: http.MethodGet,
			query: url.Values{
				"profile":  {"download"},
				"key_base": {"abc123"},
				"filename": {"my-file.zip"},
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("response not valid JSON: %v", err)
				}
				objectKey, ok := resp["object_key"].(string)
				if !ok || objectKey == "" {
					t.Errorf("expected non-empty object_key, got %v", resp["object_key"])
				}
				if objectKey != "private/downloads/abc123" {
					t.Errorf("expected object_key 'private/downloads/abc123', got %q", objectKey)
				}
				rawURL, ok := resp["url"].(string)
				if !ok || rawURL == "" {
					t.Errorf("expected non-empty url, got %v", resp["url"])
				}
				if !strings.Contains(rawURL, "response-content-disposition") {
					t.Errorf("expected url to contain content-disposition param, got %q", rawURL)
				}
				if resp["expires_at"] == nil {
					t.Errorf("expected expires_at field in response")
				}
			},
		},
		{
			name:   "happy path without filename still returns attachment disposition",
			method: http.MethodGet,
			query: url.Values{
				"profile":  {"download"},
				"key_base": {"abc123"},
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var resp map[string]any
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("response not valid JSON: %v", err)
				}
				rawURL, _ := resp["url"].(string)
				// "attachment" with no filename still gets encoded in the query param
				if !strings.Contains(rawURL, "response-content-disposition") {
					t.Errorf("expected content-disposition in url, got %q", rawURL)
				}
			},
		},
		{
			name:   "non-file profile returns 400",
			method: http.MethodGet,
			query: url.Values{
				"profile":  {"photo"},
				"key_base": {"abc123"},
			},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var resp ErrorResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("response not valid JSON: %v", err)
				}
				if resp.Code != ErrBadRequest {
					t.Errorf("expected code %q, got %q", ErrBadRequest, resp.Code)
				}
			},
		},
		{
			name:   "missing key_base returns 400",
			method: http.MethodGet,
			query: url.Values{
				"profile": {"download"},
			},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var resp ErrorResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("response not valid JSON: %v", err)
				}
				if resp.Code != ErrBadRequest {
					t.Errorf("expected code %q, got %q", ErrBadRequest, resp.Code)
				}
			},
		},
		{
			name:   "missing profile returns 400",
			method: http.MethodGet,
			query: url.Values{
				"key_base": {"abc123"},
			},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var resp ErrorResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("response not valid JSON: %v", err)
				}
				if resp.Code != ErrBadRequest {
					t.Errorf("expected code %q, got %q", ErrBadRequest, resp.Code)
				}
			},
		},
		{
			name:   "POST method returns 405",
			method: http.MethodPost,
			query: url.Values{
				"profile":  {"download"},
				"key_base": {"abc123"},
			},
			expectedStatus: http.StatusMethodNotAllowed,
			checkBody: func(t *testing.T, body []byte) {
				var resp ErrorResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("response not valid JSON: %v", err)
				}
				if resp.Code != ErrBadRequest {
					t.Errorf("expected code %q, got %q", ErrBadRequest, resp.Code)
				}
			},
		},
		{
			name:   "asset not found returns 404",
			method: http.MethodGet,
			query: url.Values{
				"profile":  {"download"},
				"key_base": {"missing-key"},
			},
			headObjectErr:  errors.New("not found"),
			expectedStatus: http.StatusNotFound,
			checkBody: func(t *testing.T, body []byte) {
				var resp ErrorResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("response not valid JSON: %v", err)
				}
				if resp.Code != ErrBadRequest {
					t.Errorf("expected code %q, got %q", ErrBadRequest, resp.Code)
				}
			},
		},
		{
			name:   "unknown profile returns 400",
			method: http.MethodGet,
			query: url.Values{
				"profile":  {"nonexistent"},
				"key_base": {"abc123"},
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, mockS3 := newDownloadTestHandler()
			if tt.headObjectErr != nil {
				headErr := tt.headObjectErr
				mockS3.headObjectFunc = func(ctx context.Context, key string) error {
					return headErr
				}
			}

			target := "/v1/downloads/presign?" + tt.query.Encode()
			req := httptest.NewRequest(tt.method, target, nil)
			rr := httptest.NewRecorder()

			h.HandleDownloadPresign(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d; body: %s", tt.expectedStatus, rr.Code, rr.Body.String())
			}

			if tt.checkBody != nil {
				tt.checkBody(t, rr.Body.Bytes())
			}
		})
	}
}

func TestHandler_HandleDownloadPresign_ExpiresAtInFuture(t *testing.T) {
	h, _ := newDownloadTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/v1/downloads/presign?profile=download&key_base=abc123&filename=test.zip", nil)
	rr := httptest.NewRecorder()
	h.HandleDownloadPresign(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	expiresStr, ok := resp["expires_at"].(string)
	if !ok {
		t.Fatalf("expires_at not a string, got %T: %v", resp["expires_at"], resp["expires_at"])
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expiresStr)
	if err != nil {
		t.Fatalf("could not parse expires_at %q: %v", expiresStr, err)
	}
	if !expiresAt.After(time.Now()) {
		t.Errorf("expires_at %v should be in the future", expiresAt)
	}
}
