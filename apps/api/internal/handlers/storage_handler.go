package handlers

import (
	"encoding/json"

	"nvr-ezviz/api/internal/models"

	"github.com/gofiber/fiber/v2"
)

func (h *Handler) ListStorageTargets(c *fiber.Ctx) error {
	workspaceID := c.Params("workspaceId")
	var targets []models.StorageTarget
	if err := h.DB.Where("workspace_id = ?", workspaceID).Find(&targets).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(targets)
}

type storageTargetRequest struct {
	Name       string                 `json:"name"`
	Type       models.StorageType     `json:"type"`
	Config     map[string]interface{} `json:"config"`
	IsDefault  bool                   `json:"is_default"`
	RetainDays int                    `json:"retain_days"`
}

func (h *Handler) CreateStorageTarget(c *fiber.Ctx) error {
	workspaceID := c.Params("workspaceId")
	var req storageTargetRequest
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	switch req.Type {
	case models.StorageS3, models.StorageMinIO, models.StorageGDrive:
	default:
		return fiber.NewError(fiber.StatusBadRequest, "type must be 's3', 'minio' or 'gdrive'")
	}
	configJSON, err := json.Marshal(req.Config)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid config")
	}
	encryptedConfig, err := h.encryptConfig(string(configJSON))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to secure storage config")
	}
	if req.RetainDays == 0 {
		req.RetainDays = 30
	}

	target := models.StorageTarget{
		WorkspaceID: workspaceID,
		Name:        req.Name,
		Type:        req.Type,
		Config:      encryptedConfig,
		IsDefault:   req.IsDefault,
		RetainDays:  req.RetainDays,
	}
	if err := h.DB.Create(&target).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "storage_target.create", "storage_target", target.ID, &workspaceID, target.Name+" ("+string(target.Type)+")")
	target.Config = "" // never echo credentials back
	return c.Status(fiber.StatusCreated).JSON(target)
}

func (h *Handler) UpdateStorageTarget(c *fiber.Ctx) error {
	id := c.Params("id")
	workspaceID := c.Params("workspaceId")
	var req storageTargetRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Config != nil {
		configJSON, err := json.Marshal(req.Config)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid config")
		}
		encryptedConfig, err := h.encryptConfig(string(configJSON))
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to secure storage config")
		}
		updates["config"] = encryptedConfig
	}
	if req.RetainDays != 0 {
		updates["retain_days"] = req.RetainDays
	}
	updates["is_default"] = req.IsDefault

	if err := h.DB.Model(&models.StorageTarget{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "storage_target.update", "storage_target", id, &workspaceID, "")
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) DeleteStorageTarget(c *fiber.Ctx) error {
	id := c.Params("id")
	workspaceID := c.Params("workspaceId")
	if err := h.DB.Delete(&models.StorageTarget{}, "id = ?", id).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "storage_target.delete", "storage_target", id, &workspaceID, "")
	return c.SendStatus(fiber.StatusNoContent)
}
