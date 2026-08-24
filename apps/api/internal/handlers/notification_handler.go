package handlers

import (
	"strings"

	"nvr-ezviz/api/internal/models"

	"github.com/gofiber/fiber/v2"
)

var validNotificationEvents = map[string]bool{
	"camera_offline": true,
	"upload_failed":  true,
}

func (h *Handler) ListNotificationChannels(c *fiber.Ctx) error {
	workspaceID := c.Params("workspaceId")
	var channels []models.NotificationChannel
	if err := h.DB.Where("workspace_id = ?", workspaceID).Find(&channels).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(channels)
}

type notificationChannelRequest struct {
	Name       string   `json:"name"`
	WebhookURL string   `json:"webhook_url"`
	Events     []string `json:"events"`
}

func (h *Handler) CreateNotificationChannel(c *fiber.Ctx) error {
	workspaceID := c.Params("workspaceId")
	var req notificationChannelRequest
	if err := c.BodyParser(&req); err != nil || req.Name == "" || req.WebhookURL == "" || len(req.Events) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "name, webhook_url and at least one event are required")
	}
	for _, e := range req.Events {
		if !validNotificationEvents[e] {
			return fiber.NewError(fiber.StatusBadRequest, "unknown event \""+e+"\" (valid: camera_offline, upload_failed)")
		}
	}
	if !strings.HasPrefix(req.WebhookURL, "http://") && !strings.HasPrefix(req.WebhookURL, "https://") {
		return fiber.NewError(fiber.StatusBadRequest, "webhook_url must start with http:// or https://")
	}

	channel := models.NotificationChannel{
		WorkspaceID: workspaceID,
		Name:        req.Name,
		WebhookURL:  req.WebhookURL,
		Events:      strings.Join(req.Events, ","),
	}
	if err := h.DB.Create(&channel).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "notification_channel.create", "notification_channel", channel.ID, &workspaceID, channel.Name)
	return c.Status(fiber.StatusCreated).JSON(channel)
}

func (h *Handler) DeleteNotificationChannel(c *fiber.Ctx) error {
	id := c.Params("id")
	workspaceID := c.Params("workspaceId")
	if err := h.DB.Delete(&models.NotificationChannel{}, "id = ?", id).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "notification_channel.delete", "notification_channel", id, &workspaceID, "")
	return c.SendStatus(fiber.StatusNoContent)
}
