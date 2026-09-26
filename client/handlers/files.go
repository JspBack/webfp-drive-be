package handlers

import (
	"errors"
	"net/http"
	"strings"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	"github.com/JspBack/webfp-drive/store"
	"github.com/google/uuid"
)

type createFileRequest struct {
	Filename    string  `json:"filename"`
	ContentType string  `json:"content_type"`
	SizeBytes   int64   `json:"size_bytes"`
	FolderID    *string `json:"folder_id"`
}

type updateFileRequest struct {
	Filename *string `json:"filename"`
}

func (h *Handler) CreateFileUpload(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	var req createFileRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		httphelper.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.SizeBytes <= 0 || req.SizeBytes > h.maxFileSize {
		httphelper.Error(w, http.StatusBadRequest, "file size must be positive and within the configured limit")
		return
	}
	if strings.TrimSpace(req.Filename) == "" {
		httphelper.Error(w, http.StatusBadRequest, "filename is required")
		return
	}
	var folderID *uuid.UUID
	if req.FolderID != nil {
		id, err := uuid.Parse(*req.FolderID)
		if err != nil {
			httphelper.Error(w, http.StatusBadRequest, "invalid folder_id")
			return
		}
		if _, err := h.store.Folders.Get(r.Context(), userID, id); err != nil {
			if errors.Is(err, store.ErrFolderNotFound) {
				httphelper.Error(w, http.StatusNotFound, "folder not found")
				return
			}
			h.internalError(w, r, "validate upload folder", err)
			return
		}
		folderID = &id
	}
	file, uploadURL, err := h.store.Files.CreateUpload(
		r.Context(), userID, folderID, req.Filename, req.ContentType, req.SizeBytes,
	)
	if err != nil {
		h.internalError(w, r, "create file upload", err)
		return
	}
	httphelper.JSON(w, http.StatusCreated, map[string]any{
		"file":       file,
		"upload_url": uploadURL,
	})
}

func (h *Handler) CompleteFileUpload(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	fileID, ok := httphelper.PathUUID(w, r, "id", "invalid file ID")
	if !ok {
		return
	}
	file, err := h.store.Files.CompleteUpload(r.Context(), userID, fileID)
	if err != nil {
		h.fileError(w, r, "complete file upload", err)
		return
	}
	httphelper.JSON(w, http.StatusOK, file)
}

func (h *Handler) ListFiles(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	folderID, ok := httphelper.OptionalQueryUUID(w, r, "folder_id", "invalid folder_id")
	if !ok {
		return
	}
	files, err := h.store.Files.List(r.Context(), userID, folderID)
	if err != nil {
		h.internalError(w, r, "list files", err)
		return
	}
	httphelper.JSON(w, http.StatusOK, files)
}

func (h *Handler) GetFile(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	fileID, ok := httphelper.PathUUID(w, r, "id", "invalid file ID")
	if !ok {
		return
	}
	file, err := h.store.Files.Get(r.Context(), userID, fileID)
	if err != nil {
		h.fileError(w, r, "get file", err)
		return
	}
	httphelper.JSON(w, http.StatusOK, file)
}

func (h *Handler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	fileID, ok := httphelper.PathUUID(w, r, "id", "invalid file ID")
	if !ok {
		return
	}
	downloadURL, err := h.store.Files.DownloadURL(r.Context(), userID, fileID)
	if err != nil {
		h.fileError(w, r, "create file download URL", err)
		return
	}
	http.Redirect(w, r, downloadURL, http.StatusTemporaryRedirect)
}

func (h *Handler) UpdateFile(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	fileID, ok := httphelper.PathUUID(w, r, "id", "invalid file ID")
	if !ok {
		return
	}
	var req updateFileRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		httphelper.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Filename == nil || strings.TrimSpace(*req.Filename) == "" {
		httphelper.Error(w, http.StatusBadRequest, "filename is required")
		return
	}
	current, err := h.store.Files.Get(r.Context(), userID, fileID)
	if err != nil {
		h.fileError(w, r, "get file for update", err)
		return
	}
	file, err := h.store.Files.Update(r.Context(), userID, fileID, *req.Filename, current.FolderID)
	if err != nil {
		h.fileError(w, r, "update file", err)
		return
	}
	httphelper.JSON(w, http.StatusOK, file)
}

func (h *Handler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	fileID, ok := httphelper.PathUUID(w, r, "id", "invalid file ID")
	if !ok {
		return
	}
	if err := h.store.Files.Delete(r.Context(), userID, fileID); err != nil {
		h.fileError(w, r, "delete file", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) fileError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if errors.Is(err, store.ErrNotFound) {
		httphelper.Error(w, http.StatusNotFound, "file not found")
		return
	}
	h.internalError(w, r, operation, err)
}
