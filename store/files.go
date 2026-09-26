package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	storehelper "github.com/JspBack/webfp-drive/helper/store"
)

var (
	ErrNotFound             = errors.New("file not found")
	ErrStorageLimitExceeded = errors.New("storage limit exceeded")
)

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
	schemaTables
	s3           *s3.Client
	db           *pgxpool.Pool
	readDB       *pgxpool.Pool
	bucket       string
	storageLimit int64
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
		return nil, fmt.Errorf("scan file row: %w", err)
	}
	if folderID.Valid {
		folderUUID, parseErr := uuid.Parse(folderID.String)
		if parseErr != nil {
			return nil, fmt.Errorf("parse file folder ID: %w", parseErr)
		}
		file.FolderID = &folderUUID
	}
	if checksum.Valid {
		file.ChecksumSHA = &checksum.String
	}
	return file, nil
}

const fileColumns = `id, owner_id, folder_id::text, object_key, filename, content_type,
	size_bytes, checksum_sha256, status, created_at, updated_at`

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
		if err := fs.reserveQuota(ctx, tx, ownerID, sizeBytes); err != nil {
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
		if errors.Is(err, ErrStorageLimitExceeded) {
			return nil, fmt.Errorf("%w: limit is %d bytes", ErrStorageLimitExceeded, fs.storageLimit)
		}
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

func (fs *FileStore) reserveQuota(ctx context.Context, tx pgx.Tx, ownerID uuid.UUID, sizeBytes int64) error {
	if fs.storageLimit <= 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), 0)`, ownerID.String()); err != nil {
		return fmt.Errorf("lock storage usage: %w", err)
	}
	var used int64
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(size_bytes), 0) FROM `+fs.table("files")+` WHERE owner_id = $1`,
		ownerID,
	).Scan(&used); err != nil {
		return fmt.Errorf("read storage usage: %w", err)
	}
	if used+sizeBytes > fs.storageLimit {
		return ErrStorageLimitExceeded
	}
	return nil
}

func (fs *FileStore) StorageLimit() int64 {
	return fs.storageLimit
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

func (fs *FileStore) Usage(ctx context.Context, ownerID uuid.UUID) (int64, int64, error) {
	var usedBytes, fileCount int64
	if err := fs.db.QueryRow(ctx,
		`SELECT COALESCE(SUM(size_bytes), 0), COUNT(*) FROM `+fs.table("files")+` WHERE owner_id = $1`,
		ownerID,
	).Scan(&usedBytes, &fileCount); err != nil {
		return 0, 0, fmt.Errorf("read storage usage: %w", err)
	}
	return usedBytes, fileCount, nil
}

func (fs *FileStore) GetFilenames(ctx context.Context, ownerID uuid.UUID, fileIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	filenames := make(map[uuid.UUID]string, len(fileIDs))
	if len(fileIDs) == 0 {
		return filenames, nil
	}
	ids := make([]string, 0, len(fileIDs))
	for _, id := range fileIDs {
		ids = append(ids, id.String())
	}
	rows, err := fs.db.Query(ctx,
		`SELECT id::text, filename FROM `+fs.table("files")+` WHERE owner_id = $1 AND id::text = ANY($2)`,
		ownerID, ids,
	)
	if err != nil {
		return nil, fmt.Errorf("load file names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rawID, filename string
		if scanErr := rows.Scan(&rawID, &filename); scanErr != nil {
			return nil, fmt.Errorf("scan file name: %w", scanErr)
		}
		fileID, parseErr := uuid.Parse(rawID)
		if parseErr != nil {
			continue
		}
		filenames[fileID] = filename
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate file names: %w", rowsErr)
	}
	return filenames, nil
}

func (fs *FileStore) Get(ctx context.Context, ownerID, fileID uuid.UUID) (*File, error) {
	file, err := scanFile(fs.db.QueryRow(ctx,
		`SELECT `+fileColumns+` FROM `+fs.table("files")+` WHERE id = $1 AND owner_id = $2`,
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
	readDB := fs.readDB
	if readDB == nil {
		readDB = fs.db
	}
	rows, err := readDB.Query(ctx, `
		SELECT `+fileColumns+`
		FROM `+fs.table("files")+`
		WHERE owner_id = $1 AND folder_id IS NOT DISTINCT FROM $2
		ORDER BY created_at DESC
	`, ownerID, folderID)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	defer rows.Close()

	files := make([]File, 0)
	for rows.Next() {
		file, scanErr := scanFile(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan file: %w", scanErr)
		}
		files = append(files, *file)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate files: %w", rowsErr)
	}
	return files, nil
}

func (fs *FileStore) Update(
	ctx context.Context,
	ownerID, fileID uuid.UUID,
	filename *string,
	folderID *uuid.UUID,
	folderIDSet bool,
) (*File, error) {
	var filenameValue any
	if filename != nil {
		filenameValue = strings.TrimSpace(*filename)
	}
	file, err := scanFile(fs.db.QueryRow(ctx, `
		UPDATE `+fs.table("files")+`
		SET filename = CASE WHEN $3 THEN $4::text ELSE filename END,
			folder_id = CASE WHEN $5 THEN $6::uuid ELSE folder_id END,
			updated_at = now()
		WHERE id = $1 AND owner_id = $2
		RETURNING `+fileColumns,
		fileID, ownerID, filename != nil, filenameValue, folderIDSet, folderID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if storehelper.IsForeignKeyViolation(err, "files_folder_owner_fk") {
			return nil, ErrFolderNotFound
		}
		return nil, fmt.Errorf("update file: %w", err)
	}
	return file, nil
}

func (fs *FileStore) Download(ctx context.Context, ownerID, fileID uuid.UUID) (*File, io.ReadCloser, error) {
	file, err := scanFile(fs.db.QueryRow(ctx,
		`SELECT `+fileColumns+` FROM `+fs.table("files")+` WHERE id = $1 AND owner_id = $2 AND status = 'ready'`,
		fileID, ownerID,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, fmt.Errorf("get file for download: %w", err)
	}
	object, err := fs.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(fs.bucket),
		Key:    aws.String(file.objectKey),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("get file object: %w", err)
	}
	return file, object.Body, nil
}

func (fs *FileStore) deleteMetadata(ctx context.Context, fileID, ownerID uuid.UUID) error {
	if _, err := fs.db.Exec(ctx, `DELETE FROM `+fs.table("files")+` WHERE id = $1 AND owner_id = $2`, fileID, ownerID); err != nil {
		return fmt.Errorf("delete file metadata: %w", err)
	}
	return nil
}

func wrapCleanupError(operation string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
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
	tag, err := fs.db.Exec(ctx, `DELETE FROM `+fs.table("files")+` WHERE id = $1 AND owner_id = $2`, fileID, ownerID)
	if err != nil {
		return fmt.Errorf("delete file metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
