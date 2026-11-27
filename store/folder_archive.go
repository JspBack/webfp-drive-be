package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxArchiveEntries  = 10_000
	maxArchiveBytes    = 1 << 30
	maxArchiveManifest = 2 << 20
	maxArchiveLifetime = 30 * 24 * time.Hour
)

type archiveEntry struct {
	Path []string `json:"path"`
	Key  string   `json:"key,omitempty"`
	Size int64    `json:"size,omitempty"`
}

type ArchiveDelivery struct {
	Email      string
	LinkPrefix string
}

type folderArchive struct {
	delivery ArchiveDelivery
	name     string
	entries  []byte
}

func (ss *ShareStore) CreateFolderShare(
	ctx context.Context, ownerID, folderID uuid.UUID, expiresAt time.Time, maxDownloads *int64, password string, delivery ArchiveDelivery,
) (*Share, error) {
	if err := validateShareOptions(expiresAt, maxDownloads); err != nil {
		return nil, err
	}
	if time.Until(expiresAt) > maxArchiveLifetime {
		return nil, fmt.Errorf("%w: folder ZIP shares can last up to 30 days", ErrInvalidShareOptions)
	}
	archive, err := ss.folderSnapshot(ctx, ownerID, folderID)
	if err != nil {
		return nil, err
	}
	archive.delivery = delivery
	return ss.createShare(ctx, ownerID, folderID, expiresAt, maxDownloads, password, archive)
}

func (ss *ShareStore) folderSnapshot(ctx context.Context, ownerID, folderID uuid.UUID) (*folderArchive, error) {
	folders, files := ss.files.table("folders"), ss.files.table("files")
	query := `WITH RECURSIVE tree AS (
		SELECT id, ARRAY[name] AS path, ARRAY[id] AS visited FROM ` + folders + ` WHERE owner_id=$1 AND id=$2
		UNION ALL
		SELECT child.id, tree.path || child.name, tree.visited || child.id
		FROM ` + folders + ` child JOIN tree ON child.parent_id=tree.id
		WHERE child.owner_id=$1 AND NOT child.id=ANY(tree.visited)
	)
	SELECT path, '' AS object_key, 0::bigint AS bytes FROM tree
	UNION ALL
	SELECT tree.path || f.filename, f.object_key, f.size_bytes
	FROM ` + files + ` f JOIN tree ON f.folder_id=tree.id
	WHERE f.owner_id=$1 AND f.status='ready'
	LIMIT $3`
	rows, err := ss.files.db.Query(ctx, query, ownerID, folderID, maxArchiveEntries+1)
	if err != nil {
		return nil, fmt.Errorf("list folder snapshot: %w", err)
	}
	defer rows.Close()

	var entries []archiveEntry
	var total int64
	var root string
	for rows.Next() {
		var entry archiveEntry
		if scanErr := rows.Scan(&entry.Path, &entry.Key, &entry.Size); scanErr != nil {
			return nil, fmt.Errorf("scan archive entry: %w", scanErr)
		}
		if len(entry.Path) == 1 {
			root = entry.Path[0]
		}
		total += entry.Size
		if len(entries) >= maxArchiveEntries || total > maxArchiveBytes {
			return nil, fmt.Errorf("%w: ZIP shares support up to 10,000 entries and 1 GiB of files", ErrInvalidShareOptions)
		}
		entries = append(entries, entry)
	}
	if rowErr := rows.Err(); rowErr != nil {
		return nil, fmt.Errorf("read folder snapshot: %w", rowErr)
	}
	if len(entries) == 0 {
		return nil, ErrNotFound
	}
	payload, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("encode folder snapshot: %w", err)
	}
	if len(payload) > maxArchiveManifest {
		return nil, fmt.Errorf("%w: folder paths are too large for a ZIP share", ErrInvalidShareOptions)
	}
	return &folderArchive{name: archiveSegment(root) + ".zip", entries: payload}, nil
}

func archiveObjectKey(id uuid.UUID) string {
	return "share-archives/" + id.String() + ".zip"
}

func archiveSegment(name string) string {
	if name == "" {
		return "%00"
	}
	escaped := strings.NewReplacer("%", "%25", "/", "%2F", "\\", "%5C", ":", "%3A", "\x00", "%00").Replace(name)
	prefix := strings.TrimRight(escaped, ". ")
	return prefix + strings.NewReplacer(".", "%2E", " ", "%20").Replace(escaped[len(prefix):])
}
