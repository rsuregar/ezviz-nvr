package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"time"

	"nvr-ezviz/api/internal/middleware"
	"nvr-ezviz/api/internal/models"

	"github.com/gofiber/fiber/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const driveFileScope = "https://www.googleapis.com/auth/drive.file"

func (h *Handler) googleOAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     h.Cfg.GoogleOAuthClientID,
		ClientSecret: h.Cfg.GoogleOAuthClientSecret,
		RedirectURL:  h.Cfg.GoogleOAuthRedirectURL,
		Endpoint:     google.Endpoint,
		Scopes:       []string{driveFileScope},
	}
}

type oauthStatePayload struct {
	WorkspaceID     string `json:"workspace_id"`
	Name            string `json:"name"`
	InitiatorUserID string `json:"initiator_user_id"`
	Exp             int64  `json:"exp"`
}

// signState/verifyState let the callback (an unauthenticated redirect from
// Google, carrying no Authorization header) trust which workspace/name this
// flow was started for, without needing server-side session storage: the
// context was only ever signed for a request that had already passed
// RequireWorkspaceRole(admin) at the /start step.
func (h *Handler) signState(p oauthStatePayload) (string, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, []byte(h.Cfg.JWTSecret))
	mac.Write(payload)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (h *Handler) verifyState(state string) (*oauthStatePayload, error) {
	dot := -1
	for i, c := range state {
		if c == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		return nil, fiber.NewError(fiber.StatusBadRequest, "malformed state")
	}
	payload, err := base64.RawURLEncoding.DecodeString(state[:dot])
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(state[dot+1:])
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, []byte(h.Cfg.JWTSecret))
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return nil, fiber.NewError(fiber.StatusBadRequest, "invalid state signature")
	}
	var p oauthStatePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, err
	}
	if time.Now().Unix() > p.Exp {
		return nil, fiber.NewError(fiber.StatusBadRequest, "state expired, try connecting again")
	}
	return &p, nil
}

// GoogleOAuthStart redirects a workspace admin to Google's consent screen.
// AccessTypeOffline + ApprovalForce ensures a refresh_token comes back even
// if this Google account already authorized the app before (Google only
// issues a refresh_token on the *first* consent otherwise).
func (h *Handler) GoogleOAuthStart(c *fiber.Ctx) error {
	if h.Cfg.GoogleOAuthClientID == "" {
		return fiber.NewError(fiber.StatusPreconditionFailed, "Google OAuth is not configured on this server (GOOGLE_OAUTH_CLIENT_ID/SECRET)")
	}
	workspaceID := c.Params("workspaceId")
	name := c.Query("name", "Google Drive")
	initiatorUserID, _ := c.Locals(middleware.LocalsUserID).(string)

	state, err := h.signState(oauthStatePayload{
		WorkspaceID:     workspaceID,
		Name:            name,
		InitiatorUserID: initiatorUserID,
		Exp:             time.Now().Add(10 * time.Minute).Unix(),
	})
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to prepare oauth state")
	}

	url := h.googleOAuthConfig().AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	return c.Redirect(url, fiber.StatusFound)
}

// GoogleOAuthCallback exchanges the auth code for tokens and creates the
// gdrive StorageTarget, then bounces the browser back to the dashboard.
func (h *Handler) GoogleOAuthCallback(c *fiber.Ctx) error {
	state, err := h.verifyState(c.Query("state"))
	if err != nil {
		return err
	}

	code := c.Query("code")
	if code == "" {
		return fiber.NewError(fiber.StatusBadRequest, "missing code")
	}

	token, err := h.googleOAuthConfig().Exchange(context.Background(), code)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, "failed to exchange code with Google: "+err.Error())
	}
	if token.RefreshToken == "" {
		return fiber.NewError(fiber.StatusBadGateway, "Google did not return a refresh token; revoke app access at myaccount.google.com/permissions and try connecting again")
	}

	config := map[string]interface{}{
		"client_id":     h.Cfg.GoogleOAuthClientID,
		"client_secret": h.Cfg.GoogleOAuthClientSecret,
		"refresh_token": token.RefreshToken,
		"folder_id":     "",
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to encode storage config")
	}
	encrypted, err := h.encryptConfig(string(configJSON))
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to secure storage config")
	}

	target := models.StorageTarget{
		WorkspaceID: state.WorkspaceID,
		Name:        state.Name,
		Type:        models.StorageGDrive,
		Config:      encrypted,
		RetainDays:  30,
	}
	if err := h.DB.Create(&target).Error; err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to save storage target: "+err.Error())
	}
	h.auditAs(state.InitiatorUserID, "storage_target.create", "storage_target", target.ID, &state.WorkspaceID, target.Name+" (gdrive, via OAuth)")

	return c.Redirect(h.Cfg.WebBaseURL+"/workspaces/"+state.WorkspaceID+"?connected=gdrive", fiber.StatusFound)
}
