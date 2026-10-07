// @title                       WebFP Drive API
// @version                     1.0
// @description                 API for account, file, folder, and share management.
// @BasePath                    /
// @schemes                     http https
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/JspBack/webfp-drive/client"
	"github.com/JspBack/webfp-drive/config"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := config.Parse()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}
	l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	cl := client.New(*cfg, l)

	l.Debug("client created successfully")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go cl.StartUnverifiedUserCleanup(ctx)

	go func() {
		<-ctx.Done()
		l.Info("shutting down...")
		cl.Shutdown()
	}()

	cl.Listen()
}
