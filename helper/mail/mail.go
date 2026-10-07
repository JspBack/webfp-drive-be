// Package mail delivers account emails through the Mailtrap API. A client
// built without an API token reports ErrNotConfigured so callers can fall back
// to logging instead of failing registration.
package mail

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"time"

	"github.com/mailtrap/mailtrap-go"
)

//go:embed verify_email.html
var verificationHTML string

// ErrNotConfigured reports that no API token is available for sending.
var ErrNotConfigured = errors.New("mail delivery is not configured")

// VerificationRequest describes one verification-code email.
type VerificationRequest struct {
	ToEmail string
	ToName  string
	Code    string
	TTL     time.Duration
	SiteURL string
}

// Client sends email through Mailtrap; the zero value is disabled.
type Client struct {
	api     *mailtrap.Client
	from    mailtrap.Address
	enabled bool
}

// New builds a Mailtrap client. An empty token yields a disabled client with
// no error so local development can run without credentials.
func New(token, fromEmail, fromName string) (*Client, error) {
	if token == "" {
		return &Client{}, nil
	}
	api, err := mailtrap.NewClient(token)
	if err != nil {
		return nil, fmt.Errorf("create mailtrap client: %w", err)
	}
	return &Client{
		api:     api,
		from:    mailtrap.Address{Email: fromEmail, Name: fromName},
		enabled: true,
	}, nil
}

// Enabled reports whether sending is configured.
func (c *Client) Enabled() bool {
	return c != nil && c.enabled
}

// SendVerification emails the six-digit code using the embedded template.
func (c *Client) SendVerification(ctx context.Context, req VerificationRequest) error {
	if !c.Enabled() {
		return ErrNotConfigured
	}
	body, err := renderVerificationHTML(req)
	if err != nil {
		return err
	}
	_, _, err = c.api.Send(ctx, &mailtrap.SendRequest{
		From:     c.from,
		To:       []mailtrap.Address{{Email: req.ToEmail, Name: req.ToName}},
		Subject:  "Verify your email address",
		Text:     verificationText(req),
		HTML:     body,
		Category: "Email Verification",
	})
	if err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}
	return nil
}

type verificationTemplateData struct {
	Name          string
	Code          string
	ExpiryMinutes int
	SiteURL       string
}

func renderVerificationHTML(req VerificationRequest) (string, error) {
	tmpl, err := template.New("verification").Parse(verificationHTML)
	if err != nil {
		return "", fmt.Errorf("parse verification template: %w", err)
	}
	var buf bytes.Buffer
	if execErr := tmpl.Execute(&buf, verificationTemplateData{
		Name:          req.ToName,
		Code:          req.Code,
		ExpiryMinutes: int(req.TTL.Minutes()),
		SiteURL:       req.SiteURL,
	}); execErr != nil {
		return "", fmt.Errorf("render verification template: %w", execErr)
	}
	return buf.String(), nil
}

func verificationText(req VerificationRequest) string {
	name := req.ToName
	if name == "" {
		name = "there"
	}
	return fmt.Sprintf(
		"WebFP Drive - email verification\n\n"+
			"Hi %s,\n\n"+
			"Use this code to verify your email address:\n\n"+
			"    %s\n\n"+
			"The code expires in %d minutes. If you did not create a WebFP Drive\n"+
			"account, you can safely ignore this email.\n\n%s\n",
		name, req.Code, int(req.TTL.Minutes()), req.SiteURL,
	)
}
