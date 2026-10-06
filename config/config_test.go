//nolint:testpackage // These tests exercise package-private environment parsing helpers.
package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoadApplicationConfig(t *testing.T) {
	setEnv(t, map[string]string{
		"LOG_LEVEL": "debug", "PORT": "8081", "TIMEOUT": "15s",
		"TRANSFER_TIMEOUT": "30m", "RATE_LIMIT": "100", "RATE_WINDOW": "1m",
		"MAX_FILE_SIZE": "52428800", "MAX_REQUEST_BODY_BYTES": "1048576",
		"SHUTDOWN_TIMEOUT": "5s", "ACCESS_TOKEN_LIFETIME": "10m",
		"REFRESH_TOKEN_LIFETIME": "24h", "PUBLIC_HOST": "drive.example.com:8443",
		"PUBLIC_SCHEME":            "https",
		"USER_STORAGE_LIMIT_BYTES": "",
	})

	var got Config
	if err := loadApplicationConfig(&got); err != nil {
		t.Fatalf("loadApplicationConfig() error = %v", err)
	}
	if got.UserStorageLimitBytes != DefaultUserStorageLimitBytes {
		t.Errorf("UserStorageLimitBytes = %d, want default %d", got.UserStorageLimitBytes, DefaultUserStorageLimitBytes)
	}
	if got.LogLevel != slog.LevelDebug || got.Port != 8081 || got.PublicHost != "drive.example.com:8443" {
		t.Errorf("unexpected application settings: %+v", got)
	}
	if got.PublicScheme != "https" {
		t.Errorf("PublicScheme = %q, want https", got.PublicScheme)
	}
	if got.Timeout != 15*time.Second || got.TransferTimeout != 30*time.Minute {
		t.Errorf("unexpected timeouts: %s, %s", got.Timeout, got.TransferTimeout)
	}
}

func TestLoadApplicationConfigRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  string
	}{
		{"log level", "LOG_LEVEL", "verbose"},
		{"port", "PORT", "not-a-number"},
		{"timeout", "TIMEOUT", "soon"},
		{"public host", "PUBLIC_HOST", "https://example.com"},
		{"public scheme", "PUBLIC_SCHEME", "ftp"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setEnv(t, map[string]string{
				"LOG_LEVEL": "info", "PORT": "8080", "TIMEOUT": "15s",
				"TRANSFER_TIMEOUT": "30m", "RATE_LIMIT": "100", "RATE_WINDOW": "1m",
				"MAX_FILE_SIZE": "52428800", "MAX_REQUEST_BODY_BYTES": "1048576",
				"SHUTDOWN_TIMEOUT": "5s", "ACCESS_TOKEN_LIFETIME": "10m",
				"REFRESH_TOKEN_LIFETIME": "24h", "PUBLIC_HOST": "",
				test.key: test.val,
			})
			if err := loadApplicationConfig(&Config{}); err == nil {
				t.Fatal("loadApplicationConfig() succeeded, want error")
			}
		})
	}
}

func TestLoadApplicationConfigStorageLimit(t *testing.T) {
	setEnv(t, map[string]string{
		"LOG_LEVEL": "info", "PORT": "8080", "TIMEOUT": "15s",
		"TRANSFER_TIMEOUT": "30m", "RATE_LIMIT": "100", "RATE_WINDOW": "1m",
		"MAX_FILE_SIZE": "52428800", "MAX_REQUEST_BODY_BYTES": "1048576",
		"SHUTDOWN_TIMEOUT": "5s", "ACCESS_TOKEN_LIFETIME": "10m",
		"REFRESH_TOKEN_LIFETIME": "24h", "PUBLIC_HOST": "",
		"USER_STORAGE_LIMIT_BYTES": " 1048576 ",
	})
	var got Config
	if err := loadApplicationConfig(&got); err != nil {
		t.Fatalf("loadApplicationConfig() error = %v", err)
	}
	if got.UserStorageLimitBytes != 1_048_576 {
		t.Errorf("UserStorageLimitBytes = %d, want 1048576", got.UserStorageLimitBytes)
	}

	for _, invalid := range []string{"0", "-5", "many"} {
		t.Setenv("USER_STORAGE_LIMIT_BYTES", invalid)
		if err := loadApplicationConfig(&Config{}); err == nil {
			t.Errorf("loadApplicationConfig() succeeded for USER_STORAGE_LIMIT_BYTES=%q", invalid)
		}
	}
}

func TestLoadDatabaseConfig(t *testing.T) {
	setEnv(t, map[string]string{
		"CONN_STR": "postgres://primary", "DB_READ_REPLICA_CONN_STR": "  postgres://replica  ",
		"DB_SCHEMA": " webfp ", "DB_MAX_CONNS": "20", "DB_MIN_CONNS": "5",
		"DB_MAX_CONN_IDLE_TIME": "15m", "DB_MAX_CONN_LIFETIME": "1h",
	})

	var got Config
	if err := loadDatabaseConfig(&got); err != nil {
		t.Fatalf("loadDatabaseConfig() error = %v", err)
	}
	if got.ConnStr != "postgres://primary" || got.DBReadReplicaConnStr != "postgres://replica" ||
		got.DBSchema != "webfp" || got.DBMaxConns != 20 || got.DBMinConns != 5 {
		t.Errorf("unexpected database settings: %+v", got)
	}
}

