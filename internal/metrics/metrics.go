package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var commonLabelNames = []string{"transmission_host", "exporter_instance", "transmission_port"}

var (
	TransmissionDownloadSpeedBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_download_speed_bytes",
		Help: "Current download speed in bytes per second",
	}, commonLabelNames)

	TransmissionUploadSpeedBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_upload_speed_bytes",
		Help: "Current upload speed in bytes per second",
	}, commonLabelNames)

	TransmissionActiveTorrents = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_active_torrents",
		Help: "Number of active torrents",
	}, commonLabelNames)

	TransmissionTotalTorrents = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_total_torrents",
		Help: "Total number of torrents",
	}, commonLabelNames)

	TransmissionPausedTorrents = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_paused_torrents",
		Help: "Number of paused torrents",
	}, commonLabelNames)

	TransmissionCumulativeDownloadedBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_cumulative_downloaded_bytes",
		Help: "Total bytes downloaded since Transmission started",
	}, commonLabelNames)

	TransmissionCumulativeUploadedBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_cumulative_uploaded_bytes",
		Help: "Total bytes uploaded since Transmission started",
	}, commonLabelNames)

	TransmissionSpeedLimitDownBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_speed_limit_down_bytes",
		Help: "Download speed limit in bytes per second (0 if unlimited)",
	}, commonLabelNames)

	TransmissionSpeedLimitUpBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_speed_limit_up_bytes",
		Help: "Upload speed limit in bytes per second (0 if unlimited)",
	}, commonLabelNames)

	TransmissionFreeSpaceBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_free_space_bytes",
		Help: "Free space available in download directory in bytes",
	}, commonLabelNames)
)

var (
	torrentLabelNames = append([]string{"torrent_id"}, commonLabelNames...)

	TransmissionTorrentDownloadRateBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_torrent_download_rate_bytes",
		Help: "Download rate for individual torrent in bytes per second",
	}, torrentLabelNames)

	TransmissionTorrentUploadRateBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_torrent_upload_rate_bytes",
		Help: "Upload rate for individual torrent in bytes per second",
	}, torrentLabelNames)

	TransmissionTorrentProgressRatio = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_torrent_progress_ratio",
		Help: "Download progress as a ratio from 0.0 to 1.0",
	}, torrentLabelNames)

	TransmissionTorrentPeersConnected = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_torrent_peers_connected",
		Help: "Number of peers connected to this torrent",
	}, torrentLabelNames)

	TransmissionTorrentPeersSendingToUs = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_torrent_peers_sending_to_us",
		Help: "Number of peers sending data to us for this torrent",
	}, torrentLabelNames)

	TransmissionTorrentPeersGettingFromUs = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_torrent_peers_getting_from_us",
		Help: "Number of peers getting data from us for this torrent",
	}, torrentLabelNames)

	// Error metric includes error_string label for debugging
	TransmissionTorrentError = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_torrent_error",
		Help: "Error state for torrent (1 if error, 0 if no error)",
	}, append([]string{"torrent_id", "error_string"}, commonLabelNames...))

	TransmissionTorrentStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_torrent_status",
		Help: "Status of torrent (0=stopped, 1=check_wait, 2=check, 3=download_wait, 4=download, 5=seed_wait, 6=seed)",
	}, torrentLabelNames)
)

