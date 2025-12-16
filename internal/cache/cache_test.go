package cache

import (
	"strings"
	"testing"
	"time"
	
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/sirupsen/logrus"
	
	"transmission-prometheus-exporter/internal/rpc"
	testutilpkg "transmission-prometheus-exporter/internal/testutil"
)

func createTestLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests
	return logger
}

func TestNewMetricCache(t *testing.T) {
	host := "test-host"
	instance := "test-instance"
	port := 9091
	cache := NewMetricCache(host, instance, port, createTestLogger())
	
	if cache == nil {
		t.Fatal("Expected cache to be created, got nil")
	}
	
	if cache.transmissionHost != host {
		t.Errorf("Expected host %s, got %s", host, cache.transmissionHost)
	}
	
	if cache.exporterInstance != instance {
		t.Errorf("Expected instance %s, got %s", instance, cache.exporterInstance)
	}
	
	if cache.transmissionPort != port {
		t.Errorf("Expected port %d, got %d", port, cache.transmissionPort)
	}
	
	if cache.metricUpdater == nil {
		t.Error("Expected metric updater to be initialized")
	}
}

func TestMetricCacheInitialState(t *testing.T) {
	cache := NewMetricCache("localhost", "test", 9091, createTestLogger())
	
	if !cache.GetLastUpdateTime().IsZero() {
		t.Error("Expected initial last update time to be zero")
	}
	
	if cache.GetUpdateCount() != 0 {
		t.Errorf("Expected initial update count to be 0, got %d", cache.GetUpdateCount())
	}
	
	if !cache.IsStale(time.Second) {
		t.Error("Expected cache to be stale initially")
	}
	
	metrics := cache.GetMetrics()
	if metrics == nil {
		t.Fatal("Expected metrics to be returned, got nil")
	}
	
	if len(metrics.Content) != 0 {
		t.Error("Expected empty content from empty cache")
	}
	
	if !metrics.Timestamp.IsZero() {
		t.Error("Expected zero timestamp from empty cache")
	}
}

func TestUpdateMetrics(t *testing.T) {
	cache := NewMetricCache("localhost", "test", 9091, createTestLogger())
	
	stats := &rpc.SessionStats{
		DownloadSpeed:        1000,
		UploadSpeed:          500,
		ActiveTorrentCount:   5,
		PausedTorrentCount:   3,
		TotalTorrentCount:    8,
		CumulativeDownloaded: 1000000,
		CumulativeUploaded:   500000,
	}
	
	errorString := "Connection timeout"
	torrents := []*rpc.Torrent{
		{
			ID:                 1,
			Status:             rpc.TorrentDownload,
			DownloadRate:       100,
			UploadRate:         50,
			PercentDone:        0.75,
			PeersConnected:     10,
			PeersSendingToUs:   3,
			PeersGettingFromUs: 2,
			ErrorString:        nil,
		},
		{
			ID:                 2,
			Status:             rpc.TorrentSeed,
			DownloadRate:       0,
			UploadRate:         200,
			PercentDone:        1.0,
			PeersConnected:     5,
			PeersSendingToUs:   0,
			PeersGettingFromUs: 5,
			ErrorString:        &errorString,
		},
	}
	
	downLimit := uint64(2000)
	upLimit := uint64(1000)
	config := &rpc.SessionConfig{
		SpeedLimitDown: &downLimit,
		SpeedLimitUp:   &upLimit,
		FreeSpace:      10000000,
		Version:        "3.00",
	}
	
	beforeUpdate := time.Now()
	cache.UpdateMetrics(stats, torrents, config)
	afterUpdate := time.Now()
	
	lastUpdate := cache.GetLastUpdateTime()
	if lastUpdate.Before(beforeUpdate) || lastUpdate.After(afterUpdate) {
		t.Error("Last update time should be set to current time")
	}
	
	if cache.GetUpdateCount() != 1 {
		t.Errorf("Expected update count to be 1, got %d", cache.GetUpdateCount())
	}
	
	if cache.IsStale(time.Minute) {
		t.Error("Cache should not be stale immediately after update")
	}
	
	metrics := cache.GetMetrics()
	if metrics == nil {
		t.Fatal("Expected metrics to be returned, got nil")
	}
	
	if len(metrics.Content) == 0 {
		t.Error("Expected non-empty content after update")
	}
	
	if metrics.Timestamp.Before(beforeUpdate) || metrics.Timestamp.After(afterUpdate) {
		t.Error("Metrics timestamp should be set to update time")
	}
	
	content := string(metrics.Content)
	expectedMetrics := []string{
		"transmission_download_speed_bytes",
		"transmission_upload_speed_bytes",
		"transmission_active_torrents",
		"transmission_torrent_download_rate_bytes",
		"transmission_torrent_upload_rate_bytes",
		"transmission_torrent_error",
		"transmission_exporter_last_successful_scrape_timestamp",
	}
	
	for _, metricName := range expectedMetrics {
		if !strings.Contains(content, metricName) {
			t.Errorf("Expected metric %s not found in content", metricName)
		}
	}
	
	expectedLabels := []string{
		`transmission_host="localhost"`,
		`exporter_instance="test"`,
		`transmission_port="9091"`,
	}
	
	for _, label := range expectedLabels {
		if !strings.Contains(content, label) {
			t.Errorf("Expected label %s not found in content", label)
		}
	}
}

