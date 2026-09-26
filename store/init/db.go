package initializer

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/JspBack/webfp-drive/config"
)

func newDatabase(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	db, err := newPool(ctx, cfg, cfg.ConnStr, false)
	if err != nil {
		return nil, fmt.Errorf("initialize primary PostgreSQL pool: %w", err)
	}
	if schemaErr := initializeSchema(ctx, db, cfg.DBSchema); schemaErr != nil {
		db.Close()
		return nil, fmt.Errorf("initialize PostgreSQL schema: %w", schemaErr)
	}
	return db, nil
}

func newReadReplica(ctx context.Context, cfg config.Config, primary *pgxpool.Pool) (*pgxpool.Pool, error) {
	if cfg.DBReadReplicaConnStr == "" {
		return primary, nil
	}
	db, err := newPool(ctx, cfg, cfg.DBReadReplicaConnStr, true)
	if err != nil {
		return nil, fmt.Errorf("initialize read-replica PostgreSQL pool: %w", err)
	}
	return db, nil
}

func newPool(ctx context.Context, cfg config.Config, connString string, readOnly bool) (*pgxpool.Pool, error) {
	dbConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL connection string: %w", err)
	}
	dbConfig.MaxConns = cfg.DBMaxConns
	dbConfig.MinConns = cfg.DBMinConns
	dbConfig.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	dbConfig.MaxConnLifetime = cfg.DBMaxConnLifetime
	dbConfig.ConnConfig.RuntimeParams["search_path"] = pgx.Identifier{cfg.DBSchema}.Sanitize()
	if readOnly {
		dbConfig.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	}

	db, err := pgxpool.NewWithConfig(ctx, dbConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	if pingErr := db.Ping(ctx); pingErr != nil {
		db.Close()
		return nil, fmt.Errorf("healthcheck PostgreSQL: %w", pingErr)
	}
	return db, nil
}
