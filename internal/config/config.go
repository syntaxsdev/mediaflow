package config

import (
	"os"
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

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}