var (
	TransmissionExporterScrapeErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "transmission_exporter_scrape_errors_total",
		Help: "Total number of scrape errors by error type",
	}, append([]string{"error_type"}, commonLabelNames...))

	TransmissionExporterRPCLatencySeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "transmission_exporter_rpc_latency_seconds",
		Help:    "Latency of RPC calls to Transmission in seconds",
		Buckets: prometheus.DefBuckets,
	}, append([]string{"method"}, commonLabelNames...))

	TransmissionExporterLastSuccessfulScrape = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_exporter_last_successful_scrape_timestamp",
		Help: "Unix timestamp of the last successful scrape",
	}, commonLabelNames)

	TransmissionExporterConnectionState = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_exporter_connection_state",
		Help: "Connection state to Transmission (1=connected, 0=disconnected)",
	}, commonLabelNames)

	TransmissionExporterRetryAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "transmission_exporter_retry_attempts_total",
		Help: "Total number of retry attempts by operation type",
	}, append([]string{"operation"}, commonLabelNames...))

	TransmissionExporterCacheHits = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "transmission_exporter_cache_hits_total",
		Help: "Total number of cache hits when serving metrics",
	}, commonLabelNames)

	TransmissionExporterCacheMisses = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "transmission_exporter_cache_misses_total",
		Help: "Total number of cache misses when serving metrics",
	}, commonLabelNames)

	TransmissionExporterCacheAge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_exporter_cache_age_seconds",
		Help: "Age of cached data in seconds",
	}, commonLabelNames)

	TransmissionExporterBuildInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_exporter_build_info",
		Help: "Build information for the exporter",
	}, append([]string{"version", "go_version", "git_commit", "build_date"}, commonLabelNames...))

	TransmissionExporterConfigInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_exporter_config_info",
		Help: "Configuration information for the exporter",
	}, append([]string{"polling_interval", "max_stale_age", "transmission_timeout"}, commonLabelNames...))

	TransmissionExporterBackoffDelaySeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "transmission_exporter_backoff_delay_seconds",
		Help:    "Histogram of exponential backoff delays in seconds",
		Buckets: []float64{0.1, 0.5, 1.0, 2.0, 5.0, 10.0, 30.0, 60.0},
	}, append([]string{"operation"}, commonLabelNames...))

	TransmissionExporterMemoryUsageBytes = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_exporter_memory_usage_bytes",
		Help: "Current memory usage of the exporter process in bytes",
	}, append([]string{"type"}, commonLabelNames...))

	TransmissionExporterCacheSize = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_exporter_cache_size_bytes",
		Help: "Size of cached metrics data in bytes",
	}, commonLabelNames)

	TransmissionExporterUptime = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_exporter_uptime_seconds",
		Help: "Uptime of the exporter process in seconds",
	}, commonLabelNames)

	TransmissionExporterStartTime = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "transmission_exporter_start_time_timestamp",
		Help: "Unix timestamp when the exporter started",
	}, commonLabelNames)
)

// MetricUpdater provides methods to update all metrics with consistent common labels
type MetricUpdater struct {
	previousTorrentIDs map[string]bool
	commonLabelValues  []string
}

// NewMetricUpdater creates a new MetricUpdater instance with common label values
func NewMetricUpdater(transmissionHost string, exporterInstance string, transmissionPort int) *MetricUpdater {
	return &MetricUpdater{
		previousTorrentIDs: make(map[string]bool),
		commonLabelValues:  []string{transmissionHost, exporterInstance, strconv.Itoa(transmissionPort)},
	}
}

// UpdateGlobalMetrics updates global Transmission metrics with common labels
func (m *MetricUpdater) UpdateGlobalMetrics(downloadSpeed, uploadSpeed float64, activeTorrents, pausedTorrents, totalTorrents float64) {
	TransmissionDownloadSpeedBytes.WithLabelValues(m.commonLabelValues...).Set(downloadSpeed)
	TransmissionUploadSpeedBytes.WithLabelValues(m.commonLabelValues...).Set(uploadSpeed)
	TransmissionActiveTorrents.WithLabelValues(m.commonLabelValues...).Set(activeTorrents)
	TransmissionPausedTorrents.WithLabelValues(m.commonLabelValues...).Set(pausedTorrents)
	TransmissionTotalTorrents.WithLabelValues(m.commonLabelValues...).Set(totalTorrents)
}

// UpdateCumulativeMetrics updates cumulative statistics with common labels
func (m *MetricUpdater) UpdateCumulativeMetrics(downloadedBytes, uploadedBytes float64) {
	TransmissionCumulativeDownloadedBytes.WithLabelValues(m.commonLabelValues...).Set(downloadedBytes)
	TransmissionCumulativeUploadedBytes.WithLabelValues(m.commonLabelValues...).Set(uploadedBytes)
}

// UpdateSessionConfig updates session configuration metrics with common labels
func (m *MetricUpdater) UpdateSessionConfig(speedLimitDown, speedLimitUp, freeSpace float64) {
	TransmissionSpeedLimitDownBytes.WithLabelValues(m.commonLabelValues...).Set(speedLimitDown)
	TransmissionSpeedLimitUpBytes.WithLabelValues(m.commonLabelValues...).Set(speedLimitUp)
	TransmissionFreeSpaceBytes.WithLabelValues(m.commonLabelValues...).Set(freeSpace)
}

