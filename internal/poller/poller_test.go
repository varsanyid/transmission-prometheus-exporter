package poller

import (
	"context"
	"testing"
	"time"
	
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/sirupsen/logrus"
	
	"transmission-prometheus-exporter/internal/cache"
	"transmission-prometheus-exporter/internal/metrics"
	"transmission-prometheus-exporter/internal/rpc"
	"transmission-prometheus-exporter/internal/testutil"
)

// Helper function to create a test logger
func createTestLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests
	return logger
}

func TestProperty_ConfigurablePollingIntervals(t *testing.T) {
	property := prop.ForAll(func(intervalSeconds int) bool {
		// Create mock RPC client and cache
		mockRPC := &MockRPCClient{}
		mockCache := &MockCache{}
		
		// Create mock metric updater (can be nil for this test)
		var mockMetricUpdater *metrics.MetricUpdater = nil
		
		// Create poller instance
		poller := NewBackgroundPoller(mockRPC, mockCache, mockMetricUpdater, createTestLogger())
		
		// Convert seconds to duration
		interval := time.Duration(intervalSeconds) * time.Second
		
		// Create context with timeout for the test
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		
		// Test 1: Valid intervals (1-60 seconds) should be accepted
		if intervalSeconds >= 1 && intervalSeconds <= 60 {
			err := poller.Start(ctx, interval)
			if err != nil {
				t.Logf("Valid interval %v was rejected: %v", interval, err)
				return false
			}
			
			// Stop the poller
			stopErr := poller.Stop()
			if stopErr != nil {
				t.Logf("Failed to stop poller: %v", stopErr)
				return false
			}
			
			return true
		}
		
		// Test 2: Invalid intervals (outside 1-60 seconds) should be rejected
		err := poller.Start(ctx, interval)
		if err == nil {
			// Invalid interval was accepted - this is wrong
			poller.Stop() // Clean up
			t.Logf("Invalid interval %v was accepted when it should be rejected", interval)
			return false
		}
		
		// Invalid interval was correctly rejected
		return true
	}, gen.IntRange(-10, 120)) // Test range includes invalid values
	
	testutil.RunPropertyTest(t, "configurable polling intervals", property, nil)
}

// MockRPCClient implements the rpc.Client interface for testing
type MockRPCClient struct{}

func (m *MockRPCClient) Authenticate(ctx context.Context) error {
	return nil
}

func (m *MockRPCClient) GetSessionStats(ctx context.Context) (*rpc.SessionStats, error) {
	return &rpc.SessionStats{
		DownloadSpeed:        1000,
		UploadSpeed:          500,
		ActiveTorrentCount:   5,
		PausedTorrentCount:   2,
		TotalTorrentCount:    7,
		CumulativeDownloaded: 1000000,
		CumulativeUploaded:   500000,
	}, nil
}

func (m *MockRPCClient) GetTorrents(ctx context.Context, fields []string) ([]*rpc.Torrent, error) {
	return []*rpc.Torrent{}, nil
}

func (m *MockRPCClient) GetSessionConfig(ctx context.Context) (*rpc.SessionConfig, error) {
	return &rpc.SessionConfig{
		Version:   "3.00",
		FreeSpace: 1000000000,
	}, nil
}

// MockCache implements the cache.Cache interface for testing
type MockCache struct{}

func (m *MockCache) UpdateMetrics(stats *rpc.SessionStats, torrents []*rpc.Torrent, config *rpc.SessionConfig) {
	// No-op for testing
}

func (m *MockCache) GetMetrics() *cache.PrometheusMetrics {
	return &cache.PrometheusMetrics{
		Content:   []byte("# Mock metrics"),
		Timestamp: time.Now(),
	}
}

func (m *MockCache) GetLastUpdateTime() time.Time {
	return time.Now()
}

func (m *MockCache) IsStale(maxAge time.Duration) bool {
	return false
}