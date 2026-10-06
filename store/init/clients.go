package initializer

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/JspBack/webfp-drive/config"
)

type Clients struct {
	DB     *pgxpool.Pool
	ReadDB *pgxpool.Pool
	Redis  *redis.Client
	S3     *s3.Client

	bucket string
}

func New(ctx context.Context, cfg config.Config) *Clients {
	db, err := newDatabase(ctx, cfg)
	if err != nil {
		panic(fmt.Errorf("initialize PostgreSQL client: %w", err))
	}

	readDB, err := newReadReplica(ctx, cfg, db)
	if err != nil {
		db.Close()
		panic(fmt.Errorf("initialize PostgreSQL read replica: %w", err))
	}

	cache, err := newCache(ctx, cfg)
	if err != nil {
		db.Close()
		if readDB != db {
			readDB.Close()
		}
		panic(fmt.Errorf("initialize Redis client: %w", err))
	}

	objectStore, err := newObjectStore(ctx, cfg)
	if err != nil {
		db.Close()
		if readDB != db {
			readDB.Close()
		}
		if closeErr := cache.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close Redis client after initialization failure: %w", closeErr))
		}
		panic(fmt.Errorf("initialize R2 client: %w", err))
	}

	return &Clients{
		DB:     db,
		ReadDB: readDB,
		Redis:  cache,
		S3:     objectStore,
		bucket: cfg.R2BucketName,
	}
}

func (c *Clients) HealthCheck(ctx context.Context) error {
	if c.DB != nil {
		if err := c.DB.Ping(ctx); err != nil {
			return fmt.Errorf("healthcheck PostgreSQL: %w", err)
		}
	}
	if c.ReadDB != nil && c.ReadDB != c.DB {
		if err := c.ReadDB.Ping(ctx); err != nil {
			return fmt.Errorf("healthcheck PostgreSQL read replica: %w", err)
		}
	}
	if c.Redis != nil {
		if err := c.Redis.Ping(ctx).Err(); err != nil {
			return fmt.Errorf("healthcheck Redis: %w", err)
		}
	}
	if c.S3 != nil {
		if _, err := c.S3.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(c.bucket)}); err != nil {
			return fmt.Errorf("healthcheck R2 bucket %q: %w", c.bucket, err)
		}
	}
	return nil
}

func (c *Clients) Close() error {
	if c == nil {
		return nil
	}
	var errs []error
	if c.DB != nil {
		c.DB.Close()
	}
	if c.ReadDB != nil && c.ReadDB != c.DB {
		c.ReadDB.Close()
	}
	if c.Redis != nil {
		if err := c.Redis.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close Redis client: %w", err))
		}
	}
	return errors.Join(errs...)
}
