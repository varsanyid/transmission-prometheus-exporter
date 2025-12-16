package poller

import (
	"context"
	"fmt"
	"sync"
	"time"
	
	"github.com/sirupsen/logrus"
	
	"transmission-prometheus-exporter/internal/cache"
	"transmission-prometheus-exporter/internal/metrics"
	"transmission-prometheus-exporter/internal/rpc"
)

// Status represents the current state of the background poller
type Status struct {
	IsRunning           bool
	LastPollTime        time.Time
	LastSuccessfulPoll  time.Time
	ConsecutiveFailures int
	TotalPolls          uint64
	TotalErrors         uint64
}

// Poller defines the interface for continuous metric collection
type Poller interface {
	// Start begins the background polling loop with the specified interval
	Start(ctx context.Context, interval time.Duration) error
	
	// Stop gracefully shuts down the polling loop
	Stop() error
	
	// GetStatus returns the current poller status and statistics
	GetStatus() Status
}

// BackgroundPoller implements the Poller interface with goroutine-based polling
type BackgroundPoller struct {
	rpcClient rpc.Client
	cache     cache.Cache
	
	// Status tracking
	mu                  sync.RWMutex
	isRunning           bool
	lastPollTime        time.Time
	lastSuccessfulPoll  time.Time
	consecutiveFailures int
	totalPolls          uint64
	totalErrors         uint64
	
	// Control channels
	stopCh   chan struct{}
	stoppedCh chan struct{}
	
	// Configuration
	minInterval time.Duration
	maxInterval time.Duration
	
	// Metrics updater for recording latency and errors
	metricUpdater *metrics.MetricUpdater
	
	// Logger for structured logging
	logger *logrus.Logger
}

// NewBackgroundPoller creates a new background poller instance
func NewBackgroundPoller(rpcClient rpc.Client, cache cache.Cache, metricUpdater *metrics.MetricUpdater, logger *logrus.Logger) *BackgroundPoller {
	return &BackgroundPoller{
		rpcClient:     rpcClient,
		cache:         cache,
		stopCh:        make(chan struct{}),
		stoppedCh:     make(chan struct{}),
		minInterval:   1 * time.Second,
		maxInterval:   60 * time.Second,
		metricUpdater: metricUpdater,
		logger:        logger,
	}
}

// Start begins the background polling loop with the specified interval
func (p *BackgroundPoller) Start(ctx context.Context, interval time.Duration) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// Validate interval is within acceptable range (1-60 seconds)
	if interval < p.minInterval || interval > p.maxInterval {
		p.logger.WithFields(logrus.Fields{
			"requested_interval": interval,
			"min_interval":       p.minInterval,
			"max_interval":       p.maxInterval,
		}).Error("Invalid polling interval requested")
		return fmt.Errorf("poll interval must be between %v and %v, got %v", 
			p.minInterval, p.maxInterval, interval)
	}
	
	// Check if already running
	if p.isRunning {
		p.logger.Warn("Attempted to start poller that is already running")
		return fmt.Errorf("poller is already running")
	}
	
	// Reset channels if they were closed from a previous run
	select {
	case <-p.stopCh:
		p.stopCh = make(chan struct{})
	default:
	}
	
	select {
	case <-p.stoppedCh:
		p.stoppedCh = make(chan struct{})
	default:
	}
	
	p.isRunning = true
	
	// Start the polling goroutine
	go p.pollLoop(ctx, interval)
	
	p.logger.WithFields(logrus.Fields{
		"poll_interval": interval,
		"min_interval":  p.minInterval,
		"max_interval":  p.maxInterval,
	}).Info("Background poller started successfully")
	return nil
}

