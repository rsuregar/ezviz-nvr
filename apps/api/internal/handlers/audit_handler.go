package handlers

import (
	"nvr-ezviz/api/internal/models"

	"github.com/gofiber/fiber/v2"
)

// ListAuditLog returns the most recent admin actions, newest first.
// Superadmin-only for now — a workspace-scoped variant (filtered to that
// workspace's own entries) would be a reasonable follow-up once workspace
// admins need it.
func (h *Handler) ListAuditLog(c *fiber.Ctx) error {
	query := h.DB.Order("created_at DESC").Limit(200)
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		query = query.Where("workspace_id = ?", workspaceID)
	}

	var entries []models.AuditLog
	if err := query.Find(&entries).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(entries)
}
