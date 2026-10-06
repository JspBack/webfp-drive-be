package httphelper_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	httphelper "github.com/JspBack/webfp-drive/helper/http"
)

func TestDecodeJSONLimit(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	tests := []struct {
		name string
		body string
		max  int64
		want bool
	}{
		{name: "valid object", body: `{"name":"file"}`, max: 100},
		{name: "unknown field", body: `{"other":"value"}`, max: 100, want: true},
		{name: "second value", body: `{"name":"file"} {}`, max: 100, want: true},
		{name: "oversized", body: `{"name":"file"}`, max: 4, want: true},
		{name: "malformed", body: `{`, max: 100, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var dst payload
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			err := httphelper.DecodeJSONLimit(recorder, request, &dst, test.max)
			if (err != nil) != test.want {
				t.Fatalf("DecodeJSONLimit() error = %v, want error %v", err, test.want)
			}
			if err == nil && dst.Name != "file" {
				t.Errorf("decoded name = %q, want file", dst.Name)
			}
		})
	}
}

func TestJSONAndErrorResponses(t *testing.T) {
	recorder := httptest.NewRecorder()
	httphelper.JSON(recorder, http.StatusCreated, map[string]string{"id": "abc"})
	if recorder.Code != http.StatusCreated || recorder.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected JSON response: status=%d content-type=%q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	var response map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response["id"] != "abc" {
		t.Fatalf("JSON response body = %q, decode error = %v", recorder.Body.String(), err)
	}

	recorder = httptest.NewRecorder()
	httphelper.Error(recorder, http.StatusBadRequest, "invalid")
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"error":"invalid"`) {
		t.Fatalf("unexpected error response: %d %q", recorder.Code, recorder.Body.String())
	}
}

func TestPathUUID(t *testing.T) {
	id := uuid.New()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.SetPathValue("id", id.String())
	if got, ok := httphelper.PathUUID(httptest.NewRecorder(), request, "id", "invalid"); !ok || got != id {
		t.Fatalf("PathUUID() = (%s, %v), want (%s, true)", got, ok, id)
	}

	request.SetPathValue("id", "bad")
	recorder := httptest.NewRecorder()
	if _, ok := httphelper.PathUUID(recorder, request, "id", "invalid"); ok || recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid PathUUID() = ok %v, status %d", ok, recorder.Code)
	}
}

func TestOptionalQueryUUID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	if got, ok := httphelper.OptionalQueryUUID(httptest.NewRecorder(), request, "folder", "invalid"); !ok || got != nil {
		t.Fatalf("empty OptionalQueryUUID() = (%v, %v), want (nil, true)", got, ok)
	}
	id := uuid.New()
	request = httptest.NewRequest(http.MethodGet, "/?folder="+id.String(), nil)
	if got, ok := httphelper.OptionalQueryUUID(httptest.NewRecorder(), request, "folder", "invalid"); !ok || *got != id {
		t.Fatalf("valid OptionalQueryUUID() = (%v, %v), want (%s, true)", got, ok, id)
	}
	request = httptest.NewRequest(http.MethodGet, "/?folder=bad", nil)
	recorder := httptest.NewRecorder()
	if _, ok := httphelper.OptionalQueryUUID(recorder, request, "folder", "invalid"); ok || recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid OptionalQueryUUID() = ok %v, status %d", ok, recorder.Code)
	}
}
