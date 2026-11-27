package store

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
)

const MaxBulkDeleteItems = 1000

func (fs *FileStore) DeleteFiles(ctx context.Context, ownerID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	failures := make(map[uuid.UUID]string)
	if len(ids) == 0 {
		return failures, nil
	}
	rows, err := fs.db.Query(ctx, `SELECT id, object_key FROM `+fs.table("files")+`
		WHERE owner_id=$1 AND id=ANY($2) AND status='ready'`, ownerID, ids)
	if err != nil {
		return nil, fmt.Errorf("list files for bulk delete: %w", err)
	}

	keys := make(map[string]uuid.UUID, len(ids))
	var objects []types.ObjectIdentifier
	for rows.Next() {
		var id uuid.UUID
		var key string
		if scanErr := rows.Scan(&id, &key); scanErr != nil {
			rows.Close()
			return nil, fmt.Errorf("scan file for bulk delete: %w", scanErr)
		}
		keys[key] = id
		objects = append(objects, types.ObjectIdentifier{Key: aws.String(key)})
	}
	rows.Close()
	if rowErr := rows.Err(); rowErr != nil {
		return nil, fmt.Errorf("read files for bulk delete: %w", rowErr)
	}

	for _, id := range ids {
		failures[id] = "File not found or unavailable"
	}
	if len(objects) == 0 {
		return failures, nil
	}
	result, err := fs.s3.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(fs.bucket),
		Delete: &types.Delete{Objects: objects},
	})
	if err != nil {
		return nil, fmt.Errorf("delete file objects: %w", err)
	}

	for _, object := range result.Errors {
		if id, ok := keys[aws.ToString(object.Key)]; ok {
			failures[id] = "Unable to delete the file"
		}
	}
	deleted := make([]uuid.UUID, 0, len(result.Deleted))
	for _, object := range result.Deleted {
		if id, ok := keys[aws.ToString(object.Key)]; ok {
			deleted = append(deleted, id)
			delete(failures, id)
		}
	}
	if len(deleted) > 0 {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), uploadCleanupTimeout)
		defer cancel()
		if _, execErr := fs.db.Exec(cleanupCtx, `DELETE FROM `+fs.table("files")+`
			WHERE owner_id=$1 AND id=ANY($2)`, ownerID, deleted); execErr != nil {
			return nil, fmt.Errorf("delete file metadata: %w", execErr)
		}
	}
	return failures, nil
}
