// Package recorder owns the per-camera ffmpeg recording loop and hands
// finished segments off to an uploader. One Recorder runs per edge agent
// process (one per site).
package recorder

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"nvr-ezviz/edge-agent/internal/apiclient"
	"nvr-ezviz/edge-agent/internal/config"
	"nvr-ezviz/edge-agent/internal/overlayfont"
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
	r.reconcile(ctx, hb.SiteName, hb.Cameras)
}

func (r *Recorder) reconcile(ctx context.Context, siteName string, cameras []apiclient.Camera) {
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
		go r.runCamera(workerCtx, siteName, cam)
	}

	for id, cancel := range r.workers {
		if !seen[id] {
			cancel()
			delete(r.workers, id)
		}
	}
}

func (r *Recorder) runCamera(ctx context.Context, siteName string, cam apiclient.Camera) {
	dir := filepath.Join(r.cfg.RecordDir, cam.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("camera %s: cannot create record dir: %v", cam.ID, err)
		return
	}

	go r.uploadLoop(ctx, cam, dir)
	if cam.LocalRTSPURLSub != "" {
		go r.subStreamLoop(ctx, cam)
	}
	r.ffmpegLoop(ctx, siteName, cam, dir)
}

// subStreamLoop pushes the camera's secondary (lower-resolution) stream to
// MediaMTX for live view only — no local segments, no upload, no recording,
// since storage should only ever hold one copy of a camera's footage.
func (r *Recorder) subStreamLoop(ctx context.Context, cam apiclient.Camera) {
	pushURL := r.cfg.LivePushURLSub(cam.ID)
	if pushURL == "" {
		return // MediaMTX live push isn't configured on this agent at all
	}

	backoff := 5 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}

		cmd := exec.CommandContext(ctx, "ffmpeg",
			"-rtsp_transport", "tcp",
			"-i", cam.LocalRTSPURLSub,
			"-c", "copy",
			"-f", "rtsp",
			"-rtsp_transport", "tcp",
			pushURL,
		)
		log.Printf("camera %s: starting sub-stream ffmpeg", cam.Name)
		err := cmd.Run()
		if ctx.Err() != nil {
			return
		}
		log.Printf("camera %s: sub-stream ffmpeg exited (%v), retrying in %s", cam.Name, err, backoff)

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
	}
}

// ffmpegLoop keeps ffmpeg running against the camera's RTSP feed, segmenting
// output into fixed-length mp4 files, and restarts it (with backoff) if the
// camera drops off the network.
func (r *Recorder) ffmpegLoop(ctx context.Context, siteName string, cam apiclient.Camera, dir string) {
	backoff := 5 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}

		_ = r.api.ReportCameraStatus(ctx, cam.ID, "online")
		args := r.buildFFmpegArgs(siteName, cam, dir)
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

// buildFFmpegArgs constructs the two-output ffmpeg command: the recording
// segment (with a "camera - site" label burned into the video so an
// exported/downloaded file is still self-identifying outside this app) and,
// optionally, a copy-only live push to MediaMTX.
//
// The recording branch necessarily decodes and re-encodes (drawtext can't
// operate on compressed bitstream packets) — real CPU cost, unlike the rest
// of this pipeline, which is deliberately stream-copy-only. The live push
// stays -c copy and untouched by the filter, so live view latency/CPU cost
// is unaffected regardless of how many cameras are burning in text.
func (r *Recorder) buildFFmpegArgs(siteName string, cam apiclient.Camera, dir string) []string {
	args := []string{
		"-rtsp_transport", "tcp",
		"-i", cam.LocalRTSPURL,
	}

	label := drawtextEscape(cam.Name + " - " + siteName)
	fontPath, fontErr := overlayfont.Path()
	switch {
	case !drawtextAvailable():
		// Not every ffmpeg build has this — it needs --enable-libfreetype,
		// which isn't universal (confirmed missing from this project's own
		// dev machine's Homebrew ffmpeg). Falling back to a plain stream
		// copy is essential here, not optional: attempting the filter
		// anyway would make ffmpeg exit with "No such filter", which
		// ffmpegLoop's retry loop would treat as the camera being offline
		// — recording would silently stop entirely rather than just
		// losing a cosmetic label.
		logOnce("drawtext-unsupported:"+cam.ID, func() {
			log.Printf("camera %s: ffmpeg build has no drawtext filter (needs --enable-libfreetype) — recording without camera/site label", cam.Name)
		})
		args = append(args, "-c", "copy")
	case fontErr != nil:
		// Font extraction failing is unusual (temp dir unwritable/full) —
		// same reasoning, fall back rather than block recording over a
		// cosmetic overlay.
		logOnce("drawtext-font-error:"+cam.ID, func() {
			log.Printf("camera %s: overlay font unavailable (%v), recording without label", cam.Name, fontErr)
		})
		args = append(args, "-c", "copy")
	default:
		filter := fmt.Sprintf(
			"[0:v]drawtext=fontfile='%s':text='%s':expansion=none:x=10:y=10:fontsize=22:fontcolor=white:box=1:boxcolor=black@0.5:boxborderw=6[vrec]",
			drawtextEscape(fontPath), label,
		)
		args = append(args,
			"-filter_complex", filter,
			"-map", "[vrec]",
			"-map", "0:a?",
			"-c:v", "libx264",
			"-preset", r.cfg.RecordingPreset,
			"-crf", strconv.Itoa(r.cfg.RecordingCRF),
			"-c:a", "copy",
		)
	}
	args = append(args,
		"-f", "segment",
		"-segment_time", strconv.Itoa(r.cfg.SegmentSeconds),
		"-reset_timestamps", "1",
		"-strftime", "1",
		// Kept even though it doesn't relocate moov when combined with the
		// segment muxer (confirmed by inspecting real output) — harmless,
		// and -movflags without +faststart isn't worth a special case.
		// Playback doesn't depend on this: the API's Range-request support
		// is what actually makes recordings playable regardless of moov
		// position (see README "Rekaman").
		"-movflags", "+faststart",
		filepath.Join(dir, "%Y%m%d-%H%M%S.mp4"),
	)

	// Second output, still from the same input: push an unmodified live
	// copy to the central MediaMTX relay so the dashboard can watch it, no
	// second RTSP pull from the camera and no drawtext cost.
	if pushURL := r.cfg.LivePushURL(cam.ID); pushURL != "" {
		args = append(args,
			"-map", "0:v",
			"-map", "0:a?",
			"-c", "copy",
			"-f", "rtsp",
			"-rtsp_transport", "tcp",
			pushURL,
		)
	}
	return args
}

