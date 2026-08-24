package storage

import (
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type s3Deleter struct {
	client *minio.Client
	bucket string
}

func newS3Deleter(config map[string]interface{}) (*s3Deleter, error) {
	endpoint, _ := config["endpoint"].(string)
	accessKey, _ := config["access_key"].(string)
	secretKey, _ := config["secret_key"].(string)
	bucket, _ := config["bucket"].(string)
	useSSL, _ := config["use_ssl"].(bool)

	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		return nil, fmt.Errorf("s3 config requires endpoint, access_key, secret_key, bucket")
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, err
	}
	return &s3Deleter{client: client, bucket: bucket}, nil
}

func (d *s3Deleter) Delete(ctx context.Context, remoteID string) error {
	return d.client.RemoveObject(ctx, d.bucket, remoteID, minio.RemoveObjectOptions{})
}
