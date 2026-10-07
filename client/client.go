package client

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/go-chi/httprate"
	"github.com/google/uuid"

	"github.com/JspBack/webfp-drive/config"
	authhelper "github.com/JspBack/webfp-drive/helper/auth"
	"github.com/JspBack/webfp-drive/helper/mail"
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
	publicScheme        string
	githubOAuth         *authhelper.GitHubOAuth
	googleOAuth         *authhelper.GoogleOAuth
	mailer              *mail.Client

	srv *http.Server
}

type Info struct {
	Name       string
	ProfilePic uuid.UUID
}

func New(cfg config.Config, logger *slog.Logger) *Client {
	st := store.New(cfg)
	mailer, err := mail.New(cfg.MailtrapAPIToken, cfg.MailtrapFromEmail, cfg.MailtrapFromName)
	if err != nil {
		logger.Warn("email delivery disabled", "error", err)
		mailer = nil
	}
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
		publicScheme:        cfg.PublicScheme,
		githubOAuth:         newGitHubOAuth(cfg),
		googleOAuth:         newGoogleOAuth(cfg),
		mailer:              mailer,
	}

	return c
}

func newGitHubOAuth(cfg config.Config) *authhelper.GitHubOAuth {
	if cfg.GitHubClientID == "" {
		return nil
	}
	return authhelper.NewGitHubOAuth(
		cfg.GitHubClientID, cfg.GitHubClientSecret, oauthRedirectURL(cfg), cfg.OAuthReturnURL,
	)
}

func newGoogleOAuth(cfg config.Config) *authhelper.GoogleOAuth {
	if cfg.GoogleClientID == "" {
		return nil
	}
	return authhelper.NewGoogleOAuth(
		cfg.GoogleClientID, cfg.GoogleClientSecret, oauthRedirectURL(cfg), cfg.OAuthReturnURL,
	)
}

func oauthRedirectURL(cfg config.Config) string {
	scheme := cfg.PublicScheme
	if scheme != config.SchemeHTTP && scheme != config.SchemeHTTPS {
		scheme = config.SchemeHTTP
	}
	host := cfg.PublicHost
	if host == "" {
		host = fmt.Sprintf("localhost:%d", cfg.Port)
	}
	return scheme + "://" + host + "/api/v1/callback"
}

func (c *Client) publicHTTPS() bool {
	return c.publicScheme == config.SchemeHTTPS
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
	if serveErr := c.srv.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
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
