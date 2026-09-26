package upload_test

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	uploadhelper "github.com/JspBack/webfp-drive/helper/upload"
)

func TestParseMultipart(t *testing.T) {
	request := multipartRequest(t, func(writer *multipart.Writer) {
		if fieldErr := writer.WriteField("folder_id", "folder-123"); fieldErr != nil {
			t.Fatal(fieldErr)
		}
		part, createErr := writer.CreateFormFile("file", `C:\upload\sample.txt`)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := io.WriteString(part, "file contents"); writeErr != nil {
			t.Fatal(writeErr)
		}
	})

	upload, parseErr := uploadhelper.ParseMultipart(request, 100, 100)
	if parseErr != nil {
		t.Fatalf("ParseMultipart() error = %v", parseErr)
	}
	filename := upload.File.Name()
	defer func() {
		if cleanupErr := upload.Cleanup(); cleanupErr != nil {
			t.Errorf("Cleanup() error = %v", cleanupErr)
		}
	}()

	if upload.Filename != "sample.txt" {
		t.Errorf("Filename = %q, want sample.txt", upload.Filename)
	}
	if upload.ContentType != "application/octet-stream" {
		t.Errorf("ContentType = %q, want application/octet-stream", upload.ContentType)
	}
	if upload.Size != int64(len("file contents")) {
		t.Errorf("Size = %d, want %d", upload.Size, len("file contents"))
	}
	if upload.FolderID != "folder-123" {
		t.Errorf("FolderID = %q, want folder-123", upload.FolderID)
	}
	if _, seekErr := upload.File.Seek(0, io.SeekStart); seekErr != nil {
		t.Fatal(seekErr)
	}
	content, readErr := io.ReadAll(upload.File)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "file contents" {
		t.Errorf("file content = %q, want %q", content, "file contents")
	}
	if cleanupErr := upload.Cleanup(); cleanupErr != nil {
		t.Fatalf("Cleanup() error = %v", cleanupErr)
	}
	if _, statErr := os.Stat(filename); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("temporary upload file still exists, stat error = %v", statErr)
	}
}

func TestParseMultipartRejectsOversizedAndDuplicateFields(t *testing.T) {
	tests := []struct {
		name string
		body func(*multipart.Writer)
		want error
	}{
		{
			name: "oversized file",
			body: func(writer *multipart.Writer) {
				writeTestFile(t, writer, "file", "sample.txt", "too large")
			},
			want: uploadhelper.ErrFileTooLarge,
		},
		{
			name: "duplicate empty folder fields",
			body: func(writer *multipart.Writer) {
				if err := writer.WriteField("folder_id", ""); err != nil {
					t.Fatal(err)
				}
				if err := writer.WriteField("folder_id", ""); err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, writer, "file", "sample.txt", "content")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := multipartRequest(t, test.body)
			_, err := uploadhelper.ParseMultipart(request, 5, 100)
			if err == nil {
				t.Fatal("ParseMultipart() succeeded, want an error")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Errorf("ParseMultipart() error = %v, want %v", err, test.want)
			}
		})
	}
}

func multipartRequest(t *testing.T, write func(*multipart.Writer)) *http.Request {
	t.Helper()
	var body strings.Builder
	writer := multipart.NewWriter(&body)
	write(writer)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body.String()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func writeTestFile(t *testing.T, writer *multipart.Writer, field, filename, content string) {
	t.Helper()
	part, createErr := writer.CreateFormFile(field, filename)
	if createErr != nil {
		t.Fatal(createErr)
	}
	if _, writeErr := io.WriteString(part, content); writeErr != nil {
		t.Fatal(writeErr)
	}
}
