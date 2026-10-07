package client

import (
	"context"
	"time"
)

const (
	unverifiedRetention  = 24 * time.Hour
	unverifiedSweepEvery = 24 * time.Hour
)

func (c *Client) StartUnverifiedUserCleanup(ctx context.Context) {
	ticker := time.NewTicker(unverifiedSweepEvery)
	defer ticker.Stop()
	for {
		removed, err := c.Store.Users.DeleteStaleUnverifiedUsers(ctx, unverifiedRetention)
		switch {
		case err != nil && ctx.Err() == nil:
			c.log.ErrorContext(ctx, "remove unverified users", "error", err)
		case err == nil && removed > 0:
			c.log.InfoContext(ctx, "removed unverified users", "count", removed)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
