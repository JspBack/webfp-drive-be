package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	uploadhelper "github.com/JspBack/webfp-drive/helper/upload"
	"github.com/JspBack/webfp-drive/store"
)

type updateFileRequest struct {
	Filename    *string `json:"filename,omitempty"`
	FolderID    *string `json:"folder_id,omitempty"`
	folderIDSet bool
}

func (req *updateFileRequest) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("decode file update fields: %w", err)
	}
	for name, value := range fields {
		switch name {
		case "filename":
			if err := json.Unmarshal(value, &req.Filename); err != nil {
				return fmt.Errorf("decode filename: %w", err)
			}
		case "folder_id":
			req.folderIDSet = true
			if err := json.Unmarshal(value, &req.FolderID); err != nil {
				return fmt.Errorf("decode folder_id: %w", err)
			}
		default:
			return fmt.Errorf("unknown field %q", name)
		}
	}
	return nil
}

// CreateFile godoc
// @Summary      Upload a file
// @Description  Uploads a file as multipart form data through the API server to private R2 storage.
// @Tags         files
// @Accept       mpfd
// @Produce      json
// @Security     BearerAuth
// @Param        file       formData  file    true   "File contents"
// @Param        folder_id  formData  string  false  "Destination folder UUID"
// @Success      201        {object}  store.File
// @Failure      400        {object}  map[string]string
// @Failure      401        {object}  map[string]string
// @Failure      404        {object}  map[string]string  "Folder not found"
// @Failure      413        {object}  map[string]string  "File exceeds configured limit"
// @Failure      500        {object}  map[string]string
// @Router       /api/v1/files [post].
func (h *Handler) CreateFile(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxFileSize+h.maxRequestBodyBytes)
	upload, err := uploadhelper.ParseMultipart(r, h.maxFileSize, h.maxRequestBodyBytes)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.Is(err, uploadhelper.ErrFileTooLarge) || errors.As(err, &maxBytesErr) {
			httphelper.Error(w, http.StatusRequestEntityTooLarge, "upload request exceeds configured size limit")
			return
		}
		h.logger.ErrorContext(r.Context(), "parse multipart upload", "error", err)
		httphelper.Error(w, http.StatusBadRequest, "invalid multipart upload")
		return
	}
	defer func() {
		if cleanupErr := upload.Cleanup(); cleanupErr != nil {
			h.logger.ErrorContext(r.Context(), "clean up uploaded file", "error", cleanupErr)
		}
	}()

	var folderID *uuid.UUID
	if upload.FolderID != "" {
		id, parseErr := uuid.Parse(upload.FolderID)
		if parseErr != nil {
			httphelper.Error(w, http.StatusBadRequest, "invalid folder_id")
			return
		}
		folderID = &id
	}
	if _, seekErr := upload.File.Seek(0, io.SeekStart); seekErr != nil {
		h.internalError(w, r, "rewind uploaded file", seekErr)
		return
	}
	file, err := h.store.Files.Upload(
		r.Context(), userID, folderID, upload.Filename, upload.ContentType, upload.Size, upload.File,
	)
	if err != nil {
		if errors.Is(err, store.ErrFolderNotFound) {
			httphelper.Error(w, http.StatusNotFound, "folder not found")
			return
		}
		h.internalError(w, r, "create file upload", err)
		return
	}
	httphelper.JSON(w, http.StatusCreated, file)
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
// @Router       /api/v1/files [get].
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
// @Tags         files
// @Produce      json
// @Security     BearerAuth
// @Param     id   path      string  true  "File UUID"
// @Success   200  {object}  store.File
// @Failure   400  {object}  map[string]string
// @Failure   401  {object}  map[string]string
// @Failure   404  {object}  map[string]string
// @Failure   500  {object}  map[string]string
// @Router    /api/v1/files/{id} [get].
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
// @Description  Streams the file from private R2 storage through the API server.
// @Tags         files
// @Security     BearerAuth
// @Param        id  path  string  true  "File UUID"
// @Produce      octet-stream
// @Success      200  {file}    file  "File contents"
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      500  {object}  map[string]string
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

// UpdateFile godoc
// @Summary      Rename or move a file
// @Description  Updates a file name, moves it to a folder, or both. Set folder_id to null to move the file to the root.
// @Tags      files
// @Accept       json
// @Produce   json
// @Security  BearerAuth
// @Param        id       path      string             true  "File UUID"
// @Param        request  body      updateFileRequest  true  "File name and/or destination folder"
// @Success      200      {object}  store.File
// @Failure      400      {object}  map[string]string
// @Failure      401      {object}  map[string]string
// @Failure      404      {object}  map[string]string
// @Failure      500      {object}  map[string]string
// @Router       /api/v1/files/{id} [patch].
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
	if req.Filename == nil && !req.folderIDSet {
		httphelper.Error(w, http.StatusBadRequest, "provide filename and/or folder_id")
		return
	}
	if req.Filename != nil && strings.TrimSpace(*req.Filename) == "" {
		httphelper.Error(w, http.StatusBadRequest, "filename must not be empty")
		return
	}
	var folderID *uuid.UUID
	if req.folderIDSet && req.FolderID != nil {
		id, parseErr := uuid.Parse(*req.FolderID)
		if parseErr != nil {
			httphelper.Error(w, http.StatusBadRequest, "invalid folder_id")
			return
		}
		folderID = &id
	}
	file, err := h.store.Files.Update(r.Context(), userID, fileID, req.Filename, folderID, req.folderIDSet)
	if err != nil {
		if errors.Is(err, store.ErrFolderNotFound) {
			httphelper.Error(w, http.StatusNotFound, "folder not found")
			return
		}
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
// @Router       /api/v1/files/{id} [delete].
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
