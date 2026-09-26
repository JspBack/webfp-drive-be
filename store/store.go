package store

import (
	"context"
	"fmt"

	cfg "github.com/JspBack/webfp-drive/config"
	storeinit "github.com/JspBack/webfp-drive/store/init"
)

type Store struct {
	Files   *FileStore
	Folders *FolderStore
	Shares  *ShareStore
	Users   *UserStore
	clients *storeinit.Clients
}

func New(cfg cfg.Config) *Store {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	clients := storeinit.New(ctx, cfg)
	files := &FileStore{
		s3:                  clients.S3,
		presign:             clients.Presign,
		db:                  clients.DB,
		bucket:              cfg.R2BucketName,
		uploadURLLifetime:   cfg.UploadURLLifetime,
		downloadURLLifetime: cfg.DownloadURLLifetime,
	}

	return &Store{
		Files: files,
		Folders: &FolderStore{
			db: clients.DB,
		},
		Users: &UserStore{
			db:              clients.DB,
			cache:           clients.Redis,
			sessionLifetime: cfg.SessionLifetime,
		},
		Shares: &ShareStore{
			cache: clients.Redis,
			files: files,
		},
		clients: clients,
	}
}

func (s *Store) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if err := s.clients.Close(); err != nil {
		return err
	}
	return nil
}

func (s *Store) HealthCheck(ctx context.Context) error {
	if s == nil || s.clients == nil {
		return fmt.Errorf("store clients are not initialized")
	}
	return s.clients.HealthCheck(ctx)
}
