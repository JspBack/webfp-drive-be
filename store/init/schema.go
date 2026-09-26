package initializer

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var requiredTables = []string{"users", "auth_identities", "folders", "files"}

func initializeSchema(ctx context.Context, db *pgxpool.Pool) error {
	var existingTables int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_catalog.pg_tables
		WHERE schemaname = current_schema() AND tablename = ANY($1)
	`, requiredTables).Scan(&existingTables); err != nil {
		return fmt.Errorf("check existing tables: %w", err)
	}
	if existingTables == len(requiredTables) {
		return nil
	}
	if existingTables != 0 {
		return fmt.Errorf(
			"database has an incomplete application schema (%d of %d tables exist); refusing to initialize over existing tables",
			existingTables, len(requiredTables),
		)
	}
	if err := createSchema(ctx, db); err != nil {
		return err
	}
	return nil
}

func createSchema(ctx context.Context, db *pgxpool.Pool) error {
	queries := []string{
		`CREATE TABLE users (
			id UUID PRIMARY KEY,
			email TEXT,
			name TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE UNIQUE INDEX users_email_unique ON users (lower(email)) WHERE email IS NOT NULL`,
		`CREATE TABLE auth_identities (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			provider TEXT NOT NULL CHECK (provider IN ('google', 'github', 'password')),
			provider_subject TEXT NOT NULL,
			password_hash TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			CONSTRAINT auth_identities_provider_subject_unique UNIQUE (provider, provider_subject),
			CONSTRAINT auth_identities_password_hash_check CHECK (
				(provider = 'password' AND password_hash IS NOT NULL) OR
				(provider <> 'password' AND password_hash IS NULL)
			)
		)`,
		`CREATE INDEX auth_identities_user_id_idx ON auth_identities (user_id)`,
		`CREATE TABLE folders (
			id UUID PRIMARY KEY,
			owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			parent_id UUID,
			name TEXT NOT NULL CHECK (name <> ''),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			CONSTRAINT folders_id_owner_unique UNIQUE (id, owner_id),
			CONSTRAINT folders_parent_owner_fk FOREIGN KEY (parent_id, owner_id)
				REFERENCES folders(id, owner_id) ON DELETE CASCADE,
			CONSTRAINT folders_parent_not_self CHECK (parent_id IS NULL OR parent_id <> id)
		)`,
		`CREATE UNIQUE INDEX folders_root_name_unique
			ON folders (owner_id, name) WHERE parent_id IS NULL`,
		`CREATE UNIQUE INDEX folders_child_name_unique
			ON folders (owner_id, parent_id, name) WHERE parent_id IS NOT NULL`,
		`CREATE INDEX folders_parent_listing_idx
			ON folders (owner_id, parent_id, name)`,
		`CREATE TABLE files (
			id UUID PRIMARY KEY,
			owner_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			folder_id UUID,
			object_key TEXT NOT NULL UNIQUE CHECK (object_key <> ''),
			filename TEXT NOT NULL CHECK (filename <> ''),
			content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
			size_bytes BIGINT NOT NULL CHECK (size_bytes >= 0),
			checksum_sha256 TEXT,
			status TEXT NOT NULL DEFAULT 'pending'
				CHECK (status IN ('pending', 'ready', 'failed', 'deleting')),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			CONSTRAINT files_folder_owner_fk FOREIGN KEY (folder_id, owner_id)
				REFERENCES folders(id, owner_id) ON DELETE SET NULL (folder_id)
		)`,
		`CREATE INDEX files_owner_listing_idx
			ON files (owner_id, folder_id, created_at DESC)`,
	}

	for i, statement := range queries {
		if _, err := db.Exec(ctx, statement); err != nil {
			return fmt.Errorf("initialize schema statement %d: %w", i+1, err)
		}
	}
	return nil
}
