package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
)

func TestNewTokenPair(t *testing.T) {
	pair, err := authhelper.NewTokenPair()
	if err != nil {
		t.Fatalf("NewTokenPair() error = %v", err)
	}
	if len(pair.AccessToken) != 64 || len(pair.RefreshToken) != 64 {
		t.Fatalf("token lengths = (%d, %d), want (64, 64)", len(pair.AccessToken), len(pair.RefreshToken))
	}
	if pair.AccessToken == pair.RefreshToken {
		t.Fatal("access and refresh tokens should be distinct")
	}
	if pair.SessionID == uuid.Nil {
		t.Fatal("session ID should not be nil")
	}
}

func TestSessionKeysHashToken(t *testing.T) {
	token := "secret-token"
	if got, want := authhelper.SessionKey(token), authhelper.SessionKey(token); got != want {
		t.Fatalf("SessionKey() is not deterministic: %q != %q", got, want)
	}
	if authhelper.SessionKey(token) == authhelper.RefreshKey(token) {
		t.Fatal("session and refresh keys should use distinct prefixes")
	}
	if authhelper.SessionKey(token) == "session:"+token {
		t.Fatal("session key exposes the token")
	}
	if authhelper.SessionIndexKey(uuid.Nil) != "session-id:"+uuid.Nil.String() {
		t.Fatalf("SessionIndexKey() = %q", authhelper.SessionIndexKey(uuid.Nil))
	}
}

func TestBearerToken(t *testing.T) {
	for _, test := range []struct {
		header string
		want   string
		ok     bool
	}{
		{header: "Bearer abc", want: "abc", ok: true},
		{header: "bearer  abc  ", want: "abc", ok: true},
		{header: "Basic abc"},
		{header: "Bearer"},
		{header: "Bearer   "},
	} {
		got, ok := authhelper.BearerToken(test.header)
		if got != test.want || ok != test.ok {
			t.Errorf("BearerToken(%q) = (%q, %v), want (%q, %v)", test.header, got, ok, test.want, test.ok)
		}
	}
}

func TestSessionContext(t *testing.T) {
	userID, sessionID := uuid.New(), uuid.New()
	ctx := authhelper.WithSession(context.Background(), userID, sessionID, "token")
	if got, ok := authhelper.UserID(ctx); !ok || got != userID {
		t.Errorf("UserID() = (%s, %v), want (%s, true)", got, ok, userID)
	}
	if got, ok := authhelper.SessionID(ctx); !ok || got != sessionID {
		t.Errorf("SessionID() = (%s, %v), want (%s, true)", got, ok, sessionID)
	}
	if got, ok := authhelper.SessionToken(ctx); !ok || got != "token" {
		t.Errorf("SessionToken() = (%q, %v), want (token, true)", got, ok)
	}
	if _, ok := authhelper.UserID(context.Background()); ok {
		t.Error("UserID() found an ID in an empty context")
	}
}

func TestAuthenticatedUserID(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, ok := authhelper.AuthenticatedUserID(recorder, request); ok {
		t.Fatal("AuthenticatedUserID() succeeded without a session")
	}
	if recorder.Code != 401 {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}

	userID := uuid.New()
	request = request.WithContext(authhelper.WithSession(request.Context(), userID, uuid.New(), "token"))
	if got, ok := authhelper.AuthenticatedUserID(httptest.NewRecorder(), request); !ok || got != userID {
		t.Fatalf("AuthenticatedUserID() = (%s, %v), want (%s, true)", got, ok, userID)
	}
}
