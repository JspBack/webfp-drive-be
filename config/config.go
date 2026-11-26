package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var Version = "placeholder"

const (
	DefaultUserStorageLimitBytes   int64   = 500 * 1024 * 1024
	DefaultSystemStorageLimitBytes int64   = 10 * 1024 * 1024 * 1024 // 10GB
	DefaultSystemLockThreshold     float64 = 0.80
	SchemeHTTP                     string  = "http"
	SchemeHTTPS                    string  = "https"
)

type Config struct {
	LogLevel                   slog.Level
	Port                       int
	Timeout                    time.Duration
	TransferTimeout            time.Duration
	RateLimit                  int
	RateWindow                 time.Duration
	MaxFileSize                int64
	MaxRequestBodyBytes        int64
	UserStorageLimitBytes      int64
	SystemStorageLimitBytes    int64
	SystemStorageLockThreshold float64
	ShutdownTimeout            time.Duration
	AccessTokenLifetime        time.Duration
	RefreshTokenLifetime       time.Duration
	PublicHost                 string
	PublicScheme               string
	ConnStr                    string
	DBReadReplicaConnStr       string
	DBSchema                   string
	DBMaxConns                 int32
	DBMinConns                 int32
	DBMaxConnIdleTime          time.Duration
	DBMaxConnLifetime          time.Duration
	RedisURL                   string
	RedisPoolSize              int
	RedisMinIdleConns          int
	RedisPoolTimeout           time.Duration
	RedisConnMaxIdleTime       time.Duration
	R2AccountID                string
	R2BucketName               string
	R2AccessKeyID              string
	R2SecretAccessKey          string
	GitHubClientID             string
	GitHubClientSecret         string
	GoogleClientID             string
	GoogleClientSecret         string
	OAuthReturnURL             string
	SMTPHost                   string
	SMTPPort                   string
	SMTPUser                   string
	SMTPPassword               string
	SMTPFromEmail              string
	SMTPFromName               string
}

func Parse() (*Config, error) {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	verInfo := fs.Bool("v", false, "show version")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, fmt.Errorf("parse command-line flags: %w", err)
	}
	if fs.NArg() != 0 {
		return nil, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if *verInfo {
		println(Version)
		os.Exit(0)
	}

	if err := godotenv.Load(); err != nil {
		return nil, fmt.Errorf("load required .env file: %w", err)
	}

	c := &Config{}
	if err := loadApplicationConfig(c); err != nil {
		return nil, err
	}
	if err := loadDatabaseConfig(c); err != nil {
		return nil, err
	}
	if err := loadRedisConfig(c); err != nil {
		return nil, err
	}
	if err := loadR2Config(c); err != nil {
		return nil, err
	}
	if err := loadAuthConfig(c); err != nil {
		return nil, err
	}
	if err := loadSMTPConfig(c); err != nil {
		return nil, err
	}

	if err := validateConfig(c); err != nil {
		return nil, err
	}
	return c, nil
}

func loadApplicationConfig(c *Config) error {
	var err error
	if c.LogLevel, err = envLogLevel("LOG_LEVEL"); err != nil {
		return err
	}
	if c.Port, err = envInt("PORT"); err != nil {
		return err
	}
	if c.Timeout, err = envDuration("TIMEOUT"); err != nil {
		return err
	}
	if c.TransferTimeout, err = envDuration("TRANSFER_TIMEOUT"); err != nil {
		return err
	}
	if c.RateLimit, err = envInt("RATE_LIMIT"); err != nil {
		return err
	}
	if c.RateWindow, err = envDuration("RATE_WINDOW"); err != nil {
		return err
	}
	if c.MaxFileSize, err = envInt64("MAX_FILE_SIZE"); err != nil {
		return err
	}
	if c.MaxRequestBodyBytes, err = envInt64("MAX_REQUEST_BODY_BYTES"); err != nil {
		return err
	}
	if c.UserStorageLimitBytes, err = envInt64Default("USER_STORAGE_LIMIT_BYTES", DefaultUserStorageLimitBytes); err != nil {
		return err
	}
	if c.SystemStorageLimitBytes, err = envInt64Default("SYSTEM_STORAGE_LIMIT_BYTES", DefaultSystemStorageLimitBytes); err != nil {
		return err
	}
	if c.SystemStorageLockThreshold, err = envFloat64Default("SYSTEM_STORAGE_LOCK_THRESHOLD", DefaultSystemLockThreshold); err != nil {
		return err
	}
	if c.ShutdownTimeout, err = envDuration("SHUTDOWN_TIMEOUT"); err != nil {
		return err
	}
	if c.AccessTokenLifetime, err = envDuration("ACCESS_TOKEN_LIFETIME"); err != nil {
		return err
	}
	if c.RefreshTokenLifetime, err = envDuration("REFRESH_TOKEN_LIFETIME"); err != nil {
		return err
	}
	c.PublicHost = strings.TrimSpace(os.Getenv("PUBLIC_HOST"))
	if c.PublicHost != "" {
		if hostErr := validatePublicHost(c.PublicHost); hostErr != nil {
			return hostErr
		}
	}
	c.PublicScheme = strings.ToLower(strings.TrimSpace(os.Getenv("PUBLIC_SCHEME")))
	if c.PublicScheme == "" {
		c.PublicScheme = SchemeHTTP
	}
	if c.PublicScheme != SchemeHTTP && c.PublicScheme != SchemeHTTPS {
		return errors.New("PUBLIC_SCHEME must be http or https")
	}
	return nil
}
func validateAbsoluteURL(value string) error {
	if value == "" {
		return errors.New("must be set")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != SchemeHTTP && parsed.Scheme != SchemeHTTPS) {
		return errors.New("must be an absolute http(s) URL")
	}
	return nil
}

func validateConfig(c *Config) error {
	if c.MaxRequestBodyBytes <= 0 {
		return errors.New("MAX_REQUEST_BODY_BYTES must be positive")
	}
	if c.ShutdownTimeout <= 0 || c.AccessTokenLifetime.Milliseconds() <= 0 ||
		c.RefreshTokenLifetime.Milliseconds() <= 0 ||
		c.Timeout <= 0 || c.TransferTimeout <= 0 {
		return errors.New("TIMEOUT, TRANSFER_TIMEOUT, SHUTDOWN_TIMEOUT, ACCESS_TOKEN_LIFETIME, and REFRESH_TOKEN_LIFETIME must be positive")
	}
	if c.MaxFileSize <= 0 || c.MaxRequestBodyBytes <= 0 ||
		c.MaxFileSize > int64(^uint64(0)>>1)-c.MaxRequestBodyBytes {
		return errors.New("MAX_FILE_SIZE and MAX_REQUEST_BODY_BYTES must be positive and have a safe combined limit")
	}
	if c.UserStorageLimitBytes <= 0 {
		return errors.New("USER_STORAGE_LIMIT_BYTES must be positive")
	}
	return nil
}
func validatePublicHost(host string) error {
	parsed, err := url.Parse("//" + host)
	if err != nil || parsed.Hostname() == "" || parsed.Host != host ||
		parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("PUBLIC_HOST must be a hostname, optionally with a port, without a scheme or path")
	}
	return nil
}
