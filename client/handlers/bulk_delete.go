package handlers

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	"github.com/JspBack/webfp-drive/store"
)

type deleteBatchRequest struct {
	IDs []uuid.UUID `json:"ids"`
}

type deleteBatchResult struct {
	ID    uuid.UUID `json:"id"`
	Error string    `json:"error,omitempty"`
}

func (h *Handler) decodeDeleteBatch(w http.ResponseWriter, r *http.Request) ([]uuid.UUID, bool) {
	var req deleteBatchRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		httphelper.Error(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	if len(req.IDs) == 0 || len(req.IDs) > store.MaxBulkDeleteItems {
		httphelper.Error(w, http.StatusBadRequest, "send between 1 and 1000 IDs")
		return nil, false
	}
	seen := make(map[uuid.UUID]bool, len(req.IDs))
	for _, id := range req.IDs {
		if id == uuid.Nil || seen[id] {
			httphelper.Error(w, http.StatusBadRequest, "invalid or duplicate ID")
			return nil, false
		}
		seen[id] = true
	}
	return req.IDs, true
}

// DeleteFileBatch godoc
// @Summary   Delete files in one request
// @Tags      files
// @Accept    json
// @Produce   json
// @Security  BearerAuth
// @Param     request  body      deleteBatchRequest  true  "Up to 1000 file IDs"
// @Success   200      {array}   deleteBatchResult
// @Failure   400      {object}  map[string]string
// @Router    /api/v1/files/batch [delete].
func (h *Handler) DeleteFileBatch(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	ids, ok := h.decodeDeleteBatch(w, r)
	if !ok {
		return
	}
	failures, err := h.store.Files.DeleteFiles(r.Context(), ownerID, ids)
	if err != nil {
		h.internalError(w, r, "bulk delete files", err)
		return
	}
	results := make([]deleteBatchResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, deleteBatchResult{ID: id, Error: failures[id]})
	}
	httphelper.JSON(w, http.StatusOK, results)
}

// DeleteFolderBatch godoc
// @Summary   Delete empty folders in one request
// @Tags      folders
// @Accept    json
// @Produce   json
// @Security  BearerAuth
// @Param     request  body      deleteBatchRequest  true  "Up to 1000 folder IDs"
// @Success   200      {array}   deleteBatchResult
// @Failure   400      {object}  map[string]string
// @Router    /api/v1/folders/batch [delete].
func (h *Handler) DeleteFolderBatch(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	ids, ok := h.decodeDeleteBatch(w, r)
	if !ok {
		return
	}
	results := make([]deleteBatchResult, 0, len(ids))
	for _, id := range ids {
		_, message := h.deleteFolder(r, ownerID, id)
		results = append(results, deleteBatchResult{ID: id, Error: message})
	}
	httphelper.JSON(w, http.StatusOK, results)
}

func (h *Handler) deleteFolder(r *http.Request, ownerID, id uuid.UUID) (int, string) {
	err := h.store.Folders.Delete(r.Context(), ownerID, id)
	switch {
	case err == nil:
		return http.StatusNoContent, ""
	case errors.Is(err, store.ErrFolderNotEmpty):
		return http.StatusConflict, "folder must be empty before deletion"
	case errors.Is(err, store.ErrFolderNotFound):
		return http.StatusNotFound, "folder not found"
	default:
		h.logger.ErrorContext(r.Context(), "delete folder", "error", err)
		return http.StatusInternalServerError, "unable to delete the folder"
	}
}
