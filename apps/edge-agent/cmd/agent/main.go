package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"nvr-ezviz/edge-agent/internal/apiclient"
	"nvr-ezviz/edge-agent/internal/config"
	"nvr-ezviz/edge-agent/internal/pairing"
	"nvr-ezviz/edge-agent/internal/recorder"
)

func main() {
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// No AGENT_TOKEN set and nothing paired yet from a previous run: block
	// here serving a local setup page until someone pairs it, instead of
	// refusing to start. See internal/pairing for the full flow.
	token, err := pairing.LoadOrPair(ctx, cfg.APIBaseURL, cfg.AgentToken, cfg.TokenFile, cfg.PairingPort)
	if err != nil {
		log.Fatalf("failed to obtain agent token: %v", err)
	}
	cfg.AgentToken = token

	api := apiclient.New(cfg.APIBaseURL, cfg.AgentToken)
	rec := recorder.New(cfg, api)

	log.Printf("edge agent starting, api=%s record_dir=%s", cfg.APIBaseURL, cfg.RecordDir)
	rec.Run(ctx)
	log.Println("edge agent stopped")
}
