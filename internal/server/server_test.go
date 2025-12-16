package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	
	"transmission-prometheus-exporter/internal/cache"
	"transmission-prometheus-exporter/internal/poller"
	"transmission-prometheus-exporter/internal/rpc"
)

// mockCache implements the cache.Cache interface for testing
type mockCache struct {
	lastUpdateTime time.Time
	isStale        bool
}

func (m *mockCache) UpdateMetrics(stats *rpc.SessionStats, torrents []*rpc.Torrent, config *rpc.SessionConfig) {
	m.lastUpdateTime = time.Now()
}

func (m *mockCache) GetMetrics() *cache.PrometheusMetrics {
	return &cache.PrometheusMetrics{
		Content:   []byte("# Mock metrics\n"),
		Timestamp: m.lastUpdateTime,
	}
}

func (m *mockCache) GetLastUpdateTime() time.Time {
	return m.lastUpdateTime
}

func (m *mockCache) IsStale(maxAge time.Duration) bool {
	return m.isStale
}

// mockPoller implements the poller.Poller interface for testing
type mockPoller struct {
	status poller.Status
}

func (m *mockPoller) Start(ctx context.Context, interval time.Duration) error {
	m.status.IsRunning = true
	m.status.LastPollTime = time.Now()
	m.status.LastSuccessfulPoll = time.Now()
	return nil
}

func (m *mockPoller) Stop() error {
	m.status.IsRunning = false
	return nil
}

func (m *mockPoller) GetStatus() poller.Status {
	return m.status
}

// createTestServer creates a server with mock dependencies for testing
func createTestServer(port int) (*Server, *mockCache, *mockPoller) {
	logger := logrus.New()
	logger.SetOutput(io.Discard) // Suppress log output during tests

	config := Config{
		Port:         port,
		Path:         "/metrics",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		IdleTimeout:  10 * time.Second,
	}

	mockCache := &mockCache{
		lastUpdateTime: time.Now(),
		isStale:        false,
	}

	mockPoller := &mockPoller{
		status: poller.Status{
			IsRunning:           true,
			LastPollTime:        time.Now(),
			LastSuccessfulPoll:  time.Now(),
			ConsecutiveFailures: 0,
			TotalPolls:          10,
			TotalErrors:         0,
		},
	}

	server := New(config, logger, mockCache, mockPoller)
	return server, mockCache, mockPoller
}

func TestServer_StartAndStop(t *testing.T) {
	// Create server with mock dependencies
	server, _, _ := createTestServer(0) // Use port 0 to get a random available port

	// Start server
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Stop server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = server.Stop(ctx)
	if err != nil {
		t.Fatalf("Failed to stop server: %v", err)
	}
}

