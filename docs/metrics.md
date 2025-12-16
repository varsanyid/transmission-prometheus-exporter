# Metrics Reference

## Common Labels

All metrics include these labels:
- `transmission_host`: Transmission server hostname
- `exporter_instance`: Unique exporter identifier
- `transmission_port`: Transmission RPC port

## Global Metrics

### Speed and Counts
- `transmission_download_speed_bytes`: Current download speed
- `transmission_upload_speed_bytes`: Current upload speed
- `transmission_active_torrents`: Number of active torrents
- `transmission_total_torrents`: Total number of torrents

### Cumulative Stats
- `transmission_cumulative_downloaded_bytes`: Total downloaded
- `transmission_cumulative_uploaded_bytes`: Total uploaded

### Configuration
- `transmission_speed_limit_down_bytes`: Download speed limit
- `transmission_speed_limit_up_bytes`: Upload speed limit
- `transmission_free_space_bytes`: Free disk space

## Per-Torrent Metrics

All include `torrent_id` label:

- `transmission_torrent_download_rate_bytes`: Download rate
- `transmission_torrent_upload_rate_bytes`: Upload rate
- `transmission_torrent_progress_ratio`: Progress (0.0-1.0)
- `transmission_torrent_peers_connected`: Connected peers
- `transmission_torrent_status`: Status code (0-6)
- `transmission_torrent_error`: Error state (1 if error)

## Exporter Metrics

### Health
- `transmission_exporter_connection_state`: Connection status (1=connected)
- `transmission_exporter_last_successful_scrape_timestamp`: Last successful scrape

### Performance
- `transmission_exporter_rpc_latency_seconds`: RPC call latency
- `transmission_exporter_scrape_errors_total`: Error count by type
- `transmission_exporter_cache_age_seconds`: Cache age

### System
- `transmission_exporter_memory_usage_bytes`: Memory usage by type
- `transmission_exporter_uptime_seconds`: Process uptime
- `transmission_exporter_build_info`: Build information

## Useful Queries

```promql
# Current download speed
sum(transmission_download_speed_bytes)

# Top torrents by download rate
topk(5, transmission_torrent_download_rate_bytes)

# Torrents with errors
transmission_torrent_error == 1

# Exporter health
transmission_exporter_connection_state == 0
```

## Alerting Examples

```yaml
- alert: TransmissionExporterDown
  expr: up{job="transmission-exporter"} == 0
  for: 1m
  labels:
    severity: critical

- alert: TransmissionConnectionLost
  expr: transmission_exporter_connection_state == 0
  for: 2m
  labels:
    severity: warning
```