package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port                   string
	PublicBaseURL          string
	InitAdminUsername      string
	InitAdminPassword      string
	SessionTTLHours        int
	DefaultProviderHost    string
	DefaultProviderAPIKey  string
	APIKeyEncryptionSecret string
	FileRetentionHours     int
	ImageBatchMax          int
	CorsOrigins            string
	DataDir                string
	StorageDir             string
}

func Load() *Config {
	publicBaseURL := strings.TrimRight(getEnv("PUBLIC_BASE_URL", "http://localhost:4000"), "/")
	// 如果 URL 没有协议前缀，自动添加 http://
	if publicBaseURL != "" && !strings.HasPrefix(publicBaseURL, "http://") && !strings.HasPrefix(publicBaseURL, "https://") {
		publicBaseURL = "http://" + publicBaseURL
	}

	return &Config{
		Port:                   getEnv("PORT", "4000"),
		PublicBaseURL:          publicBaseURL,
		InitAdminUsername:      getEnv("INIT_ADMIN_USERNAME", "admin"),
		InitAdminPassword:      getEnv("INIT_ADMIN_PASSWORD", "admin123456"),
		SessionTTLHours:        getEnvInt("SESSION_TTL_HOURS", 168),
		DefaultProviderHost:    getEnv("DEFAULT_PROVIDER_HOST", "https://grsai.dakka.com.cn"),
		DefaultProviderAPIKey:  getEnv("DEFAULT_PROVIDER_API_KEY", ""),
		APIKeyEncryptionSecret: getEnv("API_KEY_ENCRYPTION_SECRET", "PLEASE_CHANGE_THIS_SECRET_32BYTES"),
		FileRetentionHours:     getEnvInt("FILE_RETENTION_HOURS", 168),
		ImageBatchMax:          getEnvInt("IMAGE_BATCH_MAX", 12),
		CorsOrigins:            getEnv("CORS_ORIGINS", "*"),
		DataDir:                "data",
		StorageDir:             "storage",
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
