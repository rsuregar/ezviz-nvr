package handlers

import (
	"strings"

	"nvr-ezviz/api/internal/auth"
	"nvr-ezviz/api/internal/models"

	"github.com/gofiber/fiber/v2"
)

type mediamtxAuthRequest struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Action   string `json:"action"`
	Path     string `json:"path"`
}

// MediaMTXAuth is MediaMTX's HTTP auth webhook (authMethod: http in
// infra/mediamtx.yml) — it's called for every publish/read attempt against
// the live-view relay. A 20x response allows the action, anything else
// denies it; 401 specifically tells MediaMTX's RTSP layer to (re-)challenge
// the client for credentials, which is why the empty-credentials case below
// returns 401 rather than 403 (see mediamtx.org/docs/usage/authentication).
//
// This is intentionally unauthenticated at the Fiber-route level: the
// caller is MediaMTX itself (server-to-server, not a browser/agent), and
// the actual authorization decision happens inside this handler by
// validating the camera-specific credential MediaMTX forwards to us.
func (h *Handler) MediaMTXAuth(c *fiber.Ctx) error {
	var req mediamtxAuthRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "bad payload")
	}

	cameraID := strings.TrimPrefix(req.Path, "live/")
	if cameraID == "" || cameraID == req.Path {
		return fiber.NewError(fiber.StatusForbidden, "unknown path")
	}

	switch req.Action {
	case "publish":
		return h.authorizeMediaMTXPublish(c, cameraID, req.Password)
	case "read", "playback":
		return h.authorizeMediaMTXRead(c, cameraID, req.Password)
	default:
		return fiber.NewError(fiber.StatusForbidden, "action not allowed")
	}
}

func (h *Handler) authorizeMediaMTXPublish(c *fiber.Ctx, cameraID, token string) error {
	if token == "" {
		return fiber.NewError(fiber.StatusUnauthorized, "credentials required")
	}

	var camera models.Camera
	if err := h.DB.First(&camera, "id = ?", cameraID).Error; err != nil {
		return fiber.NewError(fiber.StatusForbidden, "unknown camera")
	}
	var site models.Site
	if err := h.DB.First(&site, "id = ?", camera.SiteID).Error; err != nil {
		return fiber.NewError(fiber.StatusForbidden, "unknown site")
	}
	if token != site.AgentToken {
		return fiber.NewError(fiber.StatusForbidden, "invalid agent token")
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *Handler) authorizeMediaMTXRead(c *fiber.Ctx, cameraID, token string) error {
	if token == "" {
		return fiber.NewError(fiber.StatusUnauthorized, "credentials required")
	}

	claims, err := auth.ParseAccessToken(h.Cfg.JWTSecret, token)
	if err != nil {
		return fiber.NewError(fiber.StatusForbidden, "invalid or expired token")
	}
	if claims.IsSuperAdmin {
		return c.SendStatus(fiber.StatusOK)
	}

	var count int64
	h.DB.Table("camera_workspaces").
		Joins("JOIN user_workspaces ON user_workspaces.workspace_id = camera_workspaces.workspace_id").
		Where("camera_workspaces.camera_id = ? AND user_workspaces.user_id = ?", cameraID, claims.UserID).
		Count(&count)
	if count == 0 {
		return fiber.NewError(fiber.StatusForbidden, "not a member of any workspace this camera belongs to")
	}
	return c.SendStatus(fiber.StatusOK)
}
