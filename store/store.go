package store

import (
	"context"
	"errors"
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
		s3:     clients.S3,
		db:     clients.DB,
		bucket: cfg.R2BucketName,
	}

	return &Store{
		Files: files,
		Folders: &FolderStore{
			db: clients.DB,
		},
		Users: &UserStore{
			db:              clients.DB,
			cache:           clients.Redis,
			accessLifetime:  cfg.AccessTokenLifetime,
			refreshLifetime: cfg.RefreshTokenLifetime,
		},
		Shares: &ShareStore{
			cache: clients.Redis,
			files: files,
		},
		clients: clients,
	}
}

func (s *Store) Shutdown(_ context.Context) error {
	if s == nil {
		return nil
	}
	if err := s.clients.Close(); err != nil {
		return fmt.Errorf("close store clients: %w", err)
	}
	return nil
}

func (s *Store) HealthCheck(ctx context.Context) error {
	if s == nil || s.clients == nil {
		return errors.New("store clients are not initialized")
	}
	if err := s.clients.HealthCheck(ctx); err != nil {
		return fmt.Errorf("check store health: %w", err)
	}
	return nil
}
