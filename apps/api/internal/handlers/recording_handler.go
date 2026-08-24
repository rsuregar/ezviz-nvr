package handlers

import (
	"nvr-ezviz/api/internal/models"

	"github.com/gofiber/fiber/v2"
)

// ListCameraRecordings returns recording metadata for playback UIs, newest
// first. It does not generate download URLs yet — TODO: presign against the
// recording's storage target once the storage abstraction layer lands.
func (h *Handler) ListCameraRecordings(c *fiber.Ctx) error {
	cameraID := c.Params("cameraId")
	var recordings []models.Recording
	if err := h.DB.Where("camera_id = ?", cameraID).Order("started_at DESC").Limit(500).Find(&recordings).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(recordings)
}
