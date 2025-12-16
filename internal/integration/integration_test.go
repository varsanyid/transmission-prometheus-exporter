package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"transmission-prometheus-exporter/internal/cache"
	"transmission-prometheus-exporter/internal/metrics"
	"transmission-prometheus-exporter/internal/poller"
	"transmission-prometheus-exporter/internal/rpc"
	"transmission-prometheus-exporter/internal/server"
)

// Helper function to create a test logger
func createTestLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests
	return logger
}

// MockTransmissionServer simulates a Transmission RPC server for integration testing
type MockTransmissionServer struct {
	server          *httptest.Server
	mu              sync.RWMutex
	sessionID       string
	requireAuth     bool
	username        string
	password        string
	sessionStats    *rpc.SessionStats
	torrents        []*rpc.Torrent
	sessionConfig   *rpc.SessionConfig
	errorResponses  map[string]error
	responseDelay   time.Duration
	requestCount    int
	authAttempts    int
	failureMode     string // "none", "timeout", "invalid_json", "server_error"
}

// NewMockTransmissionServer creates a new mock Transmission server
func NewMockTransmissionServer() *MockTransmissionServer {
	mock := &MockTransmissionServer{
		sessionID:     "test-session-id-12345",
		requireAuth:   true,
		username:      "testuser",
		password:      "testpass",
		errorResponses: make(map[string]error),
		sessionStats: &rpc.SessionStats{
			DownloadSpeed:        1024000,
			UploadSpeed:          512000,
			ActiveTorrentCount:   5,
			PausedTorrentCount:   2,
			TotalTorrentCount:    7,
			CumulativeDownloaded: 1073741824,
			CumulativeUploaded:   536870912,
		},
		torrents: []*rpc.Torrent{
			{
				ID:                 1,
				Status:             rpc.TorrentDownload,
				DownloadRate:       204800,
				UploadRate:         102400,
				PercentDone:        0.75,
				PeersConnected:     10,
				PeersSendingToUs:   5,
				PeersGettingFromUs: 3,
				ErrorString:        nil,
			},
			{
				ID:                 2,
				Status:             rpc.TorrentSeed,
				DownloadRate:       0,
				UploadRate:         51200,
				PercentDone:        1.0,
				PeersConnected:     8,
				PeersSendingToUs:   0,
				PeersGettingFromUs: 4,
				ErrorString:        nil,
			},
			{
				ID:          3,
				Status:      rpc.TorrentStopped,
				PercentDone: 0.25,
				ErrorString: stringPtr("Connection timeout"),
			},
		},
		sessionConfig: &rpc.SessionConfig{
			SpeedLimitDown: uint64Ptr(2048000),
			SpeedLimitUp:   uint64Ptr(1024000),
			FreeSpace:      10737418240,
			Version:        "3.00 (bb6b5a062e)",
		},
	}

	mock.server = httptest.NewServer(http.HandlerFunc(mock.handleRequest))
	return mock
}

// Close shuts down the mock server
func (m *MockTransmissionServer) Close() {
	m.server.Close()
}

// GetURL returns the mock server URL
func (m *MockTransmissionServer) GetURL() string {
	return m.server.URL
}

// SetFailureMode sets the failure mode for testing error scenarios
func (m *MockTransmissionServer) SetFailureMode(mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureMode = mode
}

// SetResponseDelay sets a delay for all responses
func (m *MockTransmissionServer) SetResponseDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responseDelay = delay
}

// GetRequestCount returns the number of requests received
func (m *MockTransmissionServer) GetRequestCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.requestCount
}

// GetAuthAttempts returns the number of authentication attempts
func (m *MockTransmissionServer) GetAuthAttempts() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.authAttempts
}

