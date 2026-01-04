package http_handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/baechuer/real-time-ressys/services/auth-service/internal/application/auth"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/domain"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/infrastructure/security"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/logger"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/transport/http/dto"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/transport/http/middleware"
	"github.com/baechuer/real-time-ressys/services/auth-service/internal/transport/http/response"
	"github.com/go-chi/chi/v5"
)

type AuthHandler struct {
	svc           *auth.Service
	refreshTTL    time.Duration
	secureCookies bool
	cdnBaseURL    string
}

func NewAuthHandler(svc *auth.Service, refreshTTL time.Duration, secureCookies bool, cdnBaseURL string) *AuthHandler {
	return &AuthHandler{
		svc:           svc,
		refreshTTL:    refreshTTL,
		secureCookies: secureCookies,
	}
}

func (h *AuthHandler) toUserView(u domain.User) dto.UserView {
	view := dto.UserView{
		ID:            u.ID,
		Email:         u.Email,
		Role:          u.Role,
		EmailVerified: u.EmailVerified,
		Locked:        u.Locked,
		HasPassword:   u.PasswordHash != "",
	}
	if u.AvatarImageID != nil && *u.AvatarImageID != "" {
		// Construct full URL using CDNBaseURL
		// Convention: CDNBaseURL + "/" + AvatarImageID + "/512.webp" ?
		// Actually, media-service uses /public/bucket/UUID/SIZE.webp
		// If CDNBaseURL is "http://localhost:9000/public", then we likely need to append object path.
		// The object path constructed in media service is "avatars/{id}/{size}.webp"
		// Wait, media-service uses "avatars/" prefix?
		// Let's verify media-service storage logic.
		view.AvatarURL = fmt.Sprintf("%s/avatars/%s/512.webp", strings.TrimRight(h.cdnBaseURL, "/"), *u.AvatarImageID)
	}
	return view
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req dto.RegisterRequest
	if err := response.DecodeJSON(r, &req); err != nil {
		response.WriteError(w, r, err)
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if err := req.Validate(); err != nil {
		response.WriteError(w, r, err)
		return
	}

	res, err := h.svc.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	logger.WithCtx(r.Context()).Info().
		Str("user_id", res.User.ID).
		Str("email", res.User.Email).
		Msg("user_registered")

	security.SetRefreshToken(w, res.Tokens.RefreshToken, h.refreshTTL, h.secureCookies)

	data := dto.AuthData{
		User: h.toUserView(res.User),
		Tokens: dto.TokensView{
			AccessToken: res.Tokens.AccessToken,
			TokenType:   res.Tokens.TokenType,
			ExpiresIn:   res.Tokens.ExpiresIn,
		},
	}

	response.Created(w, data)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req dto.LoginRequest
	if err := response.DecodeJSON(r, &req); err != nil {
		response.WriteError(w, r, err)
		return
	}

	req.Email = strings.TrimSpace(req.Email)
	if err := req.Validate(); err != nil {
		response.WriteError(w, r, err)
		return
	}

	res, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	logger.WithCtx(r.Context()).Info().
		Str("user_id", res.User.ID).
		Msg("user_logged_in")

	security.SetRefreshToken(w, res.Tokens.RefreshToken, h.refreshTTL, h.secureCookies)

	data := dto.AuthData{
		User: h.toUserView(res.User),
		Tokens: dto.TokensView{
			AccessToken: res.Tokens.AccessToken,
			TokenType:   res.Tokens.TokenType,
			ExpiresIn:   res.Tokens.ExpiresIn,
		},
	}

	response.OK(w, data)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	refreshTok, err := security.ReadRefreshToken(r)
	if err != nil || refreshTok == "" {
		response.WriteError(w, r, domain.ErrRefreshTokenInvalid())
		return
	}

	toks, user, err := h.svc.Refresh(r.Context(), refreshTok)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	security.SetRefreshToken(w, toks.RefreshToken, h.refreshTTL, h.secureCookies)

	data := dto.RefreshData{
		Tokens: dto.TokensView{
			AccessToken: toks.AccessToken,
			TokenType:   toks.TokenType,
			ExpiresIn:   toks.ExpiresIn,
		},
		User: h.toUserView(user),
	}

	response.OK(w, data)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	refreshTok, err := security.ReadRefreshToken(r)
	if err == nil && refreshTok != "" {
		_ = h.svc.Logout(r.Context(), refreshTok) // keep idempotent
	}

	security.ClearRefreshToken(w, h.secureCookies)
	response.NoContent(w)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.WriteError(w, r, domain.ErrTokenInvalid())
		return
	}

	u, err := h.svc.GetUserByID(r.Context(), userID)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	data := dto.MeData{
		User: h.toUserView(u),
	}

	response.OK(w, data)
}
func (h *AuthHandler) BanUser(w http.ResponseWriter, r *http.Request) {
	actorID, _ := middleware.UserIDFromContext(r.Context())
	actorRole, _ := middleware.RoleFromContext(r.Context())

	targetID := chi.URLParam(r, "id")
	if strings.TrimSpace(targetID) == "" {
		response.WriteError(w, r, domain.ErrMissingField("id"))
		return
	}

	if err := h.svc.BanUser(r.Context(), actorID, actorRole, targetID); err != nil {
		response.WriteError(w, r, err)
		return
	}

	response.OK(w, dto.BanUserData{
		Status: "banned",
		UserID: targetID,
	})
}

