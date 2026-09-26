package handlers

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	httphelper "github.com/JspBack/webfp-drive/helper/http"
	"github.com/JspBack/webfp-drive/store"
)

const (
	githubStateCookie = "webfp_github_state"
	googleStateCookie = "webfp_google_state"
	oauthStateTTL     = 10 * time.Minute
)

// oauthFlow bundles everything needed to start and finish one provider's
// sign-in; both providers share the callback route and the completion logic.
type oauthFlow struct {
	provider   string
	name       string
	cookieName string
	enabled    bool
	returnURL  string
	authorize  func(state string) string
	exchange   func(ctx context.Context, code string) (string, error)
	profile    func(ctx context.Context, accessToken string) (*oauthIdentity, error)
}

type oauthIdentity struct {
	subject  string
	email    string
	verified bool
	name     string
}

func (h *Handler) githubFlow() *oauthFlow {
	return &oauthFlow{
		provider:   "github",
		name:       "GitHub",
		cookieName: githubStateCookie,
		enabled:    h.githubOAuth.Enabled(),
		returnURL:  h.oauthReturnURL(h.githubOAuth.ReturnURL()),
		authorize:  h.githubOAuth.AuthorizeURL,
		exchange:   h.githubOAuth.Exchange,
		profile: func(ctx context.Context, accessToken string) (*oauthIdentity, error) {
			profile, err := h.githubOAuth.Profile(ctx, accessToken)
			if err != nil {
				return nil, fmt.Errorf("load GitHub profile: %w", err)
			}
			return &oauthIdentity{
				subject:  profile.Subject,
				email:    profile.Email,
				verified: profile.EmailIsVerified,
				name:     profile.Name,
			}, nil
		},
	}
}

func (h *Handler) googleFlow() *oauthFlow {
	return &oauthFlow{
		provider:   "google",
		name:       "Google",
		cookieName: googleStateCookie,
		enabled:    h.googleOAuth.Enabled(),
		returnURL:  h.oauthReturnURL(h.googleOAuth.ReturnURL()),
		authorize:  h.googleOAuth.AuthorizeURL,
		exchange:   h.googleOAuth.Exchange,
		profile: func(ctx context.Context, accessToken string) (*oauthIdentity, error) {
			profile, err := h.googleOAuth.Profile(ctx, accessToken)
			if err != nil {
				return nil, fmt.Errorf("load Google profile: %w", err)
			}
			return &oauthIdentity{
				subject:  profile.Subject,
				email:    profile.Email,
				verified: profile.EmailIsVerified,
				name:     profile.Name,
			}, nil
		},
	}
}

// LoginGitHub godoc
// @Summary      Start a GitHub sign-in
// @Description  Stores a CSRF state cookie and redirects to GitHub, which calls back /api/v1/callback.
// @Tags         auth
// @Produce      json
// @Success      307  {object}  map[string]string
// @Failure      503  {object}  map[string]string  "GitHub sign-in is not configured"
// @Router       /api/v1/auth/github [get].
func (h *Handler) LoginGitHub(w http.ResponseWriter, r *http.Request) {
	h.startOAuth(w, r, h.githubFlow())
}

// LoginGoogle godoc
// @Summary      Start a Google sign-in
// @Description  Stores a CSRF state cookie and redirects to Google, which calls back /api/v1/callback.
// @Tags         auth
// @Produce      json
// @Success      307  {object}  map[string]string
// @Failure      503  {object}  map[string]string  "Google sign-in is not configured"
// @Router       /api/v1/auth/google [get].
func (h *Handler) LoginGoogle(w http.ResponseWriter, r *http.Request) {
	h.startOAuth(w, r, h.googleFlow())
}

// OAuthCallback finishes an OAuth flow. Both providers share the same redirect
// URI, so the state cookie written at login time decides which flow completes.
func (h *Handler) OAuthCallback(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie(googleStateCookie); err == nil {
		h.GoogleCallback(w, r)
		return
	}
	h.GitHubCallback(w, r)
}

// GitHubCallback finishes the GitHub OAuth flow and redirects the browser to
// the frontend with the issued access and refresh tokens in the URL fragment.
func (h *Handler) GitHubCallback(w http.ResponseWriter, r *http.Request) {
	h.completeOAuth(w, r, h.githubFlow())
}

// GoogleCallback finishes the Google OAuth flow and redirects the browser to
// the frontend with the issued access and refresh tokens in the URL fragment.
func (h *Handler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	h.completeOAuth(w, r, h.googleFlow())
}

func (h *Handler) startOAuth(w http.ResponseWriter, r *http.Request, flow *oauthFlow) {
	if !flow.enabled {
		httphelper.Error(w, http.StatusServiceUnavailable, flow.name+" sign-in is not configured")
		return
	}
	state, err := authhelper.NewSessionToken()
	if err != nil {
		h.internalError(w, r, "create "+flow.name+" OAuth state", err)
		return
	}
	h.setOAuthCookie(w, flow.cookieName, state, int(oauthStateTTL.Seconds()))
	http.Redirect(w, r, flow.authorize(state), http.StatusTemporaryRedirect)
}

