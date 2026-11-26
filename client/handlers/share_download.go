package handlers

import (
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"mime"
	"net/http"

	httphelper "github.com/JspBack/webfp-drive/helper/http"
	"github.com/JspBack/webfp-drive/store"
)

//go:embed share_password.html
var sharePasswordHTML string

var sharePasswordPage = template.Must(template.New("share-password").Parse(sharePasswordHTML))

type downloadShareRequest struct {
	Password string `json:"password"`
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
