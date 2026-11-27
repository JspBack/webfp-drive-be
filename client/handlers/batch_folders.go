package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	storehelper "github.com/JspBack/webfp-drive/helper/store"
	"github.com/JspBack/webfp-drive/store"
)

const maxFolderCreateBatch = 100

type createFolderBatchRequest struct {
	Folders []createFolderRequest `json:"folders"`
}

type createFolderBatchResult struct {
	Folder *store.Folder `json:"folder,omitempty"`
	Error  string        `json:"error,omitempty"`
	status int
}

// CreateFolderBatch godoc
// @Summary   Create folders in one request
// @Tags      folders
// @Accept    json
// @Produce   json
// @Security  BearerAuth
// @Param     request  body      createFolderBatchRequest  true  "Up to 100 folders with existing parent IDs"
// @Success   201      {array}   createFolderBatchResult
// @Failure   400      {object}  map[string]string
// @Router    /api/v1/folders/batch [post].
func (h *Handler) CreateFolderBatch(w http.ResponseWriter, r *http.Request) {
	ownerID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	var req createFolderBatchRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		httphelper.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Folders) == 0 || len(req.Folders) > maxFolderCreateBatch {
		httphelper.Error(w, http.StatusBadRequest, "send between 1 and 100 folders")
		return
	}

	results := make([]createFolderBatchResult, 0, len(req.Folders))
	for _, folder := range req.Folders {
		results = append(results, h.createFolder(r, ownerID, folder))
	}
	httphelper.JSON(w, http.StatusCreated, results)
}

func (h *Handler) createFolder(r *http.Request, ownerID uuid.UUID, req createFolderRequest) createFolderBatchResult {
	if strings.TrimSpace(req.Name) == "" {
		return createFolderBatchResult{Error: "Folder name is required", status: http.StatusBadRequest}
	}
	var parentID *uuid.UUID
	if req.ParentID != nil {
		id, parseErr := uuid.Parse(*req.ParentID)
		if parseErr != nil {
			return createFolderBatchResult{Error: "Invalid parent_id", status: http.StatusBadRequest}
		}
		parentID = &id
	}
	folder, err := h.store.Folders.Create(r.Context(), ownerID, parentID, req.Name)
	switch {
	case err == nil:
		return createFolderBatchResult{Folder: folder}
	case storehelper.IsUniqueViolation(err):
		return createFolderBatchResult{Error: "A folder with this name already exists", status: http.StatusConflict}
	case errors.Is(err, store.ErrFolderParentNotFound):
		return createFolderBatchResult{Error: "Parent folder not found", status: http.StatusNotFound}
	default:
		h.logger.ErrorContext(r.Context(), "create folder", "error", err)
		return createFolderBatchResult{Error: "Unable to create the folder", status: http.StatusInternalServerError}
	}
}
