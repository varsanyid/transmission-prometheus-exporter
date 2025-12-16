package metrics

import (
	"bytes"
	"strings"
	"testing"

	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/expfmt"
	
	testutilpkg "transmission-prometheus-exporter/internal/testutil"
)

func TestCommonLabels(t *testing.T) {
	// Test that all metrics include consistent common labels
	t.Run("CommonLabelConsistency", func(t *testing.T) {
		host := "test-transmission"
		instance := "test-exporter-instance"
		port := 9091
		
		updater := NewMetricUpdater(host, instance, port)
		
		// Update various types of metrics
		updater.UpdateGlobalMetrics(1000, 500, 5, 2, 7)
		updater.UpdateTorrentMetrics("123", 100, 50, 0.75, 10, 5, 3, 4, false, "")
		updater.RecordScrapeError("timeout")
		updater.UpdateConnectionState(true)
		
		// Verify global metrics have common labels
		downloadMetric := TransmissionDownloadSpeedBytes.WithLabelValues(host, instance, "9091")
		if testutil.ToFloat64(downloadMetric) != 1000 {
			t.Errorf("Expected download speed metric with common labels to have value 1000")
		}
		
		// Verify torrent metrics have common labels
		torrentMetric := TransmissionTorrentDownloadRateBytes.WithLabelValues("123", host, instance, "9091")
		if testutil.ToFloat64(torrentMetric) != 100 {
			t.Errorf("Expected torrent metric with common labels to have value 100")
		}
		
		// Verify exporter metrics have common labels
		errorMetric := TransmissionExporterScrapeErrorsTotal.WithLabelValues("timeout", host, instance, "9091")
		if testutil.ToFloat64(errorMetric) != 1 {
			t.Errorf("Expected error metric with common labels to have value 1")
		}
		
		connectionMetric := TransmissionExporterConnectionState.WithLabelValues(host, instance, "9091")
		if testutil.ToFloat64(connectionMetric) != 1 {
			t.Errorf("Expected connection state metric with common labels to have value 1")
		}
	})
	
	t.Run("CustomizableInstanceLabel", func(t *testing.T) {
		// Test that the instance label can be customized (Requirement 8.5)
		customInstance := "my-custom-exporter-name"
		updater := NewMetricUpdater("localhost", customInstance, 9091)
		
		updater.UpdateGlobalMetrics(100, 50, 1, 0, 1)
		
		// Verify the custom instance label is used
		metric := TransmissionDownloadSpeedBytes.WithLabelValues("localhost", customInstance, "9091")
		if testutil.ToFloat64(metric) != 100 {
			t.Errorf("Expected metric with custom instance label to work correctly")
		}
	})
}

