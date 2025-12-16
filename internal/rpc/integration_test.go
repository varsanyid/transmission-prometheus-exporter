// +build integration

package rpc

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"
)

// Integration tests for real Transmission server communication
// Run with: go test -tags=integration ./internal/rpc
//
// Environment variables for configuration:
// TRANSMISSION_HOST - Transmission server host (default: localhost)
// TRANSMISSION_PORT - Transmission server port (default: 9091)
// TRANSMISSION_PATH - RPC path (default: /transmission/rpc)
// TRANSMISSION_USERNAME - Username for authentication (optional)
// TRANSMISSION_PASSWORD - Password for authentication (optional)
// TRANSMISSION_USE_HTTPS - Use HTTPS (default: false)

func getTestConfig() (host string, port int, path string, useHTTPS bool, username, password string) {
	host = getEnvOrDefault("TRANSMISSION_HOST", "localhost")
	portStr := getEnvOrDefault("TRANSMISSION_PORT", "9091")
	port, _ = strconv.Atoi(portStr)
	path = getEnvOrDefault("TRANSMISSION_PATH", "/transmission/rpc")
	useHTTPS = getEnvOrDefault("TRANSMISSION_USE_HTTPS", "false") == "true"
	username = os.Getenv("TRANSMISSION_USERNAME")
	password = os.Getenv("TRANSMISSION_PASSWORD")
	return
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func TestIntegrationAuthentication(t *testing.T) {
	host, port, path, useHTTPS, username, password := getTestConfig()
	
	client, err := NewHTTPClient(host, port, path, useHTTPS, 10*time.Second, username, password)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Test authentication
	t.Logf("Testing authentication to %s:%d%s", host, port, path)
	if username != "" {
		t.Logf("Using Basic Auth with username: %s", username)
	}
	
	err = client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("Authentication failed: %v", err)
	}
	
	// Verify we have a session ID
	sessionID := client.getSessionID()
	if sessionID == "" {
		t.Error("Expected session ID after authentication")
	} else {
		t.Logf("Successfully authenticated, session ID: %s", sessionID)
	}
}

func TestIntegrationSessionStats(t *testing.T) {
	host, port, path, useHTTPS, username, password := getTestConfig()
	
	client, err := NewHTTPClient(host, port, path, useHTTPS, 10*time.Second, username, password)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Authenticate first
	err = client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("Authentication failed: %v", err)
	}
	
	// Get session stats
	t.Log("Retrieving session statistics...")
	stats, err := client.GetSessionStats(ctx)
	if err != nil {
		t.Fatalf("Failed to get session stats: %v", err)
	}
	
	if stats == nil {
		t.Fatal("Expected session stats, got nil")
	}
	
	t.Logf("Session Stats:")
	t.Logf("  Download Speed: %d bytes/s", stats.DownloadSpeed)
	t.Logf("  Upload Speed: %d bytes/s", stats.UploadSpeed)
	t.Logf("  Active Torrents: %d", stats.ActiveTorrentCount)
	t.Logf("  Paused Torrents: %d", stats.PausedTorrentCount)
	t.Logf("  Total Torrents: %d", stats.TotalTorrentCount)
	t.Logf("  Cumulative Downloaded: %d bytes", stats.CumulativeDownloaded)
	t.Logf("  Cumulative Uploaded: %d bytes", stats.CumulativeUploaded)
}

func TestIntegrationSessionConfig(t *testing.T) {
	host, port, path, useHTTPS, username, password := getTestConfig()
	
	client, err := NewHTTPClient(host, port, path, useHTTPS, 10*time.Second, username, password)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Authenticate first
	err = client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("Authentication failed: %v", err)
	}
	
	// Get session config
	t.Log("Retrieving session configuration...")
	config, err := client.GetSessionConfig(ctx)
	if err != nil {
		t.Fatalf("Failed to get session config: %v", err)
	}
	
	if config == nil {
		t.Fatal("Expected session config, got nil")
	}
	
	t.Logf("Session Config:")
	if config.SpeedLimitDown != nil {
		t.Logf("  Speed Limit Down: %d bytes/s", *config.SpeedLimitDown)
	} else {
		t.Log("  Speed Limit Down: unlimited")
	}
	if config.SpeedLimitUp != nil {
		t.Logf("  Speed Limit Up: %d bytes/s", *config.SpeedLimitUp)
	} else {
		t.Log("  Speed Limit Up: unlimited")
	}
	t.Logf("  Free Space: %d bytes", config.FreeSpace)
	t.Logf("  Version: %s", config.Version)
}

