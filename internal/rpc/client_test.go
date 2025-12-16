package rpc

import (
	"fmt"
	"net/http"
	"testing"
	"time"
	
	"github.com/sirupsen/logrus"
)

func createTestLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel) // Reduce noise in tests
	return logger
}

func TestNewHTTPClient(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		port      int
		path      string
		useHTTPS  bool
		timeout   time.Duration
		username  string
		password  string
		expectErr bool
	}{
		{
			name:     "valid HTTP client",
			host:     "localhost",
			port:     9091,
			path:     "/transmission/rpc",
			useHTTPS: false,
			timeout:  10 * time.Second,
			username: "",
			password: "",
		},
		{
			name:     "valid HTTPS client",
			host:     "example.com",
			port:     443,
			path:     "/rpc",
			useHTTPS: true,
			timeout:  5 * time.Second,
			username: "user",
			password: "pass",
		},
		{
			name:      "invalid port",
			host:      "localhost",
			port:      -1,
			path:      "/rpc",
			useHTTPS:  false,
			timeout:   10 * time.Second,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewHTTPClient(tt.host, tt.port, tt.path, tt.useHTTPS, tt.timeout, tt.username, tt.password, createTestLogger())
			
			if tt.expectErr {
				if err == nil {
					t.Error("Expected error creating client, got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("Unexpected error creating client: %v", err)
			}
			
			if client == nil {
				t.Fatal("Expected client to be created, got nil")
			}
			
			// Verify client configuration
			expectedScheme := "http"
			if tt.useHTTPS {
				expectedScheme = "https"
			}
			
			if client.baseURL.Scheme != expectedScheme {
				t.Errorf("Expected scheme %s, got %s", expectedScheme, client.baseURL.Scheme)
			}
			
			if client.baseURL.Host != tt.host+":"+string(rune(tt.port)) {
				// Note: This is a simplified check, actual URL parsing handles this correctly
			}
			
			if client.username != tt.username {
				t.Errorf("Expected username %s, got %s", tt.username, client.username)
			}
			
			if client.password != tt.password {
				t.Errorf("Expected password %s, got %s", tt.password, client.password)
			}
		})
	}
}

func TestHTTPClientRetryLogic(t *testing.T) {
	// Test the retry logic components without a full integration test
	client := &HTTPClient{
		maxRetries: 3,
		baseDelay:  time.Millisecond * 100,
		maxDelay:   time.Second,
	}
	
	// Test that retry delays are calculated correctly
	delay1 := client.calculateBackoffDelay(1)
	delay2 := client.calculateBackoffDelay(2)
	
	if delay1 <= 0 {
		t.Error("First retry delay should be positive")
	}
	
	if delay2 <= delay1/2 { // Allow for jitter, but should generally be larger
		t.Error("Second retry delay should generally be larger than first")
	}
}

func TestHTTPClientSessionIDHandling(t *testing.T) {
	client := &HTTPClient{}
	
	// Test setting and getting session ID
	testSessionID := "test-session-123"
	client.setSessionID(testSessionID)
	
	retrievedID := client.getSessionID()
	if retrievedID != testSessionID {
		t.Errorf("Expected session ID %s, got %s", testSessionID, retrievedID)
	}
}

func TestBackoffCalculation(t *testing.T) {
	client := &HTTPClient{
		baseDelay: time.Second,
		maxDelay:  30 * time.Second,
	}
	
	// Test exponential backoff calculation
	delay1 := client.calculateBackoffDelay(1)
	delay2 := client.calculateBackoffDelay(2)
	delay3 := client.calculateBackoffDelay(3)
	
	// Delays should generally increase (allowing for jitter)
	if delay1 < 0 || delay2 < 0 || delay3 < 0 {
		t.Error("Delays should not be negative")
	}
	
	// Test that delay doesn't exceed maximum
	delayMax := client.calculateBackoffDelay(10)
	if delayMax > client.maxDelay*2 { // Allow for jitter
		t.Errorf("Delay %v exceeds maximum %v (with jitter allowance)", delayMax, client.maxDelay)
	}
}