func TestMetricDefinitions(t *testing.T) {
	// Test that all global metrics are properly defined
	t.Run("GlobalMetrics", func(t *testing.T) {
		updater := NewMetricUpdater("localhost", "test-instance", 9091)
		
		// Update global metrics with test values
		updater.UpdateGlobalMetrics(1000, 500, 5, 2, 7)
		updater.UpdateCumulativeMetrics(1000000, 500000)
		updater.UpdateSessionConfig(2000, 1000, 10000000)
		
		// Verify metrics have expected values with common labels
		metric := TransmissionDownloadSpeedBytes.WithLabelValues("localhost", "test-instance", "9091")
		if testutil.ToFloat64(metric) != 1000 {
			t.Errorf("Expected download speed 1000, got %f", testutil.ToFloat64(metric))
		}
		
		uploadMetric := TransmissionUploadSpeedBytes.WithLabelValues("localhost", "test-instance", "9091")
		if testutil.ToFloat64(uploadMetric) != 500 {
			t.Errorf("Expected upload speed 500, got %f", testutil.ToFloat64(uploadMetric))
		}
		
		activeMetric := TransmissionActiveTorrents.WithLabelValues("localhost", "test-instance", "9091")
		if testutil.ToFloat64(activeMetric) != 5 {
			t.Errorf("Expected active torrents 5, got %f", testutil.ToFloat64(activeMetric))
		}
	})
	
	t.Run("TorrentMetrics", func(t *testing.T) {
		updater := NewMetricUpdater("localhost", "test-instance", 9091)
		
		// Update torrent metrics with test values
		updater.UpdateTorrentMetrics("123", 100, 50, 0.75, 10, 5, 3, 4, false, "")
		
		// Verify per-torrent metrics with common labels
		metric := TransmissionTorrentDownloadRateBytes.WithLabelValues("123", "localhost", "test-instance", "9091")
		if testutil.ToFloat64(metric) != 100 {
			t.Errorf("Expected torrent download rate 100, got %f", testutil.ToFloat64(metric))
		}
		
		progressMetric := TransmissionTorrentProgressRatio.WithLabelValues("123", "localhost", "test-instance", "9091")
		if testutil.ToFloat64(progressMetric) != 0.75 {
			t.Errorf("Expected torrent progress 0.75, got %f", testutil.ToFloat64(progressMetric))
		}
	})
	
	t.Run("ErrorMetrics", func(t *testing.T) {
		updater := NewMetricUpdater("localhost", "test-instance", 9091)
		
		// Test error state handling
		updater.UpdateTorrentMetrics("456", 0, 0, 0.5, 0, 0, 0, 0, true, "Connection timeout")
		
		errorMetric := TransmissionTorrentError.WithLabelValues("456", "Connection timeout", "localhost", "test-instance", "9091")
		if testutil.ToFloat64(errorMetric) != 1 {
			t.Errorf("Expected torrent error state 1, got %f", testutil.ToFloat64(errorMetric))
		}
	})
	
	t.Run("ExporterMetrics", func(t *testing.T) {
		updater := NewMetricUpdater("localhost", "test-instance", 9091)
		
		// Test exporter self-monitoring metrics
		updater.RecordScrapeError("timeout")
		updater.RecordRPCLatency("session-stats", 0.5)
		updater.UpdateLastSuccessfulScrape(1234567890)
		updater.UpdateConnectionState(true)
		
		// Verify error counter incremented with common labels
		errorCounter := TransmissionExporterScrapeErrorsTotal.WithLabelValues("timeout", "localhost", "test-instance", "9091")
		if testutil.ToFloat64(errorCounter) != 1 {
			t.Errorf("Expected scrape error count 1, got %f", testutil.ToFloat64(errorCounter))
		}
		
		// Verify connection state with common labels
		connectionState := TransmissionExporterConnectionState.WithLabelValues("localhost", "test-instance", "9091")
		if testutil.ToFloat64(connectionState) != 1 {
			t.Errorf("Expected connection state 1, got %f", testutil.ToFloat64(connectionState))
		}
	})
}

func TestMetricNames(t *testing.T) {
	// Test that all metric names follow Prometheus naming conventions
	expectedMetrics := []string{
		"transmission_download_speed_bytes",
		"transmission_upload_speed_bytes",
		"transmission_active_torrents",
		"transmission_total_torrents",
		"transmission_torrent_download_rate_bytes",
		"transmission_torrent_upload_rate_bytes",
		"transmission_torrent_progress_ratio",
		"transmission_torrent_peers_connected",
		"transmission_torrent_error",
		"transmission_exporter_scrape_errors_total",
		"transmission_exporter_rpc_latency_seconds",
		"transmission_exporter_last_successful_scrape_timestamp",
		"transmission_exporter_connection_state",
	}
	
	// Gather all metrics from the default registry
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}
	
	// Check that our expected metrics exist
	foundMetrics := make(map[string]bool)
	for _, mf := range metricFamilies {
		foundMetrics[mf.GetName()] = true
	}
	
	for _, expectedMetric := range expectedMetrics {
		if !foundMetrics[expectedMetric] {
			t.Errorf("Expected metric %s not found in registry", expectedMetric)
		}
	}
}

