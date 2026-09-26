package main

import (
	"log/slog"
	"os"
	"os/signal"

	"github.com/JspBack/webfp-drive/client"
	"github.com/JspBack/webfp-drive/config"
)

func main() {
	cfg, err := config.Parse()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}
	l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	cl := client.New(*cfg, l)

	l.Debug("client created successfully")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	go func() {
		<-sig
		l.Info("shutting down...")
		cl.Shutdown()
	}()

	cl.Listen()
}
