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

	storehelper "github.com/JspBack/webfp-drive/helper/store"
)

var (
	ErrFolderNotFound       = errors.New("folder not found")
	ErrFolderParentNotFound = errors.New("parent folder not found")
	ErrFolderNotEmpty       = errors.New("folder is not empty")
	ErrFolderMoveCycle      = errors.New("folder move would create a cycle")
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
	db     *pgxpool.Pool
	readDB *pgxpool.Pool
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
		return nil, fmt.Errorf("scan folder row: %w", err)
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
		return nil, errors.New("folder name is required")
	}
	folder, err := scanFolder(fs.db.QueryRow(ctx, `
		INSERT INTO folders (id, owner_id, parent_id, name)
		VALUES ($1, $2, $3, $4)
		RETURNING `+folderColumns,
		uuid.New(), ownerID, parentID, name,
	))
	if err != nil {
		if storehelper.IsForeignKeyViolation(err, "folders_parent_owner_fk") {
			return nil, ErrFolderParentNotFound
		}
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
	readDB := fs.readDB
	if readDB == nil {
		readDB = fs.db
	}
	rows, err := readDB.Query(ctx, `
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
		folder, scanErr := scanFolder(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan folder: %w", scanErr)
		}
		folders = append(folders, *folder)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate folders: %w", rowsErr)
	}
	return folders, nil
}

func (fs *FolderStore) Update(
	ctx context.Context,
	ownerID, folderID uuid.UUID,
	name *string,
	parentID *uuid.UUID,
	parentIDSet bool,
) (*Folder, error) {
	var nameValue any
	if name != nil {
		trimmedName := strings.TrimSpace(*name)
		if trimmedName == "" {
			return nil, errors.New("folder name is required")
		}
		nameValue = trimmedName
	}
	var folder *Folder
	err := pgx.BeginFunc(ctx, fs.db, func(tx pgx.Tx) error {
		var locked bool
		lockErr := tx.QueryRow(ctx, `
			SELECT true FROM (SELECT pg_advisory_xact_lock(hashtext($1), 0)) AS folder_tree_lock
		`, ownerID.String()).Scan(&locked)
		if lockErr != nil {
			return fmt.Errorf("lock folder tree for update: %w", lockErr)
		}
		if parentIDSet && parentID != nil {
			var createsCycle bool
			cycleErr := tx.QueryRow(ctx, `
				WITH RECURSIVE descendants(id) AS (
					SELECT id FROM folders WHERE id = $1 AND owner_id = $2
					UNION
					SELECT child.id
					FROM folders child
					JOIN descendants parent ON child.parent_id = parent.id
					WHERE child.owner_id = $2
				)
				SELECT EXISTS(SELECT 1 FROM descendants WHERE id = $3)
			`, folderID, ownerID, *parentID).Scan(&createsCycle)
			if cycleErr != nil {
				return fmt.Errorf("check folder move destination: %w", cycleErr)
			}
			if createsCycle {
				return ErrFolderMoveCycle
			}
		}
		var updateErr error
		folder, updateErr = scanFolder(tx.QueryRow(ctx, `
			UPDATE folders
			SET name = CASE WHEN $3 THEN $4::text ELSE name END,
				parent_id = CASE WHEN $5 THEN $6::uuid ELSE parent_id END,
				updated_at = now()
			WHERE id = $1 AND owner_id = $2
			RETURNING `+folderColumns,
			folderID, ownerID, name != nil, nameValue, parentIDSet, parentID,
		))
		if updateErr != nil {
			if errors.Is(updateErr, pgx.ErrNoRows) {
				return ErrFolderNotFound
			}
			if storehelper.IsForeignKeyViolation(updateErr, "folders_parent_owner_fk") {
				return ErrFolderParentNotFound
			}
			return fmt.Errorf("update folder: %w", updateErr)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("update folder transaction: %w", err)
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
		if _, getErr := fs.Get(ctx, ownerID, folderID); getErr != nil {
			return getErr
		}
		return ErrFolderNotEmpty
	}
	return nil
}
