package rpc

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Client defines the interface for communicating with Transmission RPC API
type Client interface {
	// Authenticate handles session token authentication flow
	Authenticate(ctx context.Context) error
	
	// GetSessionStats retrieves global Transmission statistics
	GetSessionStats(ctx context.Context) (*SessionStats, error)
	
	// GetTorrents retrieves torrent information with specified fields
	GetTorrents(ctx context.Context, fields []string) ([]*Torrent, error)
	
	// GetSessionConfig retrieves session configuration
	GetSessionConfig(ctx context.Context) (*SessionConfig, error)
}

// HTTPClient represents the HTTP client configuration for Transmission RPC
type HTTPClient struct {
	baseURL       *url.URL
	httpClient    *http.Client
	username      string
	password      string
	sessionID     string
	sessionMu     sync.RWMutex
	maxRetries    int
	baseDelay     time.Duration
	maxDelay      time.Duration
	logger        *logrus.Logger
	metricUpdater MetricRecorder // Interface for recording metrics
}

// MetricRecorder defines the interface for recording operational metrics
type MetricRecorder interface {
	RecordBackoffDelay(operation string, delaySeconds float64)
	RecordRetryAttempt(operation string)
}

// NewHTTPClient creates a new HTTP client for Transmission RPC communication
func NewHTTPClient(host string, port int, path string, useHTTPS bool, timeout time.Duration, username, password string, logger *logrus.Logger) (*HTTPClient, error) {
	return NewHTTPClientWithMetrics(host, port, path, useHTTPS, timeout, username, password, logger, nil)
}

// NewHTTPClientWithMetrics creates a new HTTP client with metric recording capability
func NewHTTPClientWithMetrics(host string, port int, path string, useHTTPS bool, timeout time.Duration, username, password string, logger *logrus.Logger, metricUpdater MetricRecorder) (*HTTPClient, error) {
	scheme := "http"
	if useHTTPS {
		scheme = "https"
	}
	
	baseURL, err := url.Parse(fmt.Sprintf("%s://%s:%d%s", scheme, host, port, path))
	if err != nil {
		return nil, fmt.Errorf("invalid URL parameters: %w", err)
	}
	
	// Create custom transport with timeout settings
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,  // Connection timeout
			KeepAlive: 30 * time.Second, // Keep-alive timeout
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       90 * time.Second,
	}
	
	// Configure TLS if using HTTPS
	if useHTTPS {
		transport.TLSClientConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}
	
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
	
	client := &HTTPClient{
		baseURL:       baseURL,
		httpClient:    httpClient,
		username:      username,
		password:      password,
		maxRetries:    3,
		baseDelay:     time.Second,
		maxDelay:      30 * time.Second,
		logger:        logger,
		metricUpdater: metricUpdater,
	}

	// Log client initialization
	client.logger.WithFields(logrus.Fields{
		"transmission_url": baseURL.String(),
		"use_https":        useHTTPS,
		"timeout":          timeout,
		"has_auth":         username != "",
		"max_retries":      3,
	}).Info("Transmission RPC client initialized")

	return client, nil
}