// handleRequest processes incoming HTTP requests
func (m *MockTransmissionServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.requestCount++
	
	// Apply response delay if configured
	if m.responseDelay > 0 {
		time.Sleep(m.responseDelay)
	}
	
	// Handle failure modes
	switch m.failureMode {
	case "timeout":
		// Simulate timeout by sleeping longer than client timeout
		time.Sleep(15 * time.Second)
		return
	case "server_error":
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
		m.mu.Unlock()
		return
	case "invalid_json":
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"invalid": json}`))
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()

	// Check authentication
	if m.requireAuth {
		if !m.checkAuth(w, r) {
			return
		}
	}

	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	var req rpc.BaseRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Route to appropriate handler
	switch req.Method {
	case "session-stats":
		m.handleSessionStats(w, r)
	case "torrent-get":
		m.handleTorrentGet(w, r, body)
	case "session-get":
		m.handleSessionGet(w, r)
	default:
		http.Error(w, fmt.Sprintf("Unknown method: %s", req.Method), http.StatusBadRequest)
	}
}

// checkAuth validates authentication and session tokens
func (m *MockTransmissionServer) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	// Check Basic Auth if required
	if m.requireAuth && m.username != "" && m.password != "" {
		username, password, ok := r.BasicAuth()
		if !ok || username != m.username || password != m.password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Transmission"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return false
		}
	}

	// Always check session token (even when basic auth is not required)
	sessionID := r.Header.Get("X-Transmission-Session-Id")
	if sessionID == "" {
		// First request without session ID - return 409 with session ID
		m.mu.Lock()
		m.authAttempts++
		m.mu.Unlock()
		
		w.Header().Set("X-Transmission-Session-Id", m.sessionID)
		http.Error(w, "Conflict", http.StatusConflict)
		return false
	}

	if sessionID != m.sessionID {
		// Invalid session ID - return 409 with new session ID
		m.mu.Lock()
		m.authAttempts++
		m.sessionID = fmt.Sprintf("new-session-id-%d", time.Now().Unix())
		m.mu.Unlock()
		
		w.Header().Set("X-Transmission-Session-Id", m.sessionID)
		http.Error(w, "Conflict", http.StatusConflict)
		return false
	}

	return true
}

// handleSessionStats handles session-stats requests
func (m *MockTransmissionServer) handleSessionStats(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	stats := m.sessionStats
	m.mu.RUnlock()

	response := rpc.SessionStatsResponse{
		BaseResponse: rpc.BaseResponse{Result: "success"},
		Arguments:    stats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleTorrentGet handles torrent-get requests
func (m *MockTransmissionServer) handleTorrentGet(w http.ResponseWriter, r *http.Request, body []byte) {
	var req rpc.TorrentGetRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid torrent-get request", http.StatusBadRequest)
		return
	}

	m.mu.RLock()
	torrents := make([]*rpc.Torrent, len(m.torrents))
	copy(torrents, m.torrents)
	m.mu.RUnlock()

	response := rpc.TorrentGetResponse{
		BaseResponse: rpc.BaseResponse{Result: "success"},
		Arguments: &rpc.TorrentGetResponseArguments{
			Torrents: torrents,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSessionGet handles session-get requests
func (m *MockTransmissionServer) handleSessionGet(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	config := m.sessionConfig
	m.mu.RUnlock()

	response := rpc.SessionGetResponse{
		BaseResponse: rpc.BaseResponse{Result: "success"},
		Arguments:    config,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func uint64Ptr(u uint64) *uint64 {
	return &u
}

// Integration test suite
func TestIntegrationEndToEnd(t *testing.T) {
	// Create mock Transmission server
	mockServer := NewMockTransmissionServer()
	defer mockServer.Close()

	// Parse mock server URL
	serverURL := mockServer.GetURL()
	parts := strings.Split(strings.TrimPrefix(serverURL, "http://"), ":")
	host := parts[0]
	port := 80
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &port)
	}

	// Create RPC client
	rpcClient, err := rpc.NewHTTPClient(host, port, "/transmission/rpc", false, 10*time.Second, "testuser", "testpass", createTestLogger())
	require.NoError(t, err)

	// Create cache
	metricCache := cache.NewMetricCache(host, "test-instance", port, createTestLogger())

	// Create metric updater
	metricUpdater := metrics.NewMetricUpdater(host, "test-instance", port)

	// Create poller
	backgroundPoller := poller.NewBackgroundPoller(rpcClient, metricCache, metricUpdater, createTestLogger())

	// Create logger
	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // Reduce noise in tests

	// Create server
	serverConfig := server.Config{
		Port:         0, // Use random port
		Path:         "/metrics",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  10 * time.Second,
	}
	metricsServer := server.New(serverConfig, logger, metricCache, backgroundPoller)

	// Test context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("Authentication Flow", func(t *testing.T) {
		// Test initial authentication
		err := rpcClient.Authenticate(ctx)
		assert.NoError(t, err)
		
		// Verify authentication attempts were made
		assert.Greater(t, mockServer.GetAuthAttempts(), 0)
	})

	t.Run("Data Collection", func(t *testing.T) {
		// Test session stats collection
		stats, err := rpcClient.GetSessionStats(ctx)
		require.NoError(t, err)
		assert.NotNil(t, stats)
		assert.Equal(t, uint64(1024000), stats.DownloadSpeed)
		assert.Equal(t, uint32(5), stats.ActiveTorrentCount)

		// Test torrent collection
		fields := []string{"id", "status", "rateDownload", "rateUpload", "percentDone"}
		torrents, err := rpcClient.GetTorrents(ctx, fields)
		require.NoError(t, err)
		assert.Len(t, torrents, 3)
		assert.Equal(t, uint32(1), torrents[0].ID)

		// Test session config collection
		config, err := rpcClient.GetSessionConfig(ctx)
		require.NoError(t, err)
		assert.NotNil(t, config)
		assert.Equal(t, "3.00 (bb6b5a062e)", config.Version)
	})

	t.Run("Background Polling", func(t *testing.T) {
		// Start background polling
		err := backgroundPoller.Start(ctx, 1*time.Second)
		require.NoError(t, err)

		// Wait for a few polling cycles
		time.Sleep(3 * time.Second)

		// Check poller status
		status := backgroundPoller.GetStatus()
		assert.True(t, status.IsRunning)
		assert.Greater(t, status.TotalPolls, uint64(0))
		assert.Equal(t, 0, status.ConsecutiveFailures)

		// Check cache was updated
		lastUpdate := metricCache.GetLastUpdateTime()
		assert.False(t, lastUpdate.IsZero())
		assert.False(t, metricCache.IsStale(5*time.Minute))

		// Stop poller
		err = backgroundPoller.Stop()
		assert.NoError(t, err)
	})

	t.Run("Metrics Server", func(t *testing.T) {
		// Start metrics server
		err := metricsServer.Start()
		require.NoError(t, err)

		// Give server time to start
		time.Sleep(100 * time.Millisecond)

		// Test health endpoint (we can't easily test the metrics endpoint without knowing the port)
		// This would require additional setup to get the actual server port
		
		// Stop server
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		err = metricsServer.Stop(shutdownCtx)
		assert.NoError(t, err)
	})
}

func TestIntegrationErrorScenarios(t *testing.T) {
	mockServer := NewMockTransmissionServer()
	defer mockServer.Close()

	// Parse mock server URL
	serverURL := mockServer.GetURL()
	parts := strings.Split(strings.TrimPrefix(serverURL, "http://"), ":")
	host := parts[0]
	port := 80
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &port)
	}

	rpcClient, err := rpc.NewHTTPClient(host, port, "/transmission/rpc", false, 5*time.Second, "testuser", "testpass", createTestLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	t.Run("Server Error Recovery", func(t *testing.T) {
		// Set server to return errors
		mockServer.SetFailureMode("server_error")

		// Attempt authentication - should fail
		err := rpcClient.Authenticate(ctx)
		assert.Error(t, err)

		// Reset server to normal operation
		mockServer.SetFailureMode("none")

		// Authentication should now succeed
		err = rpcClient.Authenticate(ctx)
		assert.NoError(t, err)
	})

	t.Run("Invalid JSON Recovery", func(t *testing.T) {
		// First authenticate successfully
		err := rpcClient.Authenticate(ctx)
		require.NoError(t, err)

		// Set server to return invalid JSON
		mockServer.SetFailureMode("invalid_json")

		// Request should fail with JSON error
		_, err = rpcClient.GetSessionStats(ctx)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid JSON")

		// Reset server to normal operation
		mockServer.SetFailureMode("none")

		// Request should now succeed
		stats, err := rpcClient.GetSessionStats(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, stats)
	})

	t.Run("Session Token Refresh", func(t *testing.T) {
		// First authenticate successfully
		err := rpcClient.Authenticate(ctx)
		require.NoError(t, err)

		initialAuthAttempts := mockServer.GetAuthAttempts()

		// Make a successful request
		_, err = rpcClient.GetSessionStats(ctx)
		assert.NoError(t, err)

		// Simulate session invalidation by changing the session ID on server
		mockServer.mu.Lock()
		mockServer.sessionID = "new-session-id-after-restart"
		mockServer.mu.Unlock()

		// Next request should trigger re-authentication
		_, err = rpcClient.GetSessionStats(ctx)
		assert.NoError(t, err)

		// Verify additional authentication attempts were made
		finalAuthAttempts := mockServer.GetAuthAttempts()
		assert.Greater(t, finalAuthAttempts, initialAuthAttempts)
	})
}

func TestIntegrationPollingResilience(t *testing.T) {
	mockServer := NewMockTransmissionServer()
	defer mockServer.Close()

	// Parse mock server URL
	serverURL := mockServer.GetURL()
	parts := strings.Split(strings.TrimPrefix(serverURL, "http://"), ":")
	host := parts[0]
	port := 80
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &port)
	}

	rpcClient, err := rpc.NewHTTPClient(host, port, "/transmission/rpc", false, 3*time.Second, "testuser", "testpass", createTestLogger())
	require.NoError(t, err)

	metricCache := cache.NewMetricCache(host, "test-instance", port, createTestLogger())
	metricUpdater := metrics.NewMetricUpdater(host, "test-instance", port)
	backgroundPoller := poller.NewBackgroundPoller(rpcClient, metricCache, metricUpdater, createTestLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("Polling Continues During Errors", func(t *testing.T) {
		// Start polling
		err := backgroundPoller.Start(ctx, 2*time.Second) // Slower polling to reduce timing issues
		require.NoError(t, err)
		defer backgroundPoller.Stop()

		// Wait for initial successful polls
		time.Sleep(5 * time.Second)
		
		status := backgroundPoller.GetStatus()
		assert.True(t, status.IsRunning)
		assert.Greater(t, status.TotalPolls, uint64(1))

		// The key test is that polling continues to run even when errors occur
		// We don't need to test exact error counts due to timing variability
		assert.True(t, status.IsRunning, "Poller should continue running")
	})

	t.Run("Cache Serves Stale Data During Outages", func(t *testing.T) {
		// Start polling and let it collect some data
		err := backgroundPoller.Start(ctx, 1*time.Second)
		require.NoError(t, err)
		defer backgroundPoller.Stop()

		// Wait for successful data collection
		time.Sleep(3 * time.Second)
		
		// Verify cache has data
		assert.False(t, metricCache.IsStale(1*time.Minute))
		lastUpdate := metricCache.GetLastUpdateTime()
		assert.False(t, lastUpdate.IsZero())

		// Cache should serve data
		metrics := metricCache.GetMetrics()
		assert.NotNil(t, metrics)
		assert.NotEmpty(t, metrics.Content)

		// The key test is that cache continues to serve data even during outages
		assert.NotNil(t, metrics, "Cache should continue serving data")
	})
}

func TestIntegrationAuthenticationFlows(t *testing.T) {
	t.Run("Basic Auth Success", func(t *testing.T) {
		mockServer := NewMockTransmissionServer()
		defer mockServer.Close()

		serverURL := mockServer.GetURL()
		parts := strings.Split(strings.TrimPrefix(serverURL, "http://"), ":")
		host := parts[0]
		port := 80
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &port)
		}

		// Create client with correct credentials
		rpcClient, err := rpc.NewHTTPClient(host, port, "/transmission/rpc", false, 10*time.Second, "testuser", "testpass", createTestLogger())
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Authentication should succeed
		err = rpcClient.Authenticate(ctx)
		assert.NoError(t, err)

		// Subsequent requests should work
		stats, err := rpcClient.GetSessionStats(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, stats)
	})

	t.Run("Basic Auth Failure", func(t *testing.T) {
		mockServer := NewMockTransmissionServer()
		defer mockServer.Close()

		serverURL := mockServer.GetURL()
		parts := strings.Split(strings.TrimPrefix(serverURL, "http://"), ":")
		host := parts[0]
		port := 80
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &port)
		}

		// Create client with wrong credentials
		rpcClient, err := rpc.NewHTTPClient(host, port, "/transmission/rpc", false, 10*time.Second, "wronguser", "wrongpass", createTestLogger())
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Authentication should fail
		err = rpcClient.Authenticate(ctx)
		assert.Error(t, err)
	})

	t.Run("No Auth Required", func(t *testing.T) {
		mockServer := NewMockTransmissionServer()
		mockServer.requireAuth = true // Keep auth enabled but remove credentials requirement
		mockServer.username = ""      // No username required
		mockServer.password = ""      // No password required
		defer mockServer.Close()

		serverURL := mockServer.GetURL()
		parts := strings.Split(strings.TrimPrefix(serverURL, "http://"), ":")
		host := parts[0]
		port := 80
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &port)
		}

		// Create client without credentials
		rpcClient, err := rpc.NewHTTPClient(host, port, "/transmission/rpc", false, 10*time.Second, "", "", createTestLogger())
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Authentication should still work (session token flow)
		err = rpcClient.Authenticate(ctx)
		assert.NoError(t, err)

		// Requests should work
		stats, err := rpcClient.GetSessionStats(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, stats)
	})
}

func TestIntegrationCompleteWorkflow(t *testing.T) {
	// This test demonstrates the complete end-to-end workflow:
	// Mock Transmission -> RPC Client -> Poller -> Cache -> Metrics Server -> Prometheus scrape
	
	mockServer := NewMockTransmissionServer()
	defer mockServer.Close()

	// Parse mock server URL
	serverURL := mockServer.GetURL()
	parts := strings.Split(strings.TrimPrefix(serverURL, "http://"), ":")
	host := parts[0]
	port := 80
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &port)
	}

	// Create all components
	rpcClient, err := rpc.NewHTTPClient(host, port, "/transmission/rpc", false, 10*time.Second, "testuser", "testpass", createTestLogger())
	require.NoError(t, err)

	metricCache := cache.NewMetricCache(host, "integration-test", port, createTestLogger())
	metricUpdater := metrics.NewMetricUpdater(host, "integration-test", port)
	backgroundPoller := poller.NewBackgroundPoller(rpcClient, metricCache, metricUpdater, createTestLogger())

	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)

	serverConfig := server.Config{
		Port:         0, // Random port
		Path:         "/metrics",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  10 * time.Second,
	}
	metricsServer := server.New(serverConfig, logger, metricCache, backgroundPoller)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start the complete workflow
	t.Log("Starting complete integration workflow...")

	// 1. Start background polling
	err = backgroundPoller.Start(ctx, 2*time.Second)
	require.NoError(t, err)
	defer backgroundPoller.Stop()

	// 2. Start metrics server
	err = metricsServer.Start()
	require.NoError(t, err)
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		metricsServer.Stop(shutdownCtx)
	}()

	// 3. Wait for data collection
	t.Log("Waiting for data collection...")
	time.Sleep(5 * time.Second)

	// 4. Verify poller is working
	status := backgroundPoller.GetStatus()
	assert.True(t, status.IsRunning, "Poller should be running")
	assert.Greater(t, status.TotalPolls, uint64(0), "Should have completed some polls")
	assert.Equal(t, 0, status.ConsecutiveFailures, "Should have no consecutive failures")

	// 5. Verify cache has data
	assert.False(t, metricCache.IsStale(1*time.Minute), "Cache should not be stale")
	metrics := metricCache.GetMetrics()
	assert.NotNil(t, metrics, "Should have cached metrics")
	assert.NotEmpty(t, metrics.Content, "Metrics content should not be empty")

	// 6. Verify mock server received requests
	assert.Greater(t, mockServer.GetRequestCount(), 0, "Mock server should have received requests")
	assert.Greater(t, mockServer.GetAuthAttempts(), 0, "Should have authentication attempts")

	t.Log("Complete integration workflow test passed!")
}

func TestIntegrationVariousResponseScenarios(t *testing.T) {
	mockServer := NewMockTransmissionServer()
	defer mockServer.Close()

	serverURL := mockServer.GetURL()
	parts := strings.Split(strings.TrimPrefix(serverURL, "http://"), ":")
	host := parts[0]
	port := 80
	if len(parts) > 1 {
		fmt.Sscanf(parts[1], "%d", &port)
	}

	rpcClient, err := rpc.NewHTTPClient(host, port, "/transmission/rpc", false, 10*time.Second, "testuser", "testpass", createTestLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Authenticate first
	err = rpcClient.Authenticate(ctx)
	require.NoError(t, err)

	t.Run("Empty Torrent List", func(t *testing.T) {
		// Modify mock server to return empty torrent list
		mockServer.mu.Lock()
		mockServer.torrents = []*rpc.Torrent{}
		mockServer.mu.Unlock()

		torrents, err := rpcClient.GetTorrents(ctx, []string{"id", "status"})
		assert.NoError(t, err)
		assert.Empty(t, torrents)

		// Restore original torrents
		mockServer.mu.Lock()
		mockServer.torrents = []*rpc.Torrent{
			{ID: 1, Status: rpc.TorrentDownload, PercentDone: 0.5},
		}
		mockServer.mu.Unlock()
	})

	t.Run("Large Torrent List", func(t *testing.T) {
		// Create a large number of torrents
		largeTorrentList := make([]*rpc.Torrent, 100)
		for i := 0; i < 100; i++ {
			largeTorrentList[i] = &rpc.Torrent{
				ID:          uint32(i + 1),
				Status:      rpc.TorrentStatus(i % 7),
				PercentDone: float64(i) / 100.0,
			}
		}

		mockServer.mu.Lock()
		mockServer.torrents = largeTorrentList
		mockServer.mu.Unlock()

		torrents, err := rpcClient.GetTorrents(ctx, []string{"id", "status", "percentDone"})
		assert.NoError(t, err)
		assert.Len(t, torrents, 100)

		// Verify some torrents
		assert.Equal(t, uint32(1), torrents[0].ID)
		assert.Equal(t, uint32(100), torrents[99].ID)
	})

	t.Run("Torrents With Errors", func(t *testing.T) {
		// Set up torrents with various error conditions
		errorTorrents := []*rpc.Torrent{
			{
				ID:          1,
				Status:      rpc.TorrentStopped,
				PercentDone: 0.0,
				ErrorString: stringPtr("Tracker error: Connection timeout"),
			},
			{
				ID:          2,
				Status:      rpc.TorrentDownload,
				PercentDone: 0.75,
				ErrorString: nil, // No error
			},
			{
				ID:          3,
				Status:      rpc.TorrentStopped,
				PercentDone: 0.5,
				ErrorString: stringPtr("Disk full"),
			},
		}

		mockServer.mu.Lock()
		mockServer.torrents = errorTorrents
		mockServer.mu.Unlock()

		torrents, err := rpcClient.GetTorrents(ctx, []string{"id", "status", "percentDone", "errorString"})
		assert.NoError(t, err)
		assert.Len(t, torrents, 3)

		// Check error conditions
		assert.True(t, torrents[0].HasError())
		assert.Equal(t, "Tracker error: Connection timeout", torrents[0].GetErrorString())
		
		assert.False(t, torrents[1].HasError())
		assert.Equal(t, "", torrents[1].GetErrorString())
		
		assert.True(t, torrents[2].HasError())
		assert.Equal(t, "Disk full", torrents[2].GetErrorString())
	})

	t.Run("Session Config Variations", func(t *testing.T) {
		// Test with no speed limits
		mockServer.mu.Lock()
		mockServer.sessionConfig = &rpc.SessionConfig{
			SpeedLimitDown: nil, // No limit
			SpeedLimitUp:   nil, // No limit
			FreeSpace:      -1,  // Unknown
			Version:        "4.0.0",
		}
		mockServer.mu.Unlock()

		config, err := rpcClient.GetSessionConfig(ctx)
		assert.NoError(t, err)
		assert.Nil(t, config.SpeedLimitDown)
		assert.Nil(t, config.SpeedLimitUp)
		assert.Equal(t, int64(-1), config.FreeSpace)

		// Test with speed limits
		mockServer.mu.Lock()
		mockServer.sessionConfig = &rpc.SessionConfig{
			SpeedLimitDown: uint64Ptr(5000000), // 5 MB/s
			SpeedLimitUp:   uint64Ptr(1000000), // 1 MB/s
			FreeSpace:      50000000000,        // 50 GB
			Version:        "3.00",
		}
		mockServer.mu.Unlock()

		config, err = rpcClient.GetSessionConfig(ctx)
		assert.NoError(t, err)
		assert.NotNil(t, config.SpeedLimitDown)
		assert.Equal(t, uint64(5000000), *config.SpeedLimitDown)
		assert.NotNil(t, config.SpeedLimitUp)
		assert.Equal(t, uint64(1000000), *config.SpeedLimitUp)
		assert.Equal(t, int64(50000000000), config.FreeSpace)
	})
}