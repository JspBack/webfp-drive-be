package handlers

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"time"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	"github.com/JspBack/webfp-drive/store"
)

const (
	githubStateCookie = "webfp_github_state"
	githubStateTTL    = 10 * time.Minute
)

// LoginGitHub godoc
// @Summary      Start a GitHub sign-in
// @Description  Stores a CSRF state cookie and redirects to GitHub, which calls back /api/v1/callback.
// @Tags         auth
// @Produce      json
// @Success      307  {object}  map[string]string
// @Failure      503  {object}  map[string]string  "GitHub sign-in is not configured"
// @Failure      500  {object}  map[string]string
// @Router       /api/v1/auth/github [get].
func (h *Handler) LoginGitHub(w http.ResponseWriter, r *http.Request) {
	if !h.githubOAuth.Enabled() {
		httphelper.Error(w, http.StatusServiceUnavailable, "GitHub sign-in is not configured")
		return
	}
	state, err := authhelper.NewSessionToken()
	if err != nil {
		h.internalError(w, r, "create GitHub OAuth state", err)
		return
	}
	h.setOAuthCookie(w, state, int(githubStateTTL.Seconds()))
	http.Redirect(w, r, h.githubOAuth.AuthorizeURL(state), http.StatusTemporaryRedirect)
}

// GitHubCallback finishes the GitHub OAuth flow and redirects the browser to
// the frontend with the issued access and refresh tokens in the URL fragment.
func (h *Handler) GitHubCallback(w http.ResponseWriter, r *http.Request) {
	if !h.githubOAuth.Enabled() {
		httphelper.Error(w, http.StatusServiceUnavailable, "GitHub sign-in is not configured")
		return
	}
	if !h.validOAuthState(r) {
		h.setOAuthCookie(w, "", -1)
		h.oauthFailure(w, r, http.StatusBadRequest, "validate GitHub OAuth state", nil,
			"GitHub sign-in could not be verified, please try again")
		return
	}
	h.setOAuthCookie(w, "", -1)

	if r.FormValue("error") != "" {
		h.oauthFailure(w, r, http.StatusBadRequest, "GitHub denied the sign-in request", nil,
			"GitHub sign-in was not completed")
		return
	}
	code := r.FormValue("code")
	if code == "" {
		h.oauthFailure(w, r, http.StatusBadRequest, "read GitHub OAuth callback", nil,
			"GitHub did not return an authorization code")
		return
	}

	accessToken, err := h.githubOAuth.Exchange(r.Context(), code)
	if err != nil {
		h.oauthFailure(w, r, http.StatusBadGateway, "exchange GitHub authorization code", err,
			"GitHub sign-in could not be completed")
		return
	}
	profile, err := h.githubOAuth.Profile(r.Context(), accessToken)
	if err != nil {
		h.oauthFailure(w, r, http.StatusBadGateway, "load GitHub profile", err,
			"GitHub sign-in could not be completed")
		return
	}
	email := ""
	if profile.EmailIsVerified {
		email = profile.Email
	}
	user, err := h.store.Users.LoginGitHub(r.Context(), profile.Subject, email, profile.Name)
	if err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			h.oauthFailure(w, r, http.StatusConflict, "synchronize GitHub user", err,
				"email is already registered with another sign-in method")
			return
		}
		h.oauthFailure(w, r, http.StatusInternalServerError, "synchronize GitHub user", err,
			"GitHub sign-in could not be completed")
		return
	}
	tokens, err := h.store.Users.CreateSession(r.Context(), user.ID)
	if err != nil {
		h.oauthFailure(w, r, http.StatusInternalServerError, "create GitHub login session", err,
			"GitHub sign-in could not be completed")
		return
	}
	http.Redirect(w, r, h.oauthRedirectTarget(url.Values{
		"token":         {tokens.AccessToken},
		"refresh_token": {tokens.RefreshToken},
	}), http.StatusFound)
}

func (h *Handler) validOAuthState(r *http.Request) bool {
	cookie, err := r.Cookie(githubStateCookie)
	if err != nil || cookie.Value == "" {
		return false
	}
	state := r.FormValue("state")
	return state != "" && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) == 1
}

func (h *Handler) setOAuthCookie(w http.ResponseWriter, value string, maxAge int) {
	// #nosec G124 -- Secure follows the configured TLS mode; plain HTTP is local development only.
	http.SetCookie(w, &http.Cookie{
		Name:     githubStateCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.publicScheme == "https",
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) oauthFailure(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	operation string,
	err error,
	message string,
) {
	if err != nil {
		h.logger.ErrorContext(r.Context(), operation, "error", err)
	}
	if h.githubOAuth.ReturnURL() == "" {
		httphelper.Error(w, status, message)
		return
	}
	http.Redirect(w, r, h.oauthRedirectTarget(url.Values{"error": {message}}), http.StatusFound)
}

func (h *Handler) oauthRedirectTarget(params url.Values) string {
	return h.githubOAuth.ReturnURL() + "#" + params.Encode()
}
