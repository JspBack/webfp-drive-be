package store

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type archiveTestClient struct{}

func (archiveTestClient) Do(_ *http.Request) (*http.Response, error) {
	const contents = "report contents"
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{},
		Body:          io.NopCloser(strings.NewReader(contents)),
		ContentLength: int64(len(contents)),
	}, nil
}

func TestWriteArchivePreservesDirectoriesAndSafeUniquePaths(t *testing.T) {
	client := s3.New(s3.Options{
		Region: "auto", Credentials: aws.AnonymousCredentials{}, HTTPClient: archiveTestClient{},
		BaseEndpoint: aws.String("https://objects.example.test"), UsePathStyle: true,
	})
	shares := &ShareStore{files: &FileStore{s3: client, bucket: "files"}}
	const contents = "report contents"
	entries := []archiveEntry{
		{Path: []string{"Reports"}},
		{Path: []string{"Reports", "Empty"}},
		{Path: []string{"Reports", ".. ", "Q1\\report.txt"}, Key: "a", Size: int64(len(contents))},
		{Path: []string{"Reports", ".. ", "Q1\\report.txt"}, Key: "b", Size: int64(len(contents))},
	}
	var output bytes.Buffer
	if err := shares.writeArchive(t.Context(), &output, entries); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{
		"Reports/",
		"Reports/Empty/",
		"Reports/%2E%2E%20/Q1%5Creport.txt",
		"Reports/%2E%2E%20/Q1%5Creport.txt (2)",
	}
	if len(archive.File) != len(names) {
		t.Fatalf("expected %d entries, got %d", len(names), len(archive.File))
	}
	for i, name := range names {
		if archive.File[i].Name != name {
			t.Errorf("entry %d = %q, want %q", i, archive.File[i].Name, name)
		}
	}
	got, err := archive.Open(names[2])
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	data, err := io.ReadAll(got)
	if err != nil || string(data) != contents {
		t.Fatalf("archive contents = %q, err = %v", data, err)
	}
}
