package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sort"
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

type ShareListItem struct {
	ID            uuid.UUID `json:"id"`
	FileID        uuid.UUID `json:"file_id"`
	Filename      string    `json:"filename"`
	Token         string    `json:"token"`
	ExpiresAt     time.Time `json:"expires_at"`
	MaxDownloads  int64     `json:"max_downloads"`
	DownloadCount int64     `json:"download_count"`
	HasPassword   bool      `json:"has_password"`
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
	redis.call("DEL", KEYS[1], KEYS[2])
	return -2
end
local new_count = redis.call("HINCRBY", KEYS[1], "download_count", 1)
if maximum > 0 and new_count >= maximum then
	redis.call("DEL", KEYS[1], KEYS[2])
end
return new_count
`

const revokeShareScript = `
if redis.call("HGET", KEYS[1], "owner_id") ~= ARGV[1] then
	return 0
end
redis.call("DEL", KEYS[1], KEYS[2])
return 1
`

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
		return nil, fmt.Errorf("load file for sharing: %w", err)
	}
	if file.Status != "ready" {
		return nil, ErrShareFileNotReady
	}

	randomToken := make([]byte, 32)
	if _, randomErr := rand.Read(randomToken); randomErr != nil {
		return nil, fmt.Errorf("generate share token: %w", randomErr)
	}
	token := base64.RawURLEncoding.EncodeToString(randomToken)
	tokenHash := sha256.Sum256([]byte(token))
	share := &Share{ID: uuid.New(), Token: token, ExpiresAt: expiresAt}

	passwordHash := ""
	if password != "" {
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if hashErr != nil {
			return nil, fmt.Errorf("hash share password: %w", hashErr)
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
			"token", token,
			"expires_at", expiresAt.UnixMilli(),
			"max_downloads", maxDownloadsValue,
			"download_count", 0,
			"password_hash", passwordHash,
		)
		pipe.PExpire(ctx, shareKey, ttl)
		pipe.Set(ctx, idKey, shareKey, ttl)
		pipe.SAdd(ctx, shareOwnerRedisKey(ownerID), share.ID.String())
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("save share in Redis: %w", err)
	}
	return share, nil
}

func (ss *ShareStore) SharedDownload(ctx context.Context, token, password string) (*File, io.ReadCloser, error) {
	tokenHash := sha256.Sum256([]byte(token))
	fields, err := ss.cache.HGetAll(ctx, shareRedisKey(tokenHash)).Result()
	if err != nil {
		return nil, nil, fmt.Errorf("load share from Redis: %w", err)
	}
	if len(fields) == 0 {
		return nil, nil, ErrShareUnavailable
	}

	shareID, err := uuid.Parse(fields["id"])
	if err != nil {
		return nil, nil, fmt.Errorf("parse share ID: %w", err)
	}
	expiresAt, err := strconv.ParseInt(fields["expires_at"], 10, 64)
	if err != nil {
		return nil, nil, fmt.Errorf("parse share expiry: %w", err)
	}
	maxDownloads, err := strconv.ParseInt(fields["max_downloads"], 10, 64)
	if err != nil {
		return nil, nil, fmt.Errorf("parse share download limit: %w", err)
	}
	ownerID, err := uuid.Parse(fields["owner_id"])
	if err != nil {
		return nil, nil, fmt.Errorf("parse share owner ID: %w", err)
	}
	fileID, err := uuid.Parse(fields["file_id"])
	if err != nil {
		return nil, nil, fmt.Errorf("parse share file ID: %w", err)
	}
	if passwordHash := fields["password_hash"]; passwordHash != "" {
		if password == "" {
			return nil, nil, ErrSharePasswordRequired
		}
		if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
			return nil, nil, ErrSharePasswordInvalid
		}
	}

	file, err := ss.files.Get(ctx, ownerID, fileID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil, ErrShareUnavailable
		}
		return nil, nil, fmt.Errorf("load shared file: %w", err)
	}
	if file.Status != "ready" {
		return nil, nil, ErrShareUnavailable
	}

	object, err := ss.files.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(ss.files.bucket),
		Key:    aws.String(file.objectKey),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("get shared file object: %w", err)
	}

	result, err := ss.cache.Eval(ctx, incrementShareUseScript,
		[]string{shareRedisKey(tokenHash), shareIDRedisKey(shareID)},
		expiresAt, maxDownloads, time.Now().UnixMilli(),
	).Int64()
	if err != nil {
		closeErr := object.Body.Close()
		return nil, nil, errors.Join(
			fmt.Errorf("count shared download: %w", err),
			closeObjectError(closeErr),
		)
	}
	if result < 0 {
		closeErr := object.Body.Close()
		return nil, nil, errors.Join(ErrShareUnavailable, closeObjectError(closeErr))
	}
	return file, object.Body, nil
}

func closeObjectError(err error) error {
	if err != nil {
		return fmt.Errorf("close unserved file object: %w", err)
	}
	return nil
}

func (ss *ShareStore) ListShares(ctx context.Context, ownerID uuid.UUID) ([]ShareListItem, error) {
	ownerKey := shareOwnerRedisKey(ownerID)
	rawIDs, err := ss.cache.SMembers(ctx, ownerKey).Result()
	if err != nil {
		return nil, fmt.Errorf("list share IDs: %w", err)
	}

	items := make([]ShareListItem, 0, len(rawIDs))
	stale := make([]interface{}, 0, len(rawIDs))
	var fileIDs []uuid.UUID
	for _, rawID := range rawIDs {
		shareID, parseErr := uuid.Parse(rawID)
		if parseErr != nil {
			stale = append(stale, rawID)
			continue
		}
		shareKey, getErr := ss.cache.Get(ctx, shareIDRedisKey(shareID)).Result()
		if getErr != nil {
			if errors.Is(getErr, redis.Nil) {
				stale = append(stale, rawID)
				continue
			}
			return nil, fmt.Errorf("lookup share: %w", getErr)
		}
		fields, hashErr := ss.cache.HGetAll(ctx, shareKey).Result()
		if hashErr != nil {
			return nil, fmt.Errorf("load share: %w", hashErr)
		}
		if len(fields) == 0 || fields["owner_id"] != ownerID.String() {
			stale = append(stale, rawID)
			continue
		}
		item, convErr := parseShareListItem(fields)
		if convErr != nil {
			stale = append(stale, rawID)
			continue
		}
		items = append(items, item)
		fileIDs = append(fileIDs, item.FileID)
	}
	if len(stale) > 0 {
		_ = ss.cache.SRem(ctx, ownerKey, stale...).Err()
	}

	filenames, err := ss.files.GetFilenames(ctx, ownerID, fileIDs)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Filename = filenames[items[i].FileID]
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ExpiresAt.Equal(items[j].ExpiresAt) {
			return items[i].ID.String() < items[j].ID.String()
		}
		return items[i].ExpiresAt.After(items[j].ExpiresAt)
	})
	return items, nil
}

func parseShareListItem(fields map[string]string) (ShareListItem, error) {
	shareID, err := uuid.Parse(fields["id"])
	if err != nil {
		return ShareListItem{}, fmt.Errorf("parse share ID: %w", err)
	}
	fileID, err := uuid.Parse(fields["file_id"])
	if err != nil {
		return ShareListItem{}, fmt.Errorf("parse share file ID: %w", err)
	}
	expiresMillis, err := strconv.ParseInt(fields["expires_at"], 10, 64)
	if err != nil {
		return ShareListItem{}, fmt.Errorf("parse share expiry: %w", err)
	}
	maxDownloads, err := strconv.ParseInt(fields["max_downloads"], 10, 64)
	if err != nil {
		return ShareListItem{}, fmt.Errorf("parse share download limit: %w", err)
	}
	downloadCount, err := strconv.ParseInt(fields["download_count"], 10, 64)
	if err != nil {
		return ShareListItem{}, fmt.Errorf("parse share download count: %w", err)
	}
	return ShareListItem{
		ID:            shareID,
		FileID:        fileID,
		Token:         fields["token"],
		ExpiresAt:     time.UnixMilli(expiresMillis),
		MaxDownloads:  maxDownloads,
		DownloadCount: downloadCount,
		HasPassword:   fields["password_hash"] != "",
	}, nil
}

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
	_ = ss.cache.SRem(ctx, shareOwnerRedisKey(ownerID), shareID.String()).Err()
	return nil
}

func shareRedisKey(tokenHash [sha256.Size]byte) string {
	return "share:" + base64.RawURLEncoding.EncodeToString(tokenHash[:])
}

func shareIDRedisKey(id uuid.UUID) string {
	return "share-id:" + id.String()
}

func shareOwnerRedisKey(ownerID uuid.UUID) string {
	return "share-owner:" + ownerID.String()
}
