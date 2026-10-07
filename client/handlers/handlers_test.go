package handlers_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/JspBack/webfp-drive/client/handlers"
	authhelper "github.com/JspBack/webfp-drive/helper/auth"
)

func testHandler() *handlers.Handler {
	return handlers.New(nil, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), 1024, 1024, "", false, nil, nil, nil)
}

func authenticatedRequest(method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	userID, sessionID := uuid.New(), uuid.New()
	return request.WithContext(authhelper.WithSession(context.Background(), userID, sessionID, "token"))
}

func assertStatus(t *testing.T, handler http.HandlerFunc, request *http.Request, want int) {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("%s %s status = %d, want %d; body=%s", request.Method, request.URL, recorder.Code, want, recorder.Body.String())
	}
}

func withPathID(request *http.Request, value string) *http.Request {
	request.SetPathValue("id", value)
	return request
}

func TestHealth(t *testing.T) {
	assertStatus(t, testHandler().Health, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusOK)
}

func TestAuthHandlersRejectInvalidRequests(t *testing.T) {
	h := testHandler()
	assertStatus(t, h.Register, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{`)), http.StatusBadRequest)
	assertStatus(t, h.Register,
		httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"bad","password":"short"}`)),
		http.StatusBadRequest,
	)
	assertStatus(t, h.Login, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"unexpected":true}`)), http.StatusBadRequest)
	assertStatus(t, h.Refresh, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"refresh_token":" "}`)), http.StatusBadRequest)
	assertStatus(t, h.GetUser, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusUnauthorized)
	assertStatus(t, h.VerifyEmail, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{`)), http.StatusBadRequest)
	assertStatus(t, h.VerifyEmail,
		httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"bad","code":"123456"}`)),
		http.StatusBadRequest,
	)
	assertStatus(t, h.VerifyEmail,
		httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"user@example.com","code":"12"}`)),
		http.StatusBadRequest,
	)
	assertStatus(t, h.ResendVerification,
		httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"bad"}`)),
		http.StatusBadRequest,
	)
	assertStatus(t, h.ResendVerification, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{`)), http.StatusBadRequest)
}

func TestFileHandlersRejectInvalidRequests(t *testing.T) {
	h := testHandler()
	assertStatus(t, h.CreateFile, httptest.NewRequest(http.MethodPost, "/", nil), http.StatusUnauthorized)

	request := authenticatedRequest(http.MethodGet, "/?folder_id=invalid", "")
	assertStatus(t, h.ListFiles, request, http.StatusBadRequest)
	assertStatus(t, h.DownloadFile, withPathID(authenticatedRequest(http.MethodGet, "/", ""), "invalid"), http.StatusBadRequest)
	assertStatus(t, h.DeleteFile, withPathID(authenticatedRequest(http.MethodDelete, "/", ""), "invalid"), http.StatusBadRequest)

	tests := []struct {
		body string
		want int
	}{
		{"{", http.StatusBadRequest},
		{"{}", http.StatusBadRequest},
		{`{"filename":" "}`, http.StatusBadRequest},
		{`{"folder_id":"invalid"}`, http.StatusBadRequest},
	}
	for _, test := range tests {
		request = authenticatedRequest(http.MethodPatch, "/", test.body)
		request = withPathID(request, uuid.NewString())
		assertStatus(t, h.UpdateFile, request, test.want)
	}
}

func TestFolderHandlersRejectInvalidRequests(t *testing.T) {
	h := testHandler()
	assertStatus(t, h.CreateFolder, authenticatedRequest(http.MethodPost, "/", `{"name":" "}`), http.StatusBadRequest)
	assertStatus(t, h.CreateFolder,
		authenticatedRequest(http.MethodPost, "/", `{"name":"valid","parent_id":"invalid"}`),
		http.StatusBadRequest,
	)
	assertStatus(t, h.ListFolders, authenticatedRequest(http.MethodGet, "/?parent_id=invalid", ""), http.StatusBadRequest)
	assertStatus(t, h.GetFolder, withPathID(authenticatedRequest(http.MethodGet, "/", ""), "invalid"), http.StatusBadRequest)
	assertStatus(t, h.DeleteFolder, withPathID(authenticatedRequest(http.MethodDelete, "/", ""), "invalid"), http.StatusBadRequest)

	for _, body := range []string{`{`, `{}`, `{"name":" "}`, `{"parent_id":"invalid"}`} {
		request := withPathID(authenticatedRequest(http.MethodPatch, "/", body), uuid.NewString())
		assertStatus(t, h.UpdateFolder, request, http.StatusBadRequest)
	}
}

func TestShareHandlersRejectInvalidRequests(t *testing.T) {
	h := testHandler()
	assertStatus(t, h.CreateShare,
		withPathID(authenticatedRequest(http.MethodPost, "/", `{}`), "invalid"),
		http.StatusBadRequest,
	)
	assertStatus(t, h.CreateShare,
		withPathID(authenticatedRequest(http.MethodPost, "/", `{}`), uuid.NewString()),
		http.StatusBadRequest,
	)
	assertStatus(t, h.CreateShare,
		withPathID(authenticatedRequest(http.MethodPost, "/", `{"expires_at":"tomorrow"}`), uuid.NewString()),
		http.StatusBadRequest,
	)
	assertStatus(t, h.RevokeShare,
		withPathID(authenticatedRequest(http.MethodDelete, "/", ""), "invalid"),
		http.StatusBadRequest,
	)

	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("password=x"))
	request.SetPathValue("token", "share-token")
	request.Header.Set("Content-Type", "text/plain")
	assertStatus(t, h.SubmitSharePassword, request, http.StatusBadRequest)
}

func TestGitHubAuthRequiresConfiguration(t *testing.T) {
	h := testHandler()
	assertStatus(t, h.LoginGitHub, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusServiceUnavailable)
	assertStatus(t, h.GitHubCallback, httptest.NewRequest(http.MethodGet, "/?code=x&state=y", nil), http.StatusServiceUnavailable)
}

func TestGoogleAuthRequiresConfiguration(t *testing.T) {
	h := testHandler()
	assertStatus(t, h.LoginGoogle, httptest.NewRequest(http.MethodGet, "/", nil), http.StatusServiceUnavailable)
	assertStatus(t, h.OAuthCallback, httptest.NewRequest(http.MethodGet, "/?code=x&state=y", nil), http.StatusServiceUnavailable)
}

func TestLoginGitHubRedirectsToGitHub(t *testing.T) {
	h := configuredGitHubHandler()
	recorder := httptest.NewRecorder()
	h.LoginGitHub(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusTemporaryRedirect {
		t.Fatalf("LoginGitHub() status = %d, want %d", recorder.Code, http.StatusTemporaryRedirect)
	}
	location := recorder.Header().Get("Location")
	if !strings.HasPrefix(location, "https://github.com/login/oauth/authorize?") ||
		!strings.Contains(location, "client_id=client-id") ||
		!strings.Contains(location, "state=") {
		t.Errorf("LoginGitHub() Location = %q, want a GitHub authorize URL with state", location)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "webfp_github_state" || cookies[0].Value == "" {
		t.Errorf("LoginGitHub() cookies = %+v, want a single non-empty state cookie", cookies)
	}
}

func TestGitHubCallbackRejectsMismatchedState(t *testing.T) {
	h := configuredGitHubHandler()
	request := httptest.NewRequest(http.MethodGet, "/callback?code=code&state=other", nil)
	request.AddCookie(&http.Cookie{Name: "webfp_github_state", Value: "expected"})
	recorder := httptest.NewRecorder()

	h.GitHubCallback(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("GitHubCallback() status = %d, want %d", recorder.Code, http.StatusFound)
	}
	if location := recorder.Header().Get("Location"); !strings.Contains(location, "error=") {
		t.Errorf("GitHubCallback() Location = %q, want an error redirect", location)
	}
}

func configuredGitHubHandler() *handlers.Handler {
	return handlers.New(
		nil,
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		1024, 1024, "", false,
		authhelper.NewGitHubOAuth(
			"client-id", "client-secret",
			"http://localhost:8080/callback",
			"http://localhost:5173/auth/callback",
		),
		nil,
		nil,
	)
}

func configuredGoogleHandler() *handlers.Handler {
	return handlers.New(
		nil,
		slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		1024, 1024, "", false,
		nil,
		authhelper.NewGoogleOAuth(
			"google-client-id", "google-client-secret",
			"http://localhost:8080/api/v1/callback",
			"http://localhost:5173/auth/callback",
		),
		nil,
	)
}

func TestLoginGoogleRedirectsToGoogle(t *testing.T) {
	h := configuredGoogleHandler()
	recorder := httptest.NewRecorder()
	h.LoginGoogle(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusTemporaryRedirect {
		t.Fatalf("LoginGoogle() status = %d, want %d", recorder.Code, http.StatusTemporaryRedirect)
	}
	location := recorder.Header().Get("Location")
	if !strings.HasPrefix(location, "https://accounts.google.com/o/oauth2/auth?") ||
		!strings.Contains(location, "client_id=google-client-id") ||
		!strings.Contains(location, "state=") {
		t.Errorf("LoginGoogle() Location = %q, want a Google authorize URL with state", location)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != "webfp_google_state" || cookies[0].Value == "" {
		t.Errorf("LoginGoogle() cookies = %+v, want a single non-empty state cookie", cookies)
	}
}

func TestOAuthCallbackRoutesGoogleStateToGoogle(t *testing.T) {
	h := configuredGoogleHandler()
	request := httptest.NewRequest(http.MethodGet, "/callback?code=code&state=other", nil)
	request.AddCookie(&http.Cookie{Name: "webfp_google_state", Value: "expected"})
	recorder := httptest.NewRecorder()

	h.OAuthCallback(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("OAuthCallback() status = %d, want %d", recorder.Code, http.StatusFound)
	}
	location := recorder.Header().Get("Location")
	if !strings.Contains(location, "error=") || !strings.Contains(location, "Google") {
		t.Errorf("OAuthCallback() Location = %q, want a Google error redirect", location)
	}
}

func TestGoogleCallbackRejectsMismatchedState(t *testing.T) {
	h := configuredGoogleHandler()
	request := httptest.NewRequest(http.MethodGet, "/callback?code=code&state=other", nil)
	request.AddCookie(&http.Cookie{Name: "webfp_google_state", Value: "expected"})
	recorder := httptest.NewRecorder()

	h.GoogleCallback(recorder, request)

	if recorder.Code != http.StatusFound {
		t.Fatalf("GoogleCallback() status = %d, want %d", recorder.Code, http.StatusFound)
	}
	if location := recorder.Header().Get("Location"); !strings.Contains(location, "error=") {
		t.Errorf("GoogleCallback() Location = %q, want an error redirect", location)
	}
}

func TestRequireSessionRejectsMissingBearerToken(t *testing.T) {
	h := testHandler()
	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	recorder := httptest.NewRecorder()
	h.RequireSession(next).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusUnauthorized || called {
		t.Fatalf("RequireSession() status=%d called=%v", recorder.Code, called)
	}
}
