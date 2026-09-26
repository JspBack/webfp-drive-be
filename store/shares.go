package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrShareUnavailable      = errors.New("share is unavailable")
	ErrSharePasswordRequired = errors.New("share password is required")
	ErrSharePasswordInvalid  = errors.New("share password is invalid")
	ErrInvalidShareOptions   = errors.New("invalid share options")
	ErrShareFileNotReady     = errors.New("only ready files can be shared")
)

type Share struct {
	ID        uuid.UUID `json:"id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ShareStore struct {
	cache *redis.Client
	files *FileStore
}

const incrementShareUseScript = `
local ttl = redis.call("PTTL", KEYS[1])
if ttl < 0 then
	return -1
end
if tonumber(ARGV[1]) <= tonumber(ARGV[3]) then
	return -1
end
local maximum = tonumber(ARGV[2])
local count = tonumber(redis.call("HGET", KEYS[1], "download_count") or "0")
if maximum > 0 and count >= maximum then
	return -2
end
return redis.call("HINCRBY", KEYS[1], "download_count", 1)
`

const revokeShareScript = `
if redis.call("HGET", KEYS[1], "owner_id") ~= ARGV[1] then
	return 0
end
redis.call("DEL", KEYS[1], KEYS[2])
return 1
`

// CreateShare returns an app URL token, not a presigned R2 URL.
func (ss *ShareStore) CreateShare(
	ctx context.Context,
	ownerID, fileID uuid.UUID,
	expiresAt time.Time,
	maxDownloads *int64,
	password string,
) (*Share, error) {
	if !expiresAt.After(time.Now()) {
		return nil, fmt.Errorf("%w: expiry must be in the future", ErrInvalidShareOptions)
	}
	if maxDownloads != nil && *maxDownloads <= 0 {
		return nil, fmt.Errorf("%w: maximum downloads must be positive", ErrInvalidShareOptions)
	}
	file, err := ss.files.Get(ctx, ownerID, fileID)
	if err != nil {
		return nil, err
	}
	if file.Status != "ready" {
		return nil, ErrShareFileNotReady
	}

	randomToken := make([]byte, 32)
	if _, err := rand.Read(randomToken); err != nil {
		return nil, fmt.Errorf("generate share token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(randomToken)
	tokenHash := sha256.Sum256([]byte(token))
	share := &Share{ID: uuid.New(), Token: token, ExpiresAt: expiresAt}

	passwordHash := ""
	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash share password: %w", err)
		}
		passwordHash = string(hash)
	}

	maxDownloadsValue := int64(0)
	if maxDownloads != nil {
		maxDownloadsValue = *maxDownloads
	}
	ttl := time.Until(expiresAt)
	shareKey := shareRedisKey(tokenHash)
	idKey := shareIDRedisKey(share.ID)
	_, err = ss.cache.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.HSet(ctx, shareKey,
			"id", share.ID.String(),
			"owner_id", ownerID.String(),
			"file_id", fileID.String(),
			"expires_at", expiresAt.UnixMilli(),
			"max_downloads", maxDownloadsValue,
			"download_count", 0,
			"password_hash", passwordHash,
		)
		pipe.PExpire(ctx, shareKey, ttl)
		pipe.Set(ctx, idKey, shareKey, ttl)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("save share in Redis: %w", err)
	}
	return share, nil
}

// SharedDownloadURL validates the app token and then issues a short-lived R2 URL.
func (ss *ShareStore) SharedDownloadURL(ctx context.Context, token, password string) (string, error) {
	tokenHash := sha256.Sum256([]byte(token))
	fields, err := ss.cache.HGetAll(ctx, shareRedisKey(tokenHash)).Result()
	if err != nil {
		return "", fmt.Errorf("load share from Redis: %w", err)
	}
	if len(fields) == 0 {
		return "", ErrShareUnavailable
	}

	expiresAt, err := strconv.ParseInt(fields["expires_at"], 10, 64)
	if err != nil {
		return "", fmt.Errorf("parse share expiry: %w", err)
	}
	maxDownloads, err := strconv.ParseInt(fields["max_downloads"], 10, 64)
	if err != nil {
		return "", fmt.Errorf("parse share download limit: %w", err)
	}
	ownerID, err := uuid.Parse(fields["owner_id"])
	if err != nil {
		return "", fmt.Errorf("parse share owner ID: %w", err)
	}
	fileID, err := uuid.Parse(fields["file_id"])
	if err != nil {
		return "", fmt.Errorf("parse share file ID: %w", err)
	}
	if passwordHash := fields["password_hash"]; passwordHash != "" {
		if password == "" {
			return "", ErrSharePasswordRequired
		}
		if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
			return "", ErrSharePasswordInvalid
		}
	}

	file, err := ss.files.Get(ctx, ownerID, fileID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", ErrShareUnavailable
		}
		return "", fmt.Errorf("load shared file: %w", err)
	}
	if file.Status != "ready" {
		return "", ErrShareUnavailable
	}

	presigned, err := ss.files.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(ss.files.bucket),
		Key:    aws.String(file.objectKey),
	}, func(options *s3.PresignOptions) {
		options.Expires = ss.files.downloadURLLifetime
	})
	if err != nil {
		return "", fmt.Errorf("presign shared file download: %w", err)
	}

	result, err := ss.cache.Eval(ctx, incrementShareUseScript,
		[]string{shareRedisKey(tokenHash)},
		expiresAt, maxDownloads, time.Now().UnixMilli(),
	).Int64()
	if err != nil {
		return "", fmt.Errorf("count share URL issuance: %w", err)
	}
	if result < 0 {
		return "", ErrShareUnavailable
	}
	return presigned.URL, nil
}

// RevokeShare disables the app URL; already issued R2 URLs remain valid until they expire.
func (ss *ShareStore) RevokeShare(ctx context.Context, ownerID, shareID uuid.UUID) error {
	idKey := shareIDRedisKey(shareID)
	shareKey, err := ss.cache.Get(ctx, idKey).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return ErrNotFound
		}
		return fmt.Errorf("lookup share in Redis: %w", err)
	}
	result, err := ss.cache.Eval(ctx, revokeShareScript,
		[]string{shareKey, idKey}, ownerID.String(),
	).Int64()
	if err != nil {
		return fmt.Errorf("revoke share in Redis: %w", err)
	}
	if result == 0 {
		return ErrNotFound
	}
	return nil
}

func shareRedisKey(tokenHash [sha256.Size]byte) string {
	return "share:" + base64.RawURLEncoding.EncodeToString(tokenHash[:])
}

func shareIDRedisKey(id uuid.UUID) string {
	return "share-id:" + id.String()
}
