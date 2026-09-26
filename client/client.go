package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/httprate"
	"github.com/google/uuid"

	"github.com/JspBack/webfp-drive/config"
	"github.com/JspBack/webfp-drive/store"
)

type Client struct {
	Store               *store.Store
	listenPort          int
	log                 *slog.Logger
	Timeout             time.Duration
	RateLimit           int
	RateWindow          time.Duration
	MaxFileSize         int64
	MaxRequestBodyBytes int64
	ShutdownTimeout     time.Duration

	tlsConfig  *tls.Config
	certFile   string
	keyFileTLS string

	srv *http.Server
}

type Info struct {
	Name       string
	ProfilePic uuid.UUID
}

func loadTLSConfig(cfg config.Config, logger *slog.Logger) *tls.Config {
	if !(cfg.CertFile != "" && cfg.KeyFileTLS != "") {
		return nil
	}
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFileTLS)
	if err != nil {
		logger.Error("load TLS cert", "error", err)
		os.Exit(1)
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}}
}

func New(cfg config.Config, logger *slog.Logger) *Client {
	c := &Client{
		Store:               store.New(cfg),
		listenPort:          cfg.Port,
		log:                 logger,
		Timeout:             cfg.Timeout,
		RateLimit:           cfg.RateLimit,
		RateWindow:          cfg.RateWindow,
		MaxFileSize:         cfg.MaxFileSize,
		MaxRequestBodyBytes: cfg.MaxRequestBodyBytes,
		ShutdownTimeout:     cfg.ShutdownTimeout,
		tlsConfig:           loadTLSConfig(cfg, logger),
		certFile:            cfg.CertFile,
		keyFileTLS:          cfg.KeyFileTLS,
	}

	return c
}

func (c *Client) UseTLS() bool {
	return c.tlsConfig != nil
}

func (c *Client) Listen() {
	addr := fmt.Sprintf(":%d", c.listenPort)
	mux := http.NewServeMux()
	c.registerRoutes(mux)

	c.srv = &http.Server{
		Addr:              addr,
		Handler:           httprate.LimitByIP(c.RateLimit, c.RateWindow)(mux),
		ReadHeaderTimeout: c.Timeout,
		ReadTimeout:       c.Timeout,
		WriteTimeout:      c.Timeout,
		IdleTimeout:       c.Timeout,
	}
	c.log.Info("listening", "addr", addr)
	var serveErr error
	if c.UseTLS() {
		serveErr = c.srv.ListenAndServeTLS(c.certFile, c.keyFileTLS)
	} else {
		serveErr = c.srv.ListenAndServe()
	}
	if serveErr != nil && serveErr != http.ErrServerClosed {
		c.log.Error("listen", "err", serveErr)
	}
}

func (c *Client) Shutdown() {
	if c.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), c.ShutdownTimeout)
		defer cancel()
		_ = c.srv.Shutdown(ctx)

		if c.Store != nil {
			_ = c.Store.Shutdown(ctx)
		}
	}
}
