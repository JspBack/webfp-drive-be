package upload

import (
	"errors"
	"fmt"
	"io"
	"net/http"
)

const MaxBatchFiles = 100

func ParseBatch(r *http.Request, maxFileSize int64) ([]*MultipartUpload, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf("read multipart batch: %w", err)
	}
	var files []*MultipartUpload
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			if len(files) == 0 {
				return nil, errors.New("at least one file is required")
			}
			return files, nil
		}
		if nextErr != nil {
			return nil, errors.Join(fmt.Errorf("read batch part: %w", nextErr), CleanupBatch(files))
		}
		if part.FormName() != "file" || len(files) >= MaxBatchFiles {
			return nil, errors.Join(errors.New("send between 1 and 100 file fields"), CleanupBatch(files))
		}
		file := &MultipartUpload{}
		files = append(files, file)
		if readErr := readFilePart(part, file, maxFileSize); readErr != nil {
			return nil, errors.Join(readErr, CleanupBatch(files))
		}
	}
}

func CleanupBatch(files []*MultipartUpload) error {
	var err error
	for _, file := range files {
		err = errors.Join(err, file.Cleanup())
	}
	return err
}
