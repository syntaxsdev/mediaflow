package upload

import "time"

// PresignRequest represents the request to generate presigned URLs
type PresignRequest struct {
	KeyBase   string `json:"key_base" validate:"required"`
	Ext       string `json:"ext" validate:"required"`
	Mime      string `json:"mime" validate:"required"`
	SizeBytes int64  `json:"size_bytes" validate:"required,min=1"`
	Kind      string `json:"kind" validate:"required,oneof=image video"`
	Profile   string `json:"profile" validate:"required"`
	Multipart string `json:"multipart" validate:"oneof=auto force off"`
	Shard     string `json:"shard,omitempty"`
}

// PresignResponse represents the response containing presigned URLs
type PresignResponse struct {
	ObjectKey string         `json:"object_key"`
	Upload    *UploadDetails `json:"upload"`
}

// UploadDetails contains the upload strategy details
type UploadDetails struct {
	Single    *SingleUpload    `json:"single,omitempty"`
	Multipart *MultipartUpload `json:"multipart,omitempty"`
}

// SingleUpload contains details for single PUT upload
type SingleUpload struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// MultipartUpload contains details for multipart upload
type MultipartUpload struct {
	UploadID string        `json:"upload_id"`
	PartSize int64         `json:"part_size"`
	Create   *UploadAction `json:"create"`
	Parts    []PartUpload  `json:"parts"` // Pre-generated part URLs
	Complete *UploadAction `json:"complete"`
	Abort    *UploadAction `json:"abort"`
}

// UploadAction represents an upload action (create, complete, abort)
type UploadAction struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// PartUpload represents individual part upload details
type PartUpload struct {
	PartNumber int               `json:"part_number"`
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers"`
	ExpiresAt  time.Time         `json:"expires_at"`
}


// UploadPolicy defines upload constraints for different kinds and profiles
type UploadPolicy struct {
	Kind            string   `yaml:"kind"`
	Profile         string   `yaml:"profile"`
	AllowedMimes    []string `yaml:"allowed_mimes"`
	SizeMaxBytes    int64    `yaml:"size_max_bytes"`
	MultipartThresh int64    `yaml:"multipart_threshold_bytes"`
}

// UploadConfig contains upload-related configuration
type UploadConfig struct {
	MultipartThresholdMB int64           `yaml:"multipart_threshold_mb"`
	PartSizeMB          int64           `yaml:"part_size_mb"`
	TokenTTLSeconds     int64           `yaml:"token_ttl_seconds"`
	SigningAlgorithm    string          `yaml:"signing_alg"`
	ActiveKeyID         string          `yaml:"active_kid"`
	StoragePathRaw      string          `yaml:"storage_path_raw"`
	EnableSharding      bool            `yaml:"enable_sharding"`
	Policies            []UploadPolicy  `yaml:"policies"`
}

// ErrorResponse represents error responses from the upload API
type ErrorResponse struct {
	Code              string `json:"code"`
	Message           string `json:"message"`
	Hint              string `json:"hint,omitempty"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
}

// CompleteMultipartRequest represents the request to complete a multipart upload
type CompleteMultipartRequest struct {
	Parts []CompletedPart `json:"parts" validate:"required,min=1"`
}

// CompletedPart represents a completed part with its ETag
type CompletedPart struct {
	PartNumber int    `json:"part_number" validate:"required,min=1"`
	ETag       string `json:"etag" validate:"required"`
}

// Standard error codes
const (
	ErrUnauthorized      = "unauthorized"
	ErrMimeNotAllowed    = "mime_not_allowed"
	ErrSizeTooLarge      = "size_too_large"
	ErrSignatureInvalid  = "signature_invalid"
	ErrStorageDenied     = "storage_denied"
	ErrBadRequest        = "bad_request"
	ErrRateLimited       = "rate_limited"
)