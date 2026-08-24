package handlers

import (
	"crypto/rand"
	"encoding/hex"

	"nvr-ezviz/api/internal/models"

	"github.com/gofiber/fiber/v2"
)

func (h *Handler) ListSites(c *fiber.Ctx) error {
	var sites []models.Site
	if err := h.DB.Find(&sites).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(sites)
}

type createSiteRequest struct {
	Name string `json:"name"`
}

// CreateSite provisions a new physical location and returns its one-time
// agent token; the edge agent deployed at that site uses it to authenticate.
func (h *Handler) CreateSite(c *fiber.Ctx) error {
	var req createSiteRequest
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name required")
	}

	token, err := randomToken()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to generate agent token")
	}
	site := models.Site{Name: req.Name, AgentToken: token}
	if err := h.DB.Create(&site).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "site.create", "site", site.ID, nil, site.Name)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"site":        site,
		"agent_token": token,
	})
}

func (h *Handler) DeleteSite(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.DB.Delete(&models.Site{}, "id = ?", id).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "site.delete", "site", id, nil, "")
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) RegenerateSiteToken(c *fiber.Ctx) error {
	id := c.Params("id")
	token, err := randomToken()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to generate agent token")
	}
	if err := h.DB.Model(&models.Site{}).Where("id = ?", id).Update("agent_token", token).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	h.audit(c, "site.regenerate_token", "site", id, nil, "")
	return c.JSON(fiber.Map{"agent_token": token})
}

func randomToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
