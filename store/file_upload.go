package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	storehelper "github.com/JspBack/webfp-drive/helper/store"
)

func (fs *FileStore) Upload(
	ctx context.Context,
	ownerID uuid.UUID,
	folderID *uuid.UUID,
	filename, contentType string,
	sizeBytes int64,
	body io.Reader,
) (*File, error) {
	filename = strings.TrimSpace(filename)
	contentType = strings.TrimSpace(contentType)
	if filename == "" || sizeBytes <= 0 {
		return nil, errors.New("filename must not be empty and size must be positive")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	fileID := uuid.New()
	objectKey := fmt.Sprintf("users/%s/%s", ownerID, fileID)
	var file *File
	err := withTransaction(ctx, fs.db, "file upload", func(tx pgx.Tx) error {
		if err := fs.checkSystemLock(ctx, tx); err != nil {
			return err
		}
		if err := fs.reserveQuota(ctx, tx, ownerID, sizeBytes); err != nil {
			return err
		}
		if err := fs.enforceSystemLockOnWrite(ctx, tx); err != nil {
			return err
		}
		scanned, scanErr := scanFile(tx.QueryRow(ctx, `
			INSERT INTO `+fs.table("files")+` (id, owner_id, folder_id, object_key, filename, content_type, size_bytes)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING `+fileColumns,
			fileID, ownerID, folderID, objectKey, filename, contentType, sizeBytes,
		))
		if scanErr != nil {
			return scanErr
		}
		file = scanned
		return nil
	})
	if err != nil {
		if storehelper.IsForeignKeyViolation(err, "files_folder_owner_fk") {
			return nil, ErrFolderNotFound
		}
		return nil, fmt.Errorf("create pending file: %w", err)
	}

	_, err = fs.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(fs.bucket),
		Key:           aws.String(file.objectKey),
		Body:          body,
		ContentLength: aws.Int64(sizeBytes),
		ContentType:   aws.String(file.ContentType),
	})
	if err != nil {
		return nil, fs.abortUpload(ctx, file, ownerID, fmt.Errorf("upload file object: %w", err))
	}

	updated, err := scanFile(fs.db.QueryRow(ctx, `
		UPDATE `+fs.table("files")+` SET status = 'ready', updated_at = now()
		WHERE id = $1 AND owner_id = $2
		RETURNING `+fileColumns,
		file.ID, ownerID,
	))
	if err != nil {
		_, deleteErr := fs.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(fs.bucket),
			Key:    aws.String(file.objectKey),
		})
		if deleteErr != nil {
			err = errors.Join(err, fmt.Errorf("clean up uploaded object: %w", deleteErr))
		}
		return nil, errors.Join(fmt.Errorf("mark uploaded file ready: %w", err), fs.deleteMetadata(ctx, file.ID, ownerID))
	}
	return updated, nil
}

func (fs *FileStore) abortUpload(ctx context.Context, file *File, ownerID uuid.UUID, uploadErr error) error {
	_, deleteErr := fs.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(fs.bucket),
		Key:    aws.String(file.objectKey),
	})
	return errors.Join(
		uploadErr,
		wrapCleanupError("delete incomplete file object", deleteErr),
		fs.deleteMetadata(ctx, file.ID, ownerID),
	)
}

func (fs *FileStore) checkSystemLock(ctx context.Context, tx pgx.Tx) error {
	if fs.systemStorageLimit <= 0 {
		return nil
	}
	var locked bool
	if err := tx.QueryRow(ctx, `SELECT locked FROM `+fs.table("fs_lock")+` WHERE lock_id = true`).Scan(&locked); err != nil {
		return fmt.Errorf("check fs lock: %w", err)
	}
	if locked {
		return ErrSystemLocked
	}
	return nil
}

func (fs *FileStore) enforceSystemLockOnWrite(ctx context.Context, tx pgx.Tx) error {
	if fs.systemStorageLimit <= 0 {
		return nil
	}

	var locked bool
	var total int64
	lockStateQuery := `SELECT locked, COALESCE(total_bytes, 0)
		FROM ` + fs.table("fs_lock") + ` WHERE lock_id = true`
	if err := tx.QueryRow(ctx, lockStateQuery).Scan(&locked, &total); err != nil {
		return fmt.Errorf("read fs lock state: %w", err)
	}
	if locked {
		return ErrSystemLocked
	}

	var used int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(SUM(size_bytes), 0) FROM `+fs.table("files")+``).Scan(&used); err != nil {
		return fmt.Errorf("read total storage usage: %w", err)
	}

	if used > total {
		total = used
	}
	threshold := float64(total) / float64(fs.systemStorageLimit)
	if threshold >= fs.systemStorageLockThreshold {
		if _, err := tx.Exec(ctx, `
			UPDATE `+fs.table("fs_lock")+`
			SET locked = true, locked_at = now(), total_bytes = $1, threshold_hit_percent = $2, updated_at = now()
			WHERE lock_id = true AND locked = false
		`, used, threshold); err != nil {
			return fmt.Errorf("lock fs: %w", err)
		}
		return ErrSystemLocked
	}

	if _, err := tx.Exec(ctx, `
		UPDATE `+fs.table("fs_lock")+`
		SET total_bytes = $1, updated_at = now()
		WHERE lock_id = true
	`, used); err != nil {
		return fmt.Errorf("update fs lock totals: %w", err)
	}
	return nil
}
