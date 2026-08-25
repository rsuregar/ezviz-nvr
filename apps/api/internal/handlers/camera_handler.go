package handlers

import (
	"nvr-ezviz/api/internal/models"

	"github.com/gofiber/fiber/v2"
)

// ListWorkspaceCameras returns every camera visible in a workspace
// (i.e. assigned to it via camera_workspaces), regardless of which site
// it physically lives at — including that site's name, so the dashboard
// can group/filter cameras by location without a second (superadmin-only)
// call to /api/sites.
func (h *Handler) ListWorkspaceCameras(c *fiber.Ctx) error {
	workspaceID := c.Params("workspaceId")
	var cameras []cameraWithSite
	if err := h.DB.Table("cameras").
		Select("cameras.*, sites.name as site_name").
		Joins("JOIN camera_workspaces cw ON cw.camera_id = cameras.id").
		Joins("JOIN sites ON sites.id = cameras.site_id").
		Where("cw.workspace_id = ?", workspaceID).Find(&cameras).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(cameras)
}

func (h *Handler) ListSiteCameras(c *fiber.Ctx) error {
	siteID := c.Params("siteId")
	var cameras []models.Camera
	if err := h.DB.Where("site_id = ?", siteID).Find(&cameras).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(cameras)
}

type cameraWithSite struct {
	models.Camera
	SiteName string `json:"site_name"`
}

// ListAllCameras backs the workspace "assign camera" picker in the
// dashboard — it needs every camera across every site (with its site name
// for context) so an admin can find one by name instead of pasting an ID.
func (h *Handler) ListAllCameras(c *fiber.Ctx) error {
	var cameras []models.Camera
	if err := h.DB.Find(&cameras).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	var sites []models.Site
	if err := h.DB.Find(&sites).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	siteNames := make(map[string]string, len(sites))
	for _, s := range sites {
		siteNames[s.ID] = s.Name
	}

	result := make([]cameraWithSite, 0, len(cameras))
	for _, cam := range cameras {
		result = append(result, cameraWithSite{Camera: cam, SiteName: siteNames[cam.SiteID]})
	}
	return c.JSON(result)
}

type createCameraRequest struct {
	Name            string `json:"name"`
	EzvizSerial     string `json:"ezviz_serial"`
	EzvizVerCode    string `json:"ezviz_verification_code"`
	LocalRTSPURL    string `json:"local_rtsp_url"`
	LocalRTSPURLSub string `json:"local_rtsp_url_sub"`
	ChannelNo       int    `json:"channel_no"`
}

func (h *Handler) CreateCamera(c *fiber.Ctx) error {
	siteID := c.Params("siteId")
	var req createCameraRequest
	// ezviz_serial/verification_code are reference-only metadata for now —
	// nothing in the RTSP-only recording pipeline reads them back (they'd
	// matter for EZVIZ Cloud API integration, which this deployment skips).
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name required")
	}
	if req.ChannelNo == 0 {
		req.ChannelNo = 1
	}
	camera := models.Camera{
		SiteID:          siteID,
		Name:            req.Name,
		EzvizSerial:     req.EzvizSerial,
		EzvizVerCode:    req.EzvizVerCode,
		LocalRTSPURL:    req.LocalRTSPURL,
		LocalRTSPURLSub: req.LocalRTSPURLSub,
		ChannelNo:       req.ChannelNo,
		Status:          models.CameraUnknown,
	}
	if err := h.DB.Create(&camera).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "camera.create", "camera", camera.ID, nil, camera.Name)
	return c.Status(fiber.StatusCreated).JSON(camera)
}

func (h *Handler) UpdateCamera(c *fiber.Ctx) error {
	id := c.Params("id")
	var req createCameraRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.LocalRTSPURL != "" {
		updates["local_rtsp_url"] = req.LocalRTSPURL
	}
	if req.LocalRTSPURLSub != "" {
		updates["local_rtsp_url_sub"] = req.LocalRTSPURLSub
	}
	if req.ChannelNo != 0 {
		updates["channel_no"] = req.ChannelNo
	}
	if err := h.DB.Model(&models.Camera{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) DeleteCamera(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.DB.Delete(&models.Camera{}, "id = ?", id).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "camera.delete", "camera", id, nil, "")
	return c.SendStatus(fiber.StatusNoContent)
}

type setCameraStorageRequest struct {
	StorageTargetID string `json:"storage_target_id"`
}

// SetCameraStorageTarget binds a camera to one storage target owned by the
// current workspace. It refuses to bind a target that belongs to a
// different workspace, since credentials should stay scoped to their owner.
func (h *Handler) SetCameraStorageTarget(c *fiber.Ctx) error {
	workspaceID := c.Params("workspaceId")
	cameraID := c.Params("id")
	var req setCameraStorageRequest
	if err := c.BodyParser(&req); err != nil || req.StorageTargetID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "storage_target_id required")
	}

	var target models.StorageTarget
	if err := h.DB.Where("id = ? AND workspace_id = ?", req.StorageTargetID, workspaceID).First(&target).Error; err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "storage target does not belong to this workspace")
	}

	if err := h.DB.Model(&models.Camera{}).Where("id = ?", cameraID).Update("recording_storage_target_id", target.ID).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "camera.set_storage_target", "camera", cameraID, &workspaceID, target.Name)
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) AssignCameraToWorkspace(c *fiber.Ctx) error {
	cameraID := c.Params("id")
	workspaceID := c.Params("workspaceId")
	link := models.CameraWorkspace{CameraID: cameraID, WorkspaceID: workspaceID}
	if err := h.DB.Save(&link).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "camera.assign", "camera", cameraID, &workspaceID, "")
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) UnassignCameraFromWorkspace(c *fiber.Ctx) error {
	cameraID := c.Params("id")
	workspaceID := c.Params("workspaceId")
	if err := h.DB.Delete(&models.CameraWorkspace{}, "camera_id = ? AND workspace_id = ?", cameraID, workspaceID).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "camera.unassign", "camera", cameraID, &workspaceID, "")
	return c.SendStatus(fiber.StatusNoContent)
}
