package router

import (
	"nvr-ezviz/api/internal/handlers"
	"nvr-ezviz/api/internal/middleware"
	"nvr-ezviz/api/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"gorm.io/gorm"
)

func New(h *handlers.Handler, db *gorm.DB) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: jsonErrorHandler})
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))

	app.Get("/healthz", h.Healthz)

	api := app.Group("/api")

	// --- public ---
	api.Post("/auth/login", h.Login)
	api.Post("/auth/refresh", h.RefreshToken)
	api.Post("/auth/logout", h.Logout)
	api.Get("/oauth/google/callback", h.GoogleOAuthCallback)
	api.Post("/mediamtx/auth", h.MediaMTXAuth)

	// --- edge agent (site token auth, not user JWT) ---
	agent := api.Group("/agent", middleware.RequireAgentToken(db))
	agent.Post("/heartbeat", h.AgentHeartbeat)
	agent.Post("/camera-status", h.AgentReportCameraStatus)
	agent.Post("/recordings", h.AgentReportRecording)
	agent.Post("/upload-failure", h.AgentReportUploadFailure)

	// Plain top-level navigation (window.location), so it must accept the
	// token as a query param — see RequireAuthFlexible. Registered before
	// the `authed` group below: Fiber applies Use()-style middleware (which
	// is what Group("", mw) is) to every route matching its prefix based on
	// registration order, not on which Go variable a later route happens to
	// be declared through — so this must come first or it'd get wrapped by
	// RequireAuth (header-only) too.
	api.Get(
		"/workspaces/:workspaceId/oauth/google/start",
		middleware.RequireAuthFlexible(h.Cfg.JWTSecret),
		middleware.RequireWorkspaceRole(db, models.RoleAdmin),
		h.GoogleOAuthStart,
	)

	// Same reasoning: this is a <video src="..."> target, which can't send
	// an Authorization header either.
	api.Get(
		"/workspaces/:workspaceId/cameras/:cameraId/recordings/:recordingId/stream",
		middleware.RequireAuthFlexible(h.Cfg.JWTSecret),
		middleware.RequireWorkspaceRole(db, models.RoleViewer),
		h.StreamRecording,
	)

	// --- authenticated users ---
	authed := api.Group("", middleware.RequireAuth(h.Cfg.JWTSecret))
	authed.Get("/me", h.Me)
	authed.Get("/workspaces", h.ListMyWorkspaces)

	// workspace member (viewer or admin)
	wsMember := authed.Group("/workspaces/:workspaceId", middleware.RequireWorkspaceRole(db, models.RoleViewer))
	wsMember.Get("/", h.GetWorkspace)
	wsMember.Get("/cameras", h.ListWorkspaceCameras)
	wsMember.Get("/cameras/:cameraId/recordings", h.ListCameraRecordings)
	wsMember.Get("/storage-targets", h.ListStorageTargets)
	wsMember.Get("/live-config", h.GetLiveConfig)

	// workspace admin
	wsAdmin := authed.Group("/workspaces/:workspaceId", middleware.RequireWorkspaceRole(db, models.RoleAdmin))
	wsAdmin.Put("/", h.UpdateWorkspace)
	wsAdmin.Put("/members/:userId", h.SetWorkspaceMembership)
	wsAdmin.Delete("/members/:userId", h.RemoveWorkspaceMembership)
	wsAdmin.Post("/cameras/:id/assign", h.AssignCameraToWorkspace)
	wsAdmin.Delete("/cameras/:id/assign", h.UnassignCameraFromWorkspace)
	wsAdmin.Put("/cameras/:id/storage-target", h.SetCameraStorageTarget)
	wsAdmin.Post("/storage-targets", h.CreateStorageTarget)
	wsAdmin.Put("/storage-targets/:id", h.UpdateStorageTarget)
	wsAdmin.Delete("/storage-targets/:id", h.DeleteStorageTarget)
	wsAdmin.Get("/notification-channels", h.ListNotificationChannels)
	wsAdmin.Post("/notification-channels", h.CreateNotificationChannel)
	wsAdmin.Delete("/notification-channels/:id", h.DeleteNotificationChannel)

	// superadmin only: global user/workspace/site/camera provisioning
	admin := authed.Group("", middleware.RequireSuperAdmin())
	admin.Get("/users", h.ListUsers)
	admin.Post("/users", h.CreateUser)
	admin.Delete("/users/:id", h.DeleteUser)
	admin.Post("/workspaces", h.CreateWorkspace)
	admin.Delete("/workspaces/:workspaceId", h.DeleteWorkspace)
	admin.Get("/sites", h.ListSites)
	admin.Post("/sites", h.CreateSite)
	admin.Delete("/sites/:id", h.DeleteSite)
	admin.Post("/sites/:id/regenerate-token", h.RegenerateSiteToken)
	admin.Get("/audit-log", h.ListAuditLog)
	admin.Get("/cameras", h.ListAllCameras)
	admin.Get("/sites/:siteId/cameras", h.ListSiteCameras)
	admin.Post("/sites/:siteId/cameras", h.CreateCamera)
	admin.Put("/cameras/:id", h.UpdateCamera)
	admin.Delete("/cameras/:id", h.DeleteCamera)

	return app
}

func jsonErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}
	return c.Status(code).JSON(fiber.Map{"error": err.Error()})
}
