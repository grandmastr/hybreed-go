# syntax=docker/dockerfile:1

# ── build ──────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache module downloads.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
        -ldflags "-s -w -X main.version=${VERSION}" \
        -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/seed ./cmd/seed

# ── runtime ────────────────────────────────────────────────────────────────
FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata wget && \
    adduser -D -u 10001 app
COPY --from=build /out/api /usr/local/bin/api
COPY --from=build /out/seed /usr/local/bin/seed

USER app
EXPOSE 8080
ENV HTTP_ADDR=:8080

HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=5 \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["api"]