func (h *Handler) completeOAuth(w http.ResponseWriter, r *http.Request, flow *oauthFlow) {
	if !flow.enabled {
		httphelper.Error(w, http.StatusServiceUnavailable, flow.name+" sign-in is not configured")
		return
	}
	if !h.validOAuthState(r, flow.cookieName) {
		h.clearOAuthCookie(w, flow.cookieName)
		h.oauthFailure(w, r, http.StatusBadRequest, "validate "+flow.name+" OAuth state", nil,
			flow.name+" sign-in could not be verified, please try again", flow.returnURL)
		return
	}
	h.clearOAuthCookie(w, flow.cookieName)

	if r.FormValue("error") != "" {
		h.oauthFailure(w, r, http.StatusBadRequest, flow.name+" denied the sign-in request", nil,
			flow.name+" sign-in was not completed", flow.returnURL)
		return
	}
	code := r.FormValue("code")
	if code == "" {
		h.oauthFailure(w, r, http.StatusBadRequest, "read "+flow.name+" OAuth callback", nil,
			flow.name+" did not return an authorization code", flow.returnURL)
		return
	}

	accessToken, err := flow.exchange(r.Context(), code)
	if err != nil {
		h.oauthFailure(w, r, http.StatusBadGateway, "exchange "+flow.name+" authorization code", err,
			flow.name+" sign-in could not be completed", flow.returnURL)
		return
	}
	identity, err := flow.profile(r.Context(), accessToken)
	if err != nil {
		h.oauthFailure(w, r, http.StatusBadGateway, "load "+flow.name+" profile", err,
			flow.name+" sign-in could not be completed", flow.returnURL)
		return
	}
	email := ""
	if identity.verified {
		email = identity.email
	}
	h.finishOAuthLogin(w, r, flow, identity.subject, email, identity.name)
}

func (h *Handler) finishOAuthLogin(
	w http.ResponseWriter,
	r *http.Request,
	flow *oauthFlow,
	subject, email, name string,
) {
	user, err := h.store.Users.OAuthLogin(r.Context(), flow.provider, subject, email, name)
	if err != nil {
		if errors.Is(err, store.ErrEmailTaken) {
			h.oauthFailure(w, r, http.StatusConflict, "synchronize "+flow.name+" user", err,
				"email is already registered with another sign-in method", flow.returnURL)
			return
		}
		h.oauthFailure(w, r, http.StatusInternalServerError, "synchronize "+flow.name+" user", err,
			flow.name+" sign-in could not be completed", flow.returnURL)
		return
	}
	tokens, err := h.store.Users.CreateSession(r.Context(), user.ID)
	if err != nil {
		h.oauthFailure(w, r, http.StatusInternalServerError, "create "+flow.name+" login session", err,
			flow.name+" sign-in could not be completed", flow.returnURL)
		return
	}
	http.Redirect(w, r, oauthRedirectTarget(flow.returnURL, url.Values{
		"token":         {tokens.AccessToken},
		"refresh_token": {tokens.RefreshToken},
	}), http.StatusFound)
}

func (h *Handler) validOAuthState(r *http.Request, cookieName string) bool {
	cookie, err := r.Cookie(cookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	state := r.FormValue("state")
	return state != "" && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) == 1
}

func (h *Handler) setOAuthCookie(w http.ResponseWriter, name, value string, maxAge int) {
	// #nosec G124 -- Secure follows the configured TLS mode; plain HTTP is local development only.
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   h.publicScheme == "https",
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *Handler) clearOAuthCookie(w http.ResponseWriter, name string) {
	h.setOAuthCookie(w, name, "", -1)
}

func (h *Handler) oauthFailure(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	operation string,
	err error,
	message string,
	returnURL string,
) {
	if err != nil {
		h.logger.ErrorContext(r.Context(), operation, "error", err)
	}
	if returnURL == "" {
		httphelper.Error(w, status, message)
		return
	}
	http.Redirect(w, r, oauthRedirectTarget(returnURL, url.Values{"error": {message}}), http.StatusFound)
}

// oauthReturnURL prefers the returning provider's URL and falls back to the
// other configured provider so a single-provider setup still redirects home.
func (h *Handler) oauthReturnURL(primary string) string {
	if primary != "" {
		return primary
	}
	if h.githubOAuth.ReturnURL() != "" {
		return h.githubOAuth.ReturnURL()
	}
	return h.googleOAuth.ReturnURL()
}

func oauthRedirectTarget(returnURL string, params url.Values) string {
	return returnURL + "#" + params.Encode()
}
