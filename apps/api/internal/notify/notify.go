// Package notify delivers webhook notifications (Slack/Discord/generic HTTP
// endpoint) for events like a camera going offline or an upload repeatedly
// failing. No SMTP/Telegram credentials required — a webhook URL is enough,
// and Slack/Discord incoming-webhook URLs accept this same plain JSON POST.
package notify

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

type payload struct {
	Event     string    `json:"event"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

var client = &http.Client{Timeout: 5 * time.Second}

// Send posts to url in the background so callers never block a request on
// a third-party webhook's latency or availability.
func Send(url, event, message string) {
	go func() {
		body, err := json.Marshal(payload{Event: event, Message: message, Timestamp: time.Now()})
		if err != nil {
			return
		}
		resp, err := client.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("notify: failed to deliver %q to %s: %v", event, url, err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			log.Printf("notify: webhook %s returned %d for event %q", url, resp.StatusCode, event)
		}
	}()
}

// HasEvent checks a channel's comma-separated event list, e.g.
// "camera_offline,upload_failed".
func HasEvent(events, event string) bool {
	for _, e := range strings.Split(events, ",") {
		if strings.TrimSpace(e) == event {
			return true
		}
	}
	return false
}