// drawtextEscape escapes the characters that are special inside an ffmpeg
// filtergraph string (backslash, colon) and drops single quotes rather
// than deal with drawtext's awkward quote-escaping — camera/site names are
// short cosmetic labels, not worth the complexity for an edge case.
func drawtextEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case ':':
			b.WriteString(`\:`)
		case '\'':
			// dropped
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	drawtextCheckOnce   sync.Once
	drawtextIsSupported bool
)

// drawtextAvailable checks once per process whether this machine's ffmpeg
// was built with the drawtext filter (needs --enable-libfreetype, which
// isn't a given — e.g. missing from this project's own dev machine's
// Homebrew ffmpeg build). `ffmpeg -filters` lists every filter compiled
// in; checked once since it can't change while this process is running.
func drawtextAvailable() bool {
	drawtextCheckOnce.Do(func() {
		out, err := exec.Command("ffmpeg", "-hide_banner", "-filters").Output()
		if err != nil {
			return
		}
		drawtextIsSupported = strings.Contains(string(out), " drawtext ")
	})
	return drawtextIsSupported
}

var (
	logOnceMu   sync.Mutex
	logOnceSeen = map[string]bool{}
)

// logOnce runs fn (expected to log something) at most once per distinct
// key for this process's lifetime — buildFFmpegArgs runs on every ffmpeg
// restart, so without this a persistently-missing filter/font would log on
// every retry instead of once.
func logOnce(key string, fn func()) {
	logOnceMu.Lock()
	defer logOnceMu.Unlock()
	if logOnceSeen[key] {
		return
	}
	logOnceSeen[key] = true
	fn()
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
	// The strftime-named files (%Y%m%d-%H%M%S.mp4) sort chronologically; the
	// last one is still being written by ffmpeg, so only upload everything
	// before it. Since the segment muxer starts file N+1 the instant it
	// closes file N, file N+1's name doubles as file N's precise end time —
	// far more accurate than the upload-time "now" this used to report.
	finished := names[:len(names)-1]

	var lastErr error
	for i, name := range finished {
		localPath := filepath.Join(dir, name)

		remoteID, size, err := up.Upload(ctx, localPath, cam.Name, name)
		if err != nil {
			log.Printf("camera %s: upload failed for %s: %v", cam.Name, name, err)
			lastErr = err
			continue
		}

		startedAt := segmentTimestamp(name)
		endedAt := segmentTimestamp(names[i+1])
		if startedAt.IsZero() || endedAt.IsZero() {
			startedAt = time.Now().UTC()
			endedAt = startedAt
		}
		storageTargetID := ""
		if cam.StorageTarget != nil {
			storageTargetID = cam.StorageTarget.ID
		}
		if err := r.api.ReportRecording(ctx, apiclient.ReportRecordingRequest{
			CameraID:        cam.ID,
			StorageTargetID: storageTargetID,
			ObjectKey:       remoteID,
			StartedAt:       startedAt.Format(time.RFC3339),
			EndedAt:         endedAt.Format(time.RFC3339),
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

// segmentTimestamp parses a segment filename written by ffmpeg's
// "-strftime 1" option with the "%Y%m%d-%H%M%S.mp4" pattern used in
// ffmpegLoop, returned in UTC. ffmpeg's strftime formats in the local
// system time, not UTC, so that's what we parse it as here. Returns the
// zero Time if name doesn't match the expected pattern.
func segmentTimestamp(name string) time.Time {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	t, err := time.ParseInLocation("20060102-150405", base, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
