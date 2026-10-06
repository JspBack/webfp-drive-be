FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates git \
	&& go install github.com/swaggo/swag/cmd/swag@v1.8.1

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=placeholder

RUN swag fmt -g main.go -d cmd,client/handlers,store \
	&& swag init -g main.go -d cmd,client/handlers,store -o docs --parseInternal \
	&& CGO_ENABLED=0 go build -trimpath \
		-ldflags="-s -w -X github.com/JspBack/webfp-drive/config.Version=${VERSION:-$(git describe --tags --always 2>/dev/null || echo dev)}" \
		-o /out/be ./cmd/

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
	&& addgroup -S app && adduser -S -G app -h /app app

WORKDIR /app

COPY --from=builder /out/be /app/be

RUN touch /app/.env && chown -R app:app /app

USER app
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
	CMD wget -q -O /dev/null "http://127.0.0.1:${PORT:-8080}/api/health" || exit 1

ENTRYPOINT ["/app/be"]
