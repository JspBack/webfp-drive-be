package store

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

func (ss *ShareStore) buildArchive(ctx context.Context, id uuid.UUID, entries []archiveEntry) (int64, error) {
	tmp, err := os.CreateTemp("", "webfp-share-*.zip")
	if err != nil {
		return 0, fmt.Errorf("create temporary ZIP: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	if writeErr := ss.writeArchive(ctx, tmp, entries); writeErr != nil {
		return 0, writeErr
	}
	info, err := tmp.Stat()
	if err != nil {
		return 0, fmt.Errorf("read ZIP size: %w", err)
	}
	if _, seekErr := tmp.Seek(0, io.SeekStart); seekErr != nil {
		return 0, fmt.Errorf("rewind ZIP: %w", seekErr)
	}
	if _, uploadErr := ss.files.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(ss.files.bucket),
		Key:           aws.String(archiveObjectKey(id)),
		Body:          tmp,
		ContentLength: aws.Int64(info.Size()),
		ContentType:   aws.String("application/zip"),
	}); uploadErr != nil {
		return 0, fmt.Errorf("store ZIP snapshot: %w", uploadErr)
	}
	return info.Size(), nil
}

func (ss *ShareStore) writeArchive(ctx context.Context, dst io.Writer, entries []archiveEntry) error {
	writer := zip.NewWriter(dst)
	used := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.Key == "" {
			name := archivePath(entry.Path)
			used[name] = true
			if _, err := writer.Create(name + "/"); err != nil {
				return fmt.Errorf("add ZIP directory: %w", err)
			}
		}
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("write ZIP archive: %w", err)
		}
		if entry.Key == "" {
			continue
		}
		name := archivePath(entry.Path)
		candidate := name
		for suffix := 2; used[candidate]; suffix++ {
			candidate = fmt.Sprintf("%s (%d)", name, suffix)
		}
		used[candidate] = true
		part, err := writer.Create(candidate)
		if err != nil {
			return fmt.Errorf("add ZIP file: %w", err)
		}
		if copyErr := ss.copyArchiveEntry(ctx, part, entry); copyErr != nil {
			return copyErr
		}
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish ZIP archive: %w", err)
	}
	return nil
}

func archivePath(segments []string) string {
	path := make([]string, len(segments))
	for index, segment := range segments {
		path[index] = archiveSegment(segment)
	}
	return strings.Join(path, "/")
}

func (ss *ShareStore) copyArchiveEntry(ctx context.Context, dst io.Writer, entry archiveEntry) error {
	object, err := ss.files.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(ss.files.bucket),
		Key:    aws.String(entry.Key),
	})
	if err != nil {
		return fmt.Errorf("open file for ZIP: %w", err)
	}
	n, copyErr := io.Copy(dst, io.LimitReader(object.Body, entry.Size+1))
	closeErr := object.Body.Close()
	if copyErr != nil || closeErr != nil {
		return errors.Join(wrapCleanupError("copy ZIP entry", copyErr), closeObjectError(closeErr))
	}
	if n != entry.Size {
		return errors.New("file size changed while preparing ZIP")
	}
	return nil
}
