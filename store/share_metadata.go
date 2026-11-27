package store

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/google/uuid"
)

func (ss *ShareStore) shareFields(ctx context.Context, key string, fields ...string) (map[string]string, error) {
	values, err := ss.cache.HMGet(ctx, key, fields...).Result()
	if err != nil {
		return nil, fmt.Errorf("read share metadata: %w", err)
	}
	metadata := make(map[string]string, len(fields))
	for i, value := range values {
		if field, ok := value.(string); ok {
			metadata[fields[i]] = field
		}
	}
	return metadata, nil
}

func shareRedisKey(tokenHash [sha256.Size]byte) string {
	return "share:" + base64.RawURLEncoding.EncodeToString(tokenHash[:])
}

func shareIDRedisKey(id uuid.UUID) string {
	return "share-id:" + id.String()
}

func shareOwnerRedisKey(ownerID uuid.UUID) string {
	return "share-owner:" + ownerID.String()
}

func shareStatus(fields map[string]string) string {
	if fields["archive_name"] != "" {
		return fields["status"]
	}
	return readyStatus
}
