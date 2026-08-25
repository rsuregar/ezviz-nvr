package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

func init() {
	// go run/go build don't read .env files on their own (that's a
	// docker-compose/shell feature, not a Go one) — without this, editing
	// .env and restarting `go run ./cmd/api` silently does nothing. The repo
	// root .env sits two directories up from apps/api and apps/edge-agent,
	// so try both "run from repo root" and "run from the app's own dir".
	// Missing files are fine — real env vars (Docker, CI, a real shell
	// export) always take precedence and this never overrides them.
	for _, path := range []string{".env", "../.env", "../../.env"} {
		if err := godotenv.Load(path); err == nil {
			break
		}
	}
}

type Config struct {
	ListenAddr      string
	MySQLDSN        string
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration

	// StorageEncryptionKey encrypts StorageTarget.Config (S3/MinIO keys,
	// Google Drive refresh tokens) at rest. 32 raw bytes, base64-encoded.
	StorageEncryptionKey string

	MediaMTXHLSBaseURL string

	GoogleOAuthClientID     string
	GoogleOAuthClientSecret string
	GoogleOAuthRedirectURL  string
	WebBaseURL              string

	RetentionInterval time.Duration
}

func Load() Config {
	return Config{
		ListenAddr:      getEnv("LISTEN_ADDR", ":8080"),
		MySQLDSN:        getEnv("MYSQL_DSN", "nvr:nvr@tcp(127.0.0.1:3306)/nvr?charset=utf8mb4&parseTime=True&loc=UTC"),
		JWTSecret:       getEnv("JWT_SECRET", "change-me-in-production"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,

		StorageEncryptionKey: getEnv("STORAGE_ENCRYPTION_KEY", ""),

		MediaMTXHLSBaseURL: getEnv("MEDIAMTX_HLS_BASE_URL", "http://localhost:8888"),

		GoogleOAuthClientID:     getEnv("GOOGLE_OAUTH_CLIENT_ID", ""),
		GoogleOAuthClientSecret: getEnv("GOOGLE_OAUTH_CLIENT_SECRET", ""),
		GoogleOAuthRedirectURL:  getEnv("GOOGLE_OAUTH_REDIRECT_URL", "http://localhost:8080/api/oauth/google/callback"),
		WebBaseURL:              getEnv("WEB_BASE_URL", "http://localhost:3000"),

		RetentionInterval: time.Duration(getEnvInt("RETENTION_INTERVAL_MINUTES", 60)) * time.Minute,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
