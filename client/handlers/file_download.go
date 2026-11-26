package handlers

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	"github.com/JspBack/webfp-drive/store"
)

// DownloadFile godoc
// @Summary      Download a file
// @Description  Streams the file from private R2 storage through the API server.
// @Tags         files
// @Security     BearerAuth
// @Param        id  path  string  true  "File UUID"
// @Produce      octet-stream
// @Success      200  {file}    file  "File contents"
// @Failure      400  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Router       /api/v1/files/{id}/download [get].
func (h *Handler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	fileID, ok := httphelper.PathUUID(w, r, "id", "invalid file ID")
	if !ok {
		return
	}
	file, body, err := h.store.Files.Download(r.Context(), userID, fileID)
	if err != nil {
		h.fileError(w, r, "download file", err)
		return
	}
	h.streamFile(w, r, file, body)
}

func (h *Handler) streamFile(w http.ResponseWriter, r *http.Request, file *store.File, body io.ReadCloser) {
	defer func() {
		if err := body.Close(); err != nil {
			h.logger.ErrorContext(r.Context(), "close downloaded R2 object", "error", err)
		}
	}()

	contentDisposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.Filename})
	if contentDisposition == "" {
		h.internalError(w, r, "format file download headers", fmt.Errorf("invalid file name %q", file.Filename))
		return
	}
	contentType := file.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", contentDisposition)
	w.Header().Set("Content-Length", strconv.FormatInt(file.SizeBytes, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, body); err != nil {
		h.logger.ErrorContext(r.Context(), "stream file download", "error", err)
	}
}
