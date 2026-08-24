package handlers

import (
	"nvr-ezviz/api/internal/middleware"
	"nvr-ezviz/api/internal/models"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// ListMyWorkspaces returns every workspace the caller belongs to
// (or all workspaces, if the caller is a superadmin).
func (h *Handler) ListMyWorkspaces(c *fiber.Ctx) error {
	isSuper, _ := c.Locals(middleware.LocalsIsSuperAdmin).(bool)

	var workspaces []models.Workspace
	if isSuper {
		if err := h.DB.Find(&workspaces).Error; err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(workspaces)
	}

	userID, _ := c.Locals(middleware.LocalsUserID).(string)
	if err := h.DB.Joins("JOIN user_workspaces uw ON uw.workspace_id = workspaces.id").
		Where("uw.user_id = ?", userID).Find(&workspaces).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(workspaces)
}

func (h *Handler) GetWorkspace(c *fiber.Ctx) error {
	id := c.Params("workspaceId")
	var ws models.Workspace
	if err := h.DB.First(&ws, "id = ?", id).Error; err != nil {
		return fiber.NewError(fiber.StatusNotFound, "workspace not found")
	}

	var members []models.UserWorkspace
	h.DB.Preload("User").Where("workspace_id = ?", id).Find(&members)

	return c.JSON(fiber.Map{"workspace": ws, "members": members})
}

type createWorkspaceRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (h *Handler) CreateWorkspace(c *fiber.Ctx) error {
	var req createWorkspaceRequest
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name required")
	}
	if req.Slug == "" {
		req.Slug = slugify(req.Name)
	}
	ws := models.Workspace{Name: req.Name, Slug: req.Slug}
	if err := h.DB.Create(&ws).Error; err != nil {
		return fiber.NewError(fiber.StatusConflict, "slug already in use")
	}
	h.audit(c, "workspace.create", "workspace", ws.ID, &ws.ID, ws.Name)
	return c.Status(fiber.StatusCreated).JSON(ws)
}

func (h *Handler) UpdateWorkspace(c *fiber.Ctx) error {
	id := c.Params("workspaceId")
	var req createWorkspaceRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	updates := map[string]interface{}{}
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Slug != "" {
		updates["slug"] = req.Slug
	}
	if err := h.DB.Model(&models.Workspace{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) DeleteWorkspace(c *fiber.Ctx) error {
	id := c.Params("workspaceId")
	if err := h.DB.Delete(&models.Workspace{}, "id = ?", id).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "workspace.delete", "workspace", id, nil, "")
	return c.SendStatus(fiber.StatusNoContent)
}

func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	return s
}