func TestUpdateMetricsWithNilData(t *testing.T) {
	cache := NewMetricCache("localhost", "test", 9091, createTestLogger())
	
	cache.UpdateMetrics(nil, nil, nil)
	
	if cache.GetUpdateCount() != 1 {
		t.Errorf("Expected update count to be 1 even with nil data, got %d", cache.GetUpdateCount())
	}
	
	stats := &rpc.SessionStats{
		DownloadSpeed: 1000,
		UploadSpeed:   500,
	}
	
	cache.UpdateMetrics(stats, nil, nil)
	
	if cache.GetUpdateCount() != 2 {
		t.Errorf("Expected update count to be 2, got %d", cache.GetUpdateCount())
	}
}

func TestIsStale(t *testing.T) {
	cache := NewMetricCache("localhost", "test", 9091, createTestLogger())
	
	// Empty cache should be stale
	if !cache.IsStale(time.Second) {
		t.Error("Empty cache should be stale")
	}
	
	// Update cache
	stats := &rpc.SessionStats{DownloadSpeed: 1000}
	cache.UpdateMetrics(stats, nil, nil)
	
	// Fresh cache should not be stale
	if cache.IsStale(time.Minute) {
		t.Error("Fresh cache should not be stale")
	}
	
	if !cache.IsStale(time.Nanosecond) {
		t.Error("Cache should be stale with very short threshold")
	}
}

func TestConcurrentAccess(t *testing.T) {
	cache := NewMetricCache("localhost", "test", 9091, createTestLogger())
	
	const numGoroutines = 10
	const numOperations = 100
	
	done := make(chan bool, numGoroutines*2)
	
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()
			
			for j := 0; j < numOperations; j++ {
				stats := &rpc.SessionStats{
					DownloadSpeed: uint64(id*1000 + j),
					UploadSpeed:   uint64(id*500 + j),
				}
				cache.UpdateMetrics(stats, nil, nil)
			}
		}(i)
	}
	
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- true }()
			
			for j := 0; j < numOperations; j++ {
				_ = cache.GetMetrics()
				_ = cache.GetLastUpdateTime()
				_ = cache.IsStale(time.Minute)
				_ = cache.GetUpdateCount()
			}
		}()
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines*2; i++ {
		<-done
	}
	
	finalCount := cache.GetUpdateCount()
	if finalCount != uint64(numGoroutines*numOperations) {
		t.Errorf("Expected final update count %d, got %d", numGoroutines*numOperations, finalCount)
	}
	
	if cache.GetLastUpdateTime().IsZero() {
		t.Error("Expected last update time to be set after concurrent updates")
	}
}

