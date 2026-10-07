//nolint:testpackage // Tests reach into package-private SMTP rendering helpers.
package mail

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSendVerificationDisabledWithoutCredentials(t *testing.T) {
	client, err := New("", "", "", "", "", "")
	if err != nil {
		t.Fatalf("New() with empty credentials: %v", err)
	}
	if client.Enabled() {
		t.Fatal("client enabled without credentials")
	}
	err = client.SendVerification(context.Background(), VerificationRequest{
		ToEmail: "user@example.com",
		Code:    "123456",
		TTL:     15 * time.Minute,
	})
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("SendVerification() error = %v, want ErrNotConfigured", err)
	}
}

func TestNewDefaultsToGmail(t *testing.T) {
	client, err := New("", "", "user@gmail.com", "app-password", "", "")
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	if !client.Enabled() || client.host != "smtp.gmail.com" || client.port != "587" {
		t.Fatalf("client = %+v, want enabled gmail defaults", client)
	}
	if client.fromAddr != "user@gmail.com" {
		t.Fatalf("fromAddr = %q, want default to user", client.fromAddr)
	}
}

func TestNewRejectsInvalidFromAddress(t *testing.T) {
	if _, err := New("", "", "user@gmail.com", "pw", "not-an-address", ""); err == nil {
		t.Fatal("New() accepted invalid from address")
	}
}

func TestVerificationTemplateContents(t *testing.T) {
	req := VerificationRequest{
		ToEmail:   "user@example.com",
		ToName:    "Ayse",
		Code:      "042517",
		TTL:       15 * time.Minute,
		Retention: 24 * time.Hour,
		SiteURL:   "https://webfp.dpdns.org",
	}
	html, err := renderVerificationHTML(req)
	if err != nil {
		t.Fatalf("renderVerificationHTML(): %v", err)
	}
	for _, want := range []string{"042517", "Ayse", "15 minutes", "24 hours", "permanently deleted", "https://webfp.dpdns.org"} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered template missing %q", want)
		}
	}
	text := verificationText(req)
	if !strings.Contains(text, "042517") {
		t.Error("plain-text fallback missing the code")
	}
	if !strings.Contains(text, "24 hours") {
		t.Error("plain-text fallback missing the retention window")
	}
}

func TestBuildMessageStructure(t *testing.T) {
	msg, err := buildMessage(
		"no-reply@gmail.com", "WebFP Drive", "user@example.com",
		"Verify your email address", "text body", "<p>html body</p>",
	)
	if err != nil {
		t.Fatalf("buildMessage(): %v", err)
	}
	out := string(msg)
	for _, want := range []string{
		`From: "WebFP Drive" <no-reply@gmail.com>`,
		"To: <user@example.com>",
		"Subject: Verify your email address",
		"multipart/alternative",
		`Content-Type: text/plain; charset=UTF-8`,
		`Content-Type: text/html; charset=UTF-8`,
		"text body",
		"<p>html body</p>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("message missing %q", want)
		}
	}
}