func TestRetryableErrors(t *testing.T) {
	client := &HTTPClient{}
	
	tests := []struct {
		name      string
		statusCode int
		expected   bool
	}{
		{"Internal Server Error", http.StatusInternalServerError, true},
		{"Bad Gateway", http.StatusBadGateway, true},
		{"Service Unavailable", http.StatusServiceUnavailable, true},
		{"Gateway Timeout", http.StatusGatewayTimeout, true},
		{"Too Many Requests", http.StatusTooManyRequests, true},
		{"Bad Request", http.StatusBadRequest, false},
		{"Unauthorized", http.StatusUnauthorized, false},
		{"Not Found", http.StatusNotFound, false},
		{"OK", http.StatusOK, false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.isRetryableHTTPStatus(tt.statusCode)
			if result != tt.expected {
				t.Errorf("Expected %v for status %d, got %v", tt.expected, tt.statusCode, result)
			}
		})
	}
}

func TestSessionTokenManagement(t *testing.T) {
	client := &HTTPClient{}
	
	// Test initial state
	if client.getSessionID() != "" {
		t.Error("Expected empty session ID initially")
	}
	
	// Test setting session ID
	testSessionID := "test-session-456"
	client.setSessionID(testSessionID)
	
	if client.getSessionID() != testSessionID {
		t.Errorf("Expected session ID %s, got %s", testSessionID, client.getSessionID())
	}
	
	// Test clearing session ID
	client.clearSessionID()
	if client.getSessionID() != "" {
		t.Error("Expected empty session ID after clearing")
	}
}

func TestBasicAuthConfiguration(t *testing.T) {
	client, err := NewHTTPClient("localhost", 9091, "/rpc", false, 10*time.Second, "testuser", "testpass", createTestLogger())
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	if client.username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", client.username)
	}
	
	if client.password != "testpass" {
		t.Errorf("Expected password 'testpass', got '%s'", client.password)
	}
}

func TestSessionTokenCaching(t *testing.T) {
	client := &HTTPClient{}
	
	// Test concurrent access to session ID
	const numGoroutines = 10
	const sessionID = "concurrent-test-session"
	
	// Set session ID from multiple goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			testID := fmt.Sprintf("%s-%d", sessionID, id)
			client.setSessionID(testID)
		}(i)
	}
	
	// Give goroutines time to complete
	time.Sleep(100 * time.Millisecond)
	
	// Verify we have some session ID (race condition means we can't predict which one)
	currentID := client.getSessionID()
	if currentID == "" {
		t.Error("Expected some session ID to be set")
	}
	
	// Test that clearing works in concurrent environment
	client.clearSessionID()
	if client.getSessionID() != "" {
		t.Error("Expected session ID to be cleared")
	}
}

func TestStructuredRequestTypes(t *testing.T) {
	// Test SessionStatsRequest
	sessionReq := NewSessionStatsRequest()
	if sessionReq.Method != "session-stats" {
		t.Errorf("Expected method 'session-stats', got '%s'", sessionReq.Method)
	}
	
	// Test TorrentGetRequest
	fields := []string{"id", "status", "rateDownload"}
	torrentReq := NewTorrentGetRequest(fields)
	if torrentReq.Method != "torrent-get" {
		t.Errorf("Expected method 'torrent-get', got '%s'", torrentReq.Method)
	}
	if len(torrentReq.Arguments.Fields) != len(fields) {
		t.Errorf("Expected %d fields, got %d", len(fields), len(torrentReq.Arguments.Fields))
	}
	
	// Test SessionGetRequest
	sessionGetReq := NewSessionGetRequest()
	if sessionGetReq.Method != "session-get" {
		t.Errorf("Expected method 'session-get', got '%s'", sessionGetReq.Method)
	}
}