func (h *AuthHandler) UnbanUser(w http.ResponseWriter, r *http.Request) {
	actorID, _ := middleware.UserIDFromContext(r.Context())
	actorRole, _ := middleware.RoleFromContext(r.Context())

	targetID := chi.URLParam(r, "id")
	if strings.TrimSpace(targetID) == "" {
		response.WriteError(w, r, domain.ErrMissingField("id"))
		return
	}

	if err := h.svc.UnbanUser(r.Context(), actorID, actorRole, targetID); err != nil {
		response.WriteError(w, r, err)
		return
	}

	response.OK(w, dto.UnbanUserData{
		Status: "unbanned",
		UserID: targetID,
	})
}
func (h *AuthHandler) AdminRevokeSessions(w http.ResponseWriter, r *http.Request) {
	actorID, _ := middleware.UserIDFromContext(r.Context())
	actorRole, _ := middleware.RoleFromContext(r.Context())

	targetID := chi.URLParam(r, "id")
	if strings.TrimSpace(targetID) == "" {
		response.WriteError(w, r, domain.ErrMissingField("id"))
		return
	}

	if err := h.svc.RevokeUserSessions(r.Context(), actorID, actorRole, targetID); err != nil {
		response.WriteError(w, r, err)
		return
	}

	response.OK(w, dto.RevokeSessionsData{
		Status: "revoked",
		UserID: targetID,
	})
}

func (h *AuthHandler) Admin(w http.ResponseWriter, r *http.Request) {
	userID, _ := middleware.UserIDFromContext(r.Context())
	role, _ := middleware.RoleFromContext(r.Context())

	data := dto.AdminData{
		Message: "admin ok",
		User: dto.UserView{
			ID:   userID,
			Role: role,
		},
	}

	response.OK(w, data)
}
func (h *AuthHandler) AdminSetUserRole(w http.ResponseWriter, r *http.Request) {
	actorID, _ := middleware.UserIDFromContext(r.Context())
	actorRole, _ := middleware.RoleFromContext(r.Context())

	targetID := chi.URLParam(r, "id")
	if strings.TrimSpace(targetID) == "" {
		response.WriteError(w, r, domain.ErrMissingField("id"))
		return
	}

	var req dto.SetUserRoleRequest
	if err := response.DecodeJSON(r, &req); err != nil {
		response.WriteError(w, r, err)
		return
	}
	if err := req.Validate(); err != nil {
		response.WriteError(w, r, err)
		return
	}

	if err := h.svc.SetUserRole(r.Context(), actorID, actorRole, targetID, req.Role); err != nil {
		response.WriteError(w, r, err)
		return
	}

	response.OK(w, dto.SetUserRoleData{
		Status: "role_updated",
		UserID: targetID,
		Role:   req.Role,
	})
}
func (h *AuthHandler) MeStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.WriteError(w, r, domain.ErrTokenInvalid())
		return
	}

	st, err := h.svc.GetMyStatus(r.Context(), userID)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	response.OK(w, dto.MeStatusData{
		UserID:        st.UserID,
		Role:          st.Role,
		Locked:        st.Locked,
		EmailVerified: st.EmailVerified,
		HasPassword:   st.HasPassword,
	})
}
func (h *AuthHandler) AdminUserStatus(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	if strings.TrimSpace(targetID) == "" {
		response.WriteError(w, r, domain.ErrMissingField("id"))
		return
	}

	st, err := h.svc.GetUserStatus(r.Context(), targetID)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	response.OK(w, dto.MeStatusData{
		UserID:        st.UserID,
		Role:          st.Role,
		Locked:        st.Locked,
		EmailVerified: st.EmailVerified,
		HasPassword:   st.HasPassword,
	})
}

// ---- Verify Email ----

func (h *AuthHandler) VerifyEmailRequest(w http.ResponseWriter, r *http.Request) {
	var req dto.VerifyEmailRequest
	if err := response.DecodeJSON(r, &req); err != nil {
		response.WriteError(w, r, err)
		return
	}
	if err := req.Validate(); err != nil {
		response.WriteError(w, r, err)
		return
	}

	if err := h.svc.VerifyEmailRequest(r.Context(), req.Email); err != nil {
		response.WriteError(w, r, err)
		return
	}

	response.NoContent(w)
}