// makeRequest performs an HTTP request with retry logic and exponential backoff
func (c *HTTPClient) makeRequest(ctx context.Context, method string, body interface{}) (*http.Response, error) {
	var lastErr error
	sessionRefreshAttempts := 0
	maxSessionRefreshAttempts := 2 // Allow up to 2 session refresh attempts
	
	c.logger.WithFields(logrus.Fields{
		"method":      method,
		"url":         c.baseURL.String(),
		"max_retries": c.maxRetries,
	}).Debug("Starting RPC request")
	
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff delay with jitter
			delay := c.calculateBackoffDelay(attempt)
			
			// Record backoff delay metric
			if c.metricUpdater != nil {
				c.metricUpdater.RecordBackoffDelay("rpc_request", delay.Seconds())
			}
			
			c.logger.WithFields(logrus.Fields{
				"attempt":     attempt,
				"delay":       delay,
				"last_error":  lastErr,
			}).Warn("Retrying RPC request after failure")
			
			select {
			case <-ctx.Done():
				c.logger.WithError(ctx.Err()).Error("RPC request cancelled by context")
				return nil, ctx.Err()
			case <-time.After(delay):
				// Continue with retry
			}
		}
		
		resp, err := c.doRequest(ctx, method, body)
		if err != nil {
			lastErr = err
			
			// Record retry attempt metric
			if c.metricUpdater != nil && attempt > 0 {
				c.metricUpdater.RecordRetryAttempt("rpc_request")
			}
			
			// Check if error is retryable
			if !c.isRetryableError(err) {
				return nil, err
			}
			
			continue
		}
		
		// Handle 409 Conflict response which contains the session ID
		if resp.StatusCode == http.StatusConflict {
			sessionID := resp.Header.Get("X-Transmission-Session-Id")
			if sessionID != "" {
				c.logger.WithFields(logrus.Fields{
					"session_id":         sessionID[:8] + "...", // Log partial session ID for security
					"refresh_attempts":   sessionRefreshAttempts,
				}).Info("Received new session ID from Transmission")
				
				c.setSessionID(sessionID)
				resp.Body.Close()
				
				// Limit session refresh attempts to prevent infinite loops
				if sessionRefreshAttempts < maxSessionRefreshAttempts {
					sessionRefreshAttempts++
					// Retry the request with the new session ID (don't count as a regular attempt)
					attempt-- // Decrement to not count this as a retry attempt
					continue
				} else {
					c.logger.WithField("max_attempts", maxSessionRefreshAttempts).Error("Too many session refresh attempts")
					return nil, fmt.Errorf("too many session refresh attempts")
				}
			} else {
				c.logger.Error("Received 409 Conflict but no session ID in response header")
				resp.Body.Close()
				return nil, fmt.Errorf("received 409 Conflict but no session ID in response")
			}
		}
		
		// Check for HTTP errors that might be retryable
		if c.isRetryableHTTPStatus(resp.StatusCode) {
			c.logger.WithFields(logrus.Fields{
				"status_code": resp.StatusCode,
				"status":      resp.Status,
				"attempt":     attempt,
			}).Warn("Received retryable HTTP error")
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
			continue
		}
		
		c.logger.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"attempt":     attempt,
		}).Debug("RPC request completed successfully")
		return resp, nil
	}
	
	c.logger.WithFields(logrus.Fields{
		"attempts":   c.maxRetries + 1,
		"last_error": lastErr,
	}).Error("RPC request failed after all retry attempts")
	return nil, fmt.Errorf("request failed after %d attempts: %w", c.maxRetries+1, lastErr)
}

// doRequest performs a single HTTP request
func (c *HTTPClient) doRequest(ctx context.Context, method string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}
	
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL.String(), reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "transmission-prometheus-exporter/1.0")
	
	// Add session ID if available
	c.sessionMu.RLock()
	if c.sessionID != "" {
		req.Header.Set("X-Transmission-Session-Id", c.sessionID)
	}
	c.sessionMu.RUnlock()
	
	// Add Basic Auth if configured
	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	
	return c.httpClient.Do(req)
}

// calculateBackoffDelay calculates the delay for exponential backoff with jitter
func (c *HTTPClient) calculateBackoffDelay(attempt int) time.Duration {
	// Exponential backoff: baseDelay * 2^(attempt-1)
	delay := time.Duration(float64(c.baseDelay) * math.Pow(2, float64(attempt-1)))
	
	// Cap at maximum delay
	if delay > c.maxDelay {
		delay = c.maxDelay
	}
	
	// Add jitter (±25% of delay)
	jitter := time.Duration(float64(delay) * 0.25 * (rand.Float64()*2 - 1))
	delay += jitter
	
	// Ensure delay is not negative
	if delay < 0 {
		delay = c.baseDelay
	}
	
	return delay
}

