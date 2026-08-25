package handlers

import (
	"strings"

	"nvr-ezviz/api/internal/models"

	"github.com/gofiber/fiber/v2"
)

var validNotificationEvents = map[string]bool{
	"camera_offline": true,
	"site_offline":   true,
	"upload_failed":  true,
}

var validNotificationProviders = map[string]bool{
	"generic":  true,
	"slack":    true,
	"discord":  true,
	"telegram": true,
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
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	WebhookURL string `json:"webhook_url"`
	// Telegram only — get a bot token from @BotFather, and a chat ID by
	// messaging the bot once then checking
	// https://api.telegram.org/bot<token>/getUpdates for "chat":{"id":...}.
	TelegramBotToken string   `json:"telegram_bot_token"`
	TelegramChatID   string   `json:"telegram_chat_id"`
	Events           []string `json:"events"`
}

func (h *Handler) CreateNotificationChannel(c *fiber.Ctx) error {
	workspaceID := c.Params("workspaceId")
	var req notificationChannelRequest
	if err := c.BodyParser(&req); err != nil || req.Name == "" || len(req.Events) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "name and at least one event are required")
	}
	for _, e := range req.Events {
		if !validNotificationEvents[e] {
			return fiber.NewError(fiber.StatusBadRequest, "unknown event \""+e+"\" (valid: camera_offline, site_offline, upload_failed)")
		}
	}

	provider := req.Provider
	if provider == "" {
		provider = "generic"
	}
	if !validNotificationProviders[provider] {
		return fiber.NewError(fiber.StatusBadRequest, "unknown provider \""+provider+"\" (valid: generic, slack, discord, telegram)")
	}

	channel := models.NotificationChannel{
		WorkspaceID:    workspaceID,
		Name:           req.Name,
		Provider:       provider,
		TelegramChatID: req.TelegramChatID,
		Events:         strings.Join(req.Events, ","),
	}

	if provider == "telegram" {
		if req.TelegramBotToken == "" || req.TelegramChatID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "telegram_bot_token and telegram_chat_id are required for provider \"telegram\"")
		}
		encrypted, err := h.encryptConfig(req.TelegramBotToken)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to store telegram bot token: "+err.Error())
		}
		channel.TelegramBotToken = encrypted
	} else {
		if req.WebhookURL == "" {
			return fiber.NewError(fiber.StatusBadRequest, "webhook_url is required for provider \""+provider+"\"")
		}
		if !strings.HasPrefix(req.WebhookURL, "http://") && !strings.HasPrefix(req.WebhookURL, "https://") {
			return fiber.NewError(fiber.StatusBadRequest, "webhook_url must start with http:// or https://")
		}
		channel.WebhookURL = req.WebhookURL
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