func (h *AuthHandler) VerifyEmailConfirmGET(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		response.WriteError(w, r, domain.ErrMissingField("token"))
		return
	}

	if err := h.svc.VerifyEmailConfirm(r.Context(), token); err != nil {
		response.WriteError(w, r, err)
		return
	}

	response.OK(w, map[string]string{"status": "verified"})
}

func (h *AuthHandler) VerifyEmailConfirmPOST(w http.ResponseWriter, r *http.Request) {
	var req dto.VerifyEmailConfirmRequest
	if err := response.DecodeJSON(r, &req); err != nil {
		response.WriteError(w, r, err)
		return
	}
	if err := req.Validate(); err != nil {
		response.WriteError(w, r, err)
		return
	}

	if err := h.svc.VerifyEmailConfirm(r.Context(), req.Token); err != nil {
		response.WriteError(w, r, err)
		return
	}

	response.OK(w, map[string]string{"status": "verified"})
}

// ---- Password Reset ----

func (h *AuthHandler) PasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	var req dto.PasswordResetRequest
	if err := response.DecodeJSON(r, &req); err != nil {
		response.WriteError(w, r, err)
		return
	}
	if err := req.Validate(); err != nil {
		response.WriteError(w, r, err)
		return
	}

	if err := h.svc.PasswordResetRequest(r.Context(), req.Email); err != nil {
		response.WriteError(w, r, err)
		return
	}

	response.NoContent(w)
}

func (h *AuthHandler) PasswordResetValidate(w http.ResponseWriter, r *http.Request) {
	q := dto.PasswordResetValidateQuery{Token: strings.TrimSpace(r.URL.Query().Get("token"))}
	if err := q.Validate(); err != nil {
		response.WriteError(w, r, err)
		return
	}

	if err := h.svc.PasswordResetValidate(r.Context(), q.Token); err != nil {
		response.WriteError(w, r, err)
		return
	}

	response.OK(w, map[string]bool{"valid": true})
}

func (h *AuthHandler) PasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	var req dto.PasswordResetConfirmRequest
	if err := response.DecodeJSON(r, &req); err != nil {
		response.WriteError(w, r, err)
		return
	}
	if err := req.Validate(); err != nil {
		response.WriteError(w, r, err)
		return
	}

	if err := h.svc.PasswordResetConfirm(r.Context(), req.Token, req.NewPassword); err != nil {
		response.WriteError(w, r, err)
		return
	}

	response.NoContent(w)
}
func (h *AuthHandler) PasswordChange(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.WriteError(w, r, domain.ErrTokenInvalid())
		return
	}

	var req dto.PasswordChangeRequest
	if err := response.DecodeJSON(r, &req); err != nil {
		response.WriteError(w, r, err)
		return
	}
	if err := req.Validate(); err != nil {
		response.WriteError(w, r, err)
		return
	}

	if err := h.svc.PasswordChange(r.Context(), userID, req.OldPassword, req.NewPassword); err != nil {
		response.WriteError(w, r, err)
		return
	}

	// service 内部已经 RevokeAll(userID)；这里清 cookie 让浏览器立刻失效
	security.ClearRefreshToken(w, h.secureCookies)
	response.NoContent(w)
}
func (h *AuthHandler) SessionsRevoke(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		response.WriteError(w, r, domain.ErrTokenInvalid())
		return
	}

	if err := h.svc.SessionsRevoke(r.Context(), userID); err != nil {
		response.WriteError(w, r, err)
		return
	}

	security.ClearRefreshToken(w, h.secureCookies)
	response.NoContent(w)
}

// ---- Profile ----

// UpdateAvatar updates the current user's avatar URL
func (h *AuthHandler) UpdateAvatar(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok || userID == "" {
		response.WriteError(w, r, domain.ErrTokenInvalid())
		return
	}

	var req struct {
		AvatarURL string `json:"avatar_url"`
	}
	if err := response.DecodeJSON(r, &req); err != nil {
		response.WriteError(w, r, err)
		return
	}

	if req.AvatarURL == "" {
		response.WriteError(w, r, domain.ErrMissingField("avatar_url"))
		return
	}

	// Update in DB and get previous ID for cleanup
	_, err := h.svc.UpdateAvatarURL(r.Context(), userID, req.AvatarURL)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	// Fetch updated user to get the confirmed AvatarImageID and construct proper URL
	user, err := h.svc.GetUserByID(r.Context(), userID)
	if err != nil {
		response.WriteError(w, r, err)
		return
	}

	view := h.toUserView(user)
	response.OK(w, map[string]string{
		"avatar_url": view.AvatarURL,
	})
}

// ---- Everything else stays 501 for now ----
