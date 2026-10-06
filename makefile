.PHONY: all build test lint clean docs image deploy pre-all configure-pre-commit

all: clean lint docs test build run

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags="-s -w -X github.com/JspBack/webfp-drive/config.Version=$(VERSION)"
CLUSTER_NAME := k8squest

build:
	mkdir -p bin
	go build $(LDFLAGS) -o bin/be ./cmd/
run:
	./bin/be
lint:
	golangci-lint run --fix
	go vet ./...
test:
	go test ./... -count=1
clean:
	rm -rf bin/
docs:
	swag fmt -g main.go -d cmd,client/handlers,store
	swag init -g main.go -d cmd,client/handlers,store -o docs --parseInternal
image:
	docker build --build-arg VERSION=$(VERSION) -t webfp-be:$(VERSION) .
	docker tag webfp-be:$(VERSION) webfp-be:local
deploy: image
	kind load docker-image webfp-be:$(VERSION) webfp-be:local --name $(CLUSTER_NAME)
	kubectl apply -f deploy/ns.yaml
	kubectl -n webfp create secret generic webfp-be-env --from-file=.env=./.env --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f deploy/
	kubectl -n webfp set image statefulset/webfp-be be=webfp-be:$(VERSION)
	kubectl -n webfp rollout status statefulset/webfp-be --timeout=120s
pre-all:
	pre-commit run --all-files
configure-pre-commit:
	python3 -m venv .venv
	pip install --upgrade pip
	pip install pre-commit
	pre-commit install
	pre-commit autoupdate