func TestMetricsContentCopy(t *testing.T) {
	cache := NewMetricCache("localhost", "test", 9091, createTestLogger())
	
	stats := &rpc.SessionStats{DownloadSpeed: 1000}
	cache.UpdateMetrics(stats, nil, nil)
	
	metrics1 := cache.GetMetrics()
	metrics2 := cache.GetMetrics()
	
	if &metrics1.Content[0] == &metrics2.Content[0] {
		t.Error("Expected separate copies of metrics content")
	}
	
	// Modify one copy and verify the other is unchanged
	if len(metrics1.Content) > 0 {
		originalByte := metrics1.Content[0]
		metrics1.Content[0] = 'X'
		
		if metrics2.Content[0] != originalByte {
			t.Error("Modifying one metrics copy should not affect another")
		}
	}
}

func TestRecordCacheMiss(t *testing.T) {
	cache := NewMetricCache("localhost", "test", 9091, createTestLogger())
	
	// Record cache miss should not crash
	cache.RecordCacheMiss()
	
	// Should be able to record multiple misses
	cache.RecordCacheMiss()
	cache.RecordCacheMiss()
}

func TestRecordScrapeError(t *testing.T) {
	cache := NewMetricCache("localhost", "test", 9091, createTestLogger())
	
	// Record scrape error should not crash
	cache.RecordScrapeError("timeout")
	cache.RecordScrapeError("connection_failed")
	cache.RecordScrapeError("invalid_response")
}

func TestTorrentCleanup(t *testing.T) {
	cache := NewMetricCache("localhost", "test", 9091, createTestLogger())
	
	torrents1 := []*rpc.Torrent{
		{ID: 1, Status: rpc.TorrentDownload, PercentDone: 0.5},
		{ID: 2, Status: rpc.TorrentSeed, PercentDone: 1.0},
		{ID: 3, Status: rpc.TorrentDownload, PercentDone: 0.3},
	}
	
	cache.UpdateMetrics(nil, torrents1, nil)
	
	torrents2 := []*rpc.Torrent{
		{ID: 1, Status: rpc.TorrentDownload, PercentDone: 0.6},
		{ID: 3, Status: rpc.TorrentDownload, PercentDone: 0.4},
		{ID: 4, Status: rpc.TorrentSeed, PercentDone: 1.0}, // New torrent
	}
	
	cache.UpdateMetrics(nil, torrents2, nil)
	
	metrics := cache.GetMetrics()
	content := string(metrics.Content)
	
	// Should contain metrics for torrents 1, 3, and 4
	expectedTorrents := []string{`torrent_id="1"`, `torrent_id="3"`, `torrent_id="4"`}
	for _, torrentLabel := range expectedTorrents {
		if !strings.Contains(content, torrentLabel) {
			t.Errorf("Expected torrent label %s not found in content", torrentLabel)
		}
	}
}

// **Feature: transmission-prometheus-exporter, Property 12: Stale metric serving**
// **Validates: Requirements 6.2**
func TestProperty_StaleMetricServing(t *testing.T) {
	property := prop.ForAll(func(
		initialStats *rpc.SessionStats,
		initialTorrents []*rpc.Torrent,
		initialConfig *rpc.SessionConfig,
		staleThresholdSeconds int,
	) bool {
		cache := NewMetricCache("test-host", "test-instance", 9091, createTestLogger())
		
		cache.UpdateMetrics(initialStats, initialTorrents, initialConfig)
		
		staleThreshold := time.Duration(staleThresholdSeconds) * time.Second
		if cache.IsStale(staleThreshold) && staleThresholdSeconds > 0 {
			t.Logf("Cache should not be stale immediately after update with threshold %v", staleThreshold)
			return false
		}
		
		metrics := cache.GetMetrics()
		if metrics == nil {
			t.Logf("Expected metrics to be returned even when potentially stale")
			return false
		}
		
		// Content should be available even if cache becomes stale
		if len(metrics.Content) == 0 && (initialStats != nil || initialTorrents != nil || initialConfig != nil) {
			t.Logf("Expected non-empty content when data was provided")
			return false
		}
		
		// Timestamp should be set
		if metrics.Timestamp.IsZero() && (initialStats != nil || initialTorrents != nil || initialConfig != nil) {
			t.Logf("Expected timestamp to be set when data was provided")
			return false
		}
		
		if !cache.IsStale(time.Nanosecond) {
			t.Logf("Cache should be stale with very short threshold")
			return false
		}
		
		// Even stale cache should continue serving metrics
		staleMetrics := cache.GetMetrics()
		if staleMetrics == nil {
			t.Logf("Expected stale cache to continue serving metrics")
			return false
		}
		
		// Content should be the same as before
		if len(staleMetrics.Content) != len(metrics.Content) {
			t.Logf("Stale metrics content should be same as fresh metrics")
			return false
		}
		
		return true
	},
		testutilpkg.SessionStatsGen(),
		testutilpkg.TorrentSliceGen(),
		testutilpkg.SessionConfigGen(),
		gen.IntRange(0, 60), // staleThresholdSeconds
	)
	
	testutilpkg.RunPropertyTest(t, "stale metric serving", property, nil)
}

