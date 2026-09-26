package handlers

import (
	"errors"
	"net/http"
	"strings"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	storehelper "github.com/JspBack/webfp-drive/helper/store"
	"github.com/JspBack/webfp-drive/store"
	"github.com/google/uuid"
)

type createFolderRequest struct {
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id"`
}

type updateFolderRequest struct {
	Name string `json:"name"`
}

// CreateFolder godoc
// @Summary   Create a folder
// @Tags      folders
// @Accept    json
// @Produce   json
// @Security  BearerAuth
// @Param     request  body      createFolderRequest  true  "Folder details"
// @Success   201      {object}  store.Folder
// @Failure   400      {object}  map[string]string
// @Failure   401      {object}  map[string]string
// @Failure   404      {object}  map[string]string  "Parent folder not found"
// @Failure   409      {object}  map[string]string  "Folder name already exists"
// @Failure   500      {object}  map[string]string
// @Router    /api/v1/folders [post]
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
		if _, err := h.store.Folders.Get(r.Context(), userID, id); err != nil {
			h.folderError(w, r, "get parent folder", err)
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
// @Router       /api/v1/folders [get]
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
// @Router    /api/v1/folders/{id} [get]
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
// @Summary   Rename a folder
// @Tags      folders
// @Accept    json
// @Produce   json
// @Security  BearerAuth
// @Param     id       path      string               true  "Folder UUID"
// @Param     request  body      updateFolderRequest  true  "New folder name"
// @Success   200      {object}  store.Folder
// @Failure   400      {object}  map[string]string
// @Failure   401      {object}  map[string]string
// @Failure   404      {object}  map[string]string
// @Failure   409      {object}  map[string]string  "Folder name already exists"
// @Failure   500      {object}  map[string]string
// @Router    /api/v1/folders/{id} [patch]
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
	if strings.TrimSpace(req.Name) == "" {
		httphelper.Error(w, http.StatusBadRequest, "folder name is required")
		return
	}
	folder, err := h.store.Folders.Update(r.Context(), userID, folderID, req.Name)
	if err != nil {
		if storehelper.IsUniqueViolation(err) {
			httphelper.Error(w, http.StatusConflict, "folder with this name already exists")
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
// @Router       /api/v1/folders/{id} [delete]
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
