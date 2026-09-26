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
	return handlers.New(nil, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), 1024, 1024, "", false)
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
	assertStatus(t, h.UpdateUser, authenticatedRequest(http.MethodPatch, "/", `{}`), http.StatusBadRequest)
	assertStatus(t, h.UpdateUser, authenticatedRequest(http.MethodPatch, "/", `{"unexpected":true}`), http.StatusBadRequest)
}

func TestFileHandlersRejectInvalidRequests(t *testing.T) {
	h := testHandler()
	assertStatus(t, h.CreateFile, httptest.NewRequest(http.MethodPost, "/", nil), http.StatusUnauthorized)

	request := authenticatedRequest(http.MethodGet, "/?folder_id=invalid", "")
	assertStatus(t, h.ListFiles, request, http.StatusBadRequest)
	assertStatus(t, h.GetFile, withPathID(authenticatedRequest(http.MethodGet, "/", ""), "invalid"), http.StatusBadRequest)
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
