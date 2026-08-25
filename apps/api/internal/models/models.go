package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WorkspaceRole string

const (
	RoleAdmin  WorkspaceRole = "admin"
	RoleViewer WorkspaceRole = "viewer"
)

type StorageType string

const (
	StorageS3     StorageType = "s3"
	StorageMinIO  StorageType = "minio"
	StorageGDrive StorageType = "gdrive"
)

type CameraStatus string

const (
	CameraOnline  CameraStatus = "online"
	CameraOffline CameraStatus = "offline"
	CameraUnknown CameraStatus = "unknown"
)

// BaseModel uses a UUID primary key instead of GORM's default auto-increment.
type BaseModel struct {
	ID        string    `gorm:"type:char(36);primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (b *BaseModel) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.NewString()
	}
	return nil
}

// User is a login-capable account. Only superadmins can create/manage users
// (no public self-registration) per the "hanya admin yang bisa mengatur" requirement.
type User struct {
	BaseModel
	Email        string `gorm:"uniqueIndex;size:255;not null" json:"email"`
	PasswordHash string `gorm:"size:255;not null" json:"-"`
	Name         string `gorm:"size:255" json:"name"`
	IsSuperAdmin bool   `gorm:"default:false" json:"is_superadmin"`

	Memberships []UserWorkspace `json:"-"`
}

// Workspace groups cameras and users together. A camera can belong to
// several workspaces, and a user can belong to several workspaces.
type Workspace struct {
	BaseModel
	Name string `gorm:"size:255;not null" json:"name"`
	Slug string `gorm:"uniqueIndex;size:255;not null" json:"slug"`
}

// UserWorkspace is the membership join table carrying a per-workspace role.
type UserWorkspace struct {
	UserID      string        `gorm:"type:char(36);primaryKey" json:"user_id"`
	WorkspaceID string        `gorm:"type:char(36);primaryKey" json:"workspace_id"`
	Role        WorkspaceRole `gorm:"type:varchar(20);not null" json:"role"`
	CreatedAt   time.Time     `json:"created_at"`

	User      User      `json:"-"`
	Workspace Workspace `json:"-"`
}

// Site is a physical location (a building) where an edge agent runs and
// records the cameras installed there. Sites are independent of workspaces:
// a site's cameras get assigned into whichever workspaces need to see them.
type Site struct {
	BaseModel
	Name       string     `gorm:"size:255;not null" json:"name"`
	AgentToken string     `gorm:"size:255;uniqueIndex;not null" json:"-"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	// PairingCode/PairingCodeExpiresAt back a short-lived, human-typeable
	// alternative to copying AgentToken into a .env file by hand: an admin
	// generates one here, and a freshly-installed edge agent with no token
	// yet exchanges it for the real AgentToken via POST /api/agent/pair
	// (see AgentPair) through its own local setup web page — no CLI/file
	// editing on the machine at all. Single-use: cleared the moment it's
	// exchanged, regardless of whether the 15-minute expiry was reached.
	PairingCode          *string    `gorm:"size:16;index" json:"-"`
	PairingCodeExpiresAt *time.Time `json:"-"`
}

// Camera represents one EZVIZ device physically installed at a Site.
type Camera struct {
	BaseModel
	SiteID       string `gorm:"type:char(36);index;not null" json:"site_id"`
	Name         string `gorm:"size:255;not null" json:"name"`
	EzvizSerial  string `gorm:"size:64;not null" json:"ezviz_serial"`
	EzvizVerCode string `gorm:"size:64" json:"-"`
	LocalRTSPURL string `gorm:"size:512" json:"local_rtsp_url,omitempty"`
	// LocalRTSPURLSub is the camera's secondary/sub stream (lower
	// resolution, less bandwidth) — optional. When set, the edge agent
	// pushes it to MediaMTX as an extra live-view-only quality option
	// alongside the main stream; it's never recorded to storage.
	LocalRTSPURLSub string       `gorm:"size:512" json:"local_rtsp_url_sub,omitempty"`
	ChannelNo       int          `gorm:"default:1" json:"channel_no"`
	Status          CameraStatus `gorm:"type:varchar(20);default:'unknown'" json:"status"`
	// RecordingStorageTargetID picks which single StorageTarget this camera's
	// segments upload to. A camera can be visible in several workspaces, but
	// it is only physically recorded once, so storage is bound per-camera
	// (to a target owned by one of the workspaces it belongs to) rather than
	// fanned out to every workspace's storage target.
	RecordingStorageTargetID *string `gorm:"type:char(36);index" json:"recording_storage_target_id"`

	Site       Site        `json:"-"`
	Workspaces []Workspace `gorm:"many2many:camera_workspaces;" json:"-"`
}

