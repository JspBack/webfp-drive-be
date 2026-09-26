package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrFolderNotFound = errors.New("folder not found")
	ErrFolderNotEmpty = errors.New("folder is not empty")
)

type Folder struct {
	ID        uuid.UUID  `json:"id"`
	OwnerID   uuid.UUID  `json:"owner_id"`
	ParentID  *uuid.UUID `json:"parent_id,omitempty"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type FolderStore struct {
	db *pgxpool.Pool
}

func scanFolder(row interface{ Scan(...any) error }) (*Folder, error) {
	folder := &Folder{}
	var parentID sql.NullString
	if err := row.Scan(
		&folder.ID,
		&folder.OwnerID,
		&parentID,
		&folder.Name,
		&folder.CreatedAt,
		&folder.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if parentID.Valid {
		id, err := uuid.Parse(parentID.String)
		if err != nil {
			return nil, fmt.Errorf("parse parent folder ID: %w", err)
		}
		folder.ParentID = &id
	}
	return folder, nil
}

const folderColumns = `id, owner_id, parent_id::text, name, created_at, updated_at`

func (fs *FolderStore) Create(ctx context.Context, ownerID uuid.UUID, parentID *uuid.UUID, name string) (*Folder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("folder name is required")
	}
	folder, err := scanFolder(fs.db.QueryRow(ctx, `
		INSERT INTO folders (id, owner_id, parent_id, name)
		VALUES ($1, $2, $3, $4)
		RETURNING `+folderColumns,
		uuid.New(), ownerID, parentID, name,
	))
	if err != nil {
		return nil, fmt.Errorf("create folder: %w", err)
	}
	return folder, nil
}

func (fs *FolderStore) Get(ctx context.Context, ownerID, folderID uuid.UUID) (*Folder, error) {
	folder, err := scanFolder(fs.db.QueryRow(ctx, `
		SELECT `+folderColumns+` FROM folders WHERE id = $1 AND owner_id = $2
	`, folderID, ownerID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFolderNotFound
		}
		return nil, fmt.Errorf("get folder: %w", err)
	}
	return folder, nil
}

func (fs *FolderStore) List(ctx context.Context, ownerID uuid.UUID, parentID *uuid.UUID) ([]Folder, error) {
	rows, err := fs.db.Query(ctx, `
		SELECT `+folderColumns+`
		FROM folders
		WHERE owner_id = $1 AND parent_id IS NOT DISTINCT FROM $2
		ORDER BY name
	`, ownerID, parentID)
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	defer rows.Close()

	folders := make([]Folder, 0)
	for rows.Next() {
		folder, err := scanFolder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan folder: %w", err)
		}
		folders = append(folders, *folder)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate folders: %w", err)
	}
	return folders, nil
}

func (fs *FolderStore) Update(ctx context.Context, ownerID, folderID uuid.UUID, name string) (*Folder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("folder name is required")
	}
	folder, err := scanFolder(fs.db.QueryRow(ctx, `
		UPDATE folders SET name = $3, updated_at = now()
		WHERE id = $1 AND owner_id = $2
		RETURNING `+folderColumns,
		folderID, ownerID, name,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFolderNotFound
		}
		return nil, fmt.Errorf("update folder: %w", err)
	}
	return folder, nil
}

func (fs *FolderStore) Delete(ctx context.Context, ownerID, folderID uuid.UUID) error {
	tag, err := fs.db.Exec(ctx, `
		DELETE FROM folders f
		WHERE f.id = $1 AND f.owner_id = $2
			AND NOT EXISTS (SELECT 1 FROM folders child WHERE child.parent_id = f.id)
			AND NOT EXISTS (SELECT 1 FROM files file WHERE file.folder_id = f.id)
	`, folderID, ownerID)
	if err != nil {
		return fmt.Errorf("delete folder: %w", err)
	}
	if tag.RowsAffected() == 0 {
		if _, err := fs.Get(ctx, ownerID, folderID); err != nil {
			return err
		}
		return ErrFolderNotEmpty
	}
	return nil
}
