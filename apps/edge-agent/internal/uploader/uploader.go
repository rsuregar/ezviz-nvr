// Package uploader abstracts "push this local file to the workspace's
// configured storage" so the recorder doesn't need to know whether that
// storage is S3, MinIO, or (eventually) Google Drive.
package uploader

import (
	"context"
	"fmt"
	"strings"
)

type Uploader interface {
	// Upload sends localPath (named fileName once stored) under cameraName's
	// own folder/prefix, and returns the remote identifier the recording is
	// stored under (for S3/MinIO this is the object key; for Google Drive
	// it's the created file's ID, since deleting a Drive file later needs
	// its ID, not its display name) plus the number of bytes written.
	Upload(ctx context.Context, localPath, cameraName, fileName string) (remoteID string, size int64, err error)
}

func New(storageType string, config map[string]interface{}) (Uploader, error) {
	switch storageType {
	case "s3", "minio":
		return NewS3Uploader(config)
	case "gdrive":
		return NewGDriveUploader(config)
	default:
		return nil, fmt.Errorf("unknown storage type %q", storageType)
	}
}

// sanitizeFolderName keeps a camera's display name usable as a single path
// segment (S3 prefix or Drive folder name) — mainly guarding against "/"
// turning one segment into an unintended extra level of nesting.
func sanitizeFolderName(name string) string {
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.TrimSpace(name)
	if name == "" {
		return "camera"
	}
	return name
}
