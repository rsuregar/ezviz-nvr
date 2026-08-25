// Package notify delivers alerts (camera/site offline, upload failing) to
// wherever a workspace's NotificationChannel points: a generic HTTP
// endpoint, a Slack or Discord incoming webhook, or a Telegram bot.
//
// Slack and Discord's incoming webhooks reject a payload that doesn't
// match their own expected shape ({"text": ...} and {"content": ...}
// respectively) — POSTing our own {event, message, timestamp} JSON to a
// real Slack webhook URL gets rejected outright, not silently ignored.
// Telegram has no "webhook URL" concept at all for outgoing bot messages;
// it's always the same Bot API endpoint, addressed by a bot token + chat
// ID. Send builds the right shape for each rather than assuming one
// generic POST works everywhere.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// Channel is the delivery target notify.Send needs — callers (handlers,
// sitecheck) build this from a models.NotificationChannel row, decrypting
// TelegramBotToken first since it's encrypted at rest.
type Channel struct {
	Provider         string // "generic" (default), "slack", "discord", "telegram"
	WebhookURL       string
	TelegramBotToken string
	TelegramChatID   string
}

type genericPayload struct {
	Event     string    `json:"event"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

var client = &http.Client{Timeout: 5 * time.Second}

// Send delivers in the background so callers never block a request (or
// the sitecheck/retention background loops) on a third party's latency or
// availability.
func Send(ch Channel, event, message string) {
	go func() {
		url, body, err := buildRequest(ch, event, message)
		if err != nil {
			log.Printf("notify: failed to build request for event %q (%s): %v", event, ch.Provider, err)
			return
		}
		resp, err := client.Post(url, "application/json", bytes.NewReader(body))
		if err != nil {
			log.Printf("notify: failed to deliver %q via %s: %v", event, ch.Provider, err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			log.Printf("notify: %s delivery for %q returned %d", ch.Provider, event, resp.StatusCode)
		}
	}()
}

func buildRequest(ch Channel, event, message string) (url string, body []byte, err error) {
	switch ch.Provider {
	case "slack":
		body, err = json.Marshal(map[string]string{"text": message})
		return ch.WebhookURL, body, err
	case "discord":
		body, err = json.Marshal(map[string]string{"content": message})
		return ch.WebhookURL, body, err
	case "telegram":
		if ch.TelegramBotToken == "" || ch.TelegramChatID == "" {
			return "", nil, fmt.Errorf("telegram channel missing bot token or chat id")
		}
		body, err = json.Marshal(map[string]string{"chat_id": ch.TelegramChatID, "text": message})
		return "https://api.telegram.org/bot" + ch.TelegramBotToken + "/sendMessage", body, err
	default: // "generic": custom HTTP endpoints that can read our own shape directly
		body, err = json.Marshal(genericPayload{Event: event, Message: message, Timestamp: time.Now()})
		return ch.WebhookURL, body, err
	}
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