// Stop gracefully shuts down the polling loop
func (p *BackgroundPoller) Stop() error {
	p.mu.Lock()
	if !p.isRunning {
		p.mu.Unlock()
		p.logger.Warn("Attempted to stop poller that is not running")
		return fmt.Errorf("poller is not running")
	}
	p.mu.Unlock()
	
	p.logger.Info("Stopping background poller...")
	
	// Signal the polling loop to stop
	close(p.stopCh)
	
	// Wait for the polling loop to finish with timeout
	select {
	case <-p.stoppedCh:
		p.logger.Info("Background poller stopped gracefully")
		return nil
	case <-time.After(30 * time.Second):
		p.logger.Error("Timeout waiting for poller to stop")
		return fmt.Errorf("timeout waiting for poller to stop")
	}
}

// GetStatus returns the current poller status and statistics
func (p *BackgroundPoller) GetStatus() Status {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	return Status{
		IsRunning:           p.isRunning,
		LastPollTime:        p.lastPollTime,
		LastSuccessfulPoll:  p.lastSuccessfulPoll,
		ConsecutiveFailures: p.consecutiveFailures,
		TotalPolls:          p.totalPolls,
		TotalErrors:         p.totalErrors,
	}
}

// pollLoop runs the continuous polling loop in a separate goroutine
func (p *BackgroundPoller) pollLoop(ctx context.Context, interval time.Duration) {
	defer func() {
		p.mu.Lock()
		p.isRunning = false
		p.mu.Unlock()
		close(p.stoppedCh)
	}()
	
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	// Perform initial poll immediately
	p.performPoll(ctx)
	
	for {
		select {
		case <-ctx.Done():
			p.logger.WithError(ctx.Err()).Info("Polling loop stopped due to context cancellation")
			return
		case <-p.stopCh:
			p.logger.Info("Polling loop stopped due to stop signal")
			return
		case <-ticker.C:
			p.performPoll(ctx)
		}
	}
}

// performPoll executes a single polling cycle
func (p *BackgroundPoller) performPoll(ctx context.Context) {
	startTime := time.Now()
	
	p.mu.Lock()
	p.lastPollTime = startTime
	p.totalPolls++
	pollNumber := p.totalPolls
	p.mu.Unlock()
	
	p.logger.WithFields(logrus.Fields{
		"poll_number": pollNumber,
		"start_time":  startTime,
	}).Debug("Starting polling cycle")
	
	// Create a timeout context for this poll
	pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	
	// Collect metrics from Transmission
	sessionStats, torrents, sessionConfig, err := p.collectMetrics(pollCtx)
	if err != nil {
		p.handlePollError(err)
		return
	}
	
	// Update cache with collected metrics
	p.cache.UpdateMetrics(sessionStats, torrents, sessionConfig)
	
	// Update success statistics and connection state
	p.mu.Lock()
	p.lastSuccessfulPoll = time.Now()
	p.consecutiveFailures = 0
	p.mu.Unlock()
	
	// Update connection state to connected on successful poll
	if p.metricUpdater != nil {
		p.metricUpdater.UpdateConnectionState(true)
	}
	
	duration := time.Since(startTime)
	p.logger.WithFields(logrus.Fields{
		"poll_number":      pollNumber,
		"duration":         duration,
		"torrent_count":    len(torrents),
		"active_torrents":  sessionStats.ActiveTorrentCount,
		"download_speed":   sessionStats.DownloadSpeed,
		"upload_speed":     sessionStats.UploadSpeed,
	}).Info("Successfully completed polling cycle")
}

