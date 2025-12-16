# Development Guide

## Building

```bash
go build -o transmission-exporter ./cmd/transmission-exporter
```

## Testing

### Unit Tests
```bash
go test ./...
```

### Integration Tests
```bash
# With Docker (recommended)
docker-compose -f docker-compose.test.yml up -d
TRANSMISSION_HOST=localhost \
TRANSMISSION_PORT=9091 \
TRANSMISSION_USERNAME=admin \
TRANSMISSION_PASSWORD=password \
go test -tags=integration -v ./internal/rpc
docker-compose -f docker-compose.test.yml down -v

# With local Transmission
transmission-daemon --foreground --no-auth
go test -tags=integration -v ./internal/rpc
```

## Architecture

The exporter uses a polling architecture:
1. Background poller collects metrics from Transmission
2. Metrics are cached in memory
3. HTTP server serves cached metrics to Prometheus
4. Cache continues serving stale data if Transmission is unavailable

## Adding Metrics

1. Define metric in `internal/metrics/metrics.go`
2. Add update method to `MetricUpdater`
3. Call update method from appropriate component
4. Add tests and documentation