// Package storage talks to a workspace's storage target (S3, MinIO, or
// Google Drive) for everything the API itself needs to do directly:
// deleting expired recordings (retention job) and fetching a recording for
// playback. Uploading happens on the edge agent, close to the camera, not
// here.
package storage

import (
	"context"
	"fmt"
	"io"
	"time"
)

type Deleter interface {
	Delete(ctx context.Context, remoteID string) error
}

// Getter produces a time-limited URL the browser can be redirected to
// directly — no video bytes pass through our API server. S3/MinIO support
// this natively (presigned URLs); Google Drive does not implement it.
type Getter interface {
	PresignedURL(ctx context.Context, remoteID string, expiry time.Duration) (string, error)
}

// Streamer is the fallback for backends that can't produce a presigned URL
// (Google Drive): the caller reads the object through us and relays it to
// the browser, spending our own server's bandwidth to do so.
type Streamer interface {
	Stream(ctx context.Context, remoteID string) (io.ReadCloser, string, error)
}

func New(storageType string, config map[string]interface{}) (Deleter, error) {
	switch storageType {
	case "s3", "minio":
		return newS3Deleter(config)
	case "gdrive":
		return newGDriveDeleter(config)
	default:
		return nil, fmt.Errorf("unknown storage type %q", storageType)
	}
}
