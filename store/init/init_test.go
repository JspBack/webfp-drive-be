package initializer_test

import (
	"context"
	"testing"

	initializer "github.com/JspBack/webfp-drive/store/init"
)

func TestClientsWithoutDependencies(t *testing.T) {
	clients := &initializer.Clients{}
	if err := clients.HealthCheck(context.Background()); err != nil {
		t.Fatalf("HealthCheck() on empty clients error = %v", err)
	}
	if err := clients.Close(); err != nil {
		t.Fatalf("Close() on empty clients error = %v", err)
	}
	if err := (*initializer.Clients)(nil).Close(); err != nil {
		t.Fatalf("Close() on nil clients error = %v", err)
	}
}
