// Package sitecheck periodically looks for sites whose edge agent has
// stopped heartbeating and fires a "site_offline" webhook notification the
// first time each outage is noticed — so an admin doesn't have to be
// staring at the dashboard to find out a whole location went dark, which
// also means every camera at that site now has a stale, untrustworthy
// status (see camera_handler.go's SiteOnline field on ListWorkspaceCameras
// / ListAllCameras).
package sitecheck

import (
	"context"
	"fmt"
	"log"
	"time"

	"nvr-ezviz/api/internal/cryptoutil"
	"nvr-ezviz/api/internal/models"
	"nvr-ezviz/api/internal/notify"

	"gorm.io/gorm"
)

// onlineThreshold matches the same 3x-poll-interval heuristic used in
// internal/handlers/camera_handler.go and the dashboard's Health tab —
// kept as an independent constant (all three already happen to agree on
// this number) rather than a shared import, to not couple this package to
// the HTTP handlers package for one value.
const onlineThreshold = 90 * time.Second

func Run(ctx context.Context, gdb *gorm.DB, box *cryptoutil.Box, interval time.Duration) {
	check(gdb, box)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check(gdb, box)
		}
	}
}

func check(gdb *gorm.DB, box *cryptoutil.Box) {
	var sites []models.Site
	if err := gdb.Find(&sites).Error; err != nil {
		log.Printf("sitecheck: failed to list sites: %v", err)
		return
	}

	for _, s := range sites {
		online := s.LastSeenAt != nil && time.Since(*s.LastSeenAt) < onlineThreshold
		switch {
		case !online && s.OfflineNotifiedAt == nil:
			msg := fmt.Sprintf(
				"Site %q sedang offline — mohon periksa apakah PC/edge agent di lokasi tersebut menyala dan terhubung ke internet.",
				s.Name,
			)
			notifyForSite(gdb, box, s.ID, "site_offline", msg)
			now := time.Now()
			gdb.Model(&models.Site{}).Where("id = ?", s.ID).Update("offline_notified_at", &now)
		case online && s.OfflineNotifiedAt != nil:
			gdb.Model(&models.Site{}).Where("id = ?", s.ID).Update("offline_notified_at", nil)
		}
	}
}

// notifyForSite mirrors internal/handlers.notifyForCamera's fan-out logic
// (every notification channel across every workspace touched), but keyed
// by site instead of a single camera, since an outage there affects every
// camera at that location at once.
func notifyForSite(gdb *gorm.DB, box *cryptoutil.Box, siteID, event, message string) {
	var cameraIDs []string
	gdb.Model(&models.Camera{}).Where("site_id = ?", siteID).Pluck("id", &cameraIDs)
	if len(cameraIDs) == 0 {
		return
	}

	var workspaceIDs []string
	gdb.Table("camera_workspaces").Where("camera_id IN ?", cameraIDs).Distinct().Pluck("workspace_id", &workspaceIDs)
	if len(workspaceIDs) == 0 {
		return
	}

	var channels []models.NotificationChannel
	gdb.Where("workspace_id IN ?", workspaceIDs).Find(&channels)
	for _, ch := range channels {
		if !notify.HasEvent(ch.Events, event) {
			continue
		}
		botToken := ""
		if ch.TelegramBotToken != "" {
			if plain, err := box.Decrypt(ch.TelegramBotToken); err == nil {
				botToken = plain
			} else {
				log.Printf("sitecheck: failed to decrypt telegram bot token for channel %s: %v", ch.ID, err)
			}
		}
		notify.Send(notify.Channel{
			Provider:         ch.Provider,
			WebhookURL:       ch.WebhookURL,
			TelegramBotToken: botToken,
			TelegramChatID:   ch.TelegramChatID,
		}, event, message)
	}
}
