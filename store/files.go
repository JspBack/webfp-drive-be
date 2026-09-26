package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("file not found")

type File struct {
	ID          uuid.UUID  `json:"id"`
	OwnerID     uuid.UUID  `json:"owner_id"`
	FolderID    *uuid.UUID `json:"folder_id,omitempty"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	SizeBytes   int64      `json:"size_bytes"`
	ChecksumSHA *string    `json:"checksum_sha256,omitempty"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	objectKey   string
}

type FileStore struct {
	s3                  *s3.Client
	db                  *pgxpool.Pool
	presign             *s3.PresignClient
	bucket              string
	uploadURLLifetime   time.Duration
	downloadURLLifetime time.Duration
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanFile(row rowScanner) (*File, error) {
	file := &File{}
	var folderID, checksum sql.NullString
	err := row.Scan(
		&file.ID,
		&file.OwnerID,
		&folderID,
		&file.objectKey,
		&file.Filename,
		&file.ContentType,
		&file.SizeBytes,
		&checksum,
		&file.Status,
		&file.CreatedAt,
		&file.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if folderID.Valid {
		id, err := uuid.Parse(folderID.String)
		if err != nil {
			return nil, fmt.Errorf("parse file folder ID: %w", err)
		}
		file.FolderID = &id
	}
	if checksum.Valid {
		file.ChecksumSHA = &checksum.String
	}
	return file, nil
}

const fileColumns = `id, owner_id, folder_id::text, object_key, filename, content_type,
	size_bytes, checksum_sha256, status, created_at, updated_at`

// CreateUpload records pending metadata and returns a temporary direct-to-R2 upload URL.
func (fs *FileStore) CreateUpload(
	ctx context.Context,
	ownerID uuid.UUID,
	folderID *uuid.UUID,
	filename, contentType string,
	sizeBytes int64,
) (*File, string, error) {
	filename = strings.TrimSpace(filename)
	contentType = strings.TrimSpace(contentType)
	if filename == "" || sizeBytes < 0 {
		return nil, "", fmt.Errorf("filename must not be empty and size must be non-negative")
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	fileID := uuid.New()
	objectKey := fmt.Sprintf("users/%s/%s", ownerID, fileID)
	file, err := scanFile(fs.db.QueryRow(ctx, `
		INSERT INTO files (id, owner_id, folder_id, object_key, filename, content_type, size_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+fileColumns,
		fileID, ownerID, folderID, objectKey, filename, contentType, sizeBytes,
	))
	if err != nil {
		return nil, "", fmt.Errorf("create pending file: %w", err)
	}

	presigned, err := fs.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(fs.bucket),
		Key:         aws.String(file.objectKey),
		ContentType: aws.String(file.ContentType),
	}, func(options *s3.PresignOptions) {
		options.Expires = fs.uploadURLLifetime
	})
	if err != nil {
		_, cleanupErr := fs.db.Exec(ctx, `DELETE FROM files WHERE id = $1 AND owner_id = $2`, file.ID, ownerID)
		if cleanupErr != nil {
			return nil, "", errors.Join(
				fmt.Errorf("presign file upload: %w", err),
				fmt.Errorf("clean up pending file record: %w", cleanupErr),
			)
		}
		return nil, "", fmt.Errorf("presign file upload: %w", err)
	}
	return file, presigned.URL, nil
}

// CompleteUpload verifies that the direct upload reached R2 with the declared size.
func (fs *FileStore) CompleteUpload(ctx context.Context, ownerID, fileID uuid.UUID) (*File, error) {
	file, err := scanFile(fs.db.QueryRow(ctx,
		`SELECT `+fileColumns+` FROM files WHERE id = $1 AND owner_id = $2 AND status = 'pending'`,
		fileID, ownerID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load pending file: %w", err)
	}

	head, err := fs.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(fs.bucket),
		Key:    aws.String(file.objectKey),
	})
	if err != nil {
		return nil, fmt.Errorf("verify uploaded object: %w", err)
	}
	if head.ContentLength == nil || *head.ContentLength != file.SizeBytes {
		return nil, fmt.Errorf("uploaded object size does not match declared size")
	}

	updated, err := scanFile(fs.db.QueryRow(ctx, `
		UPDATE files SET status = 'ready', updated_at = now()
		WHERE id = $1 AND owner_id = $2 AND status = 'pending'
		RETURNING `+fileColumns,
		fileID, ownerID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("mark uploaded file ready: %w", err)
	}
	return updated, nil
}

func (fs *FileStore) Get(ctx context.Context, ownerID, fileID uuid.UUID) (*File, error) {
	file, err := scanFile(fs.db.QueryRow(ctx,
		`SELECT `+fileColumns+` FROM files WHERE id = $1 AND owner_id = $2`,
		fileID, ownerID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get file: %w", err)
	}
	return file, nil
}

func (fs *FileStore) List(ctx context.Context, ownerID uuid.UUID, folderID *uuid.UUID) ([]File, error) {
	rows, err := fs.db.Query(ctx, `
		SELECT `+fileColumns+`
		FROM files
		WHERE owner_id = $1 AND folder_id IS NOT DISTINCT FROM $2
		ORDER BY created_at DESC
	`, ownerID, folderID)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()

	files := make([]File, 0)
	for rows.Next() {
		file, err := scanFile(rows)
		if err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		files = append(files, *file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate files: %w", err)
	}
	return files, nil
}

func (fs *FileStore) Update(ctx context.Context, ownerID, fileID uuid.UUID, filename string, folderID *uuid.UUID) (*File, error) {
	file, err := scanFile(fs.db.QueryRow(ctx, `
		UPDATE files
		SET filename = $3, folder_id = $4, updated_at = now()
		WHERE id = $1 AND owner_id = $2
		RETURNING `+fileColumns,
		fileID, ownerID, strings.TrimSpace(filename), folderID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update file: %w", err)
	}
	return file, nil
}

func (fs *FileStore) DownloadURL(ctx context.Context, ownerID, fileID uuid.UUID) (string, error) {
	file, err := scanFile(fs.db.QueryRow(ctx,
		`SELECT `+fileColumns+` FROM files WHERE id = $1 AND owner_id = $2 AND status = 'ready'`,
		fileID, ownerID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("get file for download: %w", err)
	}
	presigned, err := fs.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(fs.bucket),
		Key:    aws.String(file.objectKey),
	}, func(options *s3.PresignOptions) {
		options.Expires = fs.downloadURLLifetime
	})
	if err != nil {
		return "", fmt.Errorf("presign file download: %w", err)
	}
	return presigned.URL, nil
}

func (fs *FileStore) Delete(ctx context.Context, ownerID, fileID uuid.UUID) error {
	file, err := fs.Get(ctx, ownerID, fileID)
	if err != nil {
		return err
	}
	_, err = fs.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(fs.bucket),
		Key:    aws.String(file.objectKey),
	})
	if err != nil {
		return fmt.Errorf("delete file object: %w", err)
	}
	tag, err := fs.db.Exec(ctx, `DELETE FROM files WHERE id = $1 AND owner_id = $2`, fileID, ownerID)
	if err != nil {
		return fmt.Errorf("delete file metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