// isRetryableError determines if an error should trigger a retry
func (c *HTTPClient) isRetryableError(err error) bool {
	// Network errors are generally retryable
	if netErr, ok := err.(net.Error); ok {
		return netErr.Timeout() || netErr.Temporary()
	}
	
	// DNS errors are retryable
	if dnsErr, ok := err.(*net.DNSError); ok {
		return dnsErr.Temporary()
	}
	
	// Connection refused and similar errors are retryable
	errStr := strings.ToLower(err.Error())
	retryableErrors := []string{
		"connection refused",
		"connection reset",
		"connection timeout",
		"no such host",
		"network is unreachable",
		"i/o timeout",
	}
	
	for _, retryableErr := range retryableErrors {
		if strings.Contains(errStr, retryableErr) {
			return true
		}
	}
	
	return false
}

// isRetryableHTTPStatus determines if an HTTP status code should trigger a retry
func (c *HTTPClient) isRetryableHTTPStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusInternalServerError,     // 500
		http.StatusBadGateway,               // 502
		http.StatusServiceUnavailable,       // 503
		http.StatusGatewayTimeout,           // 504
		http.StatusTooManyRequests:          // 429
		return true
	default:
		return false
	}
}

// setSessionID updates the session ID in a thread-safe manner
func (c *HTTPClient) setSessionID(sessionID string) {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	c.sessionID = sessionID
}

// getSessionID retrieves the current session ID in a thread-safe manner
func (c *HTTPClient) getSessionID() string {
	c.sessionMu.RLock()
	defer c.sessionMu.RUnlock()
	return c.sessionID
}

// clearSessionID clears the current session ID in a thread-safe manner
func (c *HTTPClient) clearSessionID() {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	c.sessionID = ""
}

// refreshSessionToken attempts to refresh the session token by making a fresh authentication request
func (c *HTTPClient) refreshSessionToken(ctx context.Context) error {
	// Clear the current session ID to force a fresh authentication
	c.clearSessionID()
	
	// Perform authentication to get a new session token
	return c.Authenticate(ctx)
}

// Authenticate handles the Transmission session token authentication flow
// This method ensures we have a valid session token before making other RPC calls
func (c *HTTPClient) Authenticate(ctx context.Context) error {
	c.logger.WithFields(logrus.Fields{
		"has_basic_auth": c.username != "",
		"url":            c.baseURL.String(),
	}).Debug("Starting authentication with Transmission")
	
	// Clear any existing session ID to force a fresh authentication
	c.setSessionID("")
	
	// Create a simple request to trigger session ID retrieval
	// Using session-get as it's a lightweight method that requires authentication
	req := NewSessionGetRequest()
	
	resp, err := c.makeRequest(ctx, "POST", req)
	if err != nil {
		c.logger.WithError(err).Error("Authentication request failed")
		return fmt.Errorf("authentication request failed: %w", err)
	}
	defer resp.Body.Close()
	
	// makeRequest handles 409 Conflict responses automatically and sets session ID
	if resp.StatusCode != http.StatusOK {
		c.logger.WithField("status_code", resp.StatusCode).Error("Authentication failed with non-200 status")
		return fmt.Errorf("authentication failed with status: %d", resp.StatusCode)
	}
	
	// Verify we now have a session ID
	sessionID := c.getSessionID()
	if sessionID == "" {
		c.logger.Error("Authentication completed but no session ID was obtained")
		return fmt.Errorf("authentication completed but no session ID was obtained")
	}
	
	c.logger.WithField("session_id", sessionID[:8]+"...").Info("Successfully authenticated with Transmission")
	return nil
}

