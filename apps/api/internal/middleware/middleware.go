package middleware

import (
	"strings"

	"nvr-ezviz/api/internal/auth"
	"nvr-ezviz/api/internal/models"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

const (
	LocalsUserID       = "userID"
	LocalsIsSuperAdmin = "isSuperAdmin"
	LocalsSiteID       = "siteID"
)

func RequireAuth(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			return fiber.NewError(fiber.StatusUnauthorized, "missing bearer token")
		}
		token := strings.TrimPrefix(header, "Bearer ")
		claims, err := auth.ParseAccessToken(secret, token)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired token")
		}
		c.Locals(LocalsUserID, claims.UserID)
		c.Locals(LocalsIsSuperAdmin, claims.IsSuperAdmin)
		return c.Next()
	}
}

// RequireAuthFlexible behaves like RequireAuth but also accepts the access
// token as an ?access_token= query param. Only use this for endpoints that
// a plain top-level browser navigation must hit directly (e.g. kicking off
// an OAuth redirect), where JS can't attach an Authorization header — a
// fetch() with a manually-followed redirect can't read a cross-origin
// Location header due to CORS, so there's no way to keep this one
// header-only under our current (non-cookie) auth model.
func RequireAuthFlexible(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := c.Query("access_token")
		if token == "" {
			header := c.Get("Authorization")
			if strings.HasPrefix(header, "Bearer ") {
				token = strings.TrimPrefix(header, "Bearer ")
			}
		}
		if token == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "missing access token")
		}
		claims, err := auth.ParseAccessToken(secret, token)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid or expired token")
		}
		c.Locals(LocalsUserID, claims.UserID)
		c.Locals(LocalsIsSuperAdmin, claims.IsSuperAdmin)
		return c.Next()
	}
}

func RequireSuperAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if isSuper, _ := c.Locals(LocalsIsSuperAdmin).(bool); !isSuper {
			return fiber.NewError(fiber.StatusForbidden, "superadmin only")
		}
		return c.Next()
	}
}

// RequireAgentToken authenticates an edge agent request using the per-site
// token issued when the site was created (Authorization: Agent <token>).
func RequireAgentToken(gdb *gorm.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if !strings.HasPrefix(header, "Agent ") {
			return fiber.NewError(fiber.StatusUnauthorized, "missing agent token")
		}
		token := strings.TrimPrefix(header, "Agent ")

		var site models.Site
		if err := gdb.Where("agent_token = ?", token).First(&site).Error; err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid agent token")
		}
		c.Locals(LocalsSiteID, site.ID)
		return c.Next()
	}
}

// RequireWorkspaceRole checks that the caller is a superadmin, or a member of
// the :workspaceId route param with at least minRole (admin > viewer).
func RequireWorkspaceRole(gdb *gorm.DB, minRole models.WorkspaceRole) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if isSuper, _ := c.Locals(LocalsIsSuperAdmin).(bool); isSuper {
			return c.Next()
		}
		userID, _ := c.Locals(LocalsUserID).(string)
		workspaceID := c.Params("workspaceId")
		if workspaceID == "" {
			return fiber.NewError(fiber.StatusBadRequest, "workspaceId required")
		}

		var membership models.UserWorkspace
		if err := gdb.Where("user_id = ? AND workspace_id = ?", userID, workspaceID).First(&membership).Error; err != nil {
			return fiber.NewError(fiber.StatusForbidden, "not a member of this workspace")
		}
		if minRole == models.RoleAdmin && membership.Role != models.RoleAdmin {
			return fiber.NewError(fiber.StatusForbidden, "workspace admin only")
		}
		return c.Next()
	}
}
