//nolint:testpackage // These tests exercise package-private environment parsing helpers.
package config

import (
	"testing"
)

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
