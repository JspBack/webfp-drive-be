package handlers

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	"github.com/JspBack/webfp-drive/store"
)

type credentialsRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

type authResponse struct {
	User         *store.User `json:"user,omitempty"`
	Token        string      `json:"token"`
	RefreshToken string      `json:"refresh_token"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type updateUserRequest struct {
	Email *string `json:"email"`
	Name  *string `json:"name"`
}

const minimumPasswordLength = 8

// Register godoc
// @Summary      Register an account
// @Description  Creates an account and returns a 10-minute access token and 24-hour refresh token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      credentialsRequest  true  "Registration details"
// @Success      201      {object}  authResponse
// @Failure      400      {object}  map[string]string
// @Failure      409      {object}  map[string]string  "Email already registered"
// @Failure      500      {object}  map[string]string
// @Router       /api/v1/auth/register [post].
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		httphelper.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	email := strings.TrimSpace(req.Email)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		httphelper.Error(w, http.StatusBadRequest, "valid email is required")
		return
	}
	if len(req.Password) < minimumPasswordLength {
		httphelper.Error(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	user, err := h.store.Users.Add(r.Context(), email, req.Name, req.Password)
	if err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			httphelper.Error(w, http.StatusConflict, "email is already registered")
			return
		}
		h.internalError(w, r, "register user", err)
		return
	}
	tokens, err := h.store.Users.CreateSession(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, "create registration session", err)
		return
	}
	httphelper.JSON(w, http.StatusCreated, authResponse{
		User: user, Token: tokens.AccessToken, RefreshToken: tokens.RefreshToken,
	})
}

// Login godoc
// @Summary      Log in
// @Description  Authenticates and returns a 10-minute access token and 24-hour refresh token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      credentialsRequest  true  "Login credentials"
// @Success      200      {object}  authResponse
// @Failure      400      {object}  map[string]string
// @Failure      401      {object}  map[string]string  "Invalid credentials"
// @Failure      500      {object}  map[string]string
// @Router       /api/v1/auth/login [post].
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		httphelper.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := h.store.Users.Authenticate(r.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, store.ErrInvalidCreds) {
			httphelper.Error(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		h.internalError(w, r, "authenticate user", err)
		return
	}
	tokens, err := h.store.Users.CreateSession(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, "create login session", err)
		return
	}
	httphelper.JSON(w, http.StatusOK, authResponse{
		User: user, Token: tokens.AccessToken, RefreshToken: tokens.RefreshToken,
	})
}

// Refresh godoc
// @Summary      Refresh an access token
// @Description  Exchanges a valid refresh token for a new 10-minute access token and a rotated 24-hour refresh token.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      refreshRequest  true  "Refresh token"
// @Success      200      {object}  authResponse
// @Failure      400      {object}  map[string]string
// @Failure      401      {object}  map[string]string  "Refresh token is invalid or expired"
// @Failure      500      {object}  map[string]string
// @Router       /api/v1/auth/refresh [post].
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		httphelper.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.RefreshToken) == "" {
		httphelper.Error(w, http.StatusBadRequest, "refresh_token is required")
		return
	}
	tokens, err := h.store.Users.RefreshSession(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, store.ErrInvalidSession) {
			httphelper.Error(w, http.StatusUnauthorized, "refresh token is invalid or expired")
			return
		}
		h.internalError(w, r, "refresh session", err)
		return
	}
	httphelper.JSON(w, http.StatusOK, authResponse{
		Token: tokens.AccessToken, RefreshToken: tokens.RefreshToken,
	})
}

// Logout godoc
// @Summary      Log out
// @Description  Revokes the current bearer session.
// @Tags         auth
// @Produce      json
// @Security     BearerAuth
// @Success      204  "Session revoked"
// @Failure      401  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/auth/logout [post].
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	token, _ := authhelper.SessionToken(r.Context())
	sessionID, _ := authhelper.SessionID(r.Context())
	if err := h.store.Users.RevokeSession(r.Context(), token, sessionID); err != nil {
		h.internalError(w, r, "revoke session", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// UpdateUser godoc
// @Summary   Update the current user
// @Tags      users
// @Accept    json
// @Produce   json
// @Security  BearerAuth
// @Param     request  body      updateUserRequest  true  "Fields to update"
// @Success   200      {object}  store.User
// @Failure   400      {object}  map[string]string
// @Failure   401      {object}  map[string]string
// @Failure   404      {object}  map[string]string
// @Failure   409      {object}  map[string]string  "Email already registered"
// @Failure   500      {object}  map[string]string
// @Router    /api/v1/users/me [patch].
func (h *Handler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	var req updateUserRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		httphelper.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Email == nil && req.Name == nil {
		httphelper.Error(w, http.StatusBadRequest, "provide email or name to update")
		return
	}

	current, err := h.store.Users.Get(r.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			httphelper.Error(w, http.StatusNotFound, "user not found")
			return
		}
		h.internalError(w, r, "load user for update", err)
		return
	}
	email, name := current.Email, current.Name
	if req.Email != nil {
		email = strings.TrimSpace(*req.Email)
		address, parseErr := mail.ParseAddress(email)
		if parseErr != nil || address.Address != email {
			httphelper.Error(w, http.StatusBadRequest, "valid email is required")
			return
		}
	}
	if req.Name != nil {
		name = *req.Name
	}
	user, err := h.store.Users.Update(r.Context(), userID, email, name)
	if err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			httphelper.Error(w, http.StatusConflict, "email is already registered")
			return
		}
		if errors.Is(err, store.ErrUserNotFound) {
			httphelper.Error(w, http.StatusNotFound, "user not found")
			return
		}
		h.internalError(w, r, "update user", err)
		return
	}
	httphelper.JSON(w, http.StatusOK, user)
}

// GetUser godoc
// @Summary   Get the current user
// @Tags      users
// @Produce   json
// @Security  BearerAuth
// @Success   200  {object}  store.User
// @Failure   401  {object}  map[string]string
// @Failure   404  {object}  map[string]string
// @Failure   500  {object}  map[string]string
// @Router    /api/v1/users/me [get].
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	user, err := h.store.Users.Get(r.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			httphelper.Error(w, http.StatusNotFound, "user not found")
			return
		}
		h.internalError(w, r, "get user", err)
		return
	}
	httphelper.JSON(w, http.StatusOK, user)
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	h.logger.ErrorContext(r.Context(), operation, "error", err)
	httphelper.Error(w, http.StatusInternalServerError, "internal server error")
}
