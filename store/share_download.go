package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

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

func (ss *ShareStore) SharedDownload(ctx context.Context, token, password string) (*File, io.ReadCloser, error) {
	tokenHash := sha256.Sum256([]byte(token))
	fields, err := ss.shareFields(ctx, shareRedisKey(tokenHash),
		"id", "owner_id", "file_id", "expires_at", "max_downloads", "password_hash", "status", "archive_name", "archive_size",
	)
	if err != nil {
		return nil, nil, fmt.Errorf("load share from Redis: %w", err)
	}
	if len(fields) == 0 || shareStatus(fields) != readyStatus {
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

	file, err := ss.sharedObject(ctx, ownerID, fileID, shareID, fields)
	if err != nil {
		return nil, nil, err
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

func (ss *ShareStore) sharedObject(ctx context.Context, ownerID, fileID, shareID uuid.UUID, fields map[string]string) (*File, error) {
	if name := fields["archive_name"]; name != "" {
		size, err := strconv.ParseInt(fields["archive_size"], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse archive size: %w", err)
		}
		return &File{Filename: name, ContentType: "application/zip", SizeBytes: size, objectKey: archiveObjectKey(shareID)}, nil
	}
	file, err := ss.files.Get(ctx, ownerID, fileID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrShareUnavailable
		}
		return nil, fmt.Errorf("load shared file: %w", err)
	}
	if file.Status != readyStatus {
		return nil, ErrShareUnavailable
	}

	return file, nil
}

func closeObjectError(err error) error {
	if err != nil {
		return fmt.Errorf("close unserved file object: %w", err)
	}
	return nil
}
