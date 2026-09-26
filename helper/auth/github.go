package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

const (
	githubUserURL   = "https://api.github.com/user"
	githubEmailsURL = "https://api.github.com/user/emails"

	githubRequestTimeout = 15 * time.Second
)

type GitHubProfile struct {
	Subject         string
	Login           string
	Name            string
	Email           string
	EmailIsVerified bool
}

type GitHubOAuth struct {
	config    *oauth2.Config
	returnURL string
	client    *http.Client
}

func NewGitHubOAuth(clientID, clientSecret, redirectURL, returnURL string) *GitHubOAuth {
	return &GitHubOAuth{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"user:email", "read:user"},
			Endpoint:     github.Endpoint,
		},
		returnURL: strings.TrimRight(returnURL, "#"),
		client:    &http.Client{Timeout: githubRequestTimeout},
	}
}

func (g *GitHubOAuth) Enabled() bool {
	return g != nil && g.config != nil && g.config.ClientID != "" && g.config.ClientSecret != ""
}

func (g *GitHubOAuth) ReturnURL() string {
	if g == nil {
		return ""
	}
	return g.returnURL
}

func (g *GitHubOAuth) AuthorizeURL(state string) string {
	return g.config.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (g *GitHubOAuth) Exchange(ctx context.Context, code string) (string, error) {
	token, err := g.config.Exchange(ctx, code)
	if err != nil {
		return "", fmt.Errorf("exchange GitHub authorization code: %w", err)
	}
	if token.AccessToken == "" {
		return "", errors.New("exchange GitHub authorization code: empty access token")
	}
	return token.AccessToken, nil
}

func (g *GitHubOAuth) Profile(ctx context.Context, accessToken string) (*GitHubProfile, error) {
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := g.getJSON(ctx, accessToken, githubUserURL, &user); err != nil {
		return nil, fmt.Errorf("fetch GitHub profile: %w", err)
	}
	if user.ID == 0 || user.Login == "" {
		return nil, errors.New("fetch GitHub profile: response is missing the account id")
	}

	profile := &GitHubProfile{
		Subject: strconv.FormatInt(user.ID, 10),
		Login:   user.Login,
		Name:    strings.TrimSpace(user.Name),
		Email:   strings.ToLower(strings.TrimSpace(user.Email)),
	}
	if profile.Name == "" {
		profile.Name = user.Login
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := g.getJSON(ctx, accessToken, githubEmailsURL, &emails); err == nil {
		profile.Email, profile.EmailIsVerified = pickVerifiedEmail(user.Email, emails)
	}
	return profile, nil
}

func pickVerifiedEmail(
	publicEmail string,
	emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	},
) (string, bool) {
	var firstVerified string
	for _, entry := range emails {
		address := strings.ToLower(strings.TrimSpace(entry.Email))
		if address == "" || !entry.Verified {
			continue
		}
		if entry.Primary {
			return address, true
		}
		if firstVerified == "" {
			firstVerified = address
		}
	}
	if firstVerified != "" {
		return firstVerified, true
	}
	address := strings.ToLower(strings.TrimSpace(publicEmail))
	return address, false
}

func (g *GitHubOAuth) getJSON(ctx context.Context, accessToken, url string, dst any) error {
	return fetchJSON(ctx, g.client, accessToken, "application/vnd.github+json", "GitHub", url, dst)
}
