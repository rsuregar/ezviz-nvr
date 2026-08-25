package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

func init() {
	// go run doesn't read .env files on its own — without this, editing
	// .env and restarting `go run ./cmd/agent` silently does nothing.
	// Missing files are fine, and real env vars always take precedence.
	for _, path := range []string{".env", "../.env", "../../.env"} {
		if err := godotenv.Load(path); err == nil {
			break
		}
	}
}

type Config struct {
	APIBaseURL     string
	AgentToken     string
	RecordDir      string
	SegmentSeconds int
	PollInterval   time.Duration

	// MediaMTXHost is the central live-view relay's RTSP address
	// (host:port). Leave empty to disable live push entirely.
	MediaMTXHost string

	// TokenFile persists the AgentToken once it's been obtained through
	// pairing (see internal/pairing), so pairing only ever has to happen
	// once per install — restarts read it back instead of re-pairing.
	// Ignored entirely when AGENT_TOKEN is set directly.
	TokenFile string
	// PairingPort is the local HTTP setup page's port, used only while no
	// token is available yet (neither AGENT_TOKEN nor TokenFile).
	PairingPort int

	// RecordingPreset/RecordingCRF tune the libx264 encode the recording
	// branch needs for burning in the "camera - site" label (drawtext
	// can't run on compressed packets, so this path can't stay -c copy).
	// veryfast/23 is a reasonable real-time-capable default on modest
	// hardware (see README hardware recommendations); slower presets trade
	// CPU for smaller files at the same quality.
	RecordingPreset string
	RecordingCRF    int
}

func Load() Config {
	return Config{
		APIBaseURL:     getEnv("API_BASE_URL", "http://localhost:8080"),
		AgentToken:     getEnv("AGENT_TOKEN", ""),
		RecordDir:      getEnv("RECORD_DIR", "./recordings"),
		SegmentSeconds: getEnvInt("SEGMENT_SECONDS", 600),
		PollInterval:   time.Duration(getEnvInt("POLL_INTERVAL_SECONDS", 30)) * time.Second,

		MediaMTXHost: getEnv("MEDIAMTX_HOST", ""),

		TokenFile:   getEnv("TOKEN_FILE", "./agent_token.json"),
		PairingPort: getEnvInt("PAIRING_PORT", 8091),

		RecordingPreset: getEnv("RECORDING_PRESET", "veryfast"),
		RecordingCRF:    getEnvInt("RECORDING_CRF", 23),
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

// LivePushURLSub is the equivalent push target for a camera's secondary
// (lower-resolution) stream — live view only, MediaMTX auth strips the
// "_sub" suffix and treats it as the same camera (see MediaMTXAuth).
func (c Config) LivePushURLSub(cameraID string) string {
	if c.MediaMTXHost == "" {
		return ""
	}
	return "rtsp://agent:" + c.AgentToken + "@" + c.MediaMTXHost + "/live/" + cameraID + "_sub"
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
