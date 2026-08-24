package uploader

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// GDriveUploader uploads segments into a Google Drive folder using an
// OAuth2 refresh token. Getting that refresh token today is a manual step
// (Google OAuth Playground, or `gcloud auth` against a Drive-scoped client) —
// TODO: add a "Connect Google Drive" flow in the web app + API
// (/api/oauth/google/*) that runs the consent screen and stores the
// resulting refresh token as this storage target's config, instead of
// requiring an admin to paste one in by hand.
type GDriveUploader struct {
	service  *drive.Service
	folderID string
}

func NewGDriveUploader(config map[string]interface{}) (*GDriveUploader, error) {
	clientID, _ := config["client_id"].(string)
	clientSecret, _ := config["client_secret"].(string)
	refreshToken, _ := config["refresh_token"].(string)
	folderID, _ := config["folder_id"].(string)

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

	return &GDriveUploader{service: svc, folderID: folderID}, nil
}

func (u *GDriveUploader) Upload(ctx context.Context, localPath, objectKey string) (string, int64, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", 0, err
	}

	file := &drive.File{Name: objectKey}
	if u.folderID != "" {
		file.Parents = []string{u.folderID}
	}

	created, err := u.service.Files.Create(file).Media(f).Context(ctx).Do()
	if err != nil {
		return "", 0, err
	}
	// Deleting a Drive file later needs its ID, not its display name, so
	// that's what gets reported back as the recording's remote identifier.
	return created.Id, stat.Size(), nil
}