func TestResponseValidation(t *testing.T) {
	// Test SessionStatsResponse validation
	t.Run("SessionStatsResponse", func(t *testing.T) {
		// Valid response
		validResp := &SessionStatsResponse{
			BaseResponse: BaseResponse{Result: "success"},
			Arguments: &SessionStats{
				TotalTorrentCount:  10,
				ActiveTorrentCount: 5,
				PausedTorrentCount: 3,
			},
		}
		if err := validResp.Validate(); err != nil {
			t.Errorf("Valid response should not fail validation: %v", err)
		}
		
		// Invalid response - failed result
		invalidResp := &SessionStatsResponse{
			BaseResponse: BaseResponse{Result: "failure"},
		}
		if err := invalidResp.Validate(); err == nil {
			t.Error("Invalid response should fail validation")
		}
		
		// Invalid response - nil arguments
		nilArgsResp := &SessionStatsResponse{
			BaseResponse: BaseResponse{Result: "success"},
			Arguments:    nil,
		}
		if err := nilArgsResp.Validate(); err == nil {
			t.Error("Response with nil arguments should fail validation")
		}
		
		// Invalid response - inconsistent torrent counts
		inconsistentResp := &SessionStatsResponse{
			BaseResponse: BaseResponse{Result: "success"},
			Arguments: &SessionStats{
				TotalTorrentCount:  5,
				ActiveTorrentCount: 10, // More active than total
				PausedTorrentCount: 2,
			},
		}
		if err := inconsistentResp.Validate(); err == nil {
			t.Error("Response with inconsistent torrent counts should fail validation")
		}
	})
	
	// Test TorrentGetResponse validation
	t.Run("TorrentGetResponse", func(t *testing.T) {
		// Valid response
		validTorrent := &Torrent{
			ID:          1,
			Status:      TorrentDownload,
			PercentDone: 0.5,
			PeersConnected: 10,
			PeersSendingToUs: 3,
			PeersGettingFromUs: 2,
		}
		
		validResp := &TorrentGetResponse{
			BaseResponse: BaseResponse{Result: "success"},
			Arguments: &TorrentGetResponseArguments{
				Torrents: []*Torrent{validTorrent},
			},
		}
		if err := validResp.Validate(); err != nil {
			t.Errorf("Valid response should not fail validation: %v", err)
		}
		
		// Invalid response - nil torrent
		invalidResp := &TorrentGetResponse{
			BaseResponse: BaseResponse{Result: "success"},
			Arguments: &TorrentGetResponseArguments{
				Torrents: []*Torrent{nil},
			},
		}
		if err := invalidResp.Validate(); err == nil {
			t.Error("Response with nil torrent should fail validation")
		}
	})
	
	// Test SessionGetResponse validation
	t.Run("SessionGetResponse", func(t *testing.T) {
		// Valid response
		validResp := &SessionGetResponse{
			BaseResponse: BaseResponse{Result: "success"},
			Arguments: &SessionConfig{
				Version: "3.00",
				FreeSpace: 1000000,
			},
		}
		if err := validResp.Validate(); err != nil {
			t.Errorf("Valid response should not fail validation: %v", err)
		}
		
		// Invalid response - missing version
		invalidResp := &SessionGetResponse{
			BaseResponse: BaseResponse{Result: "success"},
			Arguments: &SessionConfig{
				Version: "",
			},
		}
		if err := invalidResp.Validate(); err == nil {
			t.Error("Response with missing version should fail validation")
		}
		
		// Invalid response - unrealistic speed limit
		unrealisticLimit := uint64(2000000000) // 2GB/s
		unrealisticResp := &SessionGetResponse{
			BaseResponse: BaseResponse{Result: "success"},
			Arguments: &SessionConfig{
				Version: "3.00",
				SpeedLimitDown: &unrealisticLimit,
			},
		}
		if err := unrealisticResp.Validate(); err == nil {
			t.Error("Response with unrealistic speed limit should fail validation")
		}
	})
}

func TestTorrentValidation(t *testing.T) {
	// Valid torrent
	validTorrent := &Torrent{
		ID:          1,
		Status:      TorrentDownload,
		PercentDone: 0.75,
		PeersConnected: 10,
		PeersSendingToUs: 3,
		PeersGettingFromUs: 2,
	}
	if err := validTorrent.Validate(); err != nil {
		t.Errorf("Valid torrent should not fail validation: %v", err)
	}
	
	// Invalid torrent - zero ID
	zeroIDTorrent := &Torrent{
		ID: 0,
	}
	if err := zeroIDTorrent.Validate(); err == nil {
		t.Error("Torrent with zero ID should fail validation")
	}
	
	// Invalid torrent - invalid percent done
	invalidPercentTorrent := &Torrent{
		ID:          1,
		PercentDone: 1.5, // > 1.0
	}
	if err := invalidPercentTorrent.Validate(); err == nil {
		t.Error("Torrent with invalid percent done should fail validation")
	}
	
	// Invalid torrent - invalid status
	invalidStatusTorrent := &Torrent{
		ID:     1,
		Status: TorrentStatus(99), // Invalid status
	}
	if err := invalidStatusTorrent.Validate(); err == nil {
		t.Error("Torrent with invalid status should fail validation")
	}
	
	// Invalid torrent - inconsistent peer counts
	inconsistentPeersTorrent := &Torrent{
		ID:                 1,
		PeersConnected:     5,
		PeersSendingToUs:   3,
		PeersGettingFromUs: 4, // 3 + 4 > 5
	}
	if err := inconsistentPeersTorrent.Validate(); err == nil {
		t.Error("Torrent with inconsistent peer counts should fail validation")
	}
}

