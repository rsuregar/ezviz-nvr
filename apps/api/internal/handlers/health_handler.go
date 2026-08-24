package handlers

import "github.com/gofiber/fiber/v2"

// Healthz is a plain liveness/readiness probe for infra monitoring (Docker
// HEALTHCHECK, uptime checks, CloudPanel) — not for the dashboard's own
// "Health" tab, which instead reads real site/camera status from
// /api/sites and /api/cameras (see ListSites, ListAllCameras).
func (h *Handler) Healthz(c *fiber.Ctx) error {
	sqlDB, err := h.DB.DB()
	if err != nil || sqlDB.Ping() != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"status": "error", "db": "unreachable"})
	}
	return c.JSON(fiber.Map{"status": "ok", "db": "ok"})
}
