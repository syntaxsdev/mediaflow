package upload

import (
	"context"
	"crypto/sha1"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"mediaflow/internal/config"
	"mediaflow/internal/s3"
	"mediaflow/internal/stream"
)

type Service struct {
	s3Client      S3Client
	bucketClients map[string]S3Client // logical bucket name -> client
	streamClient  *stream.Client
	config        *config.Config
}

func NewService(s3Client S3Client, config *config.Config) *Service {
	return &Service{
		s3Client:      s3Client,
		bucketClients: map[string]S3Client{},
		streamClient:  stream.NewClient(config.StreamAccountID, config.StreamAPIToken),
		config:        config,
	}
}

// RegisterBucketClient binds a logical bucket name (see S3_BUCKET_<NAME>) to a
// bucket-scoped client. Profiles with that `bucket:` use it instead of default.
func (s *Service) RegisterBucketClient(logicalName string, client S3Client) {
	s.bucketClients[logicalName] = client
}

// clientForProfile returns the S3 client for a profile's bucket, falling back
// to the default bucket when the profile names none (or nil).
func (s *Service) clientForProfile(profile *config.Profile) S3Client {
	if profile == nil || profile.Bucket == "" {
		return s.s3Client
	}
	if cl, ok := s.bucketClients[profile.Bucket]; ok {
		return cl
	}
	return s.s3Client
}

// StreamClient exposes the Stream API wrapper to handlers (probe, delete).
func (s *Service) StreamClient() *stream.Client {
	return s.streamClient
}

// PresignUpload generates presigned URLs for upload based on the request
func (s *Service) PresignUpload(ctx context.Context, req *PresignRequest, profile *config.Profile, baseURL string) (*PresignResponse, error) {
	// Validate MIME type
	if !s.isMimeAllowed(req.Mime, profile.AllowedMimes) {
		return nil, fmt.Errorf("mime type not allowed: %s", req.Mime)
	}

	// Validate file size
	if req.SizeBytes > profile.SizeMaxBytes {
		return nil, fmt.Errorf("file size exceeds maximum: %d > %d", req.SizeBytes, profile.SizeMaxBytes)
	}

	if profile.Delivery == "stream" {
		return s.presignStream(ctx, req, profile)
	}

	// Generate shard only if auto-sharding is enabled
	shard := ""
	if profile.EnableSharding {
		shard = req.Shard
		if shard == "" {
			shard = GenerateShard(req.KeyBase)
		}
	}
	// Note: If EnableSharding is false, any shard in request is ignored

	// Build object key from template
	objectKey := s.buildObjectKey(profile.StoragePath, req.KeyBase, req.Ext, shard)

	// Determine upload strategy
	strategy := s.determineStrategy(req.Multipart, req.SizeBytes, profile.MultipartThresholdMB)

	// Create required headers
	headers := s.buildRequiredHeaders(req.Mime)

	// Create presigned URLs based on strategy, scoped to the profile's bucket.
	cl := s.clientForProfile(profile)
	expiresAt := time.Now().Add(time.Duration(profile.TokenTTLSeconds) * time.Second)
	uploadDetails, err := s.createUploadDetails(ctx, cl, strategy, objectKey, req.Profile, headers, expiresAt, profile.PartSizeMB, req.SizeBytes, baseURL)
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

func (s *Service) createUploadDetails(ctx context.Context, cl S3Client, strategy, objectKey, profileName string, headers map[string]string, expiresAt time.Time, partSizeMB int64, totalSizeBytes int64, baseURL string) (*UploadDetails, error) {
	expires := time.Until(expiresAt)

	if strategy == "single" {
		// Add If-None-Match header for overwrite prevention
		singleHeaders := make(map[string]string)
		for k, v := range headers {
			singleHeaders[k] = v
		}
		singleHeaders["If-None-Match"] = "*"

		url, err := cl.PresignPutObject(ctx, objectKey, expires, singleHeaders)
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
	uploadID, err := cl.CreateMultipartUpload(ctx, objectKey, headers)
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
		partURL, err := cl.PresignUploadPart(ctx, objectKey, uploadID, int32(partNumber), expires)
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

	// Generate server-side URLs for complete and abort operations
	if baseURL == "" {
		baseURL = "http://localhost:8080" // Default fallback
	}

	// Carry the profile on the complete/abort URLs so those detached calls can
	// re-resolve the target bucket (a non-default bucket would otherwise be lost).
	q := ""
	if profileName != "" {
		q = "?profile=" + url.QueryEscape(profileName)
	}
	completeURL := fmt.Sprintf("%s/v1/uploads/%s/complete/%s%s", baseURL, objectKey, uploadID, q)
	abortURL := fmt.Sprintf("%s/v1/uploads/%s/abort/%s%s", baseURL, objectKey, uploadID, q)

	return &UploadDetails{
		Multipart: &MultipartUpload{
			UploadID: uploadID,
			PartSize: partSizeBytes,
			Parts:    parts,
			Complete: &UploadAction{
				Method:    "POST",
				URL:       completeURL,
				Headers:   map[string]string{"Content-Type": "application/json"},
				ExpiresAt: expiresAt,
			},
			Abort: &UploadAction{
				Method:    "DELETE",
				URL:       abortURL,
				Headers:   map[string]string{},
				ExpiresAt: expiresAt,
			},
		},
	}, nil
}

// CompleteMultipartUpload completes a multipart upload. profile may be nil
// (default bucket); a non-nil profile routes to its configured bucket.
func (s *Service) CompleteMultipartUpload(ctx context.Context, profile *config.Profile, objectKey, uploadID string, req *CompleteMultipartRequest) error {
	// Convert request parts to s3.PartInfo
	parts := make([]s3.PartInfo, len(req.Parts))
	for i, part := range req.Parts {
		parts[i] = s3.PartInfo{
			PartNumber: part.PartNumber,
			ETag:       part.ETag,
		}
	}

	return s.clientForProfile(profile).CompleteMultipartUpload(ctx, objectKey, uploadID, parts)
}

// AbortMultipartUpload aborts a multipart upload. profile may be nil.
func (s *Service) AbortMultipartUpload(ctx context.Context, profile *config.Profile, objectKey, uploadID string) error {
	return s.clientForProfile(profile).AbortMultipartUpload(ctx, objectKey, uploadID)
}

// presignStream provisions a Cloudflare Stream Direct Creator Upload.
func (s *Service) presignStream(ctx context.Context, req *PresignRequest, profile *config.Profile) (*PresignResponse, error) {
	if !s.streamClient.Configured() {
		return nil, fmt.Errorf("stream delivery requested but STREAM_ACCOUNT_ID/STREAM_API_TOKEN not set")
	}

	maxDur := profile.MaxDurationSeconds
	if maxDur <= 0 {
		// Stream requires a positive maxDurationSeconds for direct uploads;
		// fall back to a generous cap so misconfigured profiles still work.
		maxDur = 600
	}

	result, err := s.streamClient.CreateDirectUpload(ctx, stream.DirectUploadRequest{
		MaxDurationSeconds: maxDur,
		Meta: map[string]string{
			"key_base": req.KeyBase,
			"profile":  req.Profile,
			// Drives the stream-webhook-router worker — CF echoes this
			// back on every webhook delivery for this video.
			"env": s.config.Environment,
		},
	})
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().Add(time.Duration(profile.TokenTTLSeconds) * time.Second)
	method := "POST"
	if req.SizeBytes > 200*1024*1024 {
		method = "TUS"
	}

	return &PresignResponse{
		ObjectKey: result.UID,
		Upload: &UploadDetails{
			Stream: &StreamUpload{
				Method:    method,
				URL:       result.UploadURL,
				UID:       result.UID,
				ExpiresAt: expiresAt,
			},
		},
	}, nil
}

func (s *Service) ResolveAssetKey(profile *config.Profile, keyBase string) string {
	shard := ""
	if profile.EnableSharding {
		shard = GenerateShard(keyBase)
	}
	return s.buildObjectKey(profile.StoragePath, keyBase, "", shard)
}

// PresignGet returns a presigned GET URL for an object in the profile's bucket.
func (s *Service) PresignGet(ctx context.Context, profile *config.Profile, objectKey string, ttl time.Duration) (string, error) {
	return s.clientForProfile(profile).PresignGetObject(ctx, objectKey, ttl, "")
}

// PresignDownload resolves the object key for a file-kind profile and returns a
// presigned GET URL (from the profile's bucket) that forces a download with the
// given filename.
func (s *Service) PresignDownload(ctx context.Context, profile *config.Profile, keyBase, filename string, ttl time.Duration) (string, error) {
	objectKey := s.ResolveAssetKey(profile, keyBase)
	return s.clientForProfile(profile).PresignGetObject(ctx, objectKey, ttl, contentDisposition(filename))
}

// contentDisposition builds an attachment header. The ASCII fallback is
// sanitized to prevent header injection; filename* carries the UTF-8 original.
func contentDisposition(filename string) string {
	if filename == "" {
		return "attachment"
	}
	ascii := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' || r == '\\' || r == '/' {
			return '_'
		}
		return r
	}, filename)
	return fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", ascii, url.PathEscape(filename))
}

