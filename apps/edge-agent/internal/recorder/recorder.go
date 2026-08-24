// Package recorder owns the per-camera ffmpeg recording loop and hands
// finished segments off to an uploader. One Recorder runs per edge agent
// process (one per site).
package recorder

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"nvr-ezviz/edge-agent/internal/apiclient"
	"nvr-ezviz/edge-agent/internal/config"
	"nvr-ezviz/edge-agent/internal/uploader"
)

type Recorder struct {
	cfg config.Config
	api *apiclient.Client

	mu      sync.Mutex
	workers map[string]context.CancelFunc // cameraID -> stop func
}

func New(cfg config.Config, api *apiclient.Client) *Recorder {
	return &Recorder{
		cfg:     cfg,
		api:     api,
		workers: make(map[string]context.CancelFunc),
	}
}

// Run polls the central API for this site's camera assignment and keeps one
// recording worker alive per camera until the assignment disappears or ctx
// is cancelled.
func (r *Recorder) Run(ctx context.Context) {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	r.poll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.poll(ctx)
		}
	}
}

func (r *Recorder) poll(ctx context.Context) {
	hb, err := r.api.Heartbeat(ctx)
	if err != nil {
		log.Printf("heartbeat failed: %v", err)
		return
	}
	r.reconcile(ctx, hb.Cameras)
}

func (r *Recorder) reconcile(ctx context.Context, cameras []apiclient.Camera) {
	r.mu.Lock()
	defer r.mu.Unlock()

	seen := make(map[string]bool, len(cameras))
	for _, cam := range cameras {
		seen[cam.ID] = true
		if _, running := r.workers[cam.ID]; running {
			continue
		}
		if cam.LocalRTSPURL == "" {
			log.Printf("camera %s (%s) has no local_rtsp_url set; EZVIZ Cloud API fallback is not implemented yet, skipping", cam.Name, cam.ID)
			continue
		}
		workerCtx, cancel := context.WithCancel(ctx)
		r.workers[cam.ID] = cancel
		go r.runCamera(workerCtx, cam)
	}

	for id, cancel := range r.workers {
		if !seen[id] {
			cancel()
			delete(r.workers, id)
		}
	}
}

func (r *Recorder) runCamera(ctx context.Context, cam apiclient.Camera) {
	dir := filepath.Join(r.cfg.RecordDir, cam.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("camera %s: cannot create record dir: %v", cam.ID, err)
		return
	}

	go r.uploadLoop(ctx, cam, dir)
	r.ffmpegLoop(ctx, cam, dir)
}

// ffmpegLoop keeps ffmpeg running against the camera's RTSP feed, segmenting
// output into fixed-length mp4 files, and restarts it (with backoff) if the
// camera drops off the network.
func (r *Recorder) ffmpegLoop(ctx context.Context, cam apiclient.Camera, dir string) {
	backoff := 5 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}

		_ = r.api.ReportCameraStatus(ctx, cam.ID, "online")
		args := []string{
			"-rtsp_transport", "tcp",
			"-i", cam.LocalRTSPURL,
			"-c", "copy",
			"-f", "segment",
			"-segment_time", strconv.Itoa(r.cfg.SegmentSeconds),
			"-reset_timestamps", "1",
			"-strftime", "1",
			filepath.Join(dir, "%Y%m%d-%H%M%S.mp4"),
		}
		// Second output on the same decoded input: push a live copy to the
		// central MediaMTX relay so the dashboard can watch it (multiview),
		// without a second RTSP pull from the camera itself.
		if pushURL := r.cfg.LivePushURL(cam.ID); pushURL != "" {
			args = append(args,
				"-c", "copy",
				"-f", "rtsp",
				"-rtsp_transport", "tcp",
				pushURL,
			)
		}
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		cmd.Stdout = nil
		cmd.Stderr = nil

		log.Printf("camera %s: starting ffmpeg", cam.Name)
		err := cmd.Run()
		if ctx.Err() != nil {
			return
		}
		_ = r.api.ReportCameraStatus(context.Background(), cam.ID, "offline")
		log.Printf("camera %s: ffmpeg exited (%v), retrying in %s", cam.Name, err, backoff)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// uploadLoop periodically scans the camera's buffer directory and uploads
// every segment except the one ffmpeg is currently writing (the most
// recently created file), then reports it to the API and removes it locally.
func (r *Recorder) uploadLoop(ctx context.Context, cam apiclient.Camera, dir string) {
	var up uploader.Uploader
	if cam.StorageTarget != nil {
		var err error
		up, err = uploader.New(cam.StorageTarget.Type, cam.StorageTarget.Config)
		if err != nil {
			log.Printf("camera %s: no usable uploader (%v); segments will only buffer locally", cam.Name, err)
		}
	} else {
		log.Printf("camera %s: no storage target bound yet; segments will only buffer locally", cam.Name)
	}

	// Debounced per-camera, in this goroutine's own closure (one goroutine
	// per camera, so no shared-state locking needed): upload retries every
	// PollInterval, so without this a broken credential would fire a
	// notification every 30s instead of once per cooldown window.
	const uploadFailureNotifyCooldown = 15 * time.Minute
	var lastFailureNotified time.Time

	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if up == nil {
				continue
			}
			if err := r.uploadFinishedSegments(ctx, cam, dir, up); err != nil {
				if time.Since(lastFailureNotified) > uploadFailureNotifyCooldown {
					lastFailureNotified = time.Now()
					_ = r.api.ReportUploadFailure(ctx, cam.ID, err.Error())
				}
			}
		}
	}
}

// uploadFinishedSegments uploads every completed segment and returns the
// most recent upload error, if any (so the caller can debounce a
// "upload failing" notification without spamming one per segment).
func (r *Recorder) uploadFinishedSegments(ctx context.Context, cam apiclient.Camera, dir string, up uploader.Uploader) error {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) < 2 {
		return nil // nothing finished yet (need at least the file after the one being written)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	// The strftime-named files sort chronologically; the last one is still
	// being written by ffmpeg, so only upload everything before it.
	finished := names[:len(names)-1]

	var lastErr error
	for _, name := range finished {
		localPath := filepath.Join(dir, name)
		objectKey := cam.ID + "/" + name

		remoteID, size, err := up.Upload(ctx, localPath, objectKey)
		if err != nil {
			log.Printf("camera %s: upload failed for %s: %v", cam.Name, name, err)
			lastErr = err
			continue
		}

		now := time.Now().UTC().Format(time.RFC3339)
		storageTargetID := ""
		if cam.StorageTarget != nil {
			storageTargetID = cam.StorageTarget.ID
		}
		if err := r.api.ReportRecording(ctx, apiclient.ReportRecordingRequest{
			CameraID:        cam.ID,
			StorageTargetID: storageTargetID,
			ObjectKey:       remoteID,
			StartedAt:       now,
			EndedAt:         now,
			SizeBytes:       size,
		}); err != nil {
			log.Printf("camera %s: failed to report recording %s: %v", cam.Name, name, err)
			lastErr = err
			continue
		}

		if err := os.Remove(localPath); err != nil {
			log.Printf("camera %s: uploaded %s but failed to clear local buffer: %v", cam.Name, name, err)
		}
	}
	return lastErr
}
