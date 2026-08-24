// Package storage deletes recording objects from the storage target they
// were uploaded to, for the retention/cleanup job. It intentionally only
// deletes — uploading happens on the edge agent, close to the camera, not
// here.
package storage

import (
	"context"
	"fmt"
)

type Deleter interface {
	Delete(ctx context.Context, remoteID string) error
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
