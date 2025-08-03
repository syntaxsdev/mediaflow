package config

import (
	"fmt"
	"os"

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
	Quality       int      `yaml:"quality"`
	ConvertTo     string   `yaml:"convert_to"`
	CacheDuration int      `yaml:"cache_duration"` // in seconds
}

type StorageConfig struct {
	StorageOptions map[string]StorageOptions `yaml:"storage_options"`
}

func LoadStorageConfig() (*StorageConfig, error) {
	configPath := getEnv("STORAGE_CONFIG_PATH", "storage-config.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read storage config: %w", err)
	}

	var config StorageConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse storage config: %w", err)
	}

	return &config, nil
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