// CameraWorkspace is the explicit join table for camera<->workspace visibility.
type CameraWorkspace struct {
	CameraID    string    `gorm:"type:char(36);primaryKey" json:"camera_id"`
	WorkspaceID string    `gorm:"type:char(36);primaryKey" json:"workspace_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// StorageTarget is a workspace-scoped destination for recordings
// (S3, MinIO, or Google Drive). Config holds provider-specific credentials,
// AES-256-GCM encrypted (see internal/cryptoutil) then base64-encoded — so
// it must be a plain text column, not "json": MySQL's json type enforces a
// json_valid() CHECK constraint that rejects encrypted (non-JSON) bytes.
type StorageTarget struct {
	BaseModel
	WorkspaceID string      `gorm:"type:char(36);index;not null" json:"workspace_id"`
	Name        string      `gorm:"size:255;not null" json:"name"`
	Type        StorageType `gorm:"type:varchar(20);not null" json:"type"`
	Config      string      `gorm:"type:text" json:"-"`
	IsDefault   bool        `gorm:"default:false" json:"is_default"`
	RetainDays  int         `gorm:"default:30" json:"retain_days"`
}

// Recording indexes one uploaded segment so the web app can browse/play it
// back without listing the object storage bucket directly.
type Recording struct {
	BaseModel
	CameraID        string     `gorm:"type:char(36);index;not null" json:"camera_id"`
	StorageTargetID string     `gorm:"type:char(36);index;not null" json:"storage_target_id"`
	ObjectKey       string     `gorm:"size:1024;not null" json:"object_key"`
	StartedAt       time.Time  `json:"started_at"`
	EndedAt         *time.Time `json:"ended_at"`
	SizeBytes       int64      `json:"size_bytes"`
	Status          string     `gorm:"type:varchar(20);default:'uploaded'" json:"status"`
}

// NotificationChannel is a workspace-scoped webhook destination for
// operational alerts (camera_offline, upload_failed). Events is a
// comma-separated list rather than a normalized table — small, fixed
// vocabulary, not worth a join for.
type NotificationChannel struct {
	BaseModel
	WorkspaceID string `gorm:"type:char(36);index;not null" json:"workspace_id"`
	Name        string `gorm:"size:255;not null" json:"name"`
	WebhookURL  string `gorm:"size:1024;not null" json:"webhook_url"`
	Events      string `gorm:"size:255;not null" json:"events"`
}

// AuditLog records who did what, for accountability on top of RBAC. Actor
// email is denormalized (copied at write time) so the trail stays readable
// even if the user account is later deleted.
type AuditLog struct {
	BaseModel
	ActorUserID string  `gorm:"type:char(36);index" json:"actor_user_id"`
	ActorEmail  string  `gorm:"size:255" json:"actor_email"`
	Action      string  `gorm:"size:64;not null" json:"action"`
	TargetType  string  `gorm:"size:64;not null" json:"target_type"`
	TargetID    string  `gorm:"size:64" json:"target_id"`
	WorkspaceID *string `gorm:"type:char(36);index" json:"workspace_id"`
	Detail      string  `gorm:"size:512" json:"detail"`
}

// RefreshToken lets us revoke long-lived sessions; only the hash is stored.
type RefreshToken struct {
	BaseModel
	UserID    string     `gorm:"type:char(36);index;not null" json:"-"`
	TokenHash string     `gorm:"size:255;uniqueIndex;not null" json:"-"`
	ExpiresAt time.Time  `json:"-"`
	RevokedAt *time.Time `json:"-"`
}

func AllModels() []interface{} {
	return []interface{}{
		&User{},
		&Workspace{},
		&UserWorkspace{},
		&Site{},
		&Camera{},
		&CameraWorkspace{},
		&StorageTarget{},
		&Recording{},
		&RefreshToken{},
		&AuditLog{},
		&NotificationChannel{},
	}
}
