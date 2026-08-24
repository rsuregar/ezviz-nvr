package handlers

import "github.com/gofiber/fiber/v2"

// GetLiveConfig just hands the browser the (non-secret) base URL to build
// HLS requests against the central MediaMTX relay. Authentication for each
// stream is done per-request against /api/mediamtx/auth using the caller's
// own JWT — see mediamtx_auth_handler.go — so there's no shared secret to
// hand out here.
func (h *Handler) GetLiveConfig(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"hls_base_url": h.Cfg.MediaMTXHLSBaseURL,
	})
}
