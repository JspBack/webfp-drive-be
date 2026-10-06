package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	storehelper "github.com/JspBack/webfp-drive/helper/store"
	"github.com/JspBack/webfp-drive/store"
)

type createFolderRequest struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id"`
}

type updateFolderRequest struct {
	Name        *string `json:"name,omitempty"`
	ParentID    *string `json:"parent_id,omitempty"`
	parentIDSet bool
}

func (req *updateFolderRequest) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("decode folder update fields: %w", err)
	}
	for name, value := range fields {
		switch name {
		case "name":
			if err := json.Unmarshal(value, &req.Name); err != nil {
				return fmt.Errorf("decode name: %w", err)
			}
		case "parent_id":
			req.parentIDSet = true
			if err := json.Unmarshal(value, &req.ParentID); err != nil {
				return fmt.Errorf("decode parent_id: %w", err)
			}
		default:
			return fmt.Errorf("unknown field %q", name)
		}
	}
	return nil
}

// CreateFolder godoc
// @Summary   Create a folder
// @Tags         folders
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param     request  body      createFolderRequest  true  "Folder details"
// @Success   201      {object}  store.Folder
// @Failure      400      {object}  map[string]string
// @Failure      401      {object}  map[string]string
// @Failure      404      {object}  map[string]string  "Parent folder not found"
// @Failure      409      {object}  map[string]string  "Folder name already exists"
// @Failure      500      {object}  map[string]string
// @Router    /api/v1/folders [post].
func (h *Handler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	var req createFolderRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		httphelper.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		httphelper.Error(w, http.StatusBadRequest, "folder name is required")
		return
	}
	var parentID *uuid.UUID
	if req.ParentID != nil {
		id, err := uuid.Parse(*req.ParentID)
		if err != nil {
			httphelper.Error(w, http.StatusBadRequest, "invalid parent_id")
			return
		}
		parentID = &id
	}
	folder, err := h.store.Folders.Create(r.Context(), userID, parentID, req.Name)
	if err != nil {
		if storehelper.IsUniqueViolation(err) {
			httphelper.Error(w, http.StatusConflict, "folder with this name already exists")
			return
		}
		if errors.Is(err, store.ErrFolderParentNotFound) {
			httphelper.Error(w, http.StatusNotFound, "parent folder not found")
			return
		}
		h.internalError(w, r, "create folder", err)
		return
	}
	httphelper.JSON(w, http.StatusCreated, folder)
}

// ListFolders godoc
// @Summary      List folders
// @Description  Lists the current user's folders, optionally filtered by parent. Omit parent_id to list root folders.
// @Tags         folders
// @Produce      json
// @Security     BearerAuth
// @Param        parent_id  query     string  false  "Parent folder UUID"
// @Success      200        {array}   store.Folder
// @Failure      400        {object}  map[string]string
// @Failure      401        {object}  map[string]string
// @Failure      500        {object}  map[string]string
// @Router       /api/v1/folders [get].
func (h *Handler) ListFolders(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	parentID, ok := httphelper.OptionalQueryUUID(w, r, "parent_id", "invalid parent_id")
	if !ok {
		return
	}
	folders, err := h.store.Folders.List(r.Context(), userID, parentID)
	if err != nil {
		h.internalError(w, r, "list folders", err)
		return
	}
	httphelper.JSON(w, http.StatusOK, folders)
}

// GetFolder godoc
// @Summary   Get folder metadata
// @Tags      folders
// @Produce   json
// @Security  BearerAuth
// @Param     id   path      string  true  "Folder UUID"
// @Success   200  {object}  store.Folder
// @Failure   400  {object}  map[string]string
// @Failure   401  {object}  map[string]string
// @Failure   404  {object}  map[string]string
// @Failure   500  {object}  map[string]string
// @Router    /api/v1/folders/{id} [get].
func (h *Handler) GetFolder(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	folderID, ok := httphelper.PathUUID(w, r, "id", "invalid folder ID")
	if !ok {
		return
	}
	folder, err := h.store.Folders.Get(r.Context(), userID, folderID)
	if err != nil {
		h.folderError(w, r, "get folder", err)
		return
	}
	httphelper.JSON(w, http.StatusOK, folder)
}

