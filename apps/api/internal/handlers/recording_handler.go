package handlers

import (
	"encoding/json"
	"time"

	"nvr-ezviz/api/internal/models"
	"nvr-ezviz/api/internal/storage"

	"github.com/gofiber/fiber/v2"
)

// ListCameraRecordings returns recording metadata for playback UIs, newest
// first.
func (h *Handler) ListCameraRecordings(c *fiber.Ctx) error {
	cameraID := c.Params("cameraId")
	var recordings []models.Recording
	if err := h.DB.Where("camera_id = ?", cameraID).Order("started_at DESC").Limit(500).Find(&recordings).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(recordings)
}

// StreamRecording hands a recording's video to the browser. S3/MinIO
// redirect straight to a presigned URL (no bytes touch our server); Google
// Drive has no equivalent, so we relay the bytes ourselves.
//
// This is a plain <video src="..."> target, and <video> can't attach an
// Authorization header — so like the OAuth start redirect and the MediaMTX
// auth flow, it's registered with RequireAuthFlexible (?access_token=
// query param) instead of the header-only auth used elsewhere.
func (h *Handler) StreamRecording(c *fiber.Ctx) error {
	workspaceID := c.Params("workspaceId")
	cameraID := c.Params("cameraId")
	recordingID := c.Params("recordingId")

	var camInWorkspace int64
	h.DB.Table("camera_workspaces").
		Where("camera_id = ? AND workspace_id = ?", cameraID, workspaceID).
		Count(&camInWorkspace)
	if camInWorkspace == 0 {
		return fiber.NewError(fiber.StatusForbidden, "camera is not in this workspace")
	}

	var rec models.Recording
	if err := h.DB.First(&rec, "id = ? AND camera_id = ?", recordingID, cameraID).Error; err != nil {
		return fiber.NewError(fiber.StatusNotFound, "recording not found")
	}

	var target models.StorageTarget
	if err := h.DB.First(&target, "id = ?", rec.StorageTargetID).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "storage target not found")
	}

	plain, err := h.decryptConfig(target.Config)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to decrypt storage config")
	}
	var cfg map[string]interface{}
	_ = json.Unmarshal([]byte(plain), &cfg)

	store, err := storage.New(string(target.Type), cfg)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	if getter, ok := store.(storage.Getter); ok {
		url, err := getter.PresignedURL(c.Context(), rec.ObjectKey, 10*time.Minute)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to presign recording url: "+err.Error())
		}
		return c.Redirect(url, fiber.StatusFound)
	}

	if streamer, ok := store.(storage.Streamer); ok {
		reader, contentType, err := streamer.Stream(c.Context(), rec.ObjectKey)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to fetch recording: "+err.Error())
		}
		defer reader.Close()
		c.Set("Content-Type", contentType)
		return c.SendStream(reader)
	}

	return fiber.NewError(fiber.StatusInternalServerError, "storage backend doesn't support playback")
}
