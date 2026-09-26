.PHONY: all build test lint clean pre-all configure-pre-commit

all: clean lint test build

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags="-s -w -X github.com/JspBack/webfp-drive/config.Version=$(VERSION)"

build:
	mkdir -p bin
	go build $(LDFLAGS) -o bin/be ./cmd/

lint:
	golangci-lint run
	go vet ./...

test:
	go test ./... -count=1

clean:
	rm -rf bin/

pre-all:
	pre-commit run --all-files

configure-pre-commit:
	python3 -m venv .venv
	pip install --upgrade pip
	pip install pre-commit
	pre-commit install
	pre-commit autoupdate