func (s *Service) AssetExists(ctx context.Context, profile *config.Profile, objectKey string) error {
	return s.clientForProfile(profile).HeadObject(ctx, objectKey)
}

// DeleteAsset deletes an asset's original file and all generated thumbnails from
// the profile's bucket. It resolves the storage paths from the profile config,
// handling sharding if enabled.
func (s *Service) DeleteAsset(ctx context.Context, profile *config.Profile, keyBase string) (int, error) {
	cl := s.clientForProfile(profile)
	originalKey := s.ResolveAssetKey(profile, keyBase)

	deleted := 0

	// Delete the original file
	if err := cl.DeleteObject(ctx, originalKey); err != nil {
		return 0, fmt.Errorf("failed to delete original %s: %w", originalKey, err)
	}
	deleted++

	// Delete thumbnails if the profile has a thumb_folder
	if profile.ThumbFolder != "" {
		thumbPrefix := fmt.Sprintf("%s/%s", profile.ThumbFolder, keyBase)
		thumbKeys, err := cl.ListByPrefix(ctx, thumbPrefix)
		if err != nil {
			// Non-fatal: original is deleted, thumbs may not exist
			return deleted, nil
		}
		for _, key := range thumbKeys {
			if err := cl.DeleteObject(ctx, key); err == nil {
				deleted++
			}
		}
	}

	return deleted, nil
}

// GenerateShard creates a shard from key_base using SHA1 hash
func GenerateShard(keyBase string) string {
	hash := sha1.Sum([]byte(keyBase))
	return fmt.Sprintf("%02x", hash[:1]) // First 2 hex characters
}
