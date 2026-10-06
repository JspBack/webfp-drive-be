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
	schema := schemaTables(cfg.DBSchema)
	files := &FileStore{
		schemaTables:               schema,
		s3:                         clients.S3,
		db:                         clients.DB,
		readDB:                     clients.ReadDB,
		bucket:                     cfg.R2BucketName,
		storageLimit:               cfg.UserStorageLimitBytes,
		systemStorageLimit:         cfg.SystemStorageLimitBytes,
		systemStorageLockThreshold: cfg.SystemStorageLockThreshold,
	}

	return &Store{
		Files: files,
		Folders: &FolderStore{
			schemaTables: schema,
			db:           clients.DB,
			readDB:       clients.ReadDB,
		},
		Users: &UserStore{
			schemaTables:    schema,
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