func TestFiniteCardinality(t *testing.T) {
	// Test that we maintain finite cardinality by using only torrent IDs as labels (plus common labels)
	updater := NewMetricUpdater("localhost", "test-instance", 9091)
	
	// Update metrics for multiple torrents using only IDs
	torrentIDs := []string{"1", "2", "3", "999"}
	
	for _, id := range torrentIDs {
		updater.UpdateTorrentMetrics(id, 100, 50, 0.5, 5, 2, 1, 4, false, "")
	}
	
	// Verify that metrics exist for each torrent ID with common labels
	for _, id := range torrentIDs {
		metric := TransmissionTorrentDownloadRateBytes.WithLabelValues(id, "localhost", "test-instance", "9091")
		if testutil.ToFloat64(metric) != 100 {
			t.Errorf("Expected metric for torrent %s to exist with value 100", id)
		}
	}
	
	// Test that error metrics use torrent_id, error_string, and common labels
	updater.UpdateTorrentMetrics("error_torrent", 0, 0, 0, 0, 0, 0, 0, true, "Tracker error")
	
	errorMetric := TransmissionTorrentError.WithLabelValues("error_torrent", "Tracker error", "localhost", "test-instance", "9091")
	if testutil.ToFloat64(errorMetric) != 1 {
		t.Errorf("Expected error metric to be set to 1")
	}
}

func TestMetricHelp(t *testing.T) {
	// Verify that all metrics have helpful descriptions
	metricFamilies, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Failed to gather metrics: %v", err)
	}
	
	for _, mf := range metricFamilies {
		name := mf.GetName()
		help := mf.GetHelp()
		
		// Skip non-transmission metrics
		if !strings.HasPrefix(name, "transmission_") {
			continue
		}
		
		if help == "" {
			t.Errorf("Metric %s has empty help text", name)
		}
		
		if len(help) < 10 {
			t.Errorf("Metric %s has very short help text: %s", name, help)
		}
	}
}

