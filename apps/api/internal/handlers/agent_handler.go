package handlers

import (
	"encoding/json"
	"log"
	"time"

	"nvr-ezviz/api/internal/middleware"
	"nvr-ezviz/api/internal/models"

	"github.com/gofiber/fiber/v2"
)

type agentCamera struct {
	models.Camera
	StorageTarget *agentStorageTarget `json:"storage_target,omitempty"`
}

type agentStorageTarget struct {
	ID     string                 `json:"id"`
	Type   models.StorageType     `json:"type"`
	Config map[string]interface{} `json:"config"`
}

// AgentHeartbeat is polled periodically by each site's edge agent. It
// records liveness and hands back the current camera assignment for that
// site, including resolved storage credentials, so the agent knows what to
// record and where to upload it without any inbound connection.
func (h *Handler) AgentHeartbeat(c *fiber.Ctx) error {
	siteID, _ := c.Locals(middleware.LocalsSiteID).(string)

	now := time.Now()
	h.DB.Model(&models.Site{}).Where("id = ?", siteID).Update("last_seen_at", &now)

	var cameras []models.Camera
	if err := h.DB.Where("site_id = ?", siteID).Find(&cameras).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	result := make([]agentCamera, 0, len(cameras))
	for _, cam := range cameras {
		ac := agentCamera{Camera: cam}
		if cam.RecordingStorageTargetID != nil {
			var target models.StorageTarget
			if err := h.DB.First(&target, "id = ?", *cam.RecordingStorageTargetID).Error; err == nil {
				plain, err := h.decryptConfig(target.Config)
				if err != nil {
					log.Printf("failed to decrypt storage config for target %s: %v", target.ID, err)
				} else {
					var cfg map[string]interface{}
					_ = json.Unmarshal([]byte(plain), &cfg)
					ac.StorageTarget = &agentStorageTarget{ID: target.ID, Type: target.Type, Config: cfg}
				}
			}
		}
		result = append(result, ac)
	}

	return c.JSON(fiber.Map{
		"site_id": siteID,
		"cameras": result,
	})
}

type reportStatusRequest struct {
	CameraID string              `json:"camera_id"`
	Status   models.CameraStatus `json:"status"`
}

func (h *Handler) AgentReportCameraStatus(c *fiber.Ctx) error {
	var req reportStatusRequest
	if err := c.BodyParser(&req); err != nil || req.CameraID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "camera_id required")
	}

	var camera models.Camera
	if err := h.DB.First(&camera, "id = ?", req.CameraID).Error; err != nil {
		return fiber.NewError(fiber.StatusNotFound, "camera not found")
	}
	previousStatus := camera.Status

	if err := h.DB.Model(&models.Camera{}).Where("id = ?", req.CameraID).Update("status", req.Status).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	if previousStatus == models.CameraOnline && req.Status == models.CameraOffline {
		h.notifyForCamera(camera.ID, "camera_offline", "Kamera \""+camera.Name+"\" offline")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type reportUploadFailureRequest struct {
	CameraID string `json:"camera_id"`
	Error    string `json:"error"`
}

// AgentReportUploadFailure lets the edge agent surface a persistent upload
// problem (bad credentials, unreachable storage) as a notification instead
// of it only showing up in the agent's own local logs.
func (h *Handler) AgentReportUploadFailure(c *fiber.Ctx) error {
	var req reportUploadFailureRequest
	if err := c.BodyParser(&req); err != nil || req.CameraID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "camera_id required")
	}

	var camera models.Camera
	if err := h.DB.First(&camera, "id = ?", req.CameraID).Error; err != nil {
		return fiber.NewError(fiber.StatusNotFound, "camera not found")
	}
	h.notifyForCamera(camera.ID, "upload_failed", "Upload gagal untuk kamera \""+camera.Name+"\": "+req.Error)
	return c.SendStatus(fiber.StatusNoContent)
}

type reportRecordingRequest struct {
	CameraID        string `json:"camera_id"`
	StorageTargetID string `json:"storage_target_id"`
	ObjectKey       string `json:"object_key"`
	StartedAt       string `json:"started_at"`
	EndedAt         string `json:"ended_at"`
	SizeBytes       int64  `json:"size_bytes"`
}

// AgentReportRecording is called after the edge agent finishes uploading a
// segment, so the web app can index it without touching the bucket directly.
func (h *Handler) AgentReportRecording(c *fiber.Ctx) error {
	var req reportRecordingRequest
	if err := c.BodyParser(&req); err != nil || req.CameraID == "" || req.ObjectKey == "" {
		return fiber.NewError(fiber.StatusBadRequest, "camera_id and object_key required")
	}

	startedAt, _ := time.Parse(time.RFC3339, req.StartedAt)
	rec := models.Recording{
		CameraID:        req.CameraID,
		StorageTargetID: req.StorageTargetID,
		ObjectKey:       req.ObjectKey,
		StartedAt:       startedAt,
		SizeBytes:       req.SizeBytes,
		Status:          "uploaded",
	}
	if endedAt, err := time.Parse(time.RFC3339, req.EndedAt); err == nil {
		rec.EndedAt = &endedAt
	}
	if err := h.DB.Create(&rec).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(rec)
}
