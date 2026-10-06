package client_test

import (
	"testing"
	"time"

	"github.com/JspBack/webfp-drive/client"
)

func TestShutdownWithoutServer(_ *testing.T) {
	instance := &client.Client{ShutdownTimeout: time.Second}
	instance.Shutdown()
}
