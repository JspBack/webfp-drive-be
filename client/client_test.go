package client_test

import (
	"testing"
	"time"

	"github.com/JspBack/webfp-drive/client"
)

func TestUseTLSAndShutdownWithoutServer(t *testing.T) {
	instance := &client.Client{ShutdownTimeout: time.Second}
	if instance.UseTLS() {
		t.Fatal("UseTLS() = true without TLS configuration")
	}
	instance.Shutdown()
}