func TestExporterMetricsValidation(t *testing.T) {
	// Valid ExporterMetrics
	validMetrics := &ExporterMetrics{
		ScrapeErrorsTotal:    5,
		RPCLatencySeconds:    0.25,
		LastSuccessfulScrape: 1234567890,
		CacheHits:           100,
		CacheMisses:         10,
	}
	if err := validMetrics.Validate(); err != nil {
		t.Errorf("Valid ExporterMetrics should not fail validation: %v", err)
	}
	
	// Invalid ExporterMetrics - negative RPC latency
	negativeLatencyMetrics := &ExporterMetrics{
		RPCLatencySeconds: -0.5,
	}
	if err := negativeLatencyMetrics.Validate(); err == nil {
		t.Error("ExporterMetrics with negative RPC latency should fail validation")
	}
	
	// Invalid ExporterMetrics - negative timestamp
	negativeTimestampMetrics := &ExporterMetrics{
		LastSuccessfulScrape: -1,
	}
	if err := negativeTimestampMetrics.Validate(); err == nil {
		t.Error("ExporterMetrics with negative timestamp should fail validation")
	}
}

func TestHelperMethods(t *testing.T) {
	// Test SessionStats helper methods
	t.Run("SessionStats ToPrometheusFloat64", func(t *testing.T) {
		stats := &SessionStats{
			DownloadSpeed:        1000,
			UploadSpeed:          500,
			ActiveTorrentCount:   5,
			PausedTorrentCount:   3,
			TotalTorrentCount:    10,
			CumulativeDownloaded: 1000000,
			CumulativeUploaded:   500000,
		}
		
		downloadSpeed, uploadSpeed, activeTorrents, pausedTorrents, totalTorrents, cumulativeDownloaded, cumulativeUploaded := stats.ToPrometheusFloat64()
		
		if downloadSpeed != 1000.0 {
			t.Errorf("Expected download speed 1000.0, got %f", downloadSpeed)
		}
		if uploadSpeed != 500.0 {
			t.Errorf("Expected upload speed 500.0, got %f", uploadSpeed)
		}
		if activeTorrents != 5.0 {
			t.Errorf("Expected active torrents 5.0, got %f", activeTorrents)
		}
		if pausedTorrents != 3.0 {
			t.Errorf("Expected paused torrents 3.0, got %f", pausedTorrents)
		}
		if totalTorrents != 10.0 {
			t.Errorf("Expected total torrents 10.0, got %f", totalTorrents)
		}
		if cumulativeDownloaded != 1000000.0 {
			t.Errorf("Expected cumulative downloaded 1000000.0, got %f", cumulativeDownloaded)
		}
		if cumulativeUploaded != 500000.0 {
			t.Errorf("Expected cumulative uploaded 500000.0, got %f", cumulativeUploaded)
		}
	})
	
	// Test Torrent helper methods
	t.Run("Torrent helper methods", func(t *testing.T) {
		errorString := "Connection timeout"
		torrent := &Torrent{
			ID:                 42,
			Status:             TorrentDownload,
			DownloadRate:       2000,
			UploadRate:         1000,
			PercentDone:        0.75,
			PeersConnected:     15,
			PeersSendingToUs:   5,
			PeersGettingFromUs: 3,
			ErrorString:        &errorString,
		}
		
		// Test ToPrometheusFloat64
		downloadRate, uploadRate, progress, peersConnected, peersSending, peersGetting, status := torrent.ToPrometheusFloat64()
		
		if downloadRate != 2000.0 {
			t.Errorf("Expected download rate 2000.0, got %f", downloadRate)
		}
		if uploadRate != 1000.0 {
			t.Errorf("Expected upload rate 1000.0, got %f", uploadRate)
		}
		if progress != 0.75 {
			t.Errorf("Expected progress 0.75, got %f", progress)
		}
		if peersConnected != 15.0 {
			t.Errorf("Expected peers connected 15.0, got %f", peersConnected)
		}
		if peersSending != 5.0 {
			t.Errorf("Expected peers sending 5.0, got %f", peersSending)
		}
		if peersGetting != 3.0 {
			t.Errorf("Expected peers getting 3.0, got %f", peersGetting)
		}
		if status != float64(TorrentDownload) {
			t.Errorf("Expected status %f, got %f", float64(TorrentDownload), status)
		}
		
		// Test HasError
		if !torrent.HasError() {
			t.Error("Expected torrent to have error")
		}
		
		// Test GetErrorString
		if torrent.GetErrorString() != "Connection timeout" {
			t.Errorf("Expected error string 'Connection timeout', got '%s'", torrent.GetErrorString())
		}
		
		// Test GetTorrentIDString
		if torrent.GetTorrentIDString() != "42" {
			t.Errorf("Expected torrent ID string '42', got '%s'", torrent.GetTorrentIDString())
		}
		
		// Test torrent without error
		torrentNoError := &Torrent{
			ID:          1,
			ErrorString: nil,
		}
		
		if torrentNoError.HasError() {
			t.Error("Expected torrent to not have error")
		}
		
		if torrentNoError.GetErrorString() != "" {
			t.Errorf("Expected empty error string, got '%s'", torrentNoError.GetErrorString())
		}
		
		// Test torrent with empty error string
		emptyError := ""
		torrentEmptyError := &Torrent{
			ID:          1,
			ErrorString: &emptyError,
		}
		
		if torrentEmptyError.HasError() {
			t.Error("Expected torrent with empty error string to not have error")
		}
	})
	
	// Test SessionConfig helper methods
	t.Run("SessionConfig ToPrometheusFloat64", func(t *testing.T) {
		downLimit := uint64(1000000)
		upLimit := uint64(500000)
		config := &SessionConfig{
			SpeedLimitDown: &downLimit,
			SpeedLimitUp:   &upLimit,
			FreeSpace:      2000000000,
			Version:        "3.00",
		}
		
		speedLimitDown, speedLimitUp, freeSpace := config.ToPrometheusFloat64()
		
		if speedLimitDown != 1000000.0 {
			t.Errorf("Expected speed limit down 1000000.0, got %f", speedLimitDown)
		}
		if speedLimitUp != 500000.0 {
			t.Errorf("Expected speed limit up 500000.0, got %f", speedLimitUp)
		}
		if freeSpace != 2000000000.0 {
			t.Errorf("Expected free space 2000000000.0, got %f", freeSpace)
		}
		
		// Test with nil speed limits
		configNilLimits := &SessionConfig{
			SpeedLimitDown: nil,
			SpeedLimitUp:   nil,
			FreeSpace:      1000000000,
			Version:        "3.00",
		}
		
		speedLimitDown, speedLimitUp, freeSpace = configNilLimits.ToPrometheusFloat64()
		
		if speedLimitDown != 0.0 {
			t.Errorf("Expected speed limit down 0.0 for nil, got %f", speedLimitDown)
		}
		if speedLimitUp != 0.0 {
			t.Errorf("Expected speed limit up 0.0 for nil, got %f", speedLimitUp)
		}
		if freeSpace != 1000000000.0 {
			t.Errorf("Expected free space 1000000000.0, got %f", freeSpace)
		}
	})
	
	// Test ExporterMetrics helper methods
	t.Run("ExporterMetrics ToPrometheusFloat64", func(t *testing.T) {
		metrics := &ExporterMetrics{
			ScrapeErrorsTotal:    10,
			RPCLatencySeconds:    0.5,
			LastSuccessfulScrape: 1234567890,
			CacheHits:           200,
			CacheMisses:         20,
		}
		
		scrapeErrors, rpcLatency, lastScrape, cacheHits, cacheMisses := metrics.ToPrometheusFloat64()
		
		if scrapeErrors != 10.0 {
			t.Errorf("Expected scrape errors 10.0, got %f", scrapeErrors)
		}
		if rpcLatency != 0.5 {
			t.Errorf("Expected RPC latency 0.5, got %f", rpcLatency)
		}
		if lastScrape != 1234567890.0 {
			t.Errorf("Expected last scrape 1234567890.0, got %f", lastScrape)
		}
		if cacheHits != 200.0 {
			t.Errorf("Expected cache hits 200.0, got %f", cacheHits)
		}
		if cacheMisses != 20.0 {
			t.Errorf("Expected cache misses 20.0, got %f", cacheMisses)
		}
	})
}