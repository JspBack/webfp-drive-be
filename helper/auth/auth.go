package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	httphelper "github.com/JspBack/webfp-drive/helper/http"
)

type contextKey string

const (
	userIDKey    contextKey = "user_id"
	tokenKey     contextKey = "session_token"
	sessionIDKey contextKey = "session_id"
)

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	SessionID    uuid.UUID
}

func NewTokenPair() (*TokenPair, error) {
	accessToken, err := NewSessionToken()
	if err != nil {
		return nil, err
	}
	refreshToken, err := NewSessionToken()
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		SessionID:    uuid.New(),
	}, nil
}

func NewSessionToken() (string, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(token), nil
}

func SessionKey(token string) string {
	hash := sha256.Sum256([]byte(token))
	return "session:" + hex.EncodeToString(hash[:])
}

func RefreshKey(token string) string {
	hash := sha256.Sum256([]byte(token))
	return "refresh:" + hex.EncodeToString(hash[:])
}

func SessionIndexKey(sessionID uuid.UUID) string {
	return "session-id:" + sessionID.String()
}

func BearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

func WithSession(ctx context.Context, userID, sessionID uuid.UUID, token string) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
	ctx = context.WithValue(ctx, sessionIDKey, sessionID)
	return context.WithValue(ctx, tokenKey, token)
}

func UserID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDKey).(uuid.UUID)
	return id, ok
}

func AuthenticatedUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := UserID(r.Context())
	if !ok {
		httphelper.Error(w, http.StatusUnauthorized, "authentication required")
	}
	return id, ok
}

func SessionToken(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(tokenKey).(string)
	return token, ok
}

func SessionID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(sessionIDKey).(uuid.UUID)
	return id, ok
}
