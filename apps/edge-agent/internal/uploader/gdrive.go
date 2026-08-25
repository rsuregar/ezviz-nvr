package uploader

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

const defaultRootFolderName = "recordings"

// GDriveUploader uploads segments into Google Drive using an OAuth2 refresh
// token, organized as <root>/<camera name>/<YYYY-MM-DD>/<file>.mp4 — root
// defaults to an auto-created/found "recordings" folder, or the storage
// target's configured folder_id if set. Getting that refresh token today is
// a manual step unless created via the dashboard's "Connect Google Drive"
// OAuth flow (apps/api/internal/handlers/oauth_handler.go).
//
// Folder lookups use the drive.file OAuth scope, which only sees files/
// folders this app itself created — so a folder_id override only works if
// that folder was created (or explicitly opened) by this app; an arbitrary
// pre-existing folder ID from elsewhere in the admin's Drive won't be
// writable under this scope.
type GDriveUploader struct {
	service          *drive.Service
	configuredRootID string

	mu          sync.Mutex
	rootID      string            // resolved lazily, empty until first upload
	dayFolderID map[string]string // "camera||YYYY-MM-DD" -> folder ID
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

	return &GDriveUploader{
		service:          svc,
		configuredRootID: folderID,
		dayFolderID:      make(map[string]string),
	}, nil
}

func (u *GDriveUploader) Upload(ctx context.Context, localPath, cameraName, fileName string) (string, int64, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", 0, err
	}

	folderID, err := u.resolveDayFolder(ctx, cameraName)
	if err != nil {
		return "", 0, fmt.Errorf("failed to resolve drive folder: %w", err)
	}

	file := &drive.File{Name: fileName, Parents: []string{folderID}}
	created, err := u.service.Files.Create(file).Media(f).Context(ctx).Do()
	if err != nil {
		return "", 0, err
	}
	// Deleting a Drive file later needs its ID, not its display name, so
	// that's what gets reported back as the recording's remote identifier.
	return created.Id, stat.Size(), nil
}

// resolveDayFolder returns (creating if needed) the folder for today's
// segments of one camera, caching each level so a full day of uploads only
// resolves it once instead of on every single segment.
func (u *GDriveUploader) resolveDayFolder(ctx context.Context, cameraName string) (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	cameraName = sanitizeFolderName(cameraName)
	day := time.Now().UTC().Format("2006-01-02")
	cacheKey := cameraName + "||" + day
	if id, ok := u.dayFolderID[cacheKey]; ok {
		return id, nil
	}

	if u.rootID == "" {
		if u.configuredRootID != "" {
			u.rootID = u.configuredRootID
		} else {
			id, err := u.findOrCreateFolder(ctx, defaultRootFolderName, "")
			if err != nil {
				return "", err
			}
			u.rootID = id
		}
	}

	cameraFolderID, err := u.findOrCreateFolder(ctx, cameraName, u.rootID)
	if err != nil {
		return "", err
	}
	dayFolderID, err := u.findOrCreateFolder(ctx, day, cameraFolderID)
	if err != nil {
		return "", err
	}

	u.dayFolderID[cacheKey] = dayFolderID
	return dayFolderID, nil
}

func (u *GDriveUploader) findOrCreateFolder(ctx context.Context, name, parentID string) (string, error) {
	query := fmt.Sprintf("name = %q and mimeType = 'application/vnd.google-apps.folder' and trashed = false", name)
	if parentID != "" {
		query += fmt.Sprintf(" and %q in parents", parentID)
	} else {
		query += " and 'root' in parents"
	}

	res, err := u.service.Files.List().Q(query).Fields("files(id)").PageSize(1).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	if len(res.Files) > 0 {
		return res.Files[0].Id, nil
	}

	folder := &drive.File{Name: name, MimeType: "application/vnd.google-apps.folder"}
	if parentID != "" {
		folder.Parents = []string{parentID}
	}
	created, err := u.service.Files.Create(folder).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	return created.Id, nil
}
