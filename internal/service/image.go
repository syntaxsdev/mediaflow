package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/h2non/bimg.v1"

	"mediaflow/internal/config"
	"mediaflow/internal/s3"
)

type ImageService struct {
	S3Client *s3.Client
	config   *config.Config
}

func NewImageService(cfg *config.Config) *ImageService {
	s3Client, err := s3.NewClient(
		context.Background(),
		cfg.S3Region,
		cfg.S3Bucket,
		cfg.AWSAccessKey,
		cfg.AWSSecretKey,
		cfg.S3Endpoint,
	)
	if err != nil {
		panic(fmt.Sprintf("Failed to create S3 client: %v", err))
	}

	return &ImageService{
		S3Client: s3Client,
		config:   cfg,
	}
}

func (s *ImageService) UploadImage(ctx context.Context, profile *config.Profile, imageData []byte, thumbType, imagePath string) error {
	orig_path := fmt.Sprintf("%s/%s", profile.OriginFolder, imagePath)
	convertType := profile.ConvertTo
	
	// Upload original image in parallel with thumbnail generation
	origUploadChan := make(chan error, 1)
	go func() {
		err := s.S3Client.PutObject(ctx, orig_path, bytes.NewReader(imageData))
		if err != nil {
			origUploadChan <- fmt.Errorf("failed to upload original image to S3: %w", err)
		} else {
			origUploadChan <- nil
		}
	}()

	// Generate and upload thumbnails in parallel
	type thumbnailJob struct {
		sizeStr string
		data    []byte
		path    string
		err     error
	}
	
	thumbJobs := make(chan thumbnailJob, len(profile.Sizes))
	uploadErrors := make(chan error, len(profile.Sizes))
	
	// Generate thumbnails in parallel
	for _, sizeStr := range profile.Sizes {
		go func(size string) {
			sizeInt, err := strconv.Atoi(size)
			if err != nil {
				thumbJobs <- thumbnailJob{sizeStr: size, err: fmt.Errorf("invalid size format: %s", size)}
				return
			}

			thumbnailData, err := s.generateThumbnail(imageData, sizeInt, profile.Quality, convertType)
			if err != nil {
				thumbJobs <- thumbnailJob{sizeStr: size, err: fmt.Errorf("failed to generate thumbnail for size %s: %w", size, err)}
				return
			}

			thumbSizePath := s.createThumbnailPathForSize(imagePath, size, convertType)
			thumbFullPath := fmt.Sprintf("%s/%s", profile.ThumbFolder, thumbSizePath)
			
			thumbJobs <- thumbnailJob{
				sizeStr: size,
				data:    thumbnailData,
				path:    thumbFullPath,
				err:     nil,
			}
		}(sizeStr)
	}

	// Upload thumbnails in parallel as they're generated
	for i := 0; i < len(profile.Sizes); i++ {
		go func() {
			job := <-thumbJobs
			if job.err != nil {
				uploadErrors <- job.err
				return
			}
			
			err := s.S3Client.PutObject(ctx, job.path, bytes.NewReader(job.data))
			if err != nil {
				uploadErrors <- fmt.Errorf("failed to upload thumbnail for size %s: %w", job.sizeStr, err)
			} else {
				uploadErrors <- nil
			}
		}()
	}

	// Wait for original upload
	if err := <-origUploadChan; err != nil {
		return err
	}

	// Wait for all thumbnail uploads
	for i := 0; i < len(profile.Sizes); i++ {
		if err := <-uploadErrors; err != nil {
			return err
		}
	}

	return nil
}

func (s *ImageService) generateThumbnail(imageData []byte, width, quality int, convertTo string) ([]byte, error) {
	options := bimg.Options{
		Width:   width,
		Quality: quality,
	}
	
	// Set output format
	switch convertTo {
	case "webp":
		options.Type = bimg.WEBP
	case "jpeg", "jpg":
		options.Type = bimg.JPEG  
	case "png":
		options.Type = bimg.PNG
	default:
		// Default to JPEG if format is unknown (fallback)
		options.Type = bimg.JPEG
	}
	
	resizedData, err := bimg.NewImage(imageData).Process(options)
	if err != nil {
		return nil, fmt.Errorf("failed to process image with bimg: %w", err)
	}
	
	return resizedData, nil
}

func (s *ImageService) createThumbnailPathForSize(originalPath, size, newType string) string {
	ext := fmt.Sprintf(".%s", newType)
	origExt := filepath.Ext(originalPath)
	if ext == "." {
		ext = origExt
	}
	baseName := strings.TrimSuffix(originalPath, origExt)
	return fmt.Sprintf("%s_%s%s", baseName, size, ext)
}

// GetImage gets the image from the S3 bucket
func (s *ImageService) GetImage(ctx context.Context, profile *config.Profile, original bool, baseImageName, size string) ([]byte, error) {
	var path string
	if original {
		path = fmt.Sprintf("%s/%s", profile.OriginFolder, baseImageName)
	} else {
		if size == "" && !original {
			if profile.DefaultSize == "" {
				return nil, fmt.Errorf("please specify a size, as `default_size` is not set for this configuration")
			}
			size = profile.DefaultSize
		}
		// example -> folder/file_size.ext
		path = fmt.Sprintf("%s/%s_%s.%s", profile.ThumbFolder, baseImageName, size, profile.ConvertTo)
	}

	imageData, err := s.S3Client.GetObject(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to get image from S3: %w", err)
	}

	return imageData, nil
}

// Read the first 512 bytes to determine the MIME type
func DetermineMimeType(file multipart.File) (string, error) {
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}

	// Reset the file pointer to the beginning
	_, _ = file.Seek(0, io.SeekStart)

	contentType := http.DetectContentType(buf[:n])
	return contentType, nil
}
