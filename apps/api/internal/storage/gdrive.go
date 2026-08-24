package storage

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
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
