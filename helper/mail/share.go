package mail

import (
	"context"
	"fmt"
	"html"
)

func (c *Client) SendShareReady(ctx context.Context, to, filename, link string) error {
	if !c.Enabled() {
		return ErrNotConfigured
	}
	const subject = "Your WebFP Drive ZIP share is ready"
	text := fmt.Sprintf("Your ZIP share for %s is ready.\n\nDownload link: %s\n", filename, link)
	body := fmt.Sprintf("<p>Your ZIP share for <strong>%s</strong> is ready.</p><p><a href=\"%s\">Open the download link</a></p>",
		html.EscapeString(filename), html.EscapeString(link))
	message, err := buildMessage(c.fromAddr, c.fromName, to, subject, text, body)
	if err != nil {
		return err
	}
	if err = c.send(ctx, to, message); err != nil {
		return fmt.Errorf("send ZIP share email: %w", err)
	}
	return nil
}
