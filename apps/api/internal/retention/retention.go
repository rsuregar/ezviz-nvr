// Package retention periodically deletes recordings older than their
// storage target's retain_days, both from the storage backend and from the
// local metadata index.
package retention

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"nvr-ezviz/api/internal/cryptoutil"
	"nvr-ezviz/api/internal/models"
	"nvr-ezviz/api/internal/storage"

	"gorm.io/gorm"
)

func Run(ctx context.Context, gdb *gorm.DB, box *cryptoutil.Box, interval time.Duration) {
	cleanup(ctx, gdb, box)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup(ctx, gdb, box)
		}
	}
}

func cleanup(ctx context.Context, gdb *gorm.DB, box *cryptoutil.Box) {
	var targets []models.StorageTarget
	if err := gdb.Find(&targets).Error; err != nil {
		log.Printf("retention: failed to list storage targets: %v", err)
		return
	}

	for _, target := range targets {
		if target.RetainDays <= 0 {
			continue
		}
		cutoff := time.Now().AddDate(0, 0, -target.RetainDays)

		var recordings []models.Recording
		if err := gdb.Where("storage_target_id = ? AND started_at < ?", target.ID, cutoff).Find(&recordings).Error; err != nil {
			log.Printf("retention: failed to list recordings for target %s: %v", target.ID, err)
			continue
		}
		if len(recordings) == 0 {
			continue
		}

		plain, err := box.Decrypt(target.Config)
		if err != nil {
			log.Printf("retention: failed to decrypt config for target %s: %v", target.ID, err)
			continue
		}
		var cfg map[string]interface{}
		_ = json.Unmarshal([]byte(plain), &cfg)

		deleter, err := storage.New(string(target.Type), cfg)
		if err != nil {
			log.Printf("retention: no deleter for target %s (%s): %v", target.ID, target.Type, err)
			continue
		}

		deleted := 0
		for _, rec := range recordings {
			if err := deleter.Delete(ctx, rec.ObjectKey); err != nil {
				log.Printf("retention: failed to delete recording %s (%s): %v", rec.ID, rec.ObjectKey, err)
				continue
			}
			if err := gdb.Delete(&models.Recording{}, "id = ?", rec.ID).Error; err != nil {
				log.Printf("retention: deleted %s from storage but failed to remove its DB row: %v", rec.ID, err)
				continue
			}
			deleted++
		}
		if deleted > 0 {
			log.Printf("retention: cleaned up %d recording(s) for target %q (retain_days=%d)", deleted, target.Name, target.RetainDays)
		}
	}
}
