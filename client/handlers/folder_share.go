package handlers

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	"github.com/JspBack/webfp-drive/store"
)

// CreateFolderShare godoc
// @Summary   Prepare a folder ZIP share
// @Tags      shares
// @Accept    json
// @Produce   json
// @Security  BearerAuth
// @Param     id       path      string              true  "Folder UUID"
// @Param     request  body      createShareRequest  true  "Share options"
// @Success   201      {object}  createShareResponse
// @Failure   400      {object}  map[string]string
// @Failure   404      {object}  map[string]string
// @Router    /api/v1/folders/{id}/shares [post].
func (h *Handler) CreateFolderShare(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	folderID, ok := httphelper.PathUUID(w, r, "id", "invalid folder ID")
	if !ok {
		return
	}
	var req createShareRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		httphelper.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	delivery, ok := h.archiveDelivery(w, r, userID)
	if !ok {
		return
	}
	share, err := h.store.Shares.CreateFolderShare(
		r.Context(), userID, folderID, req.ExpiresAt, req.MaxDownloads, req.Password, delivery,
	)
	switch {
	case errors.Is(err, store.ErrNotFound):
		httphelper.Error(w, http.StatusNotFound, "folder not found")
	case errors.Is(err, store.ErrInvalidShareOptions):
		httphelper.Error(w, http.StatusBadRequest, err.Error())
	case err != nil:
		h.internalError(w, r, "queue folder ZIP", err)
	default:
		httphelper.JSON(w, http.StatusCreated, createShareResponse{
			ID: share.ID.String(), ExpiresAt: share.ExpiresAt, Status: share.Status,
		})
	}
}

func (h *Handler) archiveDelivery(w http.ResponseWriter, r *http.Request, userID uuid.UUID) (store.ArchiveDelivery, bool) {
	if h.mail == nil || !h.mail.Enabled() {
		httphelper.Error(w, http.StatusServiceUnavailable, "ZIP share email delivery is unavailable")
		return store.ArchiveDelivery{}, false
	}
	user, err := h.store.Users.Get(r.Context(), userID)
	if err != nil {
		h.internalError(w, r, "load ZIP share recipient", err)
		return store.ArchiveDelivery{}, false
	}
	if !user.EmailVerified || user.Email == "" {
		httphelper.Error(w, http.StatusBadRequest, "verify an email address before sharing a folder")
		return store.ArchiveDelivery{}, false
	}
	host := h.publicHost
	if host == "" {
		host = r.Host
	}
	prefix, err := httphelper.AbsoluteURL(h.publicScheme, host, "/api/v1/shares/")
	if err != nil {
		h.internalError(w, r, "build ZIP share URL", err)
		return store.ArchiveDelivery{}, false
	}
	return store.ArchiveDelivery{Email: user.Email, LinkPrefix: prefix}, true
}
