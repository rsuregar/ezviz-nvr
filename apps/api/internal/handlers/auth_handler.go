package handlers

import (
	"time"

	"nvr-ezviz/api/internal/auth"
	"nvr-ezviz/api/internal/middleware"
	"nvr-ezviz/api/internal/models"

	"github.com/gofiber/fiber/v2"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	User         models.User `json:"user"`
}

func (h *Handler) Login(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	var user models.User
	if err := h.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		// No user row to attach as actor, so the attempted email goes in
		// target_id instead — still traceable in the audit log without
		// revealing whether that email exists (same "invalid credentials"
		// either way).
		h.auditAs("", "auth.login_failed", "user", req.Email, nil, "user not found")
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		h.auditAs(user.ID, "auth.login_failed", "user", user.Email, nil, "wrong password")
		return fiber.NewError(fiber.StatusUnauthorized, "invalid credentials")
	}

	return h.issueTokens(c, &user)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *Handler) RefreshToken(c *fiber.Ctx) error {
	var req refreshRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}

	hash := auth.HashToken(req.RefreshToken)
	var stored models.RefreshToken
	if err := h.DB.Where("token_hash = ?", hash).First(&stored).Error; err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid refresh token")
	}
	if stored.RevokedAt != nil || time.Now().After(stored.ExpiresAt) {
		return fiber.NewError(fiber.StatusUnauthorized, "refresh token expired or revoked")
	}

	var user models.User
	if err := h.DB.First(&user, "id = ?", stored.UserID).Error; err != nil {
		return fiber.NewError(fiber.StatusUnauthorized, "user not found")
	}

	// rotate: revoke old, issue new
	now := time.Now()
	h.DB.Model(&stored).Update("revoked_at", &now)

	return h.issueTokens(c, &user)
}

func (h *Handler) Logout(c *fiber.Ctx) error {
	var req refreshRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	hash := auth.HashToken(req.RefreshToken)
	now := time.Now()
	h.DB.Model(&models.RefreshToken{}).Where("token_hash = ?", hash).Update("revoked_at", &now)
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) Me(c *fiber.Ctx) error {
	userID, _ := c.Locals(middleware.LocalsUserID).(string)
	var user models.User
	if err := h.DB.Preload("Memberships").First(&user, "id = ?", userID).Error; err != nil {
		return fiber.NewError(fiber.StatusNotFound, "user not found")
	}
	return c.JSON(user)
}

func (h *Handler) issueTokens(c *fiber.Ctx, user *models.User) error {
	access, err := auth.GenerateAccessToken(h.Cfg.JWTSecret, h.Cfg.AccessTokenTTL, user.ID, user.IsSuperAdmin)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to issue token")
	}
	plainRefresh, hashRefresh, err := auth.GenerateRefreshToken()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to issue refresh token")
	}
	rt := models.RefreshToken{
		UserID:    user.ID,
		TokenHash: hashRefresh,
		ExpiresAt: time.Now().Add(h.Cfg.RefreshTokenTTL),
	}
	if err := h.DB.Create(&rt).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to persist refresh token")
	}

	return c.JSON(tokenResponse{
		AccessToken:  access,
		RefreshToken: plainRefresh,
		User:         *user,
	})
}
