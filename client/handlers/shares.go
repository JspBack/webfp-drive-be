package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	"github.com/JspBack/webfp-drive/store"
)

type createShareRequest struct {
	ExpiresAt    time.Time `json:"expires_at"`
	MaxDownloads *int64    `json:"max_downloads"`
	Password     string    `json:"password"`
}

func (req *createShareRequest) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("decode share options: %w", err)
	}
	if fields == nil {
		return errors.New("share options must be a JSON object")
	}
	for name, value := range fields {
		switch name {
		case "expires_at":
			var timestamp string
			if err := json.Unmarshal(value, &timestamp); err != nil {
				return errors.New("expires_at must be an RFC3339 timestamp, such as 2026-09-26T05:00:00Z")
			}
			expiresAt, err := time.Parse(time.RFC3339Nano, timestamp)
			if err != nil {
				return errors.New("expires_at must be an RFC3339 timestamp, such as 2026-09-26T05:00:00Z")
			}
			req.ExpiresAt = expiresAt
		case "max_downloads":
			if err := json.Unmarshal(value, &req.MaxDownloads); err != nil {
				return fmt.Errorf("max_downloads must be a positive integer: %w", err)
			}
		case "password":
			if err := json.Unmarshal(value, &req.Password); err != nil {
				return fmt.Errorf("password must be a string: %w", err)
			}
		default:
			return fmt.Errorf("unknown share option %q", name)
		}
	}
	return nil
}

type createShareResponse struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
	URL       string    `json:"url"`
}

// CreateShare godoc
// @Summary      Create a share link
// @Description  Creates a temporary public link for a ready file.
// @Description  Password-protected links display a browser password form.
// @Description  expires_at must be an RFC3339 timestamp.
// @Tags         shares
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id       path      string              true  "File UUID"
// @Param        request  body      createShareRequest  true  "Share options"
// @Success      201      {object}  createShareResponse
// @Failure      400      {object}  map[string]string
// @Failure      401      {object}  map[string]string
// @Failure      404      {object}  map[string]string  "File not found"
// @Failure      409      {object}  map[string]string  "File is not ready"
// @Failure      500      {object}  map[string]string
// @Router       /api/v1/files/{id}/shares [post].
func (h *Handler) CreateShare(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	fileID, ok := httphelper.PathUUID(w, r, "id", "invalid file ID")
	if !ok {
		return
	}
	var req createShareRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		httphelper.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ExpiresAt.IsZero() {
		httphelper.Error(w, http.StatusBadRequest, "expires_at is required")
		return
	}
	host := h.publicHost
	if host == "" {
		host = r.Host
	}
	if _, err := httphelper.AbsoluteURL(h.publicScheme, host, "/"); err != nil {
		h.internalError(w, r, "build public share URL", err)
		return
	}
	share, err := h.store.Shares.CreateShare(
		r.Context(), userID, fileID, req.ExpiresAt, req.MaxDownloads, req.Password,
	)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httphelper.Error(w, http.StatusNotFound, "file not found")
			return
		}
		if errors.Is(err, store.ErrInvalidShareOptions) {
			httphelper.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, store.ErrShareFileNotReady) {
			httphelper.Error(w, http.StatusConflict, "only uploaded files can be shared")
			return
		}
		h.internalError(w, r, "create file share", err)
		return
	}
	shareURL, err := httphelper.AbsoluteURL(h.publicScheme, host, "/api/v1/shares/"+share.Token)
	if err != nil {
		h.internalError(w, r, "build public share URL", err)
		return
	}
	httphelper.JSON(w, http.StatusCreated, createShareResponse{
		ID: share.ID.String(), ExpiresAt: share.ExpiresAt, URL: shareURL,
	})
}

type shareListItemResponse struct {
	ID            string    `json:"id"`
	FileID        string    `json:"file_id"`
	Filename      string    `json:"filename"`
	URL           string    `json:"url"`
	ExpiresAt     time.Time `json:"expires_at"`
	MaxDownloads  int64     `json:"max_downloads"`
	DownloadCount int64     `json:"download_count"`
	HasPassword   bool      `json:"has_password"`
}

// ListShares godoc
// @Summary      List share links
// @Description  Lists the current user's active share links with their public URLs.
// @Tags         shares
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   shareListItemResponse
// @Failure      401  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/shares [get].
func (h *Handler) ListShares(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	host := h.publicHost
	if host == "" {
		host = r.Host
	}
	shares, err := h.store.Shares.ListShares(r.Context(), userID)
	if err != nil {
		h.internalError(w, r, "list shares", err)
		return
	}
	items := make([]shareListItemResponse, 0, len(shares))
	for _, share := range shares {
		if share.Token == "" {
			continue
		}
		shareURL, urlErr := httphelper.AbsoluteURL(h.publicScheme, host, "/api/v1/shares/"+share.Token)
		if urlErr != nil {
			h.internalError(w, r, "build public share URL", urlErr)
			return
		}
		items = append(items, shareListItemResponse{
			ID:            share.ID.String(),
			FileID:        share.FileID.String(),
			Filename:      share.Filename,
			URL:           shareURL,
			ExpiresAt:     share.ExpiresAt,
			MaxDownloads:  share.MaxDownloads,
			DownloadCount: share.DownloadCount,
			HasPassword:   share.HasPassword,
		})
	}
	httphelper.JSON(w, http.StatusOK, items)
}

// RevokeShare godoc
// @Summary   Revoke a share link
// @Tags      shares
// @Security  BearerAuth
// @Param     id   path  string  true  "Share UUID"
// @Success   204  "Share revoked"
// @Failure   400  {object}  map[string]string
// @Failure   401  {object}  map[string]string
// @Failure   404  {object}  map[string]string
// @Failure   500  {object}  map[string]string
// @Router    /api/v1/shares/{id} [delete].
func (h *Handler) RevokeShare(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	shareID, ok := httphelper.PathUUID(w, r, "id", "invalid share ID")
	if !ok {
		return
	}
	if err := h.store.Shares.RevokeShare(r.Context(), userID, shareID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httphelper.Error(w, http.StatusNotFound, "share not found")
			return
		}
		h.internalError(w, r, "revoke share", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
