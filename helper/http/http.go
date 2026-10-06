package httphelper

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"

	"github.com/google/uuid"
)

const DefaultMaxBodyBytes = 1 << 20

func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	return DecodeJSONLimit(w, r, dst, DefaultMaxBodyBytes)
}

func DecodeJSONLimit(w http.ResponseWriter, r *http.Request, dst any, maxBodyBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode JSON body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON object")
	}
	return nil
}

func JSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("encode HTTP response", "error", err)
	}
}

func Error(w http.ResponseWriter, status int, message string) {
	JSON(w, status, map[string]string{"error": message})
}

func AbsoluteURL(scheme, host, path string) (string, error) {
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("unsupported URL scheme %q", scheme)
	}
	parsedHost, err := url.Parse("//" + host)
	if err != nil || parsedHost.Hostname() == "" || parsedHost.Host != host ||
		parsedHost.User != nil || parsedHost.Path != "" || parsedHost.RawQuery != "" || parsedHost.Fragment != "" {
		return "", errors.New("invalid URL host")
	}
	if path == "" || path[0] != '/' {
		return "", errors.New("URL path must begin with /")
	}
	return (&url.URL{Scheme: scheme, Host: parsedHost.Host, Path: path}).String(), nil
}

func PathUUID(w http.ResponseWriter, r *http.Request, name, message string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		Error(w, http.StatusBadRequest, message)
		return uuid.Nil, false
	}
	return id, true
}

func OptionalQueryUUID(w http.ResponseWriter, r *http.Request, name, message string) (*uuid.UUID, bool) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return nil, true
	}
	id, err := uuid.Parse(value)
	if err != nil {
		Error(w, http.StatusBadRequest, message)
		return nil, false
	}
	return &id, true
}
