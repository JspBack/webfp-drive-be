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

	storehelper "github.com/JspBack/webfp-drive/helper/store"
)

var (
	ErrNotFound             = errors.New("file not found")
	ErrStorageLimitExceeded = errors.New("storage limit exceeded")
	ErrSystemLocked         = errors.New("system storage lock is active")
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
	s3                         *s3.Client
	db                         *pgxpool.Pool
	readDB                     *pgxpool.Pool
	bucket                     string
	storageLimit               int64
	systemStorageLimit         int64
	systemStorageLockThreshold float64
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
	var file *File
	err := withTransaction(ctx, fs.db, "file update", func(tx pgx.Tx) error {
		if err := fs.checkSystemLock(ctx, tx); err != nil {
			return err
		}
		scanned, scanErr := scanFile(tx.QueryRow(ctx, `
			UPDATE `+fs.table("files")+`
			SET filename = CASE WHEN $3 THEN $4::text ELSE filename END,
				folder_id = CASE WHEN $5 THEN $6::uuid ELSE folder_id END,
				updated_at = now()
			WHERE id = $1 AND owner_id = $2
			RETURNING `+fileColumns,
			fileID, ownerID, filename != nil, filenameValue, folderIDSet, folderID,
		))
		if scanErr != nil {
			return scanErr
		}
		file = scanned
		return nil
	})
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
