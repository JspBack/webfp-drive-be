package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/JspBack/webfp-drive/store"
)

func TestStoreInputValidation(t *testing.T) {
	ctx := context.Background()
	if _, err := (&store.FileStore{}).Upload(ctx, uuid.New(), nil, " ", "", 1, nil); err == nil {
		t.Fatal("Upload() accepted empty filename")
	}
	if _, err := (&store.FileStore{}).Upload(ctx, uuid.New(), nil, "file", "", 0, nil); err == nil {
		t.Fatal("Upload() accepted zero-sized file")
	}
	if _, err := (&store.FolderStore{}).Create(ctx, uuid.New(), nil, " "); err == nil {
		t.Fatal("Create() accepted empty folder name")
	}
	if _, err := (&store.UserStore{}).Add(ctx, " ", "name", "password"); err == nil {
		t.Fatal("Add() accepted empty email")
	}
	if _, err := (&store.UserStore{}).Add(ctx, "email", "name", ""); err == nil {
		t.Fatal("Add() accepted empty password")
	}
	if _, err := (&store.UserStore{}).Update(ctx, uuid.New(), " ", "name"); err == nil {
		t.Fatal("Update() accepted empty email")
	}
}

func TestShareOptionValidation(t *testing.T) {
	ctx := context.Background()
	shares := &store.ShareStore{}
	if _, err := shares.CreateShare(ctx, uuid.New(), uuid.New(), time.Now(), nil, ""); !errors.Is(err, store.ErrInvalidShareOptions) {
		t.Fatalf("CreateShare(expired) error = %v, want ErrInvalidShareOptions", err)
	}

	maxDownloads := int64(0)
	_, err := shares.CreateShare(ctx, uuid.New(), uuid.New(), time.Now().Add(time.Hour), &maxDownloads, "")
	if !errors.Is(err, store.ErrInvalidShareOptions) {
		t.Fatalf("CreateShare(nonpositive limit) error = %v, want ErrInvalidShareOptions", err)
	}
}
