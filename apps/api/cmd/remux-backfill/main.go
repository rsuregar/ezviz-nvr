// Command remux-backfill is a one-time (or occasionally re-run) maintenance
// tool: finds recordings whose MP4 doesn't have its moov atom at the start
// (uploaded before the edge agent started using -movflags +faststart) and
// remuxes them in place, so old recordings become playable in the browser
// the same as new ones. Safe to re-run — already-fixed recordings are
// detected (by inspecting the actual file, not a database flag) and
// skipped without re-uploading.
//
// Requires ffmpeg on PATH. Usage:
//
//	cd apps/api && go run ./cmd/remux-backfill [--dry-run]
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"

	"nvr-ezviz/api/internal/config"
	"nvr-ezviz/api/internal/cryptoutil"
	"nvr-ezviz/api/internal/db"
	"nvr-ezviz/api/internal/models"
	"nvr-ezviz/api/internal/storage"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "only report which recordings need remuxing, don't change anything")
	limit := flag.Int("limit", 0, "stop after fixing this many recordings (0 = no limit) — useful to try a few before running the full batch")
	flag.Parse()

	cfg := config.Load()
	gdb, err := db.Connect(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}

	keyMaterial := cfg.StorageEncryptionKey
	if keyMaterial == "" {
		// Same fallback as internal/handlers.New — must match whatever key
		// storage configs were actually encrypted with, or decryption fails.
		keyMaterial = "fallback:" + cfg.JWTSecret
	}
	box, err := cryptoutil.New(keyMaterial)
	if err != nil {
		log.Fatalf("crypto init: %v", err)
	}

	var recordings []models.Recording
	if err := gdb.Find(&recordings).Error; err != nil {
		log.Fatalf("list recordings: %v", err)
	}
	log.Printf("checking %d recordings...", len(recordings))

	storeCache := map[string]storage.Deleter{}
	getStore := func(targetID string) (storage.Deleter, error) {
		if s, ok := storeCache[targetID]; ok {
			return s, nil
		}
		var target models.StorageTarget
		if err := gdb.First(&target, "id = ?", targetID).Error; err != nil {
			return nil, err
		}
		plain, err := box.Decrypt(target.Config)
		if err != nil {
			return nil, err
		}
		var cfgMap map[string]interface{}
		if err := json.Unmarshal([]byte(plain), &cfgMap); err != nil {
			return nil, err
		}
		s, err := storage.New(string(target.Type), cfgMap)
		if err != nil {
			return nil, err
		}
		storeCache[targetID] = s
		return s, nil
	}

	ctx := context.Background()
	fixed, skipped, failed := 0, 0, 0
	for _, rec := range recordings {
		store, err := getStore(rec.StorageTargetID)
		if err != nil {
			log.Printf("[%s] storage init failed: %v", rec.ID, err)
			failed++
			continue
		}

		alreadyOK, err := isFaststart(ctx, store, rec.ObjectKey)
		if err != nil {
			log.Printf("[%s] check failed: %v", rec.ID, err)
			failed++
			continue
		}
		if alreadyOK {
			skipped++
			continue
		}

		if *dryRun {
			log.Printf("[%s] needs remux (dry-run, not touched)", rec.ID)
			continue
		}

		if err := remuxInPlace(ctx, store, rec.ObjectKey); err != nil {
			log.Printf("[%s] remux failed: %v", rec.ID, err)
			failed++
			continue
		}
		log.Printf("[%s] fixed", rec.ID)
		fixed++
		if *limit > 0 && fixed >= *limit {
			log.Printf("hit --limit=%d, stopping", *limit)
			break
		}
	}

	log.Printf("done: %d fixed, %d already OK, %d failed", fixed, skipped, failed)
}
