package initializer

import (
	"context"
	"fmt"

	"github.com/JspBack/webfp-drive/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newDatabase(ctx context.Context, cfg config.Config) (*pgxpool.Pool, error) {
	dbConfig, err := pgxpool.ParseConfig(cfg.ConnStr)
	if err != nil {
		return nil, fmt.Errorf("parse PostgreSQL connection string: %w", err)
	}

	dbConfig.MaxConns = cfg.DBMaxConns
	dbConfig.MinConns = cfg.DBMinConns
	dbConfig.MaxConnIdleTime = cfg.DBMaxConnIdleTime
	dbConfig.MaxConnLifetime = cfg.DBMaxConnLifetime

	db, err := pgxpool.NewWithConfig(ctx, dbConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("healthcheck PostgreSQL: %w", err)
	}
	if err := initializeSchema(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize PostgreSQL schema: %w", err)
	}
	return db, nil
}
