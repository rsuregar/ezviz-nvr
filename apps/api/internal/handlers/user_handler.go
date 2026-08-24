package handlers

import (
	"nvr-ezviz/api/internal/auth"
	"nvr-ezviz/api/internal/models"

	"github.com/gofiber/fiber/v2"
)

func (h *Handler) ListUsers(c *fiber.Ctx) error {
	var users []models.User
	if err := h.DB.Find(&users).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(users)
}

type createUserRequest struct {
	Email        string `json:"email"`
	Password     string `json:"password"`
	Name         string `json:"name"`
	IsSuperAdmin bool   `json:"is_superadmin"`
}

func (h *Handler) CreateUser(c *fiber.Ctx) error {
	var req createUserRequest
	if err := c.BodyParser(&req); err != nil || req.Email == "" || req.Password == "" {
		return fiber.NewError(fiber.StatusBadRequest, "email and password required")
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to hash password")
	}
	user := models.User{
		Email:        req.Email,
		PasswordHash: hash,
		Name:         req.Name,
		IsSuperAdmin: req.IsSuperAdmin,
	}
	if err := h.DB.Create(&user).Error; err != nil {
		return fiber.NewError(fiber.StatusConflict, "email already in use")
	}
	h.audit(c, "user.create", "user", user.ID, nil, user.Email)
	return c.Status(fiber.StatusCreated).JSON(user)
}

func (h *Handler) DeleteUser(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.DB.Delete(&models.User{}, "id = ?", id).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "user.delete", "user", id, nil, "")
	return c.SendStatus(fiber.StatusNoContent)
}

type setMembershipRequest struct {
	Role models.WorkspaceRole `json:"role"`
}

// SetWorkspaceMembership adds or updates a user's role within a workspace.
func (h *Handler) SetWorkspaceMembership(c *fiber.Ctx) error {
	workspaceID := c.Params("workspaceId")
	userID := c.Params("userId")
	var req setMembershipRequest
	if err := c.BodyParser(&req); err != nil || (req.Role != models.RoleAdmin && req.Role != models.RoleViewer) {
		return fiber.NewError(fiber.StatusBadRequest, "role must be 'admin' or 'viewer'")
	}

	membership := models.UserWorkspace{UserID: userID, WorkspaceID: workspaceID, Role: req.Role}
	if err := h.DB.Save(&membership).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "membership.set", "user", userID, &workspaceID, "role="+string(req.Role))
	return c.JSON(membership)
}

func (h *Handler) RemoveWorkspaceMembership(c *fiber.Ctx) error {
	workspaceID := c.Params("workspaceId")
	userID := c.Params("userId")
	if err := h.DB.Delete(&models.UserWorkspace{}, "workspace_id = ? AND user_id = ?", workspaceID, userID).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "membership.remove", "user", userID, &workspaceID, "")
	return c.SendStatus(fiber.StatusNoContent)
}
