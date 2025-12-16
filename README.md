# Transmission Prometheus Exporter
[![CI/CD](https://github.com/varsanyid/transmission-prometheus-exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/varsanyid/transmission-prometheus-exporter/actions/workflows/ci.yml)
[![Docker Pulls](https://img.shields.io/docker/pulls/varsanyidaniel/transmission-prometheus-exporter)](https://hub.docker.com/r/transmission-prometheus-exporter)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A Prometheus metrics exporter for the Transmission BitTorrent client. Exposes global statistics, per-torrent metrics, and exporter health information.

## Quick Start

```bash
# Using Docker Compose (includes Transmission, Prometheus, Grafana)
git clone https://github.com/varsanyid/transmission-prometheus-exporter
cd transmission-prometheus-exporter
cp .env.example .env
# Edit .env with your settings
docker-compose up -d
```

Access:
- Metrics: http://localhost:9190/metrics
- Grafana: http://localhost:3000 (admin/admin)

## Standalone Usage

```bash
# Docker
docker run -d \
  -p 9190:9190 \
  -e TRANSMISSION_EXPORTER_TRANSMISSION_HOST=your-host \
  -e TRANSMISSION_EXPORTER_TRANSMISSION_USERNAME=user \
  -e TRANSMISSION_EXPORTER_TRANSMISSION_PASSWORD=pass \
  transmission-prometheus-exporter:latest

# Binary
go install github.com/varsanyid/transmission-prometheus-exporter/cmd/transmission-exporter@latest
transmission-exporter
```

## Key Metrics

- `transmission_download_speed_bytes` - Current download speed
- `transmission_active_torrents` - Number of active torrents
- `transmission_torrent_progress_ratio` - Per-torrent progress
- `transmission_exporter_connection_state` - Exporter health

## Documentation

- [Deployment Guide](docs/deployment.md) - Docker, Kubernetes, configuration
- [Metrics Reference](docs/metrics.md) - Complete metrics list and queries
- [Development Guide](docs/development.md) - Building, testing, architecture

## Features

- **Polling Architecture**: Fast Prometheus scrapes with cached data
- **Comprehensive Metrics**: Global stats, per-torrent details, exporter health
- **Resilient**: Continues serving stale data when Transmission is unavailable
- **Multi-Instance**: Common labels for distinguishing multiple exporters
- **Production Ready**: Structured logging, health checks, Docker support

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.