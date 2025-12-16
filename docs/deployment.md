# Deployment Guide

## Standalone Docker

```bash
docker run -d \
  --name transmission-exporter \
  -p 9190:9190 \
  -e TRANSMISSION_EXPORTER_TRANSMISSION_HOST=your-transmission-host \
  -e TRANSMISSION_EXPORTER_TRANSMISSION_USERNAME=your-username \
  -e TRANSMISSION_EXPORTER_TRANSMISSION_PASSWORD=your-password \
  transmission-prometheus-exporter:latest
```

## Configuration

All settings can be configured via environment variables with the `TRANSMISSION_EXPORTER_` prefix:

### Required
- `TRANSMISSION_HOST`: Transmission server hostname
- `TRANSMISSION_PORT`: Transmission RPC port (default: 9091)

### Optional
- `TRANSMISSION_USERNAME`: Basic auth username
- `TRANSMISSION_PASSWORD`: Basic auth password
- `TRANSMISSION_USE_HTTPS`: Use HTTPS (default: false)
- `EXPORTER_PORT`: HTTP server port (default: 9190)
- `EXPORTER_POLL_INTERVAL`: Polling interval (default: 15s)
- `LOGGING_LEVEL`: Log level (default: info)

## Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: transmission-exporter
spec:
  replicas: 1
  selector:
    matchLabels:
      app: transmission-exporter
  template:
    metadata:
      labels:
        app: transmission-exporter
    spec:
      containers:
      - name: transmission-exporter
        image: transmission-prometheus-exporter:latest
        ports:
        - containerPort: 9190
        env:
        - name: TRANSMISSION_EXPORTER_TRANSMISSION_HOST
          value: "transmission-service"
        livenessProbe:
          httpGet:
            path: /health
            port: 9190
          initialDelaySeconds: 30
          periodSeconds: 30
```

## Health Checks

The container includes a built-in health check that tests the `/health` endpoint:

```bash
# Check container health status
docker inspect --format='{{.State.Health.Status}}' transmission-exporter

# Manual health check
curl http://localhost:9190/health
```

## Prometheus Configuration

Add to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'transmission-exporter'
    static_configs:
      - targets: ['transmission-exporter:9190']
    scrape_interval: 30s
```