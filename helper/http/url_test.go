package httphelper_test

import (
	"testing"

	httphelper "github.com/JspBack/webfp-drive/helper/http"
)

func TestAbsoluteURL(t *testing.T) {
	tests := []struct {
		name   string
		scheme string
		host   string
		path   string
		want   string
	}{
		{
			name:   "http with host port",
			scheme: "http",
			host:   "drive.example.com:8080",
			path:   "/api/v1/shares/token",
			want:   "http://drive.example.com:8080/api/v1/shares/token",
		},
		{
			name:   "https",
			scheme: "https",
			host:   "drive.example.com",
			path:   "/api/v1/shares/token",
			want:   "https://drive.example.com/api/v1/shares/token",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := httphelper.AbsoluteURL(test.scheme, test.host, test.path)
			if err != nil {
				t.Fatalf("AbsoluteURL() error = %v", err)
			}
			if got != test.want {
				t.Errorf("AbsoluteURL() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAbsoluteURLRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		scheme string
		host   string
		path   string
	}{
		{scheme: "ftp", host: "drive.example.com", path: "/share"},
		{scheme: "https", host: "https://drive.example.com", path: "/share"},
		{scheme: "https", host: "drive.example.com/path", path: "/share"},
		{scheme: "https", host: "drive.example.com", path: "share"},
	}
	for _, test := range tests {
		if _, err := httphelper.AbsoluteURL(test.scheme, test.host, test.path); err == nil {
			t.Errorf("AbsoluteURL(%q, %q, %q) succeeded, want error", test.scheme, test.host, test.path)
		}
	}
}