// UpdateTorrentMetrics updates per-torrent metrics with common labels
func (m *MetricUpdater) UpdateTorrentMetrics(torrentID string, downloadRate, uploadRate, progress float64, 
	peersConnected, peersSending, peersGetting, status float64, hasError bool, errorString string) {
	
	// Combine torrent ID with common label values
	torrentLabels := append([]string{torrentID}, m.commonLabelValues...)
	
	TransmissionTorrentDownloadRateBytes.WithLabelValues(torrentLabels...).Set(downloadRate)
	TransmissionTorrentUploadRateBytes.WithLabelValues(torrentLabels...).Set(uploadRate)
	TransmissionTorrentProgressRatio.WithLabelValues(torrentLabels...).Set(progress)
	TransmissionTorrentPeersConnected.WithLabelValues(torrentLabels...).Set(peersConnected)
	TransmissionTorrentPeersSendingToUs.WithLabelValues(torrentLabels...).Set(peersSending)
	TransmissionTorrentPeersGettingFromUs.WithLabelValues(torrentLabels...).Set(peersGetting)
	TransmissionTorrentStatus.WithLabelValues(torrentLabels...).Set(status)
	
	// Handle error state - only set if there's an error to maintain finite cardinality
	if hasError {
		errorLabels := append([]string{torrentID, errorString}, m.commonLabelValues...)
		TransmissionTorrentError.WithLabelValues(errorLabels...).Set(1)
	} else {
		// Reset any existing error metrics for this torrent
		partialLabels := prometheus.Labels{"torrent_id": torrentID}
		// Add common labels to partial match
		partialLabels["transmission_host"] = m.commonLabelValues[0]
		partialLabels["exporter_instance"] = m.commonLabelValues[1]
		partialLabels["transmission_port"] = m.commonLabelValues[2]
		TransmissionTorrentError.DeletePartialMatch(partialLabels)
	}
}

// RecordScrapeError records a scrape error with common labels
func (m *MetricUpdater) RecordScrapeError(errorType string) {
	labels := append([]string{errorType}, m.commonLabelValues...)
	TransmissionExporterScrapeErrorsTotal.WithLabelValues(labels...).Inc()
}

// RecordRPCLatency records RPC call latency with common labels
func (m *MetricUpdater) RecordRPCLatency(method string, duration float64) {
	labels := append([]string{method}, m.commonLabelValues...)
	TransmissionExporterRPCLatencySeconds.WithLabelValues(labels...).Observe(duration)
}

// UpdateLastSuccessfulScrape updates the timestamp of last successful scrape with common labels
func (m *MetricUpdater) UpdateLastSuccessfulScrape(timestamp float64) {
	TransmissionExporterLastSuccessfulScrape.WithLabelValues(m.commonLabelValues...).Set(timestamp)
}

// UpdateConnectionState updates the connection state to Transmission with common labels
func (m *MetricUpdater) UpdateConnectionState(connected bool) {
	if connected {
		TransmissionExporterConnectionState.WithLabelValues(m.commonLabelValues...).Set(1)
	} else {
		TransmissionExporterConnectionState.WithLabelValues(m.commonLabelValues...).Set(0)
	}
}

// RecordRetryAttempt records a retry attempt with common labels
func (m *MetricUpdater) RecordRetryAttempt(operation string) {
	labels := append([]string{operation}, m.commonLabelValues...)
	TransmissionExporterRetryAttempts.WithLabelValues(labels...).Inc()
}

// UpdateCacheMetrics updates cache-related metrics with common labels
func (m *MetricUpdater) UpdateCacheMetrics(hits, misses, ageSeconds float64) {
	TransmissionExporterCacheHits.WithLabelValues(m.commonLabelValues...).Add(hits)
	TransmissionExporterCacheMisses.WithLabelValues(m.commonLabelValues...).Add(misses)
	TransmissionExporterCacheAge.WithLabelValues(m.commonLabelValues...).Set(ageSeconds)
}

// SetBuildInfo sets build information metrics with common labels
func (m *MetricUpdater) SetBuildInfo(version, goVersion, gitCommit, buildDate string) {
	labels := append([]string{version, goVersion, gitCommit, buildDate}, m.commonLabelValues...)
	TransmissionExporterBuildInfo.WithLabelValues(labels...).Set(1)
}

