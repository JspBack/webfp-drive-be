package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
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

func (req *createShareRequest) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("decode share options: %w", err)
	}
	if fields == nil {
		return errors.New("share options must be a JSON object")
	}
	for name, value := range fields {
		switch name {
		case "expires_at":
			var timestamp string
			if err := json.Unmarshal(value, &timestamp); err != nil {
				return errors.New("expires_at must be an RFC3339 timestamp, such as 2026-09-26T05:00:00Z")
			}
			expiresAt, err := time.Parse(time.RFC3339Nano, timestamp)
			if err != nil {
				return errors.New("expires_at must be an RFC3339 timestamp, such as 2026-09-26T05:00:00Z")
			}
			req.ExpiresAt = expiresAt
		case "max_downloads":
			if err := json.Unmarshal(value, &req.MaxDownloads); err != nil {
				return fmt.Errorf("max_downloads must be a positive integer: %w", err)
			}
		case "password":
			if err := json.Unmarshal(value, &req.Password); err != nil {
				return fmt.Errorf("password must be a string: %w", err)
			}
		default:
			return fmt.Errorf("unknown share option %q", name)
		}
	}
	return nil
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
	<meta name="color-scheme" content="light">
	<title>Protected file · WebFP Drive</title>
	<style>
		:root {
			font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
			color: #172033;
			background: #f3f6fb;
			font-synthesis: none;
			text-rendering: optimizeLegibility;
			-webkit-font-smoothing: antialiased;
			-moz-osx-font-smoothing: grayscale;
		}
		* { box-sizing: border-box; }
		body {
			display: grid;
			min-height: 100vh;
			margin: 0;
			padding: 24px;
			place-items: center;
			background:
				radial-gradient(ellipse at 50% 0, rgba(88, 125, 255, .12), transparent 55%),
				#f3f6fb;
		}
		.page { width: min(100%, 420px); }
		.brand {
			display: flex;
			align-items: center;
			justify-content: center;
			gap: 10px;
			margin-bottom: 28px;
			color: #27334b;
			font-size: 15px;
			font-weight: 700;
			letter-spacing: -.02em;
		}
		.brand-mark {
			display: grid;
			width: 32px;
			height: 32px;
			border-radius: 10px;
			place-items: center;
			color: #fff;
			background: #536dfe;
			box-shadow: 0 4px 10px rgba(83, 109, 254, .25);
		}
		.card {
			padding: 36px;
			border: 1px solid #e5eaf2;
			border-radius: 18px;
			background: #fff;
			box-shadow: 0 20px 60px rgba(27, 43, 75, .09);
		}
		.icon {
			display: grid;
			width: 52px;
			height: 52px;
			margin-bottom: 24px;
			border-radius: 15px;
			place-items: center;
			color: #536dfe;
			background: #eef1ff;
		}
		h1 {
			margin: 0;
			color: #172033;
			font-size: clamp(23px, 6vw, 28px);
			font-weight: 700;
			letter-spacing: -.04em;
			line-height: 1.2;
		}
		.description {
			margin: 12px 0 26px;
			color: #65718a;
			font-size: 15px;
			line-height: 1.6;
		}
		.error {
			margin: 0 0 20px;
			padding: 12px 14px;
			border: 1px solid #fecaca;
			border-radius: 10px;
			color: #b42318;
			background: #fff5f4;
			font-size: 14px;
			line-height: 1.5;
		}
		label {
			display: block;
			margin-bottom: 8px;
			color: #344054;
			font-size: 13px;
			font-weight: 650;
		}
		input {
			width: 100%;
			height: 48px;
			padding: 0 14px;
			border: 1px solid #d5dce8;
			border-radius: 10px;
			outline: none;
			color: #172033;
			background: #fff;
			font: inherit;
			font-size: 15px;
			transition: border-color .15s ease, box-shadow .15s ease;
		}
		input::placeholder { color: #98a2b3; }
		input:focus {
			border-color: #6378ff;
			box-shadow: 0 0 0 4px rgba(83, 109, 254, .14);
		}
		button {
			display: flex;
			width: 100%;
			height: 48px;
			align-items: center;
			justify-content: center;
			gap: 9px;
			margin-top: 16px;
			border: 0;
			border-radius: 10px;
			color: #fff;
			background: #536dfe;
			box-shadow: 0 4px 9px rgba(83, 109, 254, .2);
			cursor: pointer;
			font: inherit;
			font-size: 14px;
			font-weight: 650;
			transition: background .15s ease, transform .15s ease;
		}
		button:hover { background: #4058e8; }
		button:active { transform: translateY(1px); }
		button:focus-visible {
			outline: 3px solid rgba(83, 109, 254, .35);
			outline-offset: 3px;
		}
		.privacy {
			margin: 22px 0 0;
			color: #8a94a8;
			font-size: 12px;
			line-height: 1.5;
			text-align: center;
		}
		@media (max-width: 480px) {
			body { padding: 20px; }
			.brand { margin-bottom: 22px; }
			.card { padding: 28px 24px; border-radius: 15px; }
		}
	</style>
</head>
<body>
	<main class="page">
		<div class="brand">
			<span class="brand-mark" aria-hidden="true">
				<svg width="19" height="19" viewBox="0 0 24 24" fill="none">
					<path
						d="M5 7.5A2.5 2.5 0 0 1 7.5 5h9A2.5 2.5 0 0 1 19 7.5v9a2.5 2.5 0 0 1-2.5 2.5h-9A2.5 2.5 0 0 1 5 16.5v-9Z"
						stroke="currentColor" stroke-width="1.8"/>
					<path
						d="m8 15 2.7-3 2.1 2 1.5-1.5L17 15M8.5 9.5h.01"
						stroke="currentColor" stroke-width="1.8" stroke-linecap="round"
						stroke-linejoin="round"/>
				</svg>
			</span>
			<span>WebFP Drive</span>
		</div>
		<section class="card" aria-labelledby="title">
			<div class="icon" aria-hidden="true">
				<svg width="25" height="25" viewBox="0 0 24 24" fill="none">
					<rect x="5" y="10" width="14" height="11" rx="2.5" stroke="currentColor" stroke-width="1.8"/>
					<path d="M8 10V7a4 4 0 1 1 8 0v3" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>
					<circle cx="12" cy="15.5" r="1.2" fill="currentColor"/>
				</svg>
			</div>
			<h1 id="title">This file is protected</h1>
			<p class="description">Enter the password provided by the person who shared this file to access it.</p>
			{{if .Message}}<p class="error" id="password-error" role="alert">{{.Message}}</p>{{end}}
			<form method="post" action="{{.Action}}">
				<label for="password">Password</label>
				<input id="password" name="password" type="password"
					autocomplete="current-password" placeholder="Enter the file password"
					aria-describedby="{{if .Message}}password-error{{end}}" required autofocus>
				<button type="submit">
					<span>Unlock file</span>
					<svg width="17" height="17" viewBox="0 0 24 24" fill="none" aria-hidden="true">
						<path d="M5 12h14m-6-6 6 6-6 6" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>
					</svg>
				</button>
			</form>
			<p class="privacy">Your password is only used to unlock this shared file.</p>
		</section>
	</main>
</body>
</html>`))

// CreateShare godoc
// @Summary      Create a share link
// @Description  Creates a temporary public link for a ready file.
// @Description  Password-protected links display a browser password form.
// @Description  expires_at must be an RFC3339 timestamp.
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
// @Router       /api/v1/files/{id}/shares [post].
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
	host := h.publicHost
	if host == "" {
		host = r.Host
	}
	if _, err := httphelper.AbsoluteURL(h.publicScheme, host, "/"); err != nil {
		h.internalError(w, r, "build public share URL", err)
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
	shareURL, err := httphelper.AbsoluteURL(h.publicScheme, host, "/api/v1/shares/"+share.Token)
	if err != nil {
		h.internalError(w, r, "build public share URL", err)
		return
	}
	httphelper.JSON(w, http.StatusCreated, createShareResponse{
		ID: share.ID.String(), ExpiresAt: share.ExpiresAt, URL: shareURL,
	})
}

func (h *Handler) DownloadSharedFile(w http.ResponseWriter, r *http.Request) {
	h.serveSharedFile(w, r, false)
}

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
	file, body, err := h.store.Shares.SharedDownload(r.Context(), token, password)
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
	h.streamFile(w, r, file, body)
}

func (h *Handler) sharePassword(w http.ResponseWriter, r *http.Request) (string, bool, error) {
	mediaType, _, mediaTypeErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaTypeErr != nil {
		return "", false, errors.New("content type must be application/json or application/x-www-form-urlencoded")
	}
	switch mediaType {
	case "application/json":
		var req downloadShareRequest
		if decodeErr := h.decodeJSON(w, r, &req); decodeErr != nil {
			return "", false, decodeErr
		}
		return req.Password, false, nil
	case "application/x-www-form-urlencoded":
		r.Body = http.MaxBytesReader(w, r.Body, h.maxRequestBodyBytes)
		if parseErr := r.ParseForm(); parseErr != nil {
			return "", true, fmt.Errorf("parse share password form: %w", parseErr)
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
// @Router    /api/v1/shares/{id} [delete].
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
