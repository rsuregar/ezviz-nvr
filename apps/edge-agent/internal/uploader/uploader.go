// Package uploader abstracts "push this local file to the workspace's
// configured storage" so the recorder doesn't need to know whether that
// storage is S3, MinIO, or (eventually) Google Drive.
package uploader

import (
	"context"
	"fmt"
)

type Uploader interface {
	// Upload sends localPath to the destination under objectKey and returns
	// the remote identifier the recording is stored under (for S3/MinIO this
	// is objectKey itself; for Google Drive it's the created file's ID,
	// since deleting a Drive file later needs its ID, not its display name)
	// plus the number of bytes written.
	Upload(ctx context.Context, localPath, objectKey string) (remoteID string, size int64, err error)
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
