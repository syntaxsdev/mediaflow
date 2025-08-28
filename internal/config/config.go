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
	S3Endpoint   string
	S3Bucket     string
	S3Region     string
	AWSAccessKey string
	AWSSecretKey string
	CacheMaxAge  string
	// API authentication
	APIKey       string
}

func Load() *Config {
	return &Config{
		Port:         getEnv("PORT", "8080"),
		S3Endpoint:   getEnv("S3_ENDPOINT", ""),
		S3Bucket:     getEnv("S3_BUCKET", ""),
		S3Region:     getEnv("S3_REGION", "us-east-1"),
		AWSAccessKey: getEnv("AWS_ACCESS_KEY_ID", ""),
		AWSSecretKey: getEnv("AWS_SECRET_ACCESS_KEY", ""),
		CacheMaxAge:  getEnv("CACHE_MAX_AGE", "86400"),
		// API authentication
		APIKey:       getEnv("API_KEY", ""),
	}
}

// Profile combines upload and processing configuration
type Profile struct {
	// Upload configuration
	Kind                 string   `yaml:"kind"`
	AllowedMimes         []string `yaml:"allowed_mimes"`
	SizeMaxBytes         int64    `yaml:"size_max_bytes"`
	MultipartThresholdMB int64    `yaml:"multipart_threshold_mb"`
	PartSizeMB           int64    `yaml:"part_size_mb"`
	TokenTTLSeconds      int64    `yaml:"token_ttl_seconds"`
	StoragePath          string   `yaml:"storage_path"`
	EnableSharding       bool     `yaml:"enable_sharding"`
	
	// Processing configuration (shared)
	ThumbFolder   string   `yaml:"thumb_folder,omitempty"`
	Quality       int      `yaml:"quality,omitempty"`
	CacheDuration int      `yaml:"cache_duration,omitempty"` // in seconds
	
	// Processing configuration (images)
	Sizes       []string `yaml:"sizes,omitempty"`
	DefaultSize string   `yaml:"default_size,omitempty"`
	ConvertTo   string   `yaml:"convert_to,omitempty"`
	
	// Processing configuration (videos)
	ProxyFolder string   `yaml:"proxy_folder,omitempty"`
	Formats     []string `yaml:"formats,omitempty"`
}

type StorageConfig struct {
	Profiles map[string]Profile `yaml:"profiles"`
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

	// Validate that all profiles have required storage_path field
	if err := validateStorageConfig(&storageConfig); err != nil {
		return nil, err
	}

	return &storageConfig, nil
}

// validateStorageConfig ensures all profiles have required fields
func validateStorageConfig(config *StorageConfig) error {
	for profileName, profile := range config.Profiles {
		if profile.StoragePath == "" {
			return fmt.Errorf("profile '%s' is missing required 'storage_path' field", profileName)
		}
	}
	return nil
}

// GetProfile returns a profile by name
func (sc *StorageConfig) GetProfile(profileName string) *Profile {
	if profile, exists := sc.Profiles[profileName]; exists {
		return &profile
	}

	// Return default profile if explicitly requested and exists
	if profileName == "default" {
		if defaultProfile, exists := sc.Profiles["default"]; exists {
			return &defaultProfile
		}
		// Fallback to hardcoded default
		return DefaultProfile()
	}

	// Return nil for non-existent profiles
	return nil
}


func DefaultProfile() *Profile {
	return &Profile{
		Kind:                 "image",
		AllowedMimes:         []string{"image/jpeg", "image/png"},
		SizeMaxBytes:         10485760, // 10MB
		MultipartThresholdMB: 15,
		PartSizeMB:          8,
		TokenTTLSeconds:     900,
		StoragePath:         "originals/{shard?}/{key_base}",
		EnableSharding:      true,
		ThumbFolder:         "thumbnails",
		Sizes:               []string{"256", "512", "1024"},
		Quality:             90,
	}
}


func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
