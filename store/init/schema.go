package initializer

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var requiredTables = []string{"users", "auth_identities", "folders", "files", "fs_lock"}

func initializeSchema(ctx context.Context, db *pgxpool.Pool, schema string) error {
	var schemaExists bool
	if err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_catalog.pg_namespace WHERE nspname = $1
		)
	`, schema).Scan(&schemaExists); err != nil {
		return fmt.Errorf("check application schema: %w", err)
	}
	if !schemaExists {
		return fmt.Errorf(
			"application schema %q does not exist",
			schema,
		)
	}

	var existingTables int
	if err := db.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_catalog.pg_tables
		WHERE schemaname = $1 AND tablename = ANY($2)
	`, schema, requiredTables).Scan(&existingTables); err != nil {
		return fmt.Errorf("check existing tables: %w", err)
	}
	if existingTables == len(requiredTables) {
		return nil
	}
	if existingTables != 0 {
		return fmt.Errorf(
			"database has an incomplete application schema (%d of %d tables exist)",
			existingTables, len(requiredTables),
		)
	}
	return createSchema(ctx, db, schema)
}

func createSchema(ctx context.Context, db *pgxpool.Pool, schema string) error {
	qualifiedSchema := pgx.Identifier{schema}.Sanitize()
	queries := []string{
		`CREATE TABLE webfp.users (
			id UUID PRIMARY KEY,
			email TEXT,
			name TEXT NOT NULL DEFAULT '',
			email_verified BOOLEAN NOT NULL DEFAULT true,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE UNIQUE INDEX users_email_unique ON webfp.users (lower(email)) WHERE email IS NOT NULL`,
		`CREATE TABLE webfp.auth_identities (
			id UUID PRIMARY KEY,
			user_id UUID NOT NULL REFERENCES webfp.users(id) ON DELETE CASCADE,
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
		`CREATE INDEX auth_identities_user_id_idx ON webfp.auth_identities (user_id)`,
		`CREATE TABLE webfp.folders (
			id UUID PRIMARY KEY,
			owner_id UUID NOT NULL REFERENCES webfp.users(id) ON DELETE CASCADE,
			parent_id UUID,
			name TEXT NOT NULL CHECK (name <> ''),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			CONSTRAINT folders_id_owner_unique UNIQUE (id, owner_id),
			CONSTRAINT folders_parent_owner_fk FOREIGN KEY (parent_id, owner_id)
				REFERENCES webfp.folders(id, owner_id) ON DELETE CASCADE,
			CONSTRAINT folders_parent_not_self CHECK (parent_id IS NULL OR parent_id <> id)
		)`,
		`CREATE UNIQUE INDEX folders_root_name_unique
			ON webfp.folders (owner_id, name) WHERE parent_id IS NULL`,
		`CREATE UNIQUE INDEX folders_child_name_unique
			ON webfp.folders (owner_id, parent_id, name) WHERE parent_id IS NOT NULL`,
		`CREATE INDEX folders_parent_listing_idx
			ON webfp.folders (owner_id, parent_id, name)`,
		`CREATE TABLE webfp.files (
			id UUID PRIMARY KEY,
			owner_id UUID NOT NULL REFERENCES webfp.users(id) ON DELETE CASCADE,
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
				REFERENCES webfp.folders(id, owner_id) ON DELETE SET NULL (folder_id)
		)`,
		`CREATE INDEX files_owner_listing_idx
			ON webfp.files (owner_id, folder_id, created_at DESC)`,
		`CREATE TABLE webfp.fs_lock (
			lock_id BOOLEAN PRIMARY KEY DEFAULT true,
			locked BOOLEAN NOT NULL DEFAULT false,
			locked_at TIMESTAMPTZ,
			total_bytes BIGINT NOT NULL DEFAULT 0,
			threshold_hit_percent NUMERIC(5,4) NOT NULL DEFAULT 0,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			CONSTRAINT fs_lock_singleton CHECK (lock_id = true)
		)`,
		`INSERT INTO webfp.fs_lock (lock_id, locked, total_bytes, threshold_hit_percent) VALUES (true, false, 0, 0) ON CONFLICT DO NOTHING`,
	}
	for i := range queries {
		queries[i] = strings.ReplaceAll(queries[i], "webfp.", qualifiedSchema+".")
	}

	return execStatements(ctx, db, queries)
}

func execStatements(ctx context.Context, db *pgxpool.Pool, queries []string) error {
	if err := pgx.BeginFunc(ctx, db, func(tx pgx.Tx) error {
		for i, statement := range queries {
			if _, execErr := tx.Exec(ctx, statement); execErr != nil {
				return fmt.Errorf("initialize schema statement %d: %w", i+1, execErr)
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("create application schema: %w", err)
	}
	return nil
}
