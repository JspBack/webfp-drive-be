package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"

	googleRequestTimeout = 15 * time.Second
)

type GoogleProfile struct {
	Subject         string
	Email           string
	Name            string
	EmailIsVerified bool
	Picture         string
}

type GoogleOAuth struct {
	config    *oauth2.Config
	returnURL string
	client    *http.Client
}

func NewGoogleOAuth(clientID, clientSecret, redirectURL, returnURL string) *GoogleOAuth {
	return &GoogleOAuth{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"openid", "email", "profile"},
			Endpoint:     google.Endpoint,
		},
		returnURL: strings.TrimRight(returnURL, "#"),
		client:    &http.Client{Timeout: googleRequestTimeout},
	}
}

func (g *GoogleOAuth) Enabled() bool {
	return g != nil && g.config != nil && g.config.ClientID != "" && g.config.ClientSecret != ""
}

func (g *GoogleOAuth) ReturnURL() string {
	if g == nil {
		return ""
	}
	return g.returnURL
}

func (g *GoogleOAuth) AuthorizeURL(state string) string {
	return g.config.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (g *GoogleOAuth) Exchange(ctx context.Context, code string) (string, error) {
	token, err := g.config.Exchange(ctx, code)
	if err != nil {
		return "", fmt.Errorf("exchange Google authorization code: %w", err)
	}
	if token.AccessToken == "" {
		return "", errors.New("exchange Google authorization code: empty access token")
	}
	return token.AccessToken, nil
}

func (g *GoogleOAuth) Profile(ctx context.Context, accessToken string) (*GoogleProfile, error) {
	var payload struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		VerifiedEmail bool   `json:"verified_email"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := g.getJSON(ctx, accessToken, googleUserInfoURL, &payload); err != nil {
		return nil, fmt.Errorf("fetch Google profile: %w", err)
	}
	if payload.ID == "" {
		return nil, errors.New("fetch Google profile: response is missing the account id")
	}
	profile := &GoogleProfile{
		Subject:         payload.ID,
		Email:           strings.ToLower(strings.TrimSpace(payload.Email)),
		Name:            strings.TrimSpace(payload.Name),
		EmailIsVerified: payload.VerifiedEmail,
		Picture:         payload.Picture,
	}
	if profile.Name == "" {
		profile.Name = strings.TrimSpace(payload.Email)
	}
	return profile, nil
}

func (g *GoogleOAuth) getJSON(ctx context.Context, accessToken, url string, dst any) error {
	return fetchJSON(ctx, g.client, accessToken, "application/json", "Google", url, dst)
}