// collectMetrics gathers all required metrics from Transmission RPC with latency tracking
func (p *BackgroundPoller) collectMetrics(ctx context.Context) (*rpc.SessionStats, []*rpc.Torrent, *rpc.SessionConfig, error) {
	p.logger.Debug("Starting metric collection from Transmission")
	
	// Ensure we have a valid session token with latency tracking
	authStart := time.Now()
	if err := p.rpcClient.Authenticate(ctx); err != nil {
		p.recordRPCLatency("authenticate", authStart)
		p.recordScrapeError("authentication_failed")
		p.logger.WithError(err).Error("Authentication failed during metric collection")
		return nil, nil, nil, fmt.Errorf("authentication failed: %w", err)
	}
	p.recordRPCLatency("authenticate", authStart)
	p.logger.WithField("auth_duration", time.Since(authStart)).Debug("Authentication completed")
	
	// Collect session statistics with latency tracking
	sessionStatsStart := time.Now()
	sessionStats, err := p.rpcClient.GetSessionStats(ctx)
	if err != nil {
		p.recordRPCLatency("session-stats", sessionStatsStart)
		p.recordScrapeError("session_stats_failed")
		p.logger.WithError(err).Error("Failed to get session statistics")
		return nil, nil, nil, fmt.Errorf("failed to get session stats: %w", err)
	}
	p.recordRPCLatency("session-stats", sessionStatsStart)
	p.logger.WithField("stats_duration", time.Since(sessionStatsStart)).Debug("Session statistics collected")
	
	// Collect torrent information with required fields and latency tracking
	torrentFields := []string{
		"id", "status", "rateDownload", "rateUpload", "percentDone",
		"peersConnected", "peersSendingToUs", "peersGettingFromUs", "errorString",
	}
	torrentGetStart := time.Now()
	torrents, err := p.rpcClient.GetTorrents(ctx, torrentFields)
	if err != nil {
		p.recordRPCLatency("torrent-get", torrentGetStart)
		p.recordScrapeError("torrent_get_failed")
		p.logger.WithError(err).Error("Failed to get torrent information")
		return nil, nil, nil, fmt.Errorf("failed to get torrents: %w", err)
	}
	p.recordRPCLatency("torrent-get", torrentGetStart)
	p.logger.WithFields(logrus.Fields{
		"torrents_duration": time.Since(torrentGetStart),
		"torrent_count":     len(torrents),
	}).Debug("Torrent information collected")
	
	// Collect session configuration with latency tracking
	sessionGetStart := time.Now()
	sessionConfig, err := p.rpcClient.GetSessionConfig(ctx)
	if err != nil {
		p.recordRPCLatency("session-get", sessionGetStart)
		p.recordScrapeError("session_config_failed")
		p.logger.WithError(err).Error("Failed to get session configuration")
		return nil, nil, nil, fmt.Errorf("failed to get session config: %w", err)
	}
	p.recordRPCLatency("session-get", sessionGetStart)
	p.logger.WithField("config_duration", time.Since(sessionGetStart)).Debug("Session configuration collected")
	
	p.logger.Debug("All metrics collected successfully from Transmission")
	return sessionStats, torrents, sessionConfig, nil
}

// handlePollError processes polling errors and updates error statistics
func (p *BackgroundPoller) handlePollError(err error) {
	p.mu.Lock()
	p.totalErrors++
	p.consecutiveFailures++
	failures := p.consecutiveFailures
	totalErrors := p.totalErrors
	p.mu.Unlock()
	
	// Update connection state to disconnected
	if p.metricUpdater != nil {
		p.metricUpdater.UpdateConnectionState(false)
		p.metricUpdater.RecordScrapeError("polling_failed")
	}
	
	// Log with appropriate level based on failure count
	logFields := logrus.Fields{
		"consecutive_failures": failures,
		"total_errors":         totalErrors,
		"error":                err.Error(),
	}
	
	if failures == 1 {
		p.logger.WithFields(logFields).Warn("First polling failure - Transmission may be temporarily unavailable")
	} else if failures >= 5 {
		p.logger.WithFields(logFields).Error("Multiple consecutive polling failures - check Transmission connectivity")
	} else {
		p.logger.WithFields(logFields).Warn("Polling error occurred")
	}
}

// recordRPCLatency records the latency of an RPC call
func (p *BackgroundPoller) recordRPCLatency(method string, startTime time.Time) {
	if p.metricUpdater != nil {
		duration := time.Since(startTime).Seconds()
		p.metricUpdater.RecordRPCLatency(method, duration)
	}
}

// recordScrapeError records a scrape error with the specified error type
func (p *BackgroundPoller) recordScrapeError(errorType string) {
	if p.metricUpdater != nil {
		p.metricUpdater.RecordScrapeError(errorType)
	}
}