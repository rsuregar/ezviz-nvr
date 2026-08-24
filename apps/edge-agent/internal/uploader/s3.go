package uploader

import (
	"context"
	"fmt"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// S3Uploader works for both AWS S3 and any S3-compatible endpoint
// (MinIO included) — same client, only the endpoint/credentials differ.
type S3Uploader struct {
	client *minio.Client
	bucket string
}

func NewS3Uploader(config map[string]interface{}) (*S3Uploader, error) {
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

	return &S3Uploader{client: client, bucket: bucket}, nil
}

func (u *S3Uploader) Upload(ctx context.Context, localPath, objectKey string) (string, int64, error) {
	info, err := u.client.FPutObject(ctx, u.bucket, objectKey, localPath, minio.PutObjectOptions{
		ContentType: "video/mp4",
	})
	if err != nil {
		return "", 0, err
	}
	return objectKey, info.Size, nil
}
