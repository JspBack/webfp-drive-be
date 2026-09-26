package handlers

import (
	"errors"
	"html/template"
	"mime"
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

type downloadShareRequest struct {
	Password string `json:"password"`
}

type createShareResponse struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
	URL       string    `json:"url"`
}

var sharePasswordPage = template.Must(template.New("share-password").Parse(`<!doctype html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>Password-protected share</title>
</head>
<body>
	<main>
		<h1>Password-protected file</h1>
		{{if .Message}}<p role="alert">{{.Message}}</p>{{end}}
		<form method="post" action="{{.Action}}">
			<label for="password">Password</label>
			<input id="password" name="password" type="password" autocomplete="current-password" required>
			<button type="submit">Continue to download</button>
		</form>
	</main>
</body>
</html>`))

// CreateShare godoc
// @Summary      Create a share link
// @Description  Creates a temporary public link for a ready file. The URL opens directly unless a password is configured, in which case it displays a browser password form.
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
// @Router       /api/v1/files/{id}/shares [post]
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
	shareURL := "/api/v1/shares/" + share.Token
	httphelper.JSON(w, http.StatusCreated, createShareResponse{
		ID: share.ID.String(), ExpiresAt: share.ExpiresAt, URL: shareURL,
	})
}

// DownloadSharedFile godoc
// @Summary      Open a shared file
// @Description  Opens a public share. A password-protected share returns an HTML password form; an unprotected share redirects to a short-lived R2 URL. Download limits count URL issuance.
// @Tags         shares
// @Param        token  path      string  true  "Share token"
// @Success      200    {string}  string  "HTML password form for a password-protected share"
// @Success      307    "Redirect to the presigned download URL"
// @Failure      404    {object}  map[string]string  "Share unavailable"
// @Failure      500    {object}  map[string]string
// @Router       /api/v1/shares/{token} [get]
func (h *Handler) DownloadSharedFile(w http.ResponseWriter, r *http.Request) {
	h.serveSharedFile(w, r, false)
}

// SubmitSharePassword godoc
// @Summary      Submit a share password
// @Description  Submits a password and redirects to the short-lived R2 download URL. Accepts JSON for API clients or application/x-www-form-urlencoded for the browser password form. The browser receives the same password form again with an error when the password is incorrect.
// @Tags         shares
// @Accept       json
// @Produce      json
// @Param        token    path  string                true   "Share token"
// @Param        request  body  downloadShareRequest  false  "Password for JSON clients"
// @Success      303      "Redirect to the presigned download URL"
// @Failure      400      {object}  map[string]string
// @Failure      401      {object}  map[string]string  "Password required or incorrect"
// @Failure      404      {object}  map[string]string  "Share unavailable"
// @Failure      500      {object}  map[string]string
// @Router       /api/v1/shares/{token}/download [post]
func (h *Handler) SubmitSharePassword(w http.ResponseWriter, r *http.Request) {
	h.serveSharedFile(w, r, true)
}

func (h *Handler) serveSharedFile(w http.ResponseWriter, r *http.Request, isPasswordPost bool) {
	password := ""
	formSubmission := false
	if isPasswordPost {
		var err error
		password, formSubmission, err = h.sharePassword(w, r)
		if err != nil {
			httphelper.Error(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	token := r.PathValue("token")
	downloadURL, err := h.store.Shares.SharedDownloadURL(r.Context(), token, password)
	if err != nil {
		if errors.Is(err, store.ErrShareUnavailable) {
			httphelper.Error(w, http.StatusNotFound, "share not found or unavailable")
			return
		}
		if errors.Is(err, store.ErrSharePasswordRequired) {
			if r.Method == http.MethodGet {
				h.renderSharePasswordPage(w, http.StatusOK, token, "")
				return
			}
			httphelper.Error(w, http.StatusUnauthorized, "share password is required")
			return
		}
		if errors.Is(err, store.ErrSharePasswordInvalid) {
			if formSubmission {
				h.renderSharePasswordPage(w, http.StatusUnauthorized, token, "That password was incorrect. Please try again.")
				return
			}
			httphelper.Error(w, http.StatusUnauthorized, "share password is incorrect")
			return
		}
		h.internalError(w, r, "download shared file", err)
		return
	}
	status := http.StatusTemporaryRedirect
	if r.Method == http.MethodPost {
		status = http.StatusSeeOther
	}
	http.Redirect(w, r, downloadURL, status)
}

func (h *Handler) sharePassword(w http.ResponseWriter, r *http.Request) (string, bool, error) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return "", false, errors.New("content type must be application/json or application/x-www-form-urlencoded")
	}
	switch mediaType {
	case "application/json":
		var req downloadShareRequest
		if err := h.decodeJSON(w, r, &req); err != nil {
			return "", false, err
		}
		return req.Password, false, nil
	case "application/x-www-form-urlencoded":
		r.Body = http.MaxBytesReader(w, r.Body, h.maxRequestBodyBytes)
		if err := r.ParseForm(); err != nil {
			return "", true, err
		}
		return r.PostForm.Get("password"), true, nil
	default:
		return "", false, errors.New("content type must be application/json or application/x-www-form-urlencoded")
	}
}

func (h *Handler) renderSharePasswordPage(w http.ResponseWriter, status int, token, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	err := sharePasswordPage.Execute(w, struct {
		Action  string
		Message string
	}{
		Action:  "/api/v1/shares/" + token + "/download",
		Message: message,
	})
	if err != nil {
		h.logger.Error("render share password page", "error", err)
	}
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
// @Router    /api/v1/shares/{id} [delete]
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
