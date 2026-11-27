package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	archivePendingTTL      = 24 * time.Hour
	archiveMailTimeout     = 30 * time.Second
	archiveMailRetryDelay  = time.Minute
	archiveMailMaxAttempts = 5
)

type ArchiveNotifier func(ctx context.Context, recipient, filename, link string) error

const recordArchiveScript = `
if redis.call("HGET", KEYS[1], "status") ~= "preparing" then return 0 end
redis.call("HSET", KEYS[1], "archive_size", ARGV[1])
redis.call("HDEL", KEYS[1], "archive_entries")
return 1
`

const activateArchiveScript = `
if redis.call("HGET", KEYS[1], "status") ~= "preparing" then return 0 end
local duration = tonumber(redis.call("HGET", KEYS[1], "archive_lifetime_ms"))
if not duration or duration <= 0 then return redis.error_reply("invalid ZIP share duration") end
local expires = tonumber(ARGV[1]) + duration
redis.call("HSET", KEYS[1], "status", "ready", "expires_at", expires)
redis.call("HDEL", KEYS[1], "archive_email", "archive_link", "archive_lifetime_ms", "archive_mail_attempts")
redis.call("PEXPIRE", KEYS[1], duration)
redis.call("SET", KEYS[2], KEYS[1], "PX", duration)
redis.call("ZREM", KEYS[3], ARGV[2])
redis.call("ZADD", KEYS[4], expires, ARGV[2])
return 1
`

const retryArchiveMailScript = `
if redis.call("HGET", KEYS[1], "status") ~= "preparing" then return 0 end
local attempts = redis.call("HINCRBY", KEYS[1], "archive_mail_attempts", 1)
if attempts >= tonumber(ARGV[3]) then return 0 end
redis.call("ZADD", KEYS[2], ARGV[2], ARGV[1])
return 1
`

func (ss *ShareStore) notifyArchive(ctx context.Context, key string, id uuid.UUID, notify ArchiveNotifier) error {
	fields, err := ss.cache.HMGet(ctx, key, "status", "archive_email", "archive_name", "archive_link").Result()
	if err != nil {
		return fmt.Errorf("read ZIP notification: %w", err)
	}
	if fields[0] != "preparing" {
		return ss.discardArchive(ctx, id)
	}
	mailCtx, cancel := context.WithTimeout(ctx, archiveMailTimeout)
	defer cancel()
	email, _ := fields[1].(string)
	name, _ := fields[2].(string)
	link, _ := fields[3].(string)
	if notifyErr := notify(mailCtx, email, name, link); notifyErr != nil {
		return ss.retryArchiveNotification(ctx, key, id, notifyErr)
	}

	result, err := ss.cache.Eval(ctx, activateArchiveScript,
		[]string{key, shareIDRedisKey(id), archiveJobsKey, archiveCleanupKey}, time.Now().UnixMilli(), id.String(),
	).Int64()
	if err != nil {
		return fmt.Errorf("activate mailed ZIP share: %w", err)
	}
	if result == 0 {
		return ss.discardArchive(ctx, id)
	}
	return nil
}

func (ss *ShareStore) retryArchiveNotification(ctx context.Context, key string, id uuid.UUID, cause error) error {
	result, err := ss.cache.Eval(ctx, retryArchiveMailScript, []string{key, archiveJobsKey},
		id.String(), time.Now().Add(archiveMailRetryDelay).UnixMilli(), archiveMailMaxAttempts,
	).Int64()
	if err != nil {
		return errors.Join(cause, fmt.Errorf("reschedule ZIP email: %w", err))
	}
	if result == 0 {
		return ss.failArchive(ctx, key, id, cause)
	}
	return cause
}
