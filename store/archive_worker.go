package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	archiveJobsKey      = "share-archive:jobs"
	archiveCleanupKey   = "share-archive:cleanup"
	archiveBuildTimeout = 15 * time.Minute
	archiveJobLease     = 20 * time.Minute
	archivePollInterval = time.Minute
	archiveCleanupBatch = 20
)

const claimArchiveScript = `
local job = redis.call("ZRANGEBYSCORE", KEYS[1], "-inf", ARGV[1], "LIMIT", 0, 1)[1]
if job then redis.call("ZADD", KEYS[1], ARGV[2], job) end
return job
`

const failArchiveScript = `
redis.call("ZREM", KEYS[2], ARGV[1])
if redis.call("HGET", KEYS[1], "status") ~= "preparing" then return 0 end
redis.call("HSET", KEYS[1], "status", "failed")
redis.call("HDEL", KEYS[1], "archive_entries", "archive_size", "archive_email",
    "archive_link", "archive_lifetime_ms", "archive_mail_attempts")
return 1
`

func (ss *ShareStore) wakeArchiveWorker() {
	select {
	case ss.archiveWake <- struct{}{}:
	default:
	}
}

func (ss *ShareStore) RunArchiveWorker(ctx context.Context, logger *slog.Logger, notify ArchiveNotifier) {
	ticker := time.NewTicker(archivePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-ss.archiveWake:
		}

		if err := ss.cleanArchives(ctx); err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "clean up ZIP shares", "error", err)
		}
		for ctx.Err() == nil {
			processed, err := ss.processArchiveJob(ctx, notify)
			if err != nil && ctx.Err() == nil {
				logger.ErrorContext(ctx, "process ZIP share", "error", err)
			}
			if !processed {
				break
			}
		}
	}
}

func (ss *ShareStore) processArchiveJob(ctx context.Context, notify ArchiveNotifier) (bool, error) {
	now := time.Now()
	rawID, err := ss.cache.Eval(ctx, claimArchiveScript, []string{archiveJobsKey},
		now.UnixMilli(), now.Add(archiveJobLease).UnixMilli(),
	).Text()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim ZIP job: %w", err)
	}
	id, err := uuid.Parse(rawID)
	if err != nil {
		return true, errors.Join(fmt.Errorf("parse ZIP job ID: %w", err), ss.removeArchiveQueueEntry(ctx, archiveJobsKey, rawID))
	}
	return true, ss.prepareArchive(ctx, id, notify)
}

func (ss *ShareStore) prepareArchive(ctx context.Context, id uuid.UUID, notify ArchiveNotifier) error {
	key, err := ss.cache.Get(ctx, shareIDRedisKey(id)).Result()
	if errors.Is(err, redis.Nil) {
		return ss.discardArchive(ctx, id)
	}
	if err != nil {
		return fmt.Errorf("find ZIP share: %w", err)
	}
	raw, err := ss.cache.HGet(ctx, key, "archive_entries").Result()
	if errors.Is(err, redis.Nil) {
		return ss.notifyArchive(ctx, key, id, notify)
	}
	if err != nil {
		return fmt.Errorf("load ZIP manifest: %w", err)
	}
	var entries []archiveEntry
	if decodeErr := json.Unmarshal([]byte(raw), &entries); decodeErr != nil {
		return ss.failArchive(ctx, key, id, fmt.Errorf("decode ZIP manifest: %w", decodeErr))
	}

	buildCtx, cancel := context.WithTimeout(ctx, archiveBuildTimeout)
	defer cancel()
	size, buildErr := ss.buildArchive(buildCtx, id, entries)
	if ctx.Err() != nil {
		return fmt.Errorf("archive worker stopped: %w", ctx.Err())
	}
	if buildErr != nil {
		return ss.failArchive(ctx, key, id, buildErr)
	}
	saved, err := ss.cache.Eval(ctx, recordArchiveScript, []string{key}, size).Int64()
	if err != nil {
		return fmt.Errorf("record prepared ZIP: %w", err)
	}
	if saved == 0 {
		return ss.discardArchive(ctx, id)
	}
	return ss.notifyArchive(ctx, key, id, notify)
}

func (ss *ShareStore) failArchive(ctx context.Context, key string, id uuid.UUID, cause error) error {
	if err := ss.cache.Eval(ctx, failArchiveScript, []string{key, archiveJobsKey}, id.String()).Err(); err != nil {
		return errors.Join(cause, fmt.Errorf("mark ZIP share failed: %w", err))
	}
	return errors.Join(cause, ss.discardArchive(ctx, id))
}

func (ss *ShareStore) discardArchive(ctx context.Context, id uuid.UUID) error {
	deleteErr := ss.deleteArchive(ctx, id)
	if deleteErr != nil {
		err := ss.cache.ZAdd(ctx, archiveCleanupKey, redis.Z{
			Score: float64(time.Now().UnixMilli()), Member: id.String(),
		}).Err()
		if err != nil {
			return errors.Join(deleteErr, fmt.Errorf("queue ZIP cleanup: %w", err))
		}
	}
	return errors.Join(deleteErr, ss.removeArchiveQueueEntry(ctx, archiveJobsKey, id.String()))
}

func (ss *ShareStore) cleanArchives(ctx context.Context) error {
	ids, err := ss.cache.ZRangeByScore(ctx, archiveCleanupKey, &redis.ZRangeBy{
		Min: "-inf", Max: strconv.FormatInt(time.Now().UnixMilli(), 10), Count: archiveCleanupBatch,
	}).Result()
	if err != nil {
		return fmt.Errorf("list expired ZIP shares: %w", err)
	}
	var cleanupErr error
	for _, rawID := range ids {
		id, parseErr := uuid.Parse(rawID)
		if parseErr != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("parse expired ZIP share ID: %w", parseErr))
			continue
		}
		if deleteErr := ss.deleteArchive(ctx, id); deleteErr != nil {
			cleanupErr = errors.Join(cleanupErr, deleteErr)
			continue
		}
		cleanupErr = errors.Join(cleanupErr, ss.removeArchiveQueueEntry(ctx, archiveCleanupKey, rawID))
	}
	return cleanupErr
}

func (ss *ShareStore) removeArchiveQueueEntry(ctx context.Context, queue, id string) error {
	if err := ss.cache.ZRem(ctx, queue, id).Err(); err != nil {
		return fmt.Errorf("remove ZIP queue entry: %w", err)
	}
	return nil
}

func (ss *ShareStore) deleteArchive(ctx context.Context, id uuid.UUID) error {
	if _, err := ss.files.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(ss.files.bucket),
		Key:    aws.String(archiveObjectKey(id)),
	}); err != nil {
		return fmt.Errorf("delete ZIP snapshot: %w", err)
	}
	return nil
}