func TestServer_MetricsEndpoint(t *testing.T) {
	// Create and start server
	server, _, _ := createTestServer(0) // Use port 0 to get a random available port
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Get the actual port the server is listening on
	// Note: Since we used port 0, we need to extract the actual port
	// For this test, we'll use a fixed port instead
	testPort := 19190
	server, _, _ = createTestServer(testPort)
	server.Start()
	time.Sleep(100 * time.Millisecond)

	// Test metrics endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/metrics", testPort))
	if err != nil {
		t.Fatalf("Failed to get metrics endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Check that we get some metrics output (should contain prometheus metrics)
	if len(body) == 0 {
		t.Error("Expected non-empty metrics response")
	}
}

func TestServer_HealthEndpoint(t *testing.T) {
	// Create and start server
	testPort := 19191
	server, _, _ := createTestServer(testPort)
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test health endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", testPort))
	if err != nil {
		t.Fatalf("Failed to get health endpoint: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}

	// Parse JSON response
	var healthStatus HealthStatus
	if err := json.Unmarshal(body, &healthStatus); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Check that we get "healthy" status
	if healthStatus.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", healthStatus.Status)
	}

	// Check that required fields are present
	if healthStatus.Timestamp.IsZero() {
		t.Error("Expected non-zero timestamp")
	}
	if !healthStatus.PollerRunning {
		t.Error("Expected poller to be running")
	}
}

func TestServer_ConfigDefaults(t *testing.T) {
	logger := logrus.New()
	logger.SetOutput(io.Discard)

	// Create server config without timeouts
	config := Config{
		Port: 19192,
		Path: "/metrics",
		// Timeouts not set - should use defaults
	}

	mockCache := &mockCache{lastUpdateTime: time.Now()}
	mockPoller := &mockPoller{status: poller.Status{IsRunning: true}}

	// Create server
	server := New(config, logger, mockCache, mockPoller)

	// Check that defaults were applied
	if server.httpServer.ReadTimeout != 30*time.Second {
		t.Errorf("Expected ReadTimeout 30s, got %v", server.httpServer.ReadTimeout)
	}
	if server.httpServer.WriteTimeout != 30*time.Second {
		t.Errorf("Expected WriteTimeout 30s, got %v", server.httpServer.WriteTimeout)
	}
	if server.httpServer.IdleTimeout != 60*time.Second {
		t.Errorf("Expected IdleTimeout 60s, got %v", server.httpServer.IdleTimeout)
	}
}

func TestServer_GetAddr(t *testing.T) {
	// Create server
	server, _, _ := createTestServer(19193)

	// Check address
	expectedAddr := ":19193"
	if server.GetAddr() != expectedAddr {
		t.Errorf("Expected address '%s', got '%s'", expectedAddr, server.GetAddr())
	}
}

func TestServer_HealthEndpoint_Degraded(t *testing.T) {
	// Create server with stale cache
	testPort := 19194
	server, mockCache, _ := createTestServer(testPort)
	
	// Make cache stale
	mockCache.isStale = true
	mockCache.lastUpdateTime = time.Now().Add(-10 * time.Minute)
	
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Test health endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", testPort))
	if err != nil {
		t.Fatalf("Failed to get health endpoint: %v", err)
	}
	defer resp.Body.Close()

	// Should still return 200 for degraded state
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for degraded state, got %d", resp.StatusCode)
	}

	// Parse JSON response
	var healthStatus HealthStatus
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &healthStatus); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Check that we get "degraded" status
	if healthStatus.Status != "degraded" {
		t.Errorf("Expected status 'degraded', got '%s'", healthStatus.Status)
	}

	// Check that cache is marked as stale
	if !healthStatus.CacheStale {
		t.Error("Expected cache to be marked as stale")
	}
}

func TestServer_HealthEndpoint_Unhealthy(t *testing.T) {
	// Create server with stopped poller
	testPort := 19195
	server, _, mockPoller := createTestServer(testPort)
	
	// Stop the poller
	mockPoller.status.IsRunning = false
	
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Test health endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", testPort))
	if err != nil {
		t.Fatalf("Failed to get health endpoint: %v", err)
	}
	defer resp.Body.Close()

	// Should return 503 for unhealthy state
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 for unhealthy state, got %d", resp.StatusCode)
	}

	// Parse JSON response
	var healthStatus HealthStatus
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &healthStatus); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Check that we get "unhealthy" status
	if healthStatus.Status != "unhealthy" {
		t.Errorf("Expected status 'unhealthy', got '%s'", healthStatus.Status)
	}

	// Check that poller is marked as not running
	if healthStatus.PollerRunning {
		t.Error("Expected poller to be marked as not running")
	}
}

func TestServer_HealthEndpoint_ConsecutiveFailures(t *testing.T) {
	// Create server with multiple consecutive failures
	testPort := 19196
	server, _, mockPoller := createTestServer(testPort)
	
	// Set multiple consecutive failures
	mockPoller.status.ConsecutiveFailures = 5
	mockPoller.status.TotalErrors = 10
	
	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Stop(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Test health endpoint
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/health", testPort))
	if err != nil {
		t.Fatalf("Failed to get health endpoint: %v", err)
	}
	defer resp.Body.Close()

	// Should still return 200 for degraded state
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for degraded state, got %d", resp.StatusCode)
	}

	// Parse JSON response
	var healthStatus HealthStatus
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &healthStatus); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// Check that we get "degraded" status
	if healthStatus.Status != "degraded" {
		t.Errorf("Expected status 'degraded', got '%s'", healthStatus.Status)
	}

	// Check that consecutive failures are reported
	if healthStatus.ConsecutiveFailures != 5 {
		t.Errorf("Expected 5 consecutive failures, got %d", healthStatus.ConsecutiveFailures)
	}

	// Check that total errors are reported
	if healthStatus.TotalErrors != 10 {
		t.Errorf("Expected 10 total errors, got %d", healthStatus.TotalErrors)
	}
}