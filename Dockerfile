FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /build

COPY go.mod go.sum ./

RUN go mod download && go mod verify

COPY . .

ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=${VERSION} -X main.GitCommit=${GIT_COMMIT} -X main.BuildDate=${BUILD_DATE}" \
    -a -installsuffix cgo \
    -o transmission-exporter \
    ./cmd/transmission-exporter


FROM alpine:3.19

RUN apk add --no-cache ca-certificates curl

COPY --from=builder /build/transmission-exporter /usr/local/bin/transmission-exporter
COPY --from=builder /build/config.example.yaml /etc/transmission-exporter/config.example.yaml
COPY --from=builder /build/config.example.json /etc/transmission-exporter/config.example.json

USER nobody:nobody

EXPOSE 9190

HEALTHCHECK --interval=30s --timeout=10s --start-period=15s --retries=3 \
    CMD curl -f http://localhost:9190/health || exit 1

ENV TRANSMISSION_EXPORTER_LOGGING_LEVEL=info \
    TRANSMISSION_EXPORTER_LOGGING_FORMAT=json \
    TRANSMISSION_EXPORTER_EXPORTER_PORT=9190 \
    TRANSMISSION_EXPORTER_EXPORTER_PATH=/metrics \
    TRANSMISSION_EXPORTER_EXPORTER_POLL_INTERVAL=15s \
    TRANSMISSION_EXPORTER_EXPORTER_MAX_STALE_AGE=5m

ENTRYPOINT ["/usr/local/bin/transmission-exporter"]
CMD []

LABEL org.opencontainers.image.title="Transmission Prometheus Exporter" \
      org.opencontainers.image.description="Prometheus metrics exporter for Transmission BitTorrent client" \
      org.opencontainers.image.vendor="transmission-prometheus-exporter" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.source="https://github.com/varsanyid/transmission-prometheus-exporter" \
      org.opencontainers.image.documentation="https://github.com/varsanyid/transmission-prometheus-exporter/blob/main/README.md"