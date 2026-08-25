package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"nvr-ezviz/api/internal/config"
	"nvr-ezviz/api/internal/db"
	"nvr-ezviz/api/internal/handlers"
	"nvr-ezviz/api/internal/retention"
	"nvr-ezviz/api/internal/router"
	"nvr-ezviz/api/internal/sitecheck"
)

func main() {
	cfg := config.Load()

	gdb, err := db.Connect(cfg.MySQLDSN)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	if err := db.Migrate(gdb); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	h := handlers.New(gdb, cfg)
	app := router.New(h, gdb)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go retention.Run(ctx, gdb, h.Crypto, cfg.RetentionInterval)
	// Checked every minute -- frequent enough to notice an outage within
	// roughly one heartbeat cycle (30s) of the 90s offline threshold
	// actually being crossed, without needing its own config knob.
	go sitecheck.Run(ctx, gdb, h.Crypto, time.Minute)

	log.Printf("api listening on %s", cfg.ListenAddr)
	if err := app.Listen(cfg.ListenAddr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
