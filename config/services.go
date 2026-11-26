package config

import (
	"errors"
	"fmt"
	"net/mail"
	"os"
	"strings"
)

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

func loadSMTPConfig(c *Config) error {
	c.SMTPHost = strings.TrimSpace(os.Getenv("SMTP_HOST"))
	c.SMTPPort = strings.TrimSpace(os.Getenv("SMTP_PORT"))
	c.SMTPUser = strings.TrimSpace(os.Getenv("SMTP_USER"))
	c.SMTPPassword = strings.TrimSpace(os.Getenv("SMTP_PASSWORD"))
	c.SMTPFromEmail = strings.TrimSpace(os.Getenv("SMTP_FROM_EMAIL"))
	c.SMTPFromName = strings.TrimSpace(os.Getenv("SMTP_FROM_NAME"))
	if c.SMTPUser == "" && c.SMTPPassword == "" {
		return nil
	}
	if c.SMTPUser == "" {
		return errors.New("SMTP_USER is required when SMTP_PASSWORD is set")
	}
	if c.SMTPPassword == "" {
		return errors.New("SMTP_PASSWORD is required when SMTP_USER is set")
	}
	if c.SMTPFromEmail == "" {
		c.SMTPFromEmail = c.SMTPUser
	}
	if _, err := mail.ParseAddress(c.SMTPFromEmail); err != nil {
		return fmt.Errorf("SMTP_FROM_EMAIL: %w", err)
	}
	if c.SMTPFromName == "" {
		c.SMTPFromName = "WebFP Drive"
	}
	return nil
}
