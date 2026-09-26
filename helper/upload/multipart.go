package upload

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

var ErrFileTooLarge = errors.New("file exceeds configured size limit")

type MultipartUpload struct {
	File        *os.File
	Filename    string
	ContentType string
	Size        int64
	FolderID    string

	folderIDSet bool
}

func ParseMultipart(r *http.Request, maxFileSize, maxFieldSize int64) (*MultipartUpload, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, fmt.Errorf("read multipart upload: %w", err)
	}
	upload := &MultipartUpload{}
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, errors.Join(fmt.Errorf("read multipart part: %w", nextErr), upload.Cleanup())
		}
		var partErr error
		switch part.FormName() {
		case "file":
			if upload.File != nil {
				partErr = errors.New("only one file field is allowed")
			} else {
				partErr = readFilePart(part, upload, maxFileSize)
			}
		case "folder_id":
			if upload.folderIDSet {
				partErr = errors.New("only one folder_id field is allowed")
			} else {
				upload.folderIDSet = true
				upload.FolderID, partErr = readFolderIDPart(part, maxFieldSize)
			}
		default:
			partErr = fmt.Errorf("unexpected multipart field %q", part.FormName())
		}
		if closeErr := part.Close(); closeErr != nil {
			partErr = errors.Join(partErr, fmt.Errorf("close multipart field: %w", closeErr))
		}
		if partErr != nil {
			return nil, errors.Join(partErr, upload.Cleanup())
		}
	}
	if upload.File == nil {
		return nil, errors.New("file field is required")
	}
	if upload.Size <= 0 {
		return nil, errors.Join(errors.New("file must not be empty"), upload.Cleanup())
	}
	return upload, nil
}

func readFilePart(part *multipart.Part, upload *MultipartUpload, maxFileSize int64) error {
	filename := filepath.Base(strings.ReplaceAll(part.FileName(), "\\", "/"))
	if filename == "." || filename == "/" || strings.TrimSpace(filename) == "" {
		return errors.New("filename is required")
	}
	file, err := os.CreateTemp("", "webfp-upload-*")
	if err != nil {
		return fmt.Errorf("create upload temp file: %w", err)
	}
	upload.File = file
	upload.Filename = filename
	contentType, _, parseErr := mime.ParseMediaType(part.Header.Get("Content-Type"))
	if parseErr != nil || contentType == "" {
		contentType = "application/octet-stream"
	}
	upload.ContentType = contentType
	size, err := io.Copy(file, io.LimitReader(part, maxFileSize+1))
	if err != nil {
		return fmt.Errorf("write upload temp file: %w", err)
	}
	if size > maxFileSize {
		return ErrFileTooLarge
	}
	upload.Size = size
	return nil
}

func readFolderIDPart(part *multipart.Part, maxFieldSize int64) (string, error) {
	value, err := io.ReadAll(io.LimitReader(part, maxFieldSize+1))
	if err != nil {
		return "", fmt.Errorf("read folder_id field: %w", err)
	}
	if int64(len(value)) > maxFieldSize {
		return "", errors.New("folder_id field is too large")
	}
	return string(value), nil
}

func (upload *MultipartUpload) Cleanup() error {
	if upload.File == nil {
		return nil
	}
	closeErr := upload.File.Close()
	removeErr := os.Remove(upload.File.Name())
	upload.File = nil
	return errors.Join(
		wrapCleanupError("close upload temp file", closeErr),
		wrapCleanupError("remove upload temp file", removeErr),
	)
}

func wrapCleanupError(operation string, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}
