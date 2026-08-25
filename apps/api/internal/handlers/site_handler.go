package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

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

// pairingCodeTTL is short on purpose: the code is typed by hand into an
// unauthenticated local setup page, so a shorter window limits how long a
// leaked/overheard code stays usable.
const pairingCodeTTL = 15 * time.Minute

// GenerateSitePairingCode issues a short-lived code that a freshly-installed
// edge agent (no AGENT_TOKEN yet) can exchange for the site's real agent
// token via its own local setup page + POST /api/agent/pair — the whole
// point being nobody has to copy the real token into a .env file by hand.
func (h *Handler) GenerateSitePairingCode(c *fiber.Ctx) error {
	id := c.Params("id")
	var site models.Site
	if err := h.DB.First(&site, "id = ?", id).Error; err != nil {
		return fiber.NewError(fiber.StatusNotFound, "site not found")
	}

	code, err := randomPairingCode()
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to generate pairing code")
	}
	expiresAt := time.Now().Add(pairingCodeTTL)
	if err := h.DB.Model(&site).Updates(map[string]interface{}{
		"pairing_code":            code,
		"pairing_code_expires_at": &expiresAt,
	}).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}

	h.audit(c, "site.pairing_code_generated", "site", site.ID, nil, "")
	return c.JSON(fiber.Map{"pairing_code": code, "expires_at": expiresAt})
}

// randomPairingCode avoids characters that are easy to mistype or confuse
// when read off a screen and typed on another device (0/O, 1/I/L).
func randomPairingCode() (string, error) {
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, b := range buf {
		sb.WriteByte(alphabet[int(b)%len(alphabet)])
	}
	return sb.String(), nil
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
