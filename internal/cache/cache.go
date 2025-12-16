package cache

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"time"
	
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/sirupsen/logrus"
	
	"transmission-prometheus-exporter/internal/metrics"
	"transmission-prometheus-exporter/internal/rpc"
)

type PrometheusMetrics struct {
	Content   []byte
	Timestamp time.Time
}

// Cache defines the interface for thread-safe metric storage and retrieval
type Cache interface {
	// UpdateMetrics atomically updates the cache with fresh metric data
	UpdateMetrics(stats *rpc.SessionStats, torrents []*rpc.Torrent, config *rpc.SessionConfig)
	
	// GetMetrics returns the current cached metrics in Prometheus format
	GetMetrics() *PrometheusMetrics
	
	// GetLastUpdateTime returns the timestamp of the last successful cache update
	GetLastUpdateTime() time.Time
	
	// IsStale checks if the cached data is older than the specified maximum age
	IsStale(maxAge time.Duration) bool
}

// MetricCache implements thread-safe caching of Transmission metrics
type MetricCache struct {
	// Mutex for protecting concurrent access to cache data
	mu sync.RWMutex
	
	// Cached metric data
	cachedMetrics *PrometheusMetrics
	
	// Metadata tracking
	lastUpdateTime time.Time
	updateCount    uint64
	
	// Metric updater for consistent label handling
	metricUpdater *metrics.MetricUpdater
	
	// Configuration
	transmissionHost string
	exporterInstance string
	transmissionPort int
	
	// Logger for structured logging
	logger *logrus.Logger
}

// NewMetricCache creates a new thread-safe metric cache
func NewMetricCache(transmissionHost string, exporterInstance string, transmissionPort int, logger *logrus.Logger) *MetricCache {
	cache := &MetricCache{
		metricUpdater:    metrics.NewMetricUpdater(transmissionHost, exporterInstance, transmissionPort),
		transmissionHost: transmissionHost,
		exporterInstance: exporterInstance,
		transmissionPort: transmissionPort,
		logger:           logger,
	}
	
	cache.logger.WithFields(logrus.Fields{
		"transmission_host": transmissionHost,
		"exporter_instance": exporterInstance,
		"transmission_port": transmissionPort,
	}).Info("Metric cache initialized")
	
	return cache
}

// UpdateMetrics atomically updates the cache with fresh metric data
func (c *MetricCache) UpdateMetrics(stats *rpc.SessionStats, torrents []*rpc.Torrent, config *rpc.SessionConfig) {
	startTime := time.Now()
	
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.logger.WithFields(logrus.Fields{
		"torrent_count":    len(torrents),
		"has_stats":        stats != nil,
		"has_config":       config != nil,
	}).Debug("Starting cache update with new metrics")
	
	if stats != nil {
		downloadSpeed, uploadSpeed, activeTorrents, pausedTorrents, totalTorrents, cumulativeDownloaded, cumulativeUploaded := stats.ToPrometheusFloat64()
		c.metricUpdater.UpdateGlobalMetrics(downloadSpeed, uploadSpeed, activeTorrents, pausedTorrents, totalTorrents)
		c.metricUpdater.UpdateCumulativeMetrics(cumulativeDownloaded, cumulativeUploaded)
	}
	
	if config != nil {
		speedLimitDown, speedLimitUp, freeSpace := config.ToPrometheusFloat64()
		c.metricUpdater.UpdateSessionConfig(speedLimitDown, speedLimitUp, freeSpace)
	}
	
	// Update per-torrent metrics and track active torrent IDs for cleanup
	var activeTorrentIDs []string
	if torrents != nil {
		for _, torrent := range torrents {
			if torrent != nil {
				torrentID := torrent.GetTorrentIDString()
				activeTorrentIDs = append(activeTorrentIDs, torrentID)
				
				downloadRate, uploadRate, progress, peersConnected, peersSending, peersGetting, status := torrent.ToPrometheusFloat64()
				c.metricUpdater.UpdateTorrentMetrics(
					torrentID,
					downloadRate,
					uploadRate,
					progress,
					peersConnected,
					peersSending,
					peersGetting,
					status,
					torrent.HasError(),
					torrent.GetErrorString(),
				)
			}
		}
		
		// Clean up metrics for torrents that no longer exist
		c.metricUpdater.CleanupTorrentMetrics(activeTorrentIDs)
	}
	
	// Update exporter self-monitoring metrics
	now := time.Now()
	c.metricUpdater.UpdateLastSuccessfulScrape(float64(now.Unix()))
	c.metricUpdater.UpdateConnectionState(true) // Connected if we got data
	
	// Generate fresh Prometheus metrics content
	content, err := c.generatePrometheusContent()
	if err != nil {
		// Log error but don't fail the update - keep serving stale data
		c.logger.WithError(err).Error("Failed to generate Prometheus content, keeping stale data")
		return
	}
	
	// Atomically update cache metadata and content
	c.cachedMetrics = &PrometheusMetrics{
		Content:   content,
		Timestamp: now,
	}
	c.lastUpdateTime = now
	c.updateCount++
	
	// Update operational metrics
	c.updateOperationalMetrics(content)
	
	duration := time.Since(startTime)
	c.logger.WithFields(logrus.Fields{
		"update_count":     c.updateCount,
		"update_duration":  duration,
		"content_size":     len(content),
		"active_torrents":  len(activeTorrentIDs),
	}).Info("Cache updated successfully with fresh metrics")
}

