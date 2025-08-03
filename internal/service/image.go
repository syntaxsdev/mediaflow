package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chai2010/webp"

	"mediacdn/internal/config"
	"mediacdn/internal/s3"

	"github.com/disintegration/imaging"
)

type ImageService struct {
	s3Client *s3.Client
	config   *config.Config
}

func NewImageService(cfg *config.Config) *ImageService {
	s3Client, err := s3.NewClient(
		context.Background(),
		cfg.S3Region,
		cfg.S3Bucket,
		cfg.AWSAccessKey,
		cfg.AWSSecretKey,
	)
	if err != nil {
		panic(fmt.Sprintf("Failed to create S3 client: %v", err))
	}

	return &ImageService{
		s3Client: s3Client,
		config:   cfg,
	}
}

func (s *ImageService) UploadImage(ctx context.Context, so *config.StorageOptions, imageData []byte, thumbType, imagePath string) error {
	orig_path := fmt.Sprintf("%s/%s", so.OriginFolder, imagePath)
	convertType := so.ConvertTo
	// Upload original image
	err := s.s3Client.PutObject(ctx, orig_path, bytes.NewReader(imageData))
	if err != nil {
		return fmt.Errorf("failed to upload original image to S3: %w", err)
	}

	// Generate and upload thumbnails for each size
	for _, sizeStr := range so.Sizes {
		size, err := strconv.Atoi(sizeStr)
		if err != nil {
			return fmt.Errorf("invalid size format: %s", sizeStr)
		}

		// Generate thumbnail
		thumbnailData, err := s.generateThumbnail(imageData, size, so.Quality, convertType)
		if err != nil {
			return fmt.Errorf("failed to generate thumbnail for size %d: %w", size, err)
		}

		// Create thumbnail path with size suffix
		thumbSizePath := s.createThumbnailPathForSize(imagePath, sizeStr, convertType)
		thumbFullPath := fmt.Sprintf("%s/%s", so.ThumbFolder, thumbSizePath)

		// Upload thumbnail
		err = s.s3Client.PutObject(ctx, thumbFullPath, bytes.NewReader(thumbnailData))
		if err != nil {
			return fmt.Errorf("failed to upload thumbnail for size %d: %w", size, err)
		}
	}

	return nil
}

func (s *ImageService) generateThumbnail(imageData []byte, width, quality int, convertTo string) ([]byte, error) {
	// Decode the original image
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	resizedImg := imaging.Resize(img, width, 0, imaging.Lanczos)

	// Encode the resized image
	var buf bytes.Buffer

	// Determine content type and encode accordingly
	if strings.Contains(convertTo, "jpeg") || strings.Contains(convertTo, "jpg") {
		opts := &jpeg.Options{Quality: quality}
		err = jpeg.Encode(&buf, resizedImg, opts)
	} else if strings.Contains(convertTo, "png") {
		err = png.Encode(&buf, resizedImg)
	} else if strings.Contains(convertTo, "webp") {
		opts := &webp.Options{Quality: float32(quality)}
		err = webp.Encode(&buf, resizedImg, opts)
	} else {
		// Default to JPEG if format is unknown (fallback)
		opts := &jpeg.Options{Quality: quality}
		err = jpeg.Encode(&buf, resizedImg, opts)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to encode thumbnail: %w", err)
	}

	return buf.Bytes(), nil
}

func (s *ImageService) createThumbnailPathForSize(originalPath, size, newType string) string {
	ext := fmt.Sprintf(".%s", newType)
	if ext == "" {
		ext = filepath.Ext(originalPath)
	}
	fmt.Println("ext", ext)
	baseName := strings.TrimSuffix(originalPath, ext)
	return fmt.Sprintf("%s_%s%s", baseName, size, ext)
}

// func (s *ImageService) UploadImage(ctx context.Context, so *config.StorageOptions, image []byte, imagePath string) error {
// 	orig_path := fmt.Sprintf("%s/%s", so.OriginFolder, imagePath)
// 	thumb_path := fmt.Sprintf("%s/%s", so.ThumbFolder, imagePath)
// }

// func (s *ImageService) ProcessImage(ctx context.Context, so *config.StorageOptions, imagePath string) ([]byte, string, error) {
// 	if so == nil {
// 		so = config.DefaultStorageOptions()
// 	}
// 	orig_path := fmt.Sprintf("%s/%s", so.OriginFolder, imagePath)
// 	thumb_path := fmt.Sprintf("%s/%s", so.ThumbFolder, imagePath)

// 	imageData, err := s.s3Client.GetObject(ctx, orig_path)
// 	if err != nil {
// 		return nil, "", fmt.Errorf("failed to fetch image from S3: %w", err)
// 	}

// 	img, format, err := image.Decode(bytes.NewReader(imageData))
// 	if err != nil {
// 		return nil, "", fmt.Errorf("failed to decode image: %w", err)
// 	}

// 	resizedImg := imaging.Resize(img, width, 0, imaging.Lanczos)

// 	var buf bytes.Buffer
// 	var contentType string

// 	ext := strings.ToLower(filepath.Ext(imagePath))
// 	switch ext {
// 	case ".jpg", ".jpeg":
// 		contentType = "image/jpeg"
// 		opts := &jpeg.Options{Quality: quality}
// 		err = jpeg.Encode(&buf, resizedImg, opts)
// 	case ".png":
// 		contentType = "image/png"
// 		err = png.Encode(&buf, resizedImg)
// 	default:
// 		if format == "jpeg" {
// 			contentType = "image/jpeg"
// 			opts := &jpeg.Options{Quality: quality}
// 			err = jpeg.Encode(&buf, resizedImg, opts)
// 		} else {
// 			contentType = "image/png"
// 			err = png.Encode(&buf, resizedImg)
// 		}
// 	}

// 	if err != nil {
// 		return nil, "", fmt.Errorf("failed to encode processed image: %w", err)
// 	}

// 	return buf.Bytes(), contentType, nil
// }

func DetermineMimeType(file multipart.File) (string, error) {
	// Read the first 512 bytes to determine the MIME type
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}

	// Reset the file pointer to the beginning
	file.Seek(0, io.SeekStart)

	contentType := http.DetectContentType(buf[:n])
	return contentType, nil
}