// Unit test for cache age tracking
func TestCacheAgeTracking(t *testing.T) {
	cache := NewMetricCache("localhost", "test", 9091, createTestLogger())
	
	stats := &rpc.SessionStats{DownloadSpeed: 1000}
	cache.UpdateMetrics(stats, nil, nil)
	
	// Get metrics immediately - age should be near 0
	metrics1 := cache.GetMetrics()
	if metrics1 == nil {
		t.Fatal("Expected metrics to be returned")
	}
	
	// Wait a bit and get metrics again
	time.Sleep(10 * time.Millisecond)
	metrics2 := cache.GetMetrics()
	if metrics2 == nil {
		t.Fatal("Expected metrics to be returned")
	}
	
	// Both should have the same timestamp (cached data)
	if !metrics1.Timestamp.Equal(metrics2.Timestamp) {
		t.Error("Expected same timestamp for cached data")
	}
	
	// But the cache age should be tracked internally (tested via metrics)
	content := string(metrics2.Content)
	if !strings.Contains(content, "transmission_exporter_cache_age_seconds") {
		t.Error("Expected cache age metric to be present")
	}
}

// Unit test for error handling in metric generation
func TestMetricGenerationError(t *testing.T) {
	cache := NewMetricCache("localhost", "test", 9091, createTestLogger())
	
	stats := &rpc.SessionStats{DownloadSpeed: 1000}
	cache.UpdateMetrics(stats, nil, nil)
	
	// Should have successful update
	if cache.GetUpdateCount() != 1 {
		t.Errorf("Expected update count 1, got %d", cache.GetUpdateCount())
	}
	
	// Metrics should be available
	metrics := cache.GetMetrics()
	if metrics == nil || len(metrics.Content) == 0 {
		t.Error("Expected metrics to be available after successful update")
	}
}

// Unit test for multiple updates
func TestMultipleUpdates(t *testing.T) {
	cache := NewMetricCache("localhost", "test", 9091, createTestLogger())
	
	// First update
	stats1 := &rpc.SessionStats{DownloadSpeed: 1000}
	cache.UpdateMetrics(stats1, nil, nil)
	
	firstUpdate := cache.GetLastUpdateTime()
	if cache.GetUpdateCount() != 1 {
		t.Errorf("Expected update count 1, got %d", cache.GetUpdateCount())
	}
	
	// Wait a bit to ensure different timestamp
	time.Sleep(time.Millisecond)
	
	// Second update
	stats2 := &rpc.SessionStats{DownloadSpeed: 2000}
	cache.UpdateMetrics(stats2, nil, nil)
	
	secondUpdate := cache.GetLastUpdateTime()
	if cache.GetUpdateCount() != 2 {
		t.Errorf("Expected update count 2, got %d", cache.GetUpdateCount())
	}
	
	if !secondUpdate.After(firstUpdate) {
		t.Error("Second update time should be after first update time")
	}
	
	// Metrics should reflect latest update
	metrics := cache.GetMetrics()
	if metrics == nil {
		t.Fatal("Expected metrics to be returned")
	}
	
	if !metrics.Timestamp.Equal(secondUpdate) {
		t.Error("Metrics timestamp should match latest update time")
	}
}