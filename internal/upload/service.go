package upload

import (
	"context"
	"crypto/sha1"
	"fmt"
	"math"
	"strings"
	"time"

	"mediaflow/internal/config"
)

type Service struct {
	s3Client S3Client
	config   *config.Config
}

func NewService(s3Client S3Client, config *config.Config) *Service {
	return &Service{
		s3Client: s3Client,
		config:   config,
	}
}

// PresignUpload generates presigned URLs for upload based on the request
func (s *Service) PresignUpload(ctx context.Context, req *PresignRequest, profile *config.Profile) (*PresignResponse, error) {
	// Validate MIME type
	if !s.isMimeAllowed(req.Mime, profile.AllowedMimes) {
		return nil, fmt.Errorf("mime type not allowed: %s", req.Mime)
	}

	// Validate file size
	if req.SizeBytes > profile.SizeMaxBytes {
		return nil, fmt.Errorf("file size exceeds maximum: %d > %d", req.SizeBytes, profile.SizeMaxBytes)
	}

	// Generate shard if not provided and sharding is enabled
	shard := req.Shard
	if shard == "" && profile.EnableSharding {
		shard = GenerateShard(req.KeyBase)
	}

	// Build object key from template
	objectKey := s.buildObjectKey(profile.PathTemplate, req.KeyBase, req.Ext, shard)

	// Determine upload strategy
	strategy := s.determineStrategy(req.Multipart, req.SizeBytes, profile.MultipartThresholdMB)

	// Create required headers
	headers := s.buildRequiredHeaders(req.Mime)

	// Create presigned URLs based on strategy
	expiresAt := time.Now().Add(time.Duration(profile.TokenTTLSeconds) * time.Second)
	uploadDetails, err := s.createUploadDetails(ctx, strategy, objectKey, headers, expiresAt, profile.PartSizeMB, req.SizeBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload details: %w", err)
	}

	return &PresignResponse{
		ObjectKey: objectKey,
		Upload:    uploadDetails,
	}, nil
}

// Helper methods

func (s *Service) isMimeAllowed(mime string, allowedMimes []string) bool {
	for _, allowed := range allowedMimes {
		if mime == allowed {
			return true
		}
	}
	return false
}

func (s *Service) buildObjectKey(template, keyBase, ext, shard string) string {
	objectKey := template
	
	// Replace placeholders in template
	objectKey = strings.ReplaceAll(objectKey, "{key_base}", keyBase)
	objectKey = strings.ReplaceAll(objectKey, "{ext}", ext)
	
	// Handle optional shard
	if shard != "" {
		objectKey = strings.ReplaceAll(objectKey, "{shard?}", shard)
		objectKey = strings.ReplaceAll(objectKey, "{shard}", shard)
	} else {
		// Remove shard placeholders if no shard
		objectKey = strings.ReplaceAll(objectKey, "/{shard?}", "")
		objectKey = strings.ReplaceAll(objectKey, "{shard?}/", "")
		objectKey = strings.ReplaceAll(objectKey, "{shard?}", "")
	}
	
	return objectKey
}

func (s *Service) determineStrategy(multipart string, sizeBytes int64, thresholdMB int64) string {
	thresholdBytes := thresholdMB * 1024 * 1024
	
	switch multipart {
	case "force":
		return "multipart"
	case "off":
		return "single"
	case "auto":
		fallthrough
	default:
		if sizeBytes > thresholdBytes {
			return "multipart"
		}
		return "single"
	}
}

func (s *Service) buildRequiredHeaders(mime string) map[string]string {
	headers := map[string]string{
		"Content-Type": mime,
	}
	
	// Note: Server-side encryption disabled for MinIO compatibility
	// In production, configure proper SSE based on your storage backend
	
	return headers
}

func (s *Service) createUploadDetails(ctx context.Context, strategy, objectKey string, headers map[string]string, expiresAt time.Time, partSizeMB int64, totalSizeBytes int64) (*UploadDetails, error) {
	expires := time.Until(expiresAt)
	
	if strategy == "single" {
		// Add If-None-Match header for overwrite prevention
		singleHeaders := make(map[string]string)
		for k, v := range headers {
			singleHeaders[k] = v
		}
		singleHeaders["If-None-Match"] = "*"
		
		url, err := s.s3Client.PresignPutObject(ctx, objectKey, expires, singleHeaders)
		if err != nil {
			return nil, err
		}
		
		return &UploadDetails{
			Single: &SingleUpload{
				Method:    "PUT",
				URL:       url,
				Headers:   singleHeaders,
				ExpiresAt: expiresAt,
			},
		}, nil
	}
	
	// For multipart uploads, create the multipart upload and generate part URLs
	uploadID, err := s.s3Client.CreateMultipartUpload(ctx, objectKey, headers)
	if err != nil {
		return nil, fmt.Errorf("failed to create multipart upload: %w", err)
	}
	
	// Calculate number of parts needed
	partSizeBytes := partSizeMB * 1024 * 1024
	numParts := int(math.Ceil(float64(totalSizeBytes) / float64(partSizeBytes)))
	
	// Generate presigned URLs for each part (limit to reasonable number)
	maxParts := 100 // Reasonable limit for batch presigning
	if numParts > maxParts {
		numParts = maxParts
	}
	
	parts := make([]PartUpload, numParts)
	for i := 0; i < numParts; i++ {
		partNumber := i + 1
		partURL, err := s.s3Client.PresignUploadPart(ctx, objectKey, uploadID, int32(partNumber), expires)
		if err != nil {
			return nil, fmt.Errorf("failed to presign part %d: %w", partNumber, err)
		}
		
		parts[i] = PartUpload{
			PartNumber: partNumber,
			Method:     "PUT",
			URL:        partURL,
			Headers:    headers,
			ExpiresAt:  expiresAt,
		}
	}
	
	return &UploadDetails{
		Multipart: &MultipartUpload{
			UploadID: uploadID,
			PartSize: partSizeBytes,
			Parts:    parts,
			// Note: Complete and Abort operations aren't presignable, 
			// client must handle these via direct API calls
		},
	}, nil
}

// GenerateShard creates a shard from key_base using SHA1 hash
func GenerateShard(keyBase string) string {
	hash := sha1.Sum([]byte(keyBase))
	return fmt.Sprintf("%02x", hash[:1]) // First 2 hex characters
}