// GetMetrics returns the current cached metrics in Prometheus format
func (c *MetricCache) GetMetrics() *PrometheusMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if c.cachedMetrics == nil {
		c.logger.Warn("No cached metrics available, returning empty metrics")
		// No cached data available - return empty metrics
		return &PrometheusMetrics{
			Content:   []byte{},
			Timestamp: time.Time{},
		}
	}
	
	// Update cache age metric
	age := time.Since(c.cachedMetrics.Timestamp).Seconds()
	c.metricUpdater.UpdateCacheMetrics(1, 0, age) // Cache hit
	
	c.logger.WithFields(logrus.Fields{
		"cache_age_seconds": age,
		"content_size":      len(c.cachedMetrics.Content),
		"last_update":       c.cachedMetrics.Timestamp,
	}).Debug("Serving cached metrics")
	
	// Return a copy to prevent external modification
	return &PrometheusMetrics{
		Content:   append([]byte(nil), c.cachedMetrics.Content...),
		Timestamp: c.cachedMetrics.Timestamp,
	}
}

// GetLastUpdateTime returns the timestamp of the last successful cache update
func (c *MetricCache) GetLastUpdateTime() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return c.lastUpdateTime
}

// IsStale checks if the cached data is older than the specified maximum age
func (c *MetricCache) IsStale(maxAge time.Duration) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if c.cachedMetrics == nil {
		c.logger.Debug("Cache is stale: no cached data available")
		return true // No data is considered stale
	}
	
	age := time.Since(c.cachedMetrics.Timestamp)
	isStale := age > maxAge
	
	if isStale {
		c.logger.WithFields(logrus.Fields{
			"cache_age":     age,
			"max_age":       maxAge,
			"last_update":   c.cachedMetrics.Timestamp,
		}).Warn("Cache is stale")
	}
	
	return isStale
}

// GetUpdateCount returns the number of successful cache updates (for monitoring)
func (c *MetricCache) GetUpdateCount() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return c.updateCount
}

// generatePrometheusContent generates the Prometheus text format content from current metrics
func (c *MetricCache) generatePrometheusContent() ([]byte, error) {
	// Gather all metrics from the default registry
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return nil, fmt.Errorf("failed to gather metrics: %w", err)
	}
	
	// Encode metrics in Prometheus text format
	var buf bytes.Buffer
	encoder := expfmt.NewEncoder(&buf, expfmt.FmtText)
	
	for _, mf := range metricFamilies {
		if err := encoder.Encode(mf); err != nil {
			return nil, fmt.Errorf("failed to encode metric family %s: %w", mf.GetName(), err)
		}
	}
	
	return buf.Bytes(), nil
}

// RecordCacheMiss records a cache miss for monitoring purposes
func (c *MetricCache) RecordCacheMiss() {
	c.metricUpdater.UpdateCacheMetrics(0, 1, 0) // Cache miss
}

// RecordScrapeError records a scrape error for monitoring purposes
func (c *MetricCache) RecordScrapeError(errorType string) {
	c.metricUpdater.RecordScrapeError(errorType)
	c.metricUpdater.UpdateConnectionState(false) // Disconnected on error
}

// updateOperationalMetrics updates operational metrics like memory usage and cache size
func (c *MetricCache) updateOperationalMetrics(content []byte) {
	c.metricUpdater.UpdateCacheSize(float64(len(content)))
	
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	c.metricUpdater.UpdateMemoryUsage(
		float64(memStats.Alloc),     // Currently allocated bytes
		float64(memStats.Sys),       // Total bytes obtained from system
		float64(memStats.HeapInuse), // Bytes in in-use heap spans
	)
}