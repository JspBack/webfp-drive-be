package upload_test

import (
	"errors"
	"mime/multipart"
	"testing"

	uploadhelper "github.com/JspBack/webfp-drive/helper/upload"
)

func TestParseBatchPreservesFileOrder(t *testing.T) {
	request := multipartRequest(t, func(writer *multipart.Writer) {
		writeTestFile(t, writer, "first.txt", "hello")
		writeTestFile(t, writer, "second.txt", "world!")
	})
	files, err := uploadhelper.ParseBatch(request, 100)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cleanupErr := uploadhelper.CleanupBatch(files); cleanupErr != nil {
			t.Error(cleanupErr)
		}
	})
	if len(files) != 2 || files[0].Filename != "first.txt" || files[0].Size != 5 ||
		files[1].Filename != "second.txt" || files[1].Size != 6 {
		t.Errorf("unexpected batch: %+v", files)
	}
}

func TestParseBatchRejectsOversizeBeforeReturningAnyFiles(t *testing.T) {
	request := multipartRequest(t, func(writer *multipart.Writer) {
		writeTestFile(t, writer, "small.txt", "ok")
		writeTestFile(t, writer, "large.txt", "too large")
	})
	files, err := uploadhelper.ParseBatch(request, 4)
	if !errors.Is(err, uploadhelper.ErrFileTooLarge) || len(files) != 0 {
		t.Errorf("ParseBatch() = %v, %v, want no files and ErrFileTooLarge", files, err)
	}
}
