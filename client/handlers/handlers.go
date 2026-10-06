package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	"github.com/JspBack/webfp-drive/store"
)

type Handler struct {
	store               *store.Store
	logger              *slog.Logger
	maxFileSize         int64
	maxRequestBodyBytes int64
	publicHost          string
	publicScheme        string
	githubOAuth         *authhelper.GitHubOAuth
	googleOAuth         *authhelper.GoogleOAuth
}

func New(
	st *store.Store,
	logger *slog.Logger,
	maxFileSize, maxRequestBodyBytes int64,
	publicHost string,
	useTLS bool,
	githubOAuth *authhelper.GitHubOAuth,
	googleOAuth *authhelper.GoogleOAuth,
) *Handler {
	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	return &Handler{
		store:               st,
		logger:              logger,
		maxFileSize:         maxFileSize,
		maxRequestBodyBytes: maxRequestBodyBytes,
		publicHost:          publicHost,
		publicScheme:        scheme,
		githubOAuth:         githubOAuth,
		googleOAuth:         googleOAuth,
	}
}

func (h *Handler) decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	if err := httphelper.DecodeJSONLimit(w, r, dst, h.maxRequestBodyBytes); err != nil {
		return fmt.Errorf("decode request JSON: %w", err)
	}
	return nil
}
