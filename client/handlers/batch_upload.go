package handlers

import (
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/google/uuid"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	uploadhelper "github.com/JspBack/webfp-drive/helper/upload"
	"github.com/JspBack/webfp-drive/store"
)

const (
	maxBatchUploadBytes        = 60 << 20
	batchMultipartHeadroom     = 1 << 20
	batchUploadWorkers         = 4
	maxConcurrentUploadBatches = 4
)

func (h *Handler) maxBatchFileSize() int64 {
	return min(h.maxFileSize, maxBatchUploadBytes-batchMultipartHeadroom)
}

type uploadBatchResult struct {
	Error string `json:"error,omitempty"`
}

// UploadBatch godoc
// @Summary   Upload multiple files to one folder
// @Tags      files
// @Accept    mpfd
// @Produce   json
// @Security  BearerAuth
// @Param     file       formData  file    true   "Repeated file field, up to 100 files"
// @Param     folder_id  query     string  false  "Destination folder UUID"
// @Success   201        {array}   uploadBatchResult
// @Failure   400        {object}  map[string]string
// @Router    /api/v1/files/batch [post].
func (h *Handler) UploadBatch(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	var folderID *uuid.UUID
	if raw := r.URL.Query().Get("folder_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httphelper.Error(w, http.StatusBadRequest, "invalid folder_id")
			return
		}
		folderID = &id
	}
	select {
	case h.batchUploads <- struct{}{}:
		defer func() { <-h.batchUploads }()
	default:
		w.Header().Set("Retry-After", "5")
		httphelper.Error(w, http.StatusTooManyRequests, "upload server is busy")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBatchUploadBytes)
	files, err := uploadhelper.ParseBatch(r, h.maxBatchFileSize())
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) || errors.Is(err, uploadhelper.ErrFileTooLarge) {
			httphelper.Error(w, http.StatusRequestEntityTooLarge, "upload batch exceeds the size limit")
			return
		}
		httphelper.Error(w, http.StatusBadRequest, "invalid multipart batch")
		return
	}
	defer func() {
		if cleanupErr := uploadhelper.CleanupBatch(files); cleanupErr != nil {
			h.logger.ErrorContext(r.Context(), "clean up batch upload", "error", cleanupErr)
		}
	}()

	results := make([]uploadBatchResult, len(files))
	var workers sync.WaitGroup
	for worker := range min(batchUploadWorkers, len(files)) {
		workers.Go(func() {
			for i := worker; i < len(files); i += batchUploadWorkers {
				if r.Context().Err() != nil {
					return
				}
				_, _, results[i].Error = h.storeUploadedFile(r, ownerID, folderID, files[i])
			}
		})
	}
	workers.Wait()
	httphelper.JSON(w, http.StatusCreated, results)
}

func (h *Handler) storeUploadedFile(
	r *http.Request, ownerID uuid.UUID, folderID *uuid.UUID, upload *uploadhelper.MultipartUpload,
) (*store.File, int, string) {
	if upload.Size == 0 {
		return nil, http.StatusBadRequest, "file must not be empty"
	}
	if _, err := upload.File.Seek(0, io.SeekStart); err != nil {
		h.logger.ErrorContext(r.Context(), "rewind uploaded file", "error", err)
		return nil, http.StatusInternalServerError, "unable to read the uploaded file"
	}
	file, err := h.store.Files.Upload(r.Context(), ownerID, folderID, upload.Filename, upload.ContentType, upload.Size, upload.File)
	switch {
	case err == nil:
		return file, http.StatusCreated, ""
	case errors.Is(err, store.ErrFolderNotFound):
		return nil, http.StatusNotFound, "folder not found"
	case errors.Is(err, store.ErrStorageLimitExceeded):
		return nil, http.StatusRequestEntityTooLarge, "this upload would exceed your storage plan limit"
	case errors.Is(err, store.ErrSystemLocked):
		return nil, http.StatusServiceUnavailable, "system storage limit reached; file operations are temporarily locked"
	default:
		h.logger.ErrorContext(r.Context(), "store uploaded file", "error", err)
		return nil, http.StatusInternalServerError, "unable to upload the file"
	}
}
