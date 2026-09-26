package handlers

import (
	"log/slog"
	"net/http"

	httphelper "github.com/JspBack/webfp-drive/helper/http"
	"github.com/JspBack/webfp-drive/store"
)

type Handler struct {
	store               *store.Store
	logger              *slog.Logger
	maxFileSize         int64
	maxRequestBodyBytes int64
}

func New(st *store.Store, logger *slog.Logger, maxFileSize, maxRequestBodyBytes int64) *Handler {
	return &Handler{
		store:               st,
		logger:              logger,
		maxFileSize:         maxFileSize,
		maxRequestBodyBytes: maxRequestBodyBytes,
	}
}

func (h *Handler) decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	return httphelper.DecodeJSONLimit(w, r, dst, h.maxRequestBodyBytes)
}
