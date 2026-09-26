package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
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
	TransferTimeout     time.Duration
	RateLimit           int
	RateWindow          time.Duration
	MaxFileSize         int64
	MaxRequestBodyBytes int64
	ShutdownTimeout     time.Duration
	publicHost          string

	tlsConfig  *tls.Config
	certFile   string
	keyFileTLS string

	srv *http.Server
}

type Info struct {
	Name       string
	ProfilePic uuid.UUID
}

func loadTLSConfig(cfg config.Config) *tls.Config {
	if cfg.CertFile == "" || cfg.KeyFileTLS == "" {
		if cfg.CertFile != "" || cfg.KeyFileTLS != "" {
			panic("both CERT_FILE and KEY_FILE_TLS must be configured to enable TLS")
		}
		return nil
	}
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFileTLS)
	if err != nil {
		panic(fmt.Errorf("load TLS certificate: %w", err))
	}
	return &tls.Config{Certificates: []tls.Certificate{cert}}
}

func New(cfg config.Config, logger *slog.Logger) *Client {
	tlsConfig := loadTLSConfig(cfg)
	st := store.New(cfg)
	c := &Client{
		Store:               st,
		listenPort:          cfg.Port,
		log:                 logger,
		Timeout:             cfg.Timeout,
		TransferTimeout:     cfg.TransferTimeout,
		RateLimit:           cfg.RateLimit,
		RateWindow:          cfg.RateWindow,
		MaxFileSize:         cfg.MaxFileSize,
		MaxRequestBodyBytes: cfg.MaxRequestBodyBytes,
		ShutdownTimeout:     cfg.ShutdownTimeout,
		publicHost:          cfg.PublicHost,
		tlsConfig:           tlsConfig,
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
	c.RegisterRoutes(mux)

	c.srv = &http.Server{
		Addr:              addr,
		Handler:           httprate.LimitBy(c.RateLimit, c.RateWindow, rateLimitKey)(mux),
		ReadHeaderTimeout: c.Timeout,
		ReadTimeout:       c.TransferTimeout,
		WriteTimeout:      c.TransferTimeout,
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

func rateLimitKey(r *http.Request) (string, error) {
	address, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", fmt.Errorf("parse remote address: %w", err)
	}
	ip, err := netip.ParseAddr(address)
	if err != nil {
		return "", fmt.Errorf("parse remote IP: %w", err)
	}
	return ip.String(), nil
}

func (c *Client) RegisterRoutes(mux *http.ServeMux) {
	c.registerRoutes(mux)
}

func (c *Client) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), c.ShutdownTimeout)
	defer cancel()

	if c.srv != nil {
		if err := c.srv.Shutdown(ctx); err != nil {
			c.log.Error("shut down HTTP server", "error", err)
		}
	}
	if c.Store != nil {
		if err := c.Store.Shutdown(ctx); err != nil {
			c.log.Error("shut down store", "error", err)
		}
	}
}
