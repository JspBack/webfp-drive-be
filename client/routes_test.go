package client_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JspBack/webfp-drive/client"
)

func TestSwaggerUI(t *testing.T) {
	mux := swaggerMux()
	for _, path := range []string{"/swagger/index.html", "/swagger/doc.json"} {
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Errorf("GET %s returned status %d", path, recorder.Code)
		}
		if path == "/swagger/index.html" {
			assertBearerPrefix(t, recorder.Body.String())
		}
	}
}

func TestSwaggerEndpointCoverage(t *testing.T) {
	spec := swaggerDocument(t)
	want := map[string][]string{
		"/api/v1/auth/register":            {"post"},
		"/api/v1/auth/login":               {"post"},
		"/api/v1/auth/verify-email":        {"post"},
		"/api/v1/auth/resend-verification": {"post"},
		"/api/v1/auth/refresh":             {"post"},
		"/api/v1/auth/logout":              {"post"},
		"/api/v1/users/me":                 {"get"},
		"/api/v1/users/me/usage":           {"get"},
		"/api/v1/files":                    {"get", "post"},
		"/api/v1/files/{id}":               {"patch", "delete"},
		"/api/v1/files/{id}/download":      {"get"},
		"/api/v1/files/{id}/shares":        {"post"},
		"/api/v1/folders":                  {"get", "post"},
		"/api/v1/folders/{id}":             {"get", "patch", "delete"},
		"/api/v1/shares/{id}":              {"delete"},
	}
	if _, documented := spec.Paths["/api/health"]; documented {
		t.Error("internal health endpoint should not be included in Swagger")
	}
	if _, documented := spec.Paths["/api/v1/users/me"]["patch"]; documented {
		t.Error("removed PATCH /api/v1/users/me is still defined in Swagger")
	}
	if _, documented := spec.Paths["/api/v1/files/{id}"]["get"]; documented {
		t.Error("removed GET /api/v1/files/{id} is still defined in Swagger")
	}
	for path, methods := range want {
		for _, method := range methods {
			if _, ok := spec.Paths[path][method]; !ok {
				t.Errorf("Swagger document does not define %s %s", method, path)
			}
		}
	}
	if _, exists := spec.Paths["/api/v1/files/{id}/complete"]; exists {
		t.Error("Swagger document still defines the removed upload-completion endpoint")
	}
	for _, path := range []string{"/api/v1/shares/{token}", "/api/v1/shares/{token}/download"} {
		if _, exists := spec.Paths[path]; exists {
			t.Errorf("internal share endpoint %s should not be included in Swagger", path)
		}
	}
	consumes := spec.Paths["/api/v1/files"]["post"].Consumes
	if len(consumes) != 1 || consumes[0] != "multipart/form-data" {
		t.Errorf("file upload consumes %v, want multipart/form-data", consumes)
	}
}

func assertBearerPrefix(t *testing.T, body string) {
	t.Helper()
	if !strings.Contains(body, "requestInterceptor") ||
		!strings.Contains(body, `"Bearer " + token`) {
		t.Error("Swagger UI does not automatically prefix bearer tokens")
	}
}

func swaggerMux() *http.ServeMux {
	mux := http.NewServeMux()
	(&client.Client{}).RegisterRoutes(mux)
	return mux
}

func swaggerDocument(t *testing.T) swaggerSpec {
	t.Helper()
	recorder := httptest.NewRecorder()
	swaggerMux().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/swagger/doc.json", nil))
	var spec swaggerSpec
	if err := json.Unmarshal(recorder.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode Swagger document: %v", err)
	}
	return spec
}

type swaggerSpec struct {
	Paths map[string]map[string]struct {
		Consumes []string `json:"consumes"`
	} `json:"paths"`
}
