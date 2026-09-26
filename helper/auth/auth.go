package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	httphelper "github.com/JspBack/webfp-drive/helper/http"
	"github.com/google/uuid"
)

type contextKey string

const (
	userIDKey contextKey = "user_id"
	tokenKey  contextKey = "session_token"
)

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

func BearerToken(header string) (string, bool) {
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

func WithSession(ctx context.Context, userID uuid.UUID, token string) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
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
