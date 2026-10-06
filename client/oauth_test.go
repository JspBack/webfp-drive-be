//nolint:testpackage // These tests exercise the package-private OAuth URL helpers.
package client

import (
	"testing"

	"github.com/JspBack/webfp-drive/config"
)

func TestOAuthRedirectURL(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{
			name: "local default",
			cfg:  config.Config{Port: 8080},
			want: "http://localhost:8080/api/v1/callback",
		},
		{
			name: "public host",
			cfg:  config.Config{Port: 8080, PublicHost: "drive.example.com:8443"},
			want: "http://drive.example.com:8443/api/v1/callback",
		},
		{
			name: "public scheme https",
			cfg:  config.Config{Port: 8080, PublicHost: "webfp.dpdns.org", PublicScheme: "https"},
			want: "https://webfp.dpdns.org/api/v1/callback",
		},
		{
			name: "public host without port",
			cfg:  config.Config{Port: 8080, PublicHost: "drive.example.com"},
			want: "http://drive.example.com/api/v1/callback",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := oauthRedirectURL(test.cfg); got != test.want {
				t.Errorf("oauthRedirectURL() = %q, want %q", got, test.want)
			}
		})
	}
}
