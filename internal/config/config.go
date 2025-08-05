package config

import (
	"context"
	"fmt"
	"mediaflow/internal/s3"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Port         string
	S3Bucket     string
	S3Region     string
	AWSAccessKey string
	AWSSecretKey string
	CacheMaxAge  string
}

func Load() *Config {
	return &Config{
		Port:         getEnv("PORT", "8080"),
		S3Bucket:     getEnv("S3_BUCKET", ""),
		S3Region:     getEnv("S3_REGION", "us-east-1"),
		AWSAccessKey: getEnv("AWS_ACCESS_KEY_ID", ""),
		AWSSecretKey: getEnv("AWS_SECRET_ACCESS_KEY", ""),
		CacheMaxAge:  getEnv("CACHE_MAX_AGE", "86400"),
	}
}

type StorageOptions struct {
	OriginFolder  string   `yaml:"origin_folder"`
	ThumbFolder   string   `yaml:"thumb_folder"`
	Sizes         []string `yaml:"sizes"`
	DefaultSize   string   `yaml:"default_size"`
	Quality       int      `yaml:"quality"`
	ConvertTo     string   `yaml:"convert_to"`
	CacheDuration int      `yaml:"cache_duration"` // in seconds
}

type StorageConfig struct {
	StorageOptions map[string]StorageOptions `yaml:"storage_options"`
}

func LoadStorageConfig(s3 *s3.Client, config *Config) (*StorageConfig, error) {
	configPath := getEnv("STORAGE_CONFIG_PATH", "examples/storage-config.yaml")

	var data []byte
	var err error

	// Extract S3 key from s3:// path
	if len(configPath) > 5 && configPath[:5] == "s3://" {
		s3Path := configPath[5:]
		bucket := strings.Split(s3Path, "/")[0]

		if bucket != config.S3Bucket {
			return nil, fmt.Errorf("bucket mismatch: %s != %s", bucket, config.S3Bucket)
		}

		key := strings.Join(strings.Split(s3Path, "/")[1:], "/")

		data, err = s3.GetObject(context.Background(), key)
		if err != nil {
			return nil, fmt.Errorf("failed to get storage config from S3: %w", err)
		}
		fmt.Printf("🔍 Loaded storage config from S3: %s\n", key)
	} else {
		data, err = os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read storage config: %w", err)
		}
	}

	var storageConfig StorageConfig
	if err := yaml.Unmarshal(data, &storageConfig); err != nil {
		return nil, fmt.Errorf("failed to parse storage config: %w", err)
	}

	return &storageConfig, nil
}

func (sc *StorageConfig) GetStorageOptions(imageType string) *StorageOptions {
	if options, exists := sc.StorageOptions[imageType]; exists {
		return &options
	}

	// Return default if type not found
	if defaultOptions, exists := sc.StorageOptions["default"]; exists {
		return &defaultOptions
	}

	// Fallback to hardcoded default
	return DefaultStorageOptions()
}

func DefaultStorageOptions() *StorageOptions {
	return &StorageOptions{
		OriginFolder: "originals",
		ThumbFolder:  "thumbnails",
		Sizes:        []string{"256", "512", "1024"},
		Quality:      90,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