// UpdateFolder godoc
// @Summary      Rename or move a folder
// @Description  Renames or moves a folder. Set parent_id to null for root; folders cannot move into themselves or descendants.
// @Tags      folders
// @Accept    json
// @Produce   json
// @Security  BearerAuth
// @Param        id       path      string               true  "Folder UUID"
// @Param        request  body      updateFolderRequest  true  "Folder name and/or destination parent"
// @Success      200      {object}  store.Folder
// @Failure   400      {object}  map[string]string
// @Failure   401      {object}  map[string]string
// @Failure   404      {object}  map[string]string
// @Failure   409      {object}  map[string]string  "Folder name already exists"
// @Failure   500      {object}  map[string]string
// @Router       /api/v1/folders/{id} [patch].
func (h *Handler) UpdateFolder(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	folderID, ok := httphelper.PathUUID(w, r, "id", "invalid folder ID")
	if !ok {
		return
	}
	var req updateFolderRequest
	if err := h.decodeJSON(w, r, &req); err != nil {
		httphelper.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == nil && !req.parentIDSet {
		httphelper.Error(w, http.StatusBadRequest, "provide name and/or parent_id")
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		httphelper.Error(w, http.StatusBadRequest, "folder name must not be empty")
		return
	}
	var parentID *uuid.UUID
	if req.parentIDSet && req.ParentID != nil {
		id, parseErr := uuid.Parse(*req.ParentID)
		if parseErr != nil {
			httphelper.Error(w, http.StatusBadRequest, "invalid parent_id")
			return
		}
		parentID = &id
	}
	folder, err := h.store.Folders.Update(
		r.Context(), userID, folderID, req.Name, parentID, req.parentIDSet,
	)
	if err != nil {
		if storehelper.IsUniqueViolation(err) {
			httphelper.Error(w, http.StatusConflict, "folder with this name already exists")
			return
		}
		if errors.Is(err, store.ErrFolderMoveCycle) {
			httphelper.Error(w, http.StatusBadRequest, "cannot move a folder into itself or one of its descendants")
			return
		}
		if errors.Is(err, store.ErrFolderParentNotFound) {
			httphelper.Error(w, http.StatusNotFound, "parent folder not found")
			return
		}
		h.folderError(w, r, "update folder", err)
		return
	}
	httphelper.JSON(w, http.StatusOK, folder)
}

// DeleteFolder godoc
// @Summary      Delete an empty folder
// @Description  Folders must have no child folders or files before deletion.
// @Tags         folders
// @Security     BearerAuth
// @Param        id   path  string  true  "Folder UUID"
// @Success      204  "Folder deleted"
// @Failure      400  {object}  map[string]string
// @Failure      401  {object}  map[string]string
// @Failure      404  {object}  map[string]string
// @Failure      409  {object}  map[string]string  "Folder is not empty"
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/folders/{id} [delete].
func (h *Handler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	userID, ok := authhelper.AuthenticatedUserID(w, r)
	if !ok {
		return
	}
	folderID, ok := httphelper.PathUUID(w, r, "id", "invalid folder ID")
	if !ok {
		return
	}
	if err := h.store.Folders.Delete(r.Context(), userID, folderID); err != nil {
		if errors.Is(err, store.ErrFolderNotEmpty) {
			httphelper.Error(w, http.StatusConflict, "folder must be empty before deletion")
			return
		}
		h.folderError(w, r, "delete folder", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) folderError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if errors.Is(err, store.ErrFolderNotFound) {
		httphelper.Error(w, http.StatusNotFound, "folder not found")
		return
	}
	h.internalError(w, r, operation, err)
}
