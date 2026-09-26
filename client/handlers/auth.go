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
	User  *store.User `json:"user"`
	Token string      `json:"token"`
}

type updateUserRequest struct {
	Email *string `json:"email"`
	Name  *string `json:"name"`
}

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
	if len(req.Password) < 8 {
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
	token, err := h.store.Users.CreateSession(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, "create registration session", err)
		return
	}
	httphelper.JSON(w, http.StatusCreated, authResponse{User: user, Token: token})
}

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
	token, err := h.store.Users.CreateSession(r.Context(), user.ID)
	if err != nil {
		h.internalError(w, r, "create login session", err)
		return
	}
	httphelper.JSON(w, http.StatusOK, authResponse{User: user, Token: token})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	token, _ := authhelper.SessionToken(r.Context())
	if err := h.store.Users.RevokeSession(r.Context(), token); err != nil {
		h.internalError(w, r, "revoke session", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

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
