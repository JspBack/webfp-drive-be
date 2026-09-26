package config

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var Version = "placeholder"

const DefaultUserStorageLimitBytes int64 = 500 * 1024 * 1024

type Config struct {
	LogLevel              slog.Level
	Port                  int
	Timeout               time.Duration
	TransferTimeout       time.Duration
	RateLimit             int
	RateWindow            time.Duration
	MaxFileSize           int64
	MaxRequestBodyBytes   int64
	UserStorageLimitBytes int64
	ShutdownTimeout       time.Duration
	AccessTokenLifetime   time.Duration
	RefreshTokenLifetime  time.Duration
	PublicHost            string
	CertFile              string
	KeyFileTLS            string
	ConnStr               string
	DBReadReplicaConnStr  string
	DBSchema              string
	DBMaxConns            int32
	DBMinConns            int32
	DBMaxConnIdleTime     time.Duration
	DBMaxConnLifetime     time.Duration
	RedisURL              string
	RedisPoolSize         int
	RedisMinIdleConns     int
	RedisPoolTimeout      time.Duration
	RedisConnMaxIdleTime  time.Duration
	R2AccountID           string
	R2BucketName          string
	R2AccessKeyID         string
	R2SecretAccessKey     string
	GitHubClientID        string
	GitHubClientSecret    string
	GoogleClientID        string
	GoogleClientSecret    string
	OAuthReturnURL        string
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
	if c.CertFile, err = envString("CERT_FILE"); err != nil {
		return err
	}
	if c.KeyFileTLS, err = envString("KEY_FILE_TLS"); err != nil {
		return err
	}
	return nil
}

func loadDatabaseConfig(c *Config) error {
	var err error
	if c.ConnStr, err = envString("CONN_STR"); err != nil {
		return err
	}
	c.DBReadReplicaConnStr = strings.TrimSpace(os.Getenv("DB_READ_REPLICA_CONN_STR"))
	if c.DBSchema, err = envString("DB_SCHEMA"); err != nil {
		return err
	}
	c.DBSchema = strings.TrimSpace(c.DBSchema)
	if c.DBSchema == "" {
		return errors.New("DB_SCHEMA must not be empty")
	}
	if c.DBMaxConns, err = envInt32("DB_MAX_CONNS"); err != nil {
		return err
	}
	if c.DBMinConns, err = envInt32("DB_MIN_CONNS"); err != nil {
		return err
	}
	if c.DBMaxConnIdleTime, err = envDuration("DB_MAX_CONN_IDLE_TIME"); err != nil {
		return err
	}
	if c.DBMaxConnLifetime, err = envDuration("DB_MAX_CONN_LIFETIME"); err != nil {
		return err
	}
	return nil
}

func loadRedisConfig(c *Config) error {
	var err error
	if c.RedisURL, err = envString("REDIS_URL"); err != nil {
		return err
	}
	if c.RedisPoolSize, err = envInt("REDIS_POOL_SIZE"); err != nil {
		return err
	}
	if c.RedisMinIdleConns, err = envInt("REDIS_MIN_IDLE_CONNS"); err != nil {
		return err
	}
	if c.RedisPoolTimeout, err = envDuration("REDIS_POOL_TIMEOUT"); err != nil {
		return err
	}
	if c.RedisConnMaxIdleTime, err = envDuration("REDIS_CONN_MAX_IDLE_TIME"); err != nil {
		return err
	}
	return nil
}

func loadR2Config(c *Config) error {
	var err error
	if c.R2AccountID, err = envString("R2_ACCOUNT_ID"); err != nil {
		return err
	}
	if c.R2BucketName, err = envString("R2_BUCKET_NAME"); err != nil {
		return err
	}
	if c.R2AccessKeyID, err = envString("R2_ACCESS_KEY_ID"); err != nil {
		return err
	}
	if c.R2SecretAccessKey, err = envString("R2_SECRET_ACCESS_KEY"); err != nil {
		return err
	}
	return nil
}

func loadAuthConfig(c *Config) error {
	c.GitHubClientID = strings.TrimSpace(os.Getenv("GITHUB_CLIENT_ID"))
	c.GitHubClientSecret = strings.TrimSpace(os.Getenv("GITHUB_CLIENT_SECRET"))
	c.GoogleClientID = strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	c.GoogleClientSecret = strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET"))
	c.OAuthReturnURL = strings.TrimSpace(os.Getenv("OAUTH_RETURN_URL"))
	if c.GitHubClientID != "" && c.GitHubClientSecret == "" {
		return errors.New("GITHUB_CLIENT_SECRET is required when GITHUB_CLIENT_ID is set")
	}
	if c.GoogleClientID != "" && c.GoogleClientSecret == "" {
		return errors.New("GOOGLE_CLIENT_SECRET is required when GOOGLE_CLIENT_ID is set")
	}
	if c.GitHubClientID == "" && c.GoogleClientID == "" {
		return nil
	}
	if err := validateAbsoluteURL(c.OAuthReturnURL); err != nil {
		return fmt.Errorf("OAUTH_RETURN_URL: %w", err)
	}
	return nil
}

func validateAbsoluteURL(value string) error {
	if value == "" {
		return errors.New("must be set")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
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

func envString(key string) (string, error) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", fmt.Errorf("required environment variable %s is not set", key)
	}
	return value, nil
}

func envInt(key string) (int, error) {
	value, err := envString(key)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func envInt32(key string) (int32, error) {
	value, err := envString(key)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return int32(parsed), nil
}

func envInt64(key string) (int64, error) {
	value, err := envString(key)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func envInt64Default(key string, fallback int64) (int64, error) {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return parsed, nil
}

func envDuration(key string) (time.Duration, error) {
	value, err := envString(key)
	if err != nil {
		return 0, err
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func envLogLevel(key string) (slog.Level, error) {
	value, err := envString(key)
	if err != nil {
		return 0, err
	}
	var level slog.Level
	if parseErr := level.UnmarshalText([]byte(value)); parseErr != nil {
		return 0, fmt.Errorf("parse %s: %w", key, parseErr)
	}
	return level, nil
}

func validatePublicHost(host string) error {
	parsed, err := url.Parse("//" + host)
	if err != nil || parsed.Hostname() == "" || parsed.Host != host ||
		parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("PUBLIC_HOST must be a hostname, optionally with a port, without a scheme or path")
	}
	return nil
}
