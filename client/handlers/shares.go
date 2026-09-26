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
	httphelper.JSON(w, http.StatusCreated, map[string]any{
		"id":         share.ID,
		"expires_at": share.ExpiresAt,
		"url":        shareURL,
	})
}

func (h *Handler) DownloadSharedFile(w http.ResponseWriter, r *http.Request) {
	password := ""
	formSubmission := false
	if r.Method == http.MethodPost {
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
