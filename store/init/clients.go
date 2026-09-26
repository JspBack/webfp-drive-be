package initializer

import (
	"context"
	"errors"
	"fmt"

	"github.com/JspBack/webfp-drive/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Clients struct {
	DB      *pgxpool.Pool
	Redis   *redis.Client
	S3      *s3.Client
	Presign *s3.PresignClient

	bucket string
}

func New(ctx context.Context, cfg config.Config) *Clients {
	db, err := newDatabase(ctx, cfg)
	if err != nil {
		panic(err)
	}

	cache, err := newCache(ctx, cfg)
	if err != nil {
		db.Close()
		panic(err)
	}

	objectStore, presign, err := newObjectStore(ctx, cfg)
	if err != nil {
		db.Close()
		_ = cache.Close()
		panic(err)
	}

	return &Clients{
		DB:      db,
		Redis:   cache,
		S3:      objectStore,
		Presign: presign,
		bucket:  cfg.R2BucketName,
	}
}

func (c *Clients) HealthCheck(ctx context.Context) error {
	if c.DB != nil {
		if err := c.DB.Ping(ctx); err != nil {
			return fmt.Errorf("healthcheck PostgreSQL: %w", err)
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
	if c.Redis != nil {
		if err := c.Redis.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close Redis client: %w", err))
		}
	}
	return errors.Join(errs...)
}
