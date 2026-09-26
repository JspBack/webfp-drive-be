package handlers

import (
	"errors"
	"net/http"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	"github.com/JspBack/webfp-drive/store"
)

func (h *Handler) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := authhelper.BearerToken(r.Header.Get("Authorization"))
		if !ok {
			httphelper.Error(w, http.StatusUnauthorized, "bearer token required")
			return
		}
		userID, err := h.store.Users.ResolveSession(r.Context(), token)
		if err != nil {
			if errors.Is(err, store.ErrInvalidSession) {
				httphelper.Error(w, http.StatusUnauthorized, "invalid or expired session")
				return
			}
			h.internalError(w, r, "resolve session", err)
			return
		}
		next.ServeHTTP(w, r.WithContext(authhelper.WithSession(r.Context(), userID, token)))
	})
}
