package s3

import (
	"context"
	"fmt"
	"io"
	utils "mediaflow/internal"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Client struct {
	s3Client   *s3.Client
	bucket     string
	presigner  *s3.PresignClient
}

func NewClient(ctx context.Context, region, bucket, accessKey, secretKey, endpoint string) (*Client, error) {
	var cfg aws.Config
	var err error

	if accessKey != "" && secretKey != "" {
		cfg, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		)
	} else if os.Getenv("ECS_CONTAINER_METADATA_URI_V4") != "" {
		cfg, err = config.LoadDefaultConfig(ctx)
	} else {
		err = fmt.Errorf("no AWS credentials provided")
	}

	if err != nil {
		utils.Shutdown(fmt.Sprintf("🚨 Failed to load AWS config: %v", err))
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		}
	})

	presigner := s3.NewPresignClient(s3Client)

	return &Client{
		s3Client:  s3Client,
		bucket:    bucket,
		presigner: presigner,
	}, nil
}

func (c *Client) GetObject(ctx context.Context, key string) ([]byte, error) {
	result, err := c.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer result.Body.Close()

	return io.ReadAll(result.Body)
}

func (c *Client) PutObject(ctx context.Context, key string, body io.Reader) error {
	_, err := c.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
		Body:   body,
	})
	return err
}

// PresignPutObject generates a presigned URL for PUT operations
func (c *Client) PresignPutObject(ctx context.Context, key string, expires time.Duration, headers map[string]string) (string, error) {
	input := &s3.PutObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}

	// Add required headers to the input
	if contentType, ok := headers["Content-Type"]; ok {
		input.ContentType = aws.String(contentType)
	}
	// Note: SSE removed for MinIO compatibility

	request, err := c.presigner.PresignPutObject(ctx, input, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return "", err
	}

	return request.URL, nil
}

// CreateMultipartUpload creates a multipart upload and returns the upload ID
func (c *Client) CreateMultipartUpload(ctx context.Context, key string, headers map[string]string) (string, error) {
	input := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}

	// Add required headers
	if contentType, ok := headers["Content-Type"]; ok {
		input.ContentType = aws.String(contentType)
	}
	// Note: SSE removed for MinIO compatibility

	result, err := c.s3Client.CreateMultipartUpload(ctx, input)
	if err != nil {
		return "", err
	}

	return *result.UploadId, nil
}

// PresignUploadPart generates a presigned URL for uploading a part
func (c *Client) PresignUploadPart(ctx context.Context, key, uploadID string, partNumber int32, expires time.Duration) (string, error) {
	input := &s3.UploadPartInput{
		Bucket:     aws.String(c.bucket),
		Key:        aws.String(key),
		UploadId:   aws.String(uploadID),
		PartNumber: aws.Int32(partNumber),
	}

	request, err := c.presigner.PresignUploadPart(ctx, input, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return "", err
	}

	return request.URL, nil
}

// CompleteMultipartUpload completes a multipart upload
func (c *Client) CompleteMultipartUpload(ctx context.Context, key, uploadID string, parts []PartInfo) error {
	completedParts := make([]s3Types.CompletedPart, len(parts))
	for i, part := range parts {
		completedParts[i] = s3Types.CompletedPart{
			ETag:       aws.String(part.ETag),
			PartNumber: aws.Int32(int32(part.PartNumber)),
		}
	}

	input := &s3.CompleteMultipartUploadInput{
		Bucket:   aws.String(c.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
		MultipartUpload: &s3Types.CompletedMultipartUpload{
			Parts: completedParts,
		},
	}

	_, err := c.s3Client.CompleteMultipartUpload(ctx, input)
	return err
}

// AbortMultipartUpload aborts a multipart upload
func (c *Client) AbortMultipartUpload(ctx context.Context, key, uploadID string) error {
	input := &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(c.bucket),
		Key:      aws.String(key),
		UploadId: aws.String(uploadID),
	}

	_, err := c.s3Client.AbortMultipartUpload(ctx, input)
	return err
}

// PartInfo represents a completed part for multipart upload
type PartInfo struct {
	ETag       string
	PartNumber int
}