func TestProperty_CompleteMetricExposure(t *testing.T) {
	property := prop.ForAll(func(
		downloadSpeed, uploadSpeed float64,
		activeTorrents, pausedTorrents, totalTorrents float64,
		cumulativeDown, cumulativeUp float64,
		speedLimitDown, speedLimitUp, freeSpace float64,
		torrentCount int,
	) bool {
		// Create a fresh metric updater for this test iteration with common labels
		updater := NewMetricUpdater("test-host", "test-instance", 9091)
		
		// Update global metrics with generated values
		updater.UpdateGlobalMetrics(downloadSpeed, uploadSpeed, activeTorrents, pausedTorrents, totalTorrents)
		updater.UpdateCumulativeMetrics(cumulativeDown, cumulativeUp)
		updater.UpdateSessionConfig(speedLimitDown, speedLimitUp, freeSpace)
		
		// Update some torrent metrics with generated data
		torrentIDs := make([]string, 0, torrentCount)
		for i := 0; i < torrentCount; i++ {
			torrentID := string(rune('1' + i%10)) // Use simple IDs to avoid cardinality issues
			torrentIDs = append(torrentIDs, torrentID)
			
			// Generate torrent-specific values
			downloadRate := float64(i * 100)
			uploadRate := float64(i * 50)
			progress := float64(i) / float64(torrentCount+1) // Ensure 0.0 to 1.0 range
			peersConnected := float64(i * 5)
			peersSending := float64(i * 2)
			peersGetting := float64(i * 1)
			status := float64(i % 7) // Status 0-6
			
			updater.UpdateTorrentMetrics(torrentID, downloadRate, uploadRate, progress,
				peersConnected, peersSending, peersGetting, status, false, "")
		}
		
		// Add some exporter metrics
		updater.RecordScrapeError("test_error")
		updater.RecordRPCLatency("session-stats", 0.5)
		updater.UpdateLastSuccessfulScrape(1234567890)
		updater.UpdateConnectionState(true)
		
		// Gather all metrics from the registry
		metricFamilies, err := prometheus.DefaultGatherer.Gather()
		if err != nil {
			t.Logf("Failed to gather metrics: %v", err)
			return false
		}
		
		// Convert to Prometheus text format
		var buf bytes.Buffer
		for _, mf := range metricFamilies {
			if _, err := expfmt.MetricFamilyToText(&buf, mf); err != nil {
				t.Logf("Failed to format metrics: %v", err)
				return false
			}
		}
		
		prometheusOutput := buf.String()
		
		requiredGlobalMetrics := []string{
			"transmission_download_speed_bytes",      // Requirement 1.2
			"transmission_upload_speed_bytes",       // Requirement 1.2
			"transmission_active_torrents",          // Requirement 1.2
			"transmission_total_torrents",           // Requirement 1.2
			"transmission_cumulative_downloaded_bytes", // Requirement 1.3
			"transmission_cumulative_uploaded_bytes",   // Requirement 1.3
			"transmission_speed_limit_down_bytes",      // Requirement 1.4
			"transmission_speed_limit_up_bytes",        // Requirement 1.4
			"transmission_free_space_bytes",            // Requirement 1.4
		}
		
		requiredTorrentMetrics := []string{
			"transmission_torrent_download_rate_bytes", // Requirement 2.4
			"transmission_torrent_upload_rate_bytes",   // Requirement 2.4
			"transmission_torrent_progress_ratio",      // Requirement 2.4
			"transmission_torrent_peers_connected",     // Requirement 2.4
		}
		
		requiredExporterMetrics := []string{
			"transmission_exporter_scrape_errors_total",           // Requirement 5.1
			"transmission_exporter_rpc_latency_seconds",           // Requirement 5.2
			"transmission_exporter_last_successful_scrape_timestamp", // Requirement 5.3
			"transmission_exporter_connection_state",              // Requirement 5.4
		}
		
		// Check that all required global metrics are present
		for _, metricName := range requiredGlobalMetrics {
			if !strings.Contains(prometheusOutput, metricName) {
				t.Logf("Required global metric %s not found in Prometheus output", metricName)
				return false
			}
		}
		
		// Check that torrent metrics are present when torrents exist
		if torrentCount > 0 {
			for _, metricName := range requiredTorrentMetrics {
				if !strings.Contains(prometheusOutput, metricName) {
					t.Logf("Required torrent metric %s not found in Prometheus output when torrents exist", metricName)
					return false
				}
			}
		}
		
		// Check that exporter metrics are present
		for _, metricName := range requiredExporterMetrics {
			if !strings.Contains(prometheusOutput, metricName) {
				t.Logf("Required exporter metric %s not found in Prometheus output", metricName)
				return false
			}
		}
		
		if !strings.Contains(prometheusOutput, "# HELP") {
			t.Logf("Prometheus output missing HELP comments")
			return false
		}
		
		if !strings.Contains(prometheusOutput, "# TYPE") {
			t.Logf("Prometheus output missing TYPE comments")
			return false
		}
		
		// Verify that metric values are properly formatted (no NaN, invalid Inf, etc.)
		// Note: "+Inf" is valid in histogram buckets, so we exclude that case
		if strings.Contains(prometheusOutput, "NaN") || 
		   (strings.Contains(prometheusOutput, "Inf") && !strings.Contains(prometheusOutput, "+Inf")) {
			t.Logf("Prometheus output contains invalid values (NaN or invalid Inf):\n%s", prometheusOutput)
			return false
		}
		
		// Verify finite cardinality - torrent metrics should only use torrent_id labels
		for _, torrentID := range torrentIDs {
			expectedLabel := `torrent_id="` + torrentID + `"`
			if !strings.Contains(prometheusOutput, expectedLabel) {
				t.Logf("Expected torrent label %s not found in output", expectedLabel)
				return false
			}
		}
		
		return true
	},
		gen.Float64Range(0, 1000000),    // downloadSpeed
		gen.Float64Range(0, 1000000),    // uploadSpeed  
		gen.Float64Range(0, 100),        // activeTorrents
		gen.Float64Range(0, 100),        // pausedTorrents
		gen.Float64Range(0, 200),        // totalTorrents
		gen.Float64Range(0, 1e12),       // cumulativeDown
		gen.Float64Range(0, 1e12),       // cumulativeUp
		gen.Float64Range(0, 1000000),    // speedLimitDown
		gen.Float64Range(0, 1000000),    // speedLimitUp
		gen.Float64Range(0, 1e12),       // freeSpace
		gen.IntRange(0, 5),              // torrentCount (keep small for test performance)
	)
	
	testutilpkg.RunPropertyTest(t, "complete metric exposure", property, nil)
}