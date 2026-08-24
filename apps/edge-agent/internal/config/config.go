package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	APIBaseURL     string
	AgentToken     string
	RecordDir      string
	SegmentSeconds int
	PollInterval   time.Duration

	// MediaMTXHost is the central live-view relay's RTSP address
	// (host:port). Leave empty to disable live push entirely.
	MediaMTXHost string
}

func Load() Config {
	return Config{
		APIBaseURL:     getEnv("API_BASE_URL", "http://localhost:8080"),
		AgentToken:     getEnv("AGENT_TOKEN", ""),
		RecordDir:      getEnv("RECORD_DIR", "./recordings"),
		SegmentSeconds: getEnvInt("SEGMENT_SECONDS", 600),
		PollInterval:   time.Duration(getEnvInt("POLL_INTERVAL_SECONDS", 30)) * time.Second,

		MediaMTXHost: getEnv("MEDIAMTX_HOST", ""),
	}
}

// LivePushURL returns the RTSP publish URL for a camera, or "" if live push
// is disabled (no MEDIAMTX_HOST configured). It authenticates with this
// site's own AgentToken — the same credential already used to talk to the
// central API — instead of a separate shared live-view secret (MediaMTX
// validates it per-request against /api/mediamtx/auth).
func (c Config) LivePushURL(cameraID string) string {
	if c.MediaMTXHost == "" {
		return ""
	}
	return "rtsp://agent:" + c.AgentToken + "@" + c.MediaMTXHost + "/live/" + cameraID
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
