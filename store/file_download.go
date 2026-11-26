package store

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

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
