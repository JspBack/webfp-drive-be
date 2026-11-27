package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const proStorageLimitBytes int64 = 1 << 30

type StorageUsage struct {
	Plan       string `json:"plan"`
	UsedBytes  int64  `json:"used_bytes"`
	LimitBytes int64  `json:"limit_bytes"`
	FileCount  int64  `json:"file_count"`
}

func (fs *FileStore) storageLimitForPlan(plan string) (int64, error) {
	switch plan {
	case "free":
		return fs.storageLimit, nil
	case "pro":
		return proStorageLimitBytes, nil
	case "admin":
		return 0, nil
	default:
		return 0, fmt.Errorf("invalid storage plan %q", plan)
	}
}

func (fs *FileStore) reserveQuota(ctx context.Context, tx pgx.Tx, ownerID uuid.UUID, sizeBytes int64) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), 0)`, ownerID.String()); err != nil {
		return fmt.Errorf("lock storage usage: %w", err)
	}

	var plan string
	if err := tx.QueryRow(ctx, `SELECT storage_plan FROM `+fs.table("users")+` WHERE id = $1 FOR SHARE`, ownerID).Scan(&plan); err != nil {
		return fmt.Errorf("read storage plan: %w", err)
	}
	limit, err := fs.storageLimitForPlan(plan)
	if err != nil {
		return err
	}
	if limit == 0 {
		return nil
	}

	var used int64
	if scanErr := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(size_bytes), 0) FROM `+fs.table("files")+` WHERE owner_id = $1`,
		ownerID,
	).Scan(&used); scanErr != nil {
		return fmt.Errorf("read storage usage: %w", scanErr)
	}
	if sizeBytes > limit-used {
		return ErrStorageLimitExceeded
	}
	return nil
}

func (fs *FileStore) Usage(ctx context.Context, ownerID uuid.UUID) (StorageUsage, error) {
	var usage StorageUsage
	query := `SELECT u.storage_plan, COALESCE(SUM(f.size_bytes), 0), COUNT(f.id)
		FROM ` + fs.table("users") + ` u LEFT JOIN ` + fs.table("files") + ` f ON f.owner_id = u.id
		WHERE u.id = $1 GROUP BY u.id`
	if err := fs.db.QueryRow(ctx, query, ownerID).Scan(&usage.Plan, &usage.UsedBytes, &usage.FileCount); err != nil {
		return StorageUsage{}, fmt.Errorf("read storage usage: %w", err)
	}
	limit, err := fs.storageLimitForPlan(usage.Plan)
	if err != nil {
		return StorageUsage{}, err
	}
	usage.LimitBytes = limit
	return usage, nil
}
