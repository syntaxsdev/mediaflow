package upload

import (
	"context"
	"time"
)

// S3Client interface for dependency injection and testing
type S3Client interface {
	CreateMultipartUpload(ctx context.Context, key string, headers map[string]string) (string, error)
	PresignPutObject(ctx context.Context, key string, expires time.Duration, headers map[string]string) (string, error)
	PresignUploadPart(ctx context.Context, key, uploadID string, partNumber int32, expires time.Duration) (string, error)
}