// SetConfigInfo sets configuration information metrics with common labels
func (m *MetricUpdater) SetConfigInfo(pollingInterval, maxStaleAge, transmissionTimeout string) {
	labels := append([]string{pollingInterval, maxStaleAge, transmissionTimeout}, m.commonLabelValues...)
	TransmissionExporterConfigInfo.WithLabelValues(labels...).Set(1)
}

// RecordBackoffDelay records exponential backoff delay timing
func (m *MetricUpdater) RecordBackoffDelay(operation string, delaySeconds float64) {
	labels := append([]string{operation}, m.commonLabelValues...)
	TransmissionExporterBackoffDelaySeconds.WithLabelValues(labels...).Observe(delaySeconds)
}

// UpdateMemoryUsage updates memory usage metrics
func (m *MetricUpdater) UpdateMemoryUsage(allocBytes, sysBytes, heapBytes float64) {
	allocLabels := append([]string{"alloc"}, m.commonLabelValues...)
	sysLabels := append([]string{"sys"}, m.commonLabelValues...)
	heapLabels := append([]string{"heap_inuse"}, m.commonLabelValues...)
	
	TransmissionExporterMemoryUsageBytes.WithLabelValues(allocLabels...).Set(allocBytes)
	TransmissionExporterMemoryUsageBytes.WithLabelValues(sysLabels...).Set(sysBytes)
	TransmissionExporterMemoryUsageBytes.WithLabelValues(heapLabels...).Set(heapBytes)
}

// UpdateCacheSize updates the cache size metric
func (m *MetricUpdater) UpdateCacheSize(sizeBytes float64) {
	TransmissionExporterCacheSize.WithLabelValues(m.commonLabelValues...).Set(sizeBytes)
}

// UpdateUptime updates the uptime metric
func (m *MetricUpdater) UpdateUptime(uptimeSeconds float64) {
	TransmissionExporterUptime.WithLabelValues(m.commonLabelValues...).Set(uptimeSeconds)
}

// SetStartTime sets the start time metric (should be called once at startup)
func (m *MetricUpdater) SetStartTime(startTimeUnix float64) {
	TransmissionExporterStartTime.WithLabelValues(m.commonLabelValues...).Set(startTimeUnix)
}

// CleanupTorrentMetrics removes metrics for torrents that no longer exist
// Uses the "last seen set" pattern for efficient cleanup
func (m *MetricUpdater) CleanupTorrentMetrics(activeTorrentIDs []string) {
	// Create a map for quick lookup of current active torrent IDs
	currentIDs := make(map[string]bool)
	for _, id := range activeTorrentIDs {
		currentIDs[id] = true
	}
	
	// Find torrent IDs that were previously seen but are no longer active
	for previousID := range m.previousTorrentIDs {
		if !currentIDs[previousID] {
			// This torrent is no longer active, remove its metrics
			// Combine torrent ID with common label values for deletion
			torrentLabels := append([]string{previousID}, m.commonLabelValues...)
			
			TransmissionTorrentDownloadRateBytes.DeleteLabelValues(torrentLabels...)
			TransmissionTorrentUploadRateBytes.DeleteLabelValues(torrentLabels...)
			TransmissionTorrentProgressRatio.DeleteLabelValues(torrentLabels...)
			TransmissionTorrentPeersConnected.DeleteLabelValues(torrentLabels...)
			TransmissionTorrentPeersSendingToUs.DeleteLabelValues(torrentLabels...)
			TransmissionTorrentPeersGettingFromUs.DeleteLabelValues(torrentLabels...)
			TransmissionTorrentStatus.DeleteLabelValues(torrentLabels...)
			
			// Clean up error metrics for this torrent (all error strings)
			partialLabels := prometheus.Labels{
				"torrent_id":         previousID,
				"transmission_host":  m.commonLabelValues[0],
				"exporter_instance":  m.commonLabelValues[1],
				"transmission_port":  m.commonLabelValues[2],
			}
			TransmissionTorrentError.DeletePartialMatch(partialLabels)
		}
	}
	
	// Update the previous torrent IDs set for next cleanup
	m.previousTorrentIDs = currentIDs
}