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

type createFileUploadResponse struct {
	File      *store.File `json:"file"`
	UploadURL string      `json:"upload_url"`
}

// CreateFileUpload godoc
// @Summary      Create a file upload
// @Description  Creates pending file metadata and returns a temporary presigned R2 PUT URL. Upload file bytes directly to that URL, then complete the upload.
// @Tags         files
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request  body      createFileRequest  true  "File metadata"
// @Success      201      {object}  createFileUploadResponse
// @Failure      400      {object}  map[string]string
// @Failure      401      {object}  map[string]string
// @Failure      404      {object}  map[string]string  "Folder not found"
// @Failure      500      {object}  map[string]string
// @Router       /api/v1/files [post]
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
	httphelper.JSON(w, http.StatusCreated, createFileUploadResponse{File: file, UploadURL: uploadURL})
}

// CompleteFileUpload godoc
// @Summary      Complete a file upload
// @Description  Verifies that the uploaded R2 object matches the declared size and marks it ready.
// @Tags         files
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      string  true  "File UUID"
// @Success      200  {object}  store.File
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string  "File not found or upload unavailable"
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/files/{id}/complete [post]
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

// ListFiles godoc
// @Summary      List files
// @Description  Lists the current user's files, optionally filtered by folder. Omit folder_id to list files at the root.
// @Tags         files
// @Produce      json
// @Security     BearerAuth
// @Param        folder_id  query     string  false  "Folder UUID"
// @Success      200        {array}   store.File
// @Failure      400        {object}  map[string]string
// @Failure      401        {object}  map[string]string
// @Failure      500        {object}  map[string]string
// @Router       /api/v1/files [get]
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

// GetFile godoc
// @Summary   Get file metadata
// @Tags      files
// @Produce   json
// @Security  BearerAuth
// @Param     id   path      string  true  "File UUID"
// @Success   200  {object}  store.File
// @Failure   400  {object}  map[string]string
// @Failure   401  {object}  map[string]string
// @Failure   404  {object}  map[string]string
// @Failure   500  {object}  map[string]string
// @Router    /api/v1/files/{id} [get]
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

// DownloadFile godoc
// @Summary      Download a file
// @Description  Redirects to a short-lived presigned R2 download URL.
// @Tags         files
// @Security     BearerAuth
// @Param        id   path  string  true  "File UUID"
// @Success      307  "Redirect to the presigned download URL"
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/files/{id}/download [get]
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

// UpdateFile godoc
// @Summary   Rename a file
// @Tags      files
// @Accept    json
// @Produce   json
// @Security  BearerAuth
// @Param     id       path      string             true  "File UUID"
// @Param     request  body      updateFileRequest  true  "New file name"
// @Success   200      {object}  store.File
// @Failure   400      {object}  map[string]string
// @Failure   401      {object}  map[string]string
// @Failure   404      {object}  map[string]string
// @Failure   500      {object}  map[string]string
// @Router    /api/v1/files/{id} [patch]
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

// DeleteFile godoc
// @Summary      Delete a file
// @Description  Deletes the stored object and its metadata.
// @Tags         files
// @Security     BearerAuth
// @Param        id   path  string  true  "File UUID"
// @Success      204  "File deleted"
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/files/{id} [delete]
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
