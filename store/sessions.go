package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	authhelper "github.com/JspBack/webfp-drive/helper/auth"
)

var ErrInvalidSession = errors.New("invalid or expired session")

const (
	accessRecordParts  = 2
	refreshRecordParts = 3
	saveSessionScript  = `
redis.call("SET", KEYS[1], ARGV[1] .. "|" .. ARGV[2], "PX", ARGV[3])
redis.call("SET", KEYS[2], ARGV[1] .. "|" .. ARGV[2] .. "|" .. KEYS[1], "PX", ARGV[4])
redis.call("SET", KEYS[3], KEYS[2], "PX", ARGV[4])
return 1
`
	rotateSessionScript = `
local value = redis.call("GET", KEYS[1])
if not value or redis.call("GET", KEYS[2]) ~= KEYS[1] then
	return 0
end
local old_access_key = string.match(value, "^[^|]+|[^|]+|(.+)$")
if not old_access_key then
	return 0
end
redis.call("DEL", KEYS[1], old_access_key)
redis.call("SET", KEYS[3], ARGV[1] .. "|" .. ARGV[2], "PX", ARGV[3])
redis.call("SET", KEYS[4], ARGV[1] .. "|" .. ARGV[2] .. "|" .. KEYS[3], "PX", ARGV[4])
redis.call("SET", KEYS[2], KEYS[4], "PX", ARGV[4])
return 1
`
	revokeSessionScript = `
local refresh_key = redis.call("GET", KEYS[2])
if refresh_key then
	local refresh_value = redis.call("GET", refresh_key)
	local access_key = refresh_value and string.match(refresh_value, "^[^|]+|[^|]+|(.+)$")
	if access_key then
		redis.call("DEL", access_key)
	end
	redis.call("DEL", refresh_key)
end
redis.call("DEL", KEYS[1], KEYS[2])
return 1
`
)

func (us *UserStore) CreateSession(ctx context.Context, userID uuid.UUID) (*authhelper.TokenPair, error) {
	pair, err := authhelper.NewTokenPair()
	if err != nil {
		return nil, fmt.Errorf("create session tokens: %w", err)
	}
	if saveErr := us.cache.Eval(ctx, saveSessionScript,
		[]string{
			authhelper.SessionKey(pair.AccessToken),
			authhelper.RefreshKey(pair.RefreshToken),
			authhelper.SessionIndexKey(pair.SessionID),
		},
		userID.String(), pair.SessionID.String(),
		us.accessLifetime.Milliseconds(), us.refreshLifetime.Milliseconds(),
	).Err(); saveErr != nil {
		return nil, fmt.Errorf("save token pair: %w", saveErr)
	}
	return pair, nil
}

func (us *UserStore) ResolveSession(ctx context.Context, token string) (uuid.UUID, uuid.UUID, error) {
	value, err := us.cache.Get(ctx, authhelper.SessionKey(token)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return uuid.Nil, uuid.Nil, ErrInvalidSession
		}
		return uuid.Nil, uuid.Nil, fmt.Errorf("load session: %w", err)
	}
	parts := strings.SplitN(value, "|", accessRecordParts)
	if len(parts) != accessRecordParts {
		return uuid.Nil, uuid.Nil, ErrInvalidSession
	}
	userID, err := uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrInvalidSession
	}
	sessionID, err := uuid.Parse(parts[1])
	if err != nil {
		return uuid.Nil, uuid.Nil, ErrInvalidSession
	}
	return userID, sessionID, nil
}

func (us *UserStore) RefreshSession(ctx context.Context, refreshToken string) (*authhelper.TokenPair, error) {
	refreshKey := authhelper.RefreshKey(refreshToken)
	refreshValue, err := us.cache.Get(ctx, refreshKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrInvalidSession
		}
		return nil, fmt.Errorf("load refresh token: %w", err)
	}
	tokenParts := strings.SplitN(refreshValue, "|", 3)
	if len(tokenParts) != refreshRecordParts {
		return nil, errors.New("invalid refresh token record")
	}
	userIDValue := tokenParts[0]
	if _, parseErr := uuid.Parse(userIDValue); parseErr != nil {
		return nil, fmt.Errorf("parse refresh token user ID: %w", parseErr)
	}
	sessionID, err := uuid.Parse(tokenParts[1])
	if err != nil {
		return nil, fmt.Errorf("parse refresh token session ID: %w", err)
	}
	pair, err := authhelper.NewTokenPair()
	if err != nil {
		return nil, fmt.Errorf("create refreshed tokens: %w", err)
	}
	pair.SessionID = sessionID
	result, err := us.cache.Eval(ctx, rotateSessionScript,
		[]string{
			refreshKey,
			authhelper.SessionIndexKey(pair.SessionID),
			authhelper.SessionKey(pair.AccessToken),
			authhelper.RefreshKey(pair.RefreshToken),
		},
		userIDValue, pair.SessionID.String(),
		us.accessLifetime.Milliseconds(), us.refreshLifetime.Milliseconds(),
	).Int64()
	if err != nil {
		return nil, fmt.Errorf("rotate refresh token: %w", err)
	}
	if result == 0 {
		return nil, ErrInvalidSession
	}
	return pair, nil
}

func (us *UserStore) RevokeSession(ctx context.Context, token string, sessionID uuid.UUID) error {
	if err := us.cache.Eval(ctx, revokeSessionScript,
		[]string{
			authhelper.SessionKey(token),
			authhelper.SessionIndexKey(sessionID),
		},
	).Err(); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}