// GetSessionStats retrieves global Transmission statistics
func (c *HTTPClient) GetSessionStats(ctx context.Context) (*SessionStats, error) {
	c.logger.Debug("Requesting session statistics from Transmission")
	
	req := NewSessionStatsRequest()
	
	resp, err := c.makeRequest(ctx, "POST", req)
	if err != nil {
		c.logger.WithError(err).Error("Session stats request failed")
		return nil, fmt.Errorf("session-stats request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		c.logger.WithField("status_code", resp.StatusCode).Error("Session stats request returned non-200 status")
		return nil, fmt.Errorf("session-stats failed with status: %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.WithError(err).Error("Failed to read session stats response body")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	// Validate JSON structure before unmarshaling
	if !json.Valid(body) {
		c.logger.WithField("body_length", len(body)).Error("Received invalid JSON in session stats response")
		return nil, fmt.Errorf("invalid JSON in response body")
	}
	
	var rpcResp SessionStatsResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal session stats response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	
	// Validate the response structure and data
	if err := rpcResp.Validate(); err != nil {
		c.logger.WithError(err).Error("Session stats response validation failed")
		return nil, fmt.Errorf("response validation failed: %w", err)
	}
	
	c.logger.WithFields(logrus.Fields{
		"download_speed":     rpcResp.Arguments.DownloadSpeed,
		"upload_speed":       rpcResp.Arguments.UploadSpeed,
		"active_torrents":    rpcResp.Arguments.ActiveTorrentCount,
		"total_torrents":     rpcResp.Arguments.TotalTorrentCount,
	}).Debug("Successfully retrieved session statistics")
	
	return rpcResp.Arguments, nil
}

// GetTorrents retrieves torrent information with specified fields
func (c *HTTPClient) GetTorrents(ctx context.Context, fields []string) ([]*Torrent, error) {
	// Validate input fields
	if len(fields) == 0 {
		c.logger.Error("GetTorrents called with empty fields list")
		return nil, fmt.Errorf("fields list cannot be empty")
	}
	
	c.logger.WithFields(logrus.Fields{
		"fields":      fields,
		"field_count": len(fields),
	}).Debug("Requesting torrent information from Transmission")
	
	req := NewTorrentGetRequest(fields)
	
	resp, err := c.makeRequest(ctx, "POST", req)
	if err != nil {
		c.logger.WithError(err).Error("Torrent get request failed")
		return nil, fmt.Errorf("torrent-get request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		c.logger.WithField("status_code", resp.StatusCode).Error("Torrent get request returned non-200 status")
		return nil, fmt.Errorf("torrent-get failed with status: %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.WithError(err).Error("Failed to read torrent get response body")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	// Validate JSON structure before unmarshaling
	if !json.Valid(body) {
		c.logger.WithField("body_length", len(body)).Error("Received invalid JSON in torrent get response")
		return nil, fmt.Errorf("invalid JSON in response body")
	}
	
	var rpcResp TorrentGetResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal torrent get response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	
	// Validate the response structure and data
	if err := rpcResp.Validate(); err != nil {
		c.logger.WithError(err).Error("Torrent get response validation failed")
		return nil, fmt.Errorf("response validation failed: %w", err)
	}
	
	c.logger.WithField("torrent_count", len(rpcResp.Arguments.Torrents)).Debug("Successfully retrieved torrent information")
	
	return rpcResp.Arguments.Torrents, nil
}

// GetSessionConfig retrieves session configuration
func (c *HTTPClient) GetSessionConfig(ctx context.Context) (*SessionConfig, error) {
	c.logger.Debug("Requesting session configuration from Transmission")
	
	req := NewSessionGetRequest()
	
	resp, err := c.makeRequest(ctx, "POST", req)
	if err != nil {
		c.logger.WithError(err).Error("Session config request failed")
		return nil, fmt.Errorf("session-get request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		c.logger.WithField("status_code", resp.StatusCode).Error("Session config request returned non-200 status")
		return nil, fmt.Errorf("session-get failed with status: %d", resp.StatusCode)
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.WithError(err).Error("Failed to read session config response body")
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	// Validate JSON structure before unmarshaling
	if !json.Valid(body) {
		c.logger.WithField("body_length", len(body)).Error("Received invalid JSON in session config response")
		return nil, fmt.Errorf("invalid JSON in response body")
	}
	
	var rpcResp SessionGetResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal session config response")
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	
	// Validate the response structure and data
	if err := rpcResp.Validate(); err != nil {
		c.logger.WithError(err).Error("Session config response validation failed")
		return nil, fmt.Errorf("response validation failed: %w", err)
	}
	
	c.logger.WithFields(logrus.Fields{
		"version":           rpcResp.Arguments.Version,
		"speed_limit_down":  rpcResp.Arguments.SpeedLimitDown,
		"speed_limit_up":    rpcResp.Arguments.SpeedLimitUp,
		"free_space":        rpcResp.Arguments.FreeSpace,
	}).Debug("Successfully retrieved session configuration")
	
	return rpcResp.Arguments, nil
}