func TestLoadDatabaseConfigRejectsInvalidValues(t *testing.T) {
	setEnv(t, map[string]string{
		"CONN_STR": "postgres://primary", "DB_READ_REPLICA_CONN_STR": "",
		"DB_SCHEMA": " ", "DB_MAX_CONNS": "20", "DB_MIN_CONNS": "5",
		"DB_MAX_CONN_IDLE_TIME": "15m", "DB_MAX_CONN_LIFETIME": "1h",
	})
	if err := loadDatabaseConfig(&Config{}); err == nil || !strings.Contains(err.Error(), "DB_SCHEMA") {
		t.Fatalf("loadDatabaseConfig() error = %v, want DB_SCHEMA error", err)
	}
}

func TestLoadRedisAndR2Config(t *testing.T) {
	setEnv(t, map[string]string{
		"REDIS_URL": "redis://localhost:6379/0", "REDIS_POOL_SIZE": "10",
		"REDIS_MIN_IDLE_CONNS": "3", "REDIS_POOL_TIMEOUT": "4s",
		"REDIS_CONN_MAX_IDLE_TIME": "5m",
		"R2_ACCOUNT_ID":            "account", "R2_BUCKET_NAME": "bucket",
		"R2_ACCESS_KEY_ID": "key", "R2_SECRET_ACCESS_KEY": "secret",
	})
	var got Config
	if err := loadRedisConfig(&got); err != nil {
		t.Fatalf("loadRedisConfig() error = %v", err)
	}
	if got.RedisPoolSize != 10 || got.RedisMinIdleConns != 3 ||
		got.RedisPoolTimeout != 4*time.Second || got.RedisConnMaxIdleTime != 5*time.Minute {
		t.Errorf("unexpected Redis configuration: %+v", got)
	}
	if err := loadR2Config(&got); err != nil {
		t.Fatalf("loadR2Config() error = %v", err)
	}
	if got.R2AccountID != "account" || got.R2BucketName != "bucket" ||
		got.R2AccessKeyID != "key" || got.R2SecretAccessKey != "secret" {
		t.Errorf("unexpected R2 configuration: %+v", got)
	}
}

func TestLoadAuthConfig(t *testing.T) {
	setEnv(t, map[string]string{
		"GITHUB_CLIENT_ID": "client-id", "GITHUB_CLIENT_SECRET": "client-secret",
		"GOOGLE_CLIENT_ID": "", "GOOGLE_CLIENT_SECRET": "",
		"OAUTH_RETURN_URL": "http://localhost:5173/auth/callback",
	})

	var got Config
	if err := loadAuthConfig(&got); err != nil {
		t.Fatalf("loadAuthConfig() error = %v", err)
	}
	if got.GitHubClientID != "client-id" || got.GitHubClientSecret != "client-secret" ||
		got.OAuthReturnURL != "http://localhost:5173/auth/callback" {
		t.Errorf("unexpected auth settings: %+v", got)
	}
}

func TestLoadAuthConfigDisabledWithoutClientID(t *testing.T) {
	setEnv(t, map[string]string{
		"GITHUB_CLIENT_ID": "", "GITHUB_CLIENT_SECRET": "client-secret",
		"GOOGLE_CLIENT_ID": "", "GOOGLE_CLIENT_SECRET": "",
		"OAUTH_RETURN_URL": "",
	})

	var got Config
	if err := loadAuthConfig(&got); err != nil {
		t.Fatalf("loadAuthConfig() error = %v, want GitHub sign-in to stay disabled", err)
	}
	if got.GitHubClientID != "" {
		t.Errorf("GitHubClientID = %q, want empty", got.GitHubClientID)
	}
}

func TestLoadAuthConfigRejectsIncompleteGitHubOAuth(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		secret    string
		returnURL string
	}{
		{"missing secret", "client-id", "", "http://localhost:5173/auth/callback"},
		{"missing return", "client-id", "client-secret", ""},
		{"relative return", "client-id", "client-secret", "/auth/callback"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setEnv(t, map[string]string{
				"GITHUB_CLIENT_ID":     test.id,
				"GITHUB_CLIENT_SECRET": test.secret,
				"GOOGLE_CLIENT_ID":     "", "GOOGLE_CLIENT_SECRET": "",
				"OAUTH_RETURN_URL": test.returnURL,
			})
			if err := loadAuthConfig(&Config{}); err == nil {
				t.Fatal("loadAuthConfig() succeeded, want error")
			}
		})
	}
}

