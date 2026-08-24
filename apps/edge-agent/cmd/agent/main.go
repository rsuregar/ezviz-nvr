package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"nvr-ezviz/edge-agent/internal/apiclient"
	"nvr-ezviz/edge-agent/internal/config"
	"nvr-ezviz/edge-agent/internal/recorder"
)

func main() {
	cfg := config.Load()
	if cfg.AgentToken == "" {
		log.Fatal("AGENT_TOKEN is required (issued when the site was created in the admin dashboard)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	api := apiclient.New(cfg.APIBaseURL, cfg.AgentToken)
	rec := recorder.New(cfg, api)

	log.Printf("edge agent starting, api=%s record_dir=%s", cfg.APIBaseURL, cfg.RecordDir)
	rec.Run(ctx)
	log.Println("edge agent stopped")
}