func TestIntegrationTorrents(t *testing.T) {
	host, port, path, useHTTPS, username, password := getTestConfig()
	
	client, err := NewHTTPClient(host, port, path, useHTTPS, 10*time.Second, username, password)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Authenticate first
	err = client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("Authentication failed: %v", err)
	}
	
	// Get torrents
	t.Log("Retrieving torrent information...")
	fields := []string{
		"id", "status", "rateDownload", "rateUpload", "percentDone",
		"peersConnected", "peersSendingToUs", "peersGettingFromUs", "errorString",
	}
	
	torrents, err := client.GetTorrents(ctx, fields)
	if err != nil {
		t.Fatalf("Failed to get torrents: %v", err)
	}
	
	t.Logf("Found %d torrents", len(torrents))
	
	for i, torrent := range torrents {
		if i >= 5 { // Limit output to first 5 torrents
			t.Logf("... and %d more torrents", len(torrents)-5)
			break
		}
		
		t.Logf("Torrent %d:", torrent.ID)
		t.Logf("  Status: %d", torrent.Status)
		t.Logf("  Download Rate: %d bytes/s", torrent.DownloadRate)
		t.Logf("  Upload Rate: %d bytes/s", torrent.UploadRate)
		t.Logf("  Progress: %.2f%%", torrent.PercentDone*100)
		t.Logf("  Peers Connected: %d", torrent.PeersConnected)
		t.Logf("  Peers Sending: %d", torrent.PeersSendingToUs)
		t.Logf("  Peers Getting: %d", torrent.PeersGettingFromUs)
		if torrent.ErrorString != nil && *torrent.ErrorString != "" {
			t.Logf("  Error: %s", *torrent.ErrorString)
		}
	}
}

func TestIntegrationSessionTokenRefresh(t *testing.T) {
	host, port, path, useHTTPS, username, password := getTestConfig()
	
	client, err := NewHTTPClient(host, port, path, useHTTPS, 10*time.Second, username, password)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Authenticate first
	err = client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("Authentication failed: %v", err)
	}
	
	firstSessionID := client.getSessionID()
	t.Logf("First session ID: %s", firstSessionID)
	
	// Clear session ID to simulate invalidation
	client.clearSessionID()
	t.Log("Cleared session ID to simulate invalidation")
	
	// Make a request that should trigger re-authentication
	t.Log("Making request that should trigger re-authentication...")
	stats, err := client.GetSessionStats(ctx)
	if err != nil {
		t.Fatalf("Failed to get session stats after token refresh: %v", err)
	}
	
	if stats == nil {
		t.Fatal("Expected session stats after token refresh")
	}
	
	newSessionID := client.getSessionID()
	t.Logf("New session ID: %s", newSessionID)
	
	if newSessionID == "" {
		t.Error("Expected new session ID after refresh")
	}
	
	if newSessionID == firstSessionID {
		t.Log("Note: Session ID is the same (this can happen if server reuses tokens)")
	} else {
		t.Log("Session ID successfully refreshed")
	}
}

func TestIntegrationFullWorkflow(t *testing.T) {
	host, port, path, useHTTPS, username, password := getTestConfig()
	
	client, err := NewHTTPClient(host, port, path, useHTTPS, 10*time.Second, username, password)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	
	t.Log("=== Full Workflow Test ===")
	
	// Step 1: Authentication
	t.Log("Step 1: Authenticating...")
	err = client.Authenticate(ctx)
	if err != nil {
		t.Fatalf("Authentication failed: %v", err)
	}
	t.Log("✓ Authentication successful")
	
	// Step 2: Get session stats
	t.Log("Step 2: Getting session statistics...")
	stats, err := client.GetSessionStats(ctx)
	if err != nil {
		t.Fatalf("Failed to get session stats: %v", err)
	}
	t.Logf("✓ Session stats retrieved (Active: %d, Total: %d)", stats.ActiveTorrentCount, stats.TotalTorrentCount)
	
	// Step 3: Get session config
	t.Log("Step 3: Getting session configuration...")
	config, err := client.GetSessionConfig(ctx)
	if err != nil {
		t.Fatalf("Failed to get session config: %v", err)
	}
	t.Logf("✓ Session config retrieved (Version: %s)", config.Version)
	
	// Step 4: Get torrents
	t.Log("Step 4: Getting torrent list...")
	fields := []string{"id", "status", "rateDownload", "rateUpload", "percentDone"}
	torrents, err := client.GetTorrents(ctx, fields)
	if err != nil {
		t.Fatalf("Failed to get torrents: %v", err)
	}
	t.Logf("✓ Torrent list retrieved (%d torrents)", len(torrents))
	
	// Step 5: Test session token persistence across multiple calls
	t.Log("Step 5: Testing session token persistence...")
	for i := 0; i < 3; i++ {
		_, err := client.GetSessionStats(ctx)
		if err != nil {
			t.Fatalf("Failed on persistence test iteration %d: %v", i+1, err)
		}
	}
	t.Log("✓ Session token persistence verified")
	
	t.Log("=== Full Workflow Test Completed Successfully ===")
}