func TestLoadAuthConfigGoogle(t *testing.T) {
	setEnv(t, map[string]string{
		"GITHUB_CLIENT_ID": "", "GITHUB_CLIENT_SECRET": "",
		"GOOGLE_CLIENT_ID": "google-client-id", "GOOGLE_CLIENT_SECRET": "google-client-secret",
		"OAUTH_RETURN_URL": "http://localhost:5173/auth/callback",
	})

	var got Config
	if err := loadAuthConfig(&got); err != nil {
		t.Fatalf("loadAuthConfig() error = %v", err)
	}
	if got.GoogleClientID != "google-client-id" || got.GoogleClientSecret != "google-client-secret" ||
		got.OAuthReturnURL != "http://localhost:5173/auth/callback" {
		t.Errorf("unexpected auth settings: %+v", got)
	}
}

func TestLoadAuthConfigRejectsIncompleteGoogleOAuth(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		secret    string
		returnURL string
	}{
		{"missing secret", "google-client-id", "", "http://localhost:5173/auth/callback"},
		{"missing return", "google-client-id", "google-client-secret", ""},
		{"relative return", "google-client-id", "google-client-secret", "/auth/callback"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setEnv(t, map[string]string{
				"GITHUB_CLIENT_ID": "", "GITHUB_CLIENT_SECRET": "",
				"GOOGLE_CLIENT_ID":     test.id,
				"GOOGLE_CLIENT_SECRET": test.secret,
				"OAUTH_RETURN_URL":     test.returnURL,
			})
			if err := loadAuthConfig(&Config{}); err == nil {
				t.Fatal("loadAuthConfig() succeeded, want error")
			}
		})
	}
}

func TestValidateConfig(t *testing.T) {
	valid := Config{
		Timeout: time.Millisecond, TransferTimeout: time.Millisecond, ShutdownTimeout: time.Millisecond,
		AccessTokenLifetime: time.Millisecond, RefreshTokenLifetime: time.Millisecond,
		MaxFileSize: 10, MaxRequestBodyBytes: 5, UserStorageLimitBytes: DefaultUserStorageLimitBytes,
	}
	if err := validateConfig(&valid); err != nil {
		t.Fatalf("validateConfig(valid) error = %v", err)
	}

	tests := []struct {
		name string
		edit func(*Config)
	}{
		{"zero body limit", func(c *Config) { c.MaxRequestBodyBytes = 0 }},
		{"zero timeout", func(c *Config) { c.Timeout = 0 }},
		{"zero transfer timeout", func(c *Config) { c.TransferTimeout = 0 }},
		{"zero shutdown timeout", func(c *Config) { c.ShutdownTimeout = 0 }},
		{"zero access lifetime", func(c *Config) { c.AccessTokenLifetime = 0 }},
		{"zero refresh lifetime", func(c *Config) { c.RefreshTokenLifetime = 0 }},
		{"zero file limit", func(c *Config) { c.MaxFileSize = 0 }},
		{"zero storage limit", func(c *Config) { c.UserStorageLimitBytes = 0 }},
		{"unsafe combined limit", func(c *Config) {
			c.MaxFileSize = int64(^uint64(0)>>1) - 1
			c.MaxRequestBodyBytes = 2
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := valid
			test.edit(&candidate)
			if err := validateConfig(&candidate); err == nil {
				t.Fatal("validateConfig() succeeded, want error")
			}
		})
	}
}

func TestValidatePublicHost(t *testing.T) {
	for _, host := range []string{"example.com", "example.com:8443", "[::1]:8443"} {
		if err := validatePublicHost(host); err != nil {
			t.Errorf("validatePublicHost(%q) error = %v", host, err)
		}
	}
	for _, host := range []string{"", "https://example.com", "example.com/path", "user@example.com", "example.com?x=1"} {
		if err := validatePublicHost(host); err == nil {
			t.Errorf("validatePublicHost(%q) succeeded, want error", host)
		}
	}
}

func TestEnvironmentParsingErrors(t *testing.T) {
	t.Setenv("TEST_NUMBER", "not-a-number")
	if _, err := envInt("TEST_NUMBER"); err == nil {
		t.Fatal("envInt() succeeded for invalid integer")
	}
	t.Setenv("TEST_DURATION", "soon")
	if _, err := envDuration("TEST_DURATION"); err == nil {
		t.Fatal("envDuration() succeeded for invalid duration")
	}
	t.Setenv("TEST_LEVEL", "verbose")
	if _, err := envLogLevel("TEST_LEVEL"); err == nil {
		t.Fatal("envLogLevel() succeeded for invalid log level")
	}
	if _, err := envString("TEST_MISSING"); err == nil {
		t.Fatal("envString() succeeded for missing variable")
	}
}

func TestEnvInt32AndInt64Overflow(t *testing.T) {
	t.Setenv("TEST_INT32", "2147483648")
	if _, err := envInt32("TEST_INT32"); err == nil {
		t.Fatal("envInt32() accepted an overflowing value")
	}
	t.Setenv("TEST_INT64", "9223372036854775808")
	if _, err := envInt64("TEST_INT64"); err == nil {
		t.Fatal("envInt64() accepted an overflowing value")
	}
}

func setEnv(t *testing.T, values map[string]string) {
	t.Helper()
	for key, value := range values {
		t.Setenv(key, value)
	}
}
