package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"

	"mediacdn/internal/config"
	"mediacdn/internal/s3"
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

func (s *ImageService) ProcessImage(ctx context.Context, imagePath string, width, quality int) ([]byte, string, error) {
	imageData, err := s.s3Client.GetObject(ctx, imagePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch image from S3: %w", err)
	}

	img, format, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode image: %w", err)
	}

	resizedImg := imaging.Resize(img, width, 0, imaging.Lanczos)

	var buf bytes.Buffer
	var contentType string

	ext := strings.ToLower(filepath.Ext(imagePath))
	switch ext {
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
		opts := &jpeg.Options{Quality: quality}
		err = jpeg.Encode(&buf, resizedImg, opts)
	case ".png":
		contentType = "image/png"
		err = png.Encode(&buf, resizedImg)
	default:
		if format == "jpeg" {
			contentType = "image/jpeg"
			opts := &jpeg.Options{Quality: quality}
			err = jpeg.Encode(&buf, resizedImg, opts)
		} else {
			contentType = "image/png"
			err = png.Encode(&buf, resizedImg)
		}
	}

	if err != nil {
		return nil, "", fmt.Errorf("failed to encode processed image: %w", err)
	}

	return buf.Bytes(), contentType, nil
}