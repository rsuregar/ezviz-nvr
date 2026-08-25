package storage

import (
	"context"
	"fmt"
	"io"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

type gdriveDeleter struct {
	service *drive.Service
}

func newGDriveDeleter(config map[string]interface{}) (*gdriveDeleter, error) {
	clientID, _ := config["client_id"].(string)
	clientSecret, _ := config["client_secret"].(string)
	refreshToken, _ := config["refresh_token"].(string)

	if clientID == "" || clientSecret == "" || refreshToken == "" {
		return nil, fmt.Errorf("gdrive config requires client_id, client_secret, refresh_token")
	}

	oauthConfig := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       []string{drive.DriveFileScope},
	}
	tokenSource := oauthConfig.TokenSource(context.Background(), &oauth2.Token{RefreshToken: refreshToken})

	svc, err := drive.NewService(context.Background(), option.WithTokenSource(tokenSource))
	if err != nil {
		return nil, err
	}
	return &gdriveDeleter{service: svc}, nil
}

// Delete expects remoteID to be the Drive file ID (what GDriveUploader.Upload
// returns), not a display name — Drive file names aren't unique or
// addressable the way S3 object keys are.
func (d *gdriveDeleter) Delete(ctx context.Context, remoteID string) error {
	return d.service.Files.Delete(remoteID).Context(ctx).Do()
}

// Stream implements Streamer: Drive has no equivalent of an S3 presigned
// URL (a file would have to be made "anyone with the link" to get a
// directly-fetchable URL, which isn't acceptable for private recordings),
// so playback is relayed through our own server instead using the stored
// OAuth credentials. Drive's media download endpoint honors a Range header
// like any other HTTP file server, so browser seeking is forwarded straight
// through rather than us having to fetch and slice the whole object.
func (d *gdriveDeleter) Stream(ctx context.Context, remoteID string, rangeHeader string) (io.ReadCloser, string, int64, string, int, error) {
	call := d.service.Files.Get(remoteID).Context(ctx)
	if rangeHeader != "" {
		call.Header().Set("Range", rangeHeader)
	}
	resp, err := call.Download()
	if err != nil {
		return nil, "", 0, "", 0, err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "video/mp4"
	}
	return resp.Body, contentType, resp.ContentLength, resp.Header.Get("Content-Range"), resp.StatusCode, nil
}

// Replace implements Replacer: overwrites remoteID's content in place via
// Drive's update-media call, keeping the same file ID — so the recording's
// object_key never has to change even though the bytes underneath do.
// size is unused here (S3/MinIO needs it upfront, Drive's resumable upload
// doesn't) but kept in the interface so both implementations share one
// signature.
func (d *gdriveDeleter) Replace(ctx context.Context, remoteID string, r io.Reader, size int64, contentType string) error {
	_, err := d.service.Files.Update(remoteID, &drive.File{}).
		Media(r, googleapi.ContentType(contentType)).
		Context(ctx).
		Do()
	return err
}
