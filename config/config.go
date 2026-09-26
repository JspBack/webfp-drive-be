package config

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

var Version = "placeholder"

type Config struct {
	LogLevel             slog.Level
	Port                 int
	Timeout              time.Duration
	RateLimit            int
	RateWindow           time.Duration
	MaxFileSize          int64
	MaxRequestBodyBytes  int64
	ShutdownTimeout      time.Duration
	SessionLifetime      time.Duration
	UploadURLLifetime    time.Duration
	DownloadURLLifetime  time.Duration
	CertFile             string
	KeyFileTLS           string
	ConnStr              string
	DBMaxConns           int32
	DBMinConns           int32
	DBMaxConnIdleTime    time.Duration
	DBMaxConnLifetime    time.Duration
	RedisURL             string
	RedisPoolSize        int
	RedisMinIdleConns    int
	RedisPoolTimeout     time.Duration
	RedisConnMaxIdleTime time.Duration
	R2AccountID          string
	R2BucketName         string
	R2AccessKeyID        string
	R2SecretAccessKey    string
}

func Parse() (*Config, error) {
	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	verInfo := fs.Bool("v", false, "show version")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return nil, err
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
	var err error
	if c.LogLevel, err = env[slog.Level]("LOG_LEVEL"); err != nil {
		return nil, err
	}
	if c.Port, err = env[int]("PORT"); err != nil {
		return nil, err
	}
	if c.Timeout, err = env[time.Duration]("TIMEOUT"); err != nil {
		return nil, err
	}
	if c.RateLimit, err = env[int]("RATE_LIMIT"); err != nil {
		return nil, err
	}
	if c.RateWindow, err = env[time.Duration]("RATE_WINDOW"); err != nil {
		return nil, err
	}
	if c.MaxFileSize, err = env[int64]("MAX_FILE_SIZE"); err != nil {
		return nil, err
	}
	if c.MaxRequestBodyBytes, err = env[int64]("MAX_REQUEST_BODY_BYTES"); err != nil {
		return nil, err
	}
	if c.ShutdownTimeout, err = env[time.Duration]("SHUTDOWN_TIMEOUT"); err != nil {
		return nil, err
	}
	if c.SessionLifetime, err = env[time.Duration]("SESSION_LIFETIME"); err != nil {
		return nil, err
	}
	if c.UploadURLLifetime, err = env[time.Duration]("UPLOAD_URL_LIFETIME"); err != nil {
		return nil, err
	}
	if c.DownloadURLLifetime, err = env[time.Duration]("DOWNLOAD_URL_LIFETIME"); err != nil {
		return nil, err
	}
	if c.CertFile, err = env[string]("CERT_FILE"); err != nil {
		return nil, err
	}
	if c.KeyFileTLS, err = env[string]("KEY_FILE_TLS"); err != nil {
		return nil, err
	}
	if c.ConnStr, err = env[string]("CONN_STR"); err != nil {
		return nil, err
	}
	if c.DBMaxConns, err = env[int32]("DB_MAX_CONNS"); err != nil {
		return nil, err
	}
	if c.DBMinConns, err = env[int32]("DB_MIN_CONNS"); err != nil {
		return nil, err
	}
	if c.DBMaxConnIdleTime, err = env[time.Duration]("DB_MAX_CONN_IDLE_TIME"); err != nil {
		return nil, err
	}
	if c.DBMaxConnLifetime, err = env[time.Duration]("DB_MAX_CONN_LIFETIME"); err != nil {
		return nil, err
	}
	if c.RedisURL, err = env[string]("REDIS_URL"); err != nil {
		return nil, err
	}
	if c.RedisPoolSize, err = env[int]("REDIS_POOL_SIZE"); err != nil {
		return nil, err
	}
	if c.RedisMinIdleConns, err = env[int]("REDIS_MIN_IDLE_CONNS"); err != nil {
		return nil, err
	}
	if c.RedisPoolTimeout, err = env[time.Duration]("REDIS_POOL_TIMEOUT"); err != nil {
		return nil, err
	}
	if c.RedisConnMaxIdleTime, err = env[time.Duration]("REDIS_CONN_MAX_IDLE_TIME"); err != nil {
		return nil, err
	}
	if c.R2AccountID, err = env[string]("R2_ACCOUNT_ID"); err != nil {
		return nil, err
	}
	if c.R2BucketName, err = env[string]("R2_BUCKET_NAME"); err != nil {
		return nil, err
	}
	if c.R2AccessKeyID, err = env[string]("R2_ACCESS_KEY_ID"); err != nil {
		return nil, err
	}
	if c.R2SecretAccessKey, err = env[string]("R2_SECRET_ACCESS_KEY"); err != nil {
		return nil, err
	}

	if c.MaxRequestBodyBytes <= 0 {
		return nil, fmt.Errorf("MAX_REQUEST_BODY_BYTES must be positive")
	}
	if c.ShutdownTimeout <= 0 || c.SessionLifetime <= 0 ||
		c.UploadURLLifetime <= 0 || c.DownloadURLLifetime <= 0 {
		return nil, fmt.Errorf("SHUTDOWN_TIMEOUT, SESSION_LIFETIME, UPLOAD_URL_LIFETIME, and DOWNLOAD_URL_LIFETIME must be positive")
	}
	const maxPresignedURLLifetime = 7 * 24 * time.Hour
	if c.UploadURLLifetime > maxPresignedURLLifetime || c.DownloadURLLifetime > maxPresignedURLLifetime {
		return nil, fmt.Errorf("UPLOAD_URL_LIFETIME and DOWNLOAD_URL_LIFETIME must not exceed 168h")
	}

	return c, nil
}

func env[T any](key string) (T, error) {
	var zero T
	value, ok := os.LookupEnv(key)
	if !ok {
		return zero, fmt.Errorf("required environment variable %s is not set", key)
	}

	var parsed any
	var err error
	switch any(zero).(type) {
	case string:
		parsed = value
	case int:
		parsed, err = strconv.Atoi(value)
	case int32:
		var n int64
		n, err = strconv.ParseInt(value, 10, 32)
		parsed = int32(n)
	case int64:
		parsed, err = strconv.ParseInt(value, 10, 64)
	case time.Duration:
		parsed, err = time.ParseDuration(value)
	case slog.Level:
		var level slog.Level
		err = level.UnmarshalText([]byte(value))
		parsed = level
	default:
		return zero, fmt.Errorf("unsupported environment value type %T for %s", zero, key)
	}
	if err != nil {
		return zero, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed.(T), nil
}
