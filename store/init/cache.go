package initializer

import (
	"context"
	"fmt"

	"github.com/JspBack/webfp-drive/config"
	"github.com/redis/go-redis/v9"
)

func newCache(ctx context.Context, cfg config.Config) (*redis.Client, error) {
	options, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}
	options.PoolSize = cfg.RedisPoolSize
	options.MinIdleConns = cfg.RedisMinIdleConns
	options.PoolTimeout = cfg.RedisPoolTimeout
	options.ConnMaxIdleTime = cfg.RedisConnMaxIdleTime

	cache := redis.NewClient(options)
	if err := cache.Ping(ctx).Err(); err != nil {
		_ = cache.Close()
		return nil, fmt.Errorf("healthcheck Redis: %w", err)
	}
	return cache, nil
}
