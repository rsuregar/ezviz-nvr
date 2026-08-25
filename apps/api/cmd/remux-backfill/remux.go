package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"nvr-ezviz/api/internal/storage"
)

// fetchRange reads remoteID from whichever backend interface store
// implements, honoring an optional Range header ("" = whole object) — the
// same pattern the recording playback handler uses, just without the HTTP
// layer around it.
func fetchRange(ctx context.Context, store storage.Deleter, remoteID string, rangeHeader string) (io.ReadCloser, error) {
	if streamer, ok := store.(storage.Streamer); ok {
		reader, _, _, _, _, err := streamer.Stream(ctx, remoteID, rangeHeader)
		return reader, err
	}
	if getter, ok := store.(storage.Getter); ok {
		url, err := getter.PresignedURL(ctx, remoteID, 10*time.Minute)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		if rangeHeader != "" {
			req.Header.Set("Range", rangeHeader)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	return nil, fmt.Errorf("storage backend doesn't support reading")
}

// isFaststart walks an MP4's top-level box structure to check whether
// "moov" (the seek index/metadata browsers need before they can play
// anything) appears before "mdat" (the actual media data, which is nearly
// the whole file) — that ordering is exactly what -movflags +faststart
// produces. Only reads the first 64KB: a non-faststart file's moov sits at
// the very end, so mdat (or, malformed/unexpected structure) is reached
// almost immediately, and a faststart file's moov is small and near the
// start too — either way this never needs the full file.
func isFaststart(ctx context.Context, store storage.Deleter, remoteID string) (bool, error) {
	reader, err := fetchRange(ctx, store, remoteID, "bytes=0-65535")
	if err != nil {
		return false, err
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return false, err
	}

	offset := 0
	for offset+8 <= len(data) {
		boxSize := binary.BigEndian.Uint32(data[offset : offset+4])
		boxType := string(data[offset+4 : offset+8])
		switch boxType {
		case "moov":
			return true, nil
		case "mdat":
			return false, nil
		}
		if boxSize < 8 {
			// Malformed, or a 64-bit "size 1" extended box (rare for these
			// short recordings) — can't safely walk further; treat as
			// needing remux rather than silently skipping it.
			return false, nil
		}
		offset += int(boxSize)
	}
	// Ran out of the 64KB window without finding either box — a faststart
	// file's ftyp+moov header is normally well under that, so treat this
	// as "needs remux" to be safe rather than assume it's fine.
	return false, nil
}

// remuxInPlace downloads remoteID in full, remuxes it locally with ffmpeg
// (stream copy, no re-encode — this only relocates the moov atom, it
// doesn't touch a single video/audio frame), and uploads the result back
// over the same object.
func remuxInPlace(ctx context.Context, store storage.Deleter, remoteID string) error {
	replacer, ok := store.(storage.Replacer)
	if !ok {
		return fmt.Errorf("storage backend doesn't support in-place replace")
	}

	reader, err := fetchRange(ctx, store, remoteID, "")
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer reader.Close()

	inFile, err := os.CreateTemp("", "nvr-remux-in-*.mp4")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(inFile.Name())
	if _, err := io.Copy(inFile, reader); err != nil {
		inFile.Close()
		return fmt.Errorf("save temp file: %w", err)
	}
	inFile.Close()

	outPath := inFile.Name() + ".out.mp4"
	defer os.Remove(outPath)

	cmd := exec.CommandContext(ctx, "ffmpeg", "-y",
		"-i", inFile.Name(),
		"-c", "copy",
		"-movflags", "+faststart",
		outPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
	}

	outFile, err := os.Open(outPath)
	if err != nil {
		return fmt.Errorf("open remuxed file: %w", err)
	}
	defer outFile.Close()
	stat, err := outFile.Stat()
	if err != nil {
		return fmt.Errorf("stat remuxed file: %w", err)
	}

	if err := replacer.Replace(ctx, remoteID, outFile, stat.Size(), "video/mp4"); err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	return nil
}
