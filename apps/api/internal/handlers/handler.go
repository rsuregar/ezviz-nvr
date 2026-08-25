package handlers

import (
	"log"

	"nvr-ezviz/api/internal/config"
	"nvr-ezviz/api/internal/cryptoutil"
	"nvr-ezviz/api/internal/middleware"
	"nvr-ezviz/api/internal/models"
	"nvr-ezviz/api/internal/notify"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type Handler struct {
	DB     *gorm.DB
	Cfg    config.Config
	Crypto *cryptoutil.Box
}

func New(db *gorm.DB, cfg config.Config) *Handler {
	keyMaterial := cfg.StorageEncryptionKey
	if keyMaterial == "" {
		// Dev fallback so storage configs are still encrypted at rest without
		// extra setup. Set STORAGE_ENCRYPTION_KEY explicitly in production —
		// rotating JWT_SECRET would otherwise also silently break decryption
		// of every stored storage credential.
		keyMaterial = "fallback:" + cfg.JWTSecret
	}
	box, err := cryptoutil.New(keyMaterial)
	if err != nil {
		panic("failed to initialize storage encryption: " + err.Error())
	}
	return &Handler{DB: db, Cfg: cfg, Crypto: box}
}

func (h *Handler) encryptConfig(plain string) (string, error) {
	return h.Crypto.Encrypt(plain)
}

func (h *Handler) decryptConfig(encrypted string) (string, error) {
	return h.Crypto.Decrypt(encrypted)
}

// audit records an admin action for accountability. Best-effort: a logging
// failure never fails the request it's attached to.
func (h *Handler) audit(c *fiber.Ctx, action, targetType, targetID string, workspaceID *string, detail string) {
	userID, _ := c.Locals(middleware.LocalsUserID).(string)
	h.auditAs(userID, action, targetType, targetID, workspaceID, detail)
}

// auditAs is for handlers that don't run behind RequireAuth (like the
// Google OAuth callback, which Google redirects to directly with no
// Authorization header) but still know who initiated the action, e.g. via
// a signed state param.
func (h *Handler) auditAs(userID, action, targetType, targetID string, workspaceID *string, detail string) {
	var actorEmail string
	if userID != "" {
		var u models.User
		if h.DB.Select("email").First(&u, "id = ?", userID).Error == nil {
			actorEmail = u.Email
		}
	}
	entry := models.AuditLog{
		ActorUserID: userID,
		ActorEmail:  actorEmail,
		Action:      action,
		TargetType:  targetType,
		TargetID:    targetID,
		WorkspaceID: workspaceID,
		Detail:      detail,
	}
	if err := h.DB.Create(&entry).Error; err != nil {
		log.Printf("audit: failed to write entry: %v", err)
	}
}

// notifyForCamera fires event to every notification channel subscribed to
// it, across every workspace the camera belongs to.
func (h *Handler) notifyForCamera(cameraID, event, message string) {
	var workspaceIDs []string
	h.DB.Table("camera_workspaces").Where("camera_id = ?", cameraID).Pluck("workspace_id", &workspaceIDs)
	if len(workspaceIDs) == 0 {
		return
	}

	var channels []models.NotificationChannel
	h.DB.Where("workspace_id IN ?", workspaceIDs).Find(&channels)
	for _, ch := range channels {
		if notify.HasEvent(ch.Events, event) {
			notify.Send(h.notifyChannel(ch), event, message)
		}
	}
}

// notifyChannel decrypts a stored NotificationChannel's Telegram bot token
// (encrypted at rest, same as StorageTarget.Config) into the plain struct
// notify.Send needs. Decryption failing just means Telegram delivery for
// that channel silently no-ops (empty token) rather than blocking every
// other channel's notification.
func (h *Handler) notifyChannel(ch models.NotificationChannel) notify.Channel {
	botToken := ""
	if ch.TelegramBotToken != "" {
		if plain, err := h.decryptConfig(ch.TelegramBotToken); err == nil {
			botToken = plain
		} else {
			log.Printf("notify: failed to decrypt telegram bot token for channel %s: %v", ch.ID, err)
		}
	}
	return notify.Channel{
		Provider:         ch.Provider,
		WebhookURL:       ch.WebhookURL,
		TelegramBotToken: botToken,
		TelegramChatID:   ch.TelegramChatID,
	}
}
