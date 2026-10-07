package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"time"
)

//go:embed verify_email.html
var verificationHTML string

var ErrNotConfigured = errors.New("mail delivery is not configured")

type VerificationRequest struct {
	ToEmail string
	ToName  string
	Code    string
	TTL     time.Duration
	SiteURL string
}

type Client struct {
	host     string
	port     string
	user     string
	password string
	fromAddr string
	fromName string
	enabled  bool
}

func New(host, port, user, password, fromEmail, fromName string) (*Client, error) {
	if user == "" || password == "" {
		return &Client{}, nil
	}
	if host == "" {
		host = "smtp.gmail.com"
	}
	if port == "" {
		port = "587"
	}
	if fromEmail == "" {
		fromEmail = user
	}
	if _, err := mail.ParseAddress(fromEmail); err != nil {
		return nil, fmt.Errorf("parse from address: %w", err)
	}
	if fromName == "" {
		fromName = "WebFP Drive"
	}
	return &Client{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		fromAddr: fromEmail,
		fromName: fromName,
		enabled:  true,
	}, nil
}

func (c *Client) Enabled() bool {
	return c != nil && c.enabled
}

func (c *Client) SendVerification(ctx context.Context, req VerificationRequest) error {
	if !c.Enabled() {
		return ErrNotConfigured
	}
	htmlBody, err := renderVerificationHTML(req)
	if err != nil {
		return err
	}
	msg, err := buildMessage(c.fromAddr, c.fromName, req.ToEmail,
		"Verify your email address", verificationText(req), htmlBody)
	if err != nil {
		return err
	}
	if err = c.send(ctx, req.ToEmail, msg); err != nil {
		return fmt.Errorf("send verification email: %w", err)
	}
	return nil
}

func (c *Client) send(ctx context.Context, to string, msg []byte) error {
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(c.host, c.port))
	if err != nil {
		return fmt.Errorf("connect to %s:%s: %w", c.host, c.port, err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	client, err := smtp.NewClient(conn, c.host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err = client.StartTLS(&tls.Config{ServerName: c.host}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}
	if err = client.Auth(smtp.PlainAuth("", c.user, c.password, c.host)); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if err = client.Mail(c.fromAddr); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	if err = client.Rcpt(to); err != nil {
		return fmt.Errorf("rcpt to: %w", err)
	}
	writer, dataErr := client.Data()
	if dataErr != nil {
		return fmt.Errorf("data: %w", dataErr)
	}
	if _, err = writer.Write(msg); err != nil {
		return fmt.Errorf("write message: %w", err)
	}
	if err = writer.Close(); err != nil {
		return fmt.Errorf("finish message: %w", err)
	}
	if err = client.Quit(); err != nil {
		return fmt.Errorf("quit: %w", err)
	}
	return nil
}

func buildMessage(fromEmail, fromName, to, subject, textBody, htmlBody string) ([]byte, error) {
	var head bytes.Buffer
	fmt.Fprintf(&head, "From: %s\r\n", (&mail.Address{Name: fromName, Address: fromEmail}).String())
	fmt.Fprintf(&head, "To: %s\r\n", (&mail.Address{Address: to}).String())
	fmt.Fprintf(&head, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&head, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	fmt.Fprintf(&head, "MIME-Version: 1.0\r\n")

	var body bytes.Buffer
	multipartWriter := multipart.NewWriter(&body)
	fmt.Fprintf(&head, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n",
		multipartWriter.Boundary())

	parts := []struct {
		contentType string
		payload     string
	}{
		{"text/plain; charset=UTF-8", textBody},
		{"text/html; charset=UTF-8", htmlBody},
	}
	for _, part := range parts {
		partWriter, err := multipartWriter.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.contentType},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, fmt.Errorf("create multipart part: %w", err)
		}
		quoted := quotedprintable.NewWriter(partWriter)
		if _, err = quoted.Write([]byte(part.payload)); err != nil {
			return nil, fmt.Errorf("encode part: %w", err)
		}
		if err = quoted.Close(); err != nil {
			return nil, fmt.Errorf("finish part: %w", err)
		}
	}
	if err := multipartWriter.Close(); err != nil {
		return nil, fmt.Errorf("close multipart: %w", err)
	}

	var msg bytes.Buffer
	msg.Write(head.Bytes())
	msg.Write(body.Bytes())
	return msg.Bytes(), nil
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
