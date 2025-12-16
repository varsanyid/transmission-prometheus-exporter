package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	
	"transmission-prometheus-exporter/internal/cache"
	"transmission-prometheus-exporter/internal/poller"
)

// Server represents the HTTP metrics server
type Server struct {
	httpServer *http.Server
	logger     *logrus.Logger
	port       int
	path       string
	cache      cache.Cache
	poller     poller.Poller
}

// Config holds the server configuration
type Config struct {
	Port         int           // HTTP server port (Requirements 7.3)
	Path         string        // Metrics endpoint path (Requirements 7.3)
	ReadTimeout  time.Duration // HTTP read timeout for graceful shutdown
	WriteTimeout time.Duration // HTTP write timeout for graceful shutdown
	IdleTimeout  time.Duration // HTTP idle timeout for graceful shutdown
}

// New creates a new HTTP metrics server
func New(config Config, logger *logrus.Logger, cache cache.Cache, poller poller.Poller) *Server {
	// Set default timeouts if not provided
	if config.ReadTimeout == 0 {
		config.ReadTimeout = 30 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 30 * time.Second
	}
	if config.IdleTimeout == 0 {
		config.IdleTimeout = 60 * time.Second
	}

	// Create server instance first to access it in handlers
	server := &Server{
		logger: logger,
		port:   config.Port,
		path:   config.Path,
		cache:  cache,
		poller: poller,
	}
	
	// Create HTTP server mux
	mux := http.NewServeMux()
	

	mux.Handle(config.Path, promhttp.Handler())
	

	mux.HandleFunc("/health", server.healthCheckHandler)
	

	server.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", config.Port),
		Handler:      mux,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
		IdleTimeout:  config.IdleTimeout,
	}

	return server
}


func (s *Server) Start() error {
	s.logger.WithFields(logrus.Fields{
		"port": s.port,
		"path": s.path,
	}).Info("Starting HTTP metrics server")

	// Start server in goroutine to avoid blocking
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.WithError(err).Error("HTTP server failed")
		}
	}()

	return nil
}


func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Shutting down HTTP metrics server")
	
	// Graceful shutdown with context timeout
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.WithError(err).Error("Failed to gracefully shutdown HTTP server")
		return err
	}
	
	s.logger.Info("HTTP metrics server stopped")
	return nil
}

// GetAddr returns the server address
func (s *Server) GetAddr() string {
	return s.httpServer.Addr
}

// HealthStatus represents the health check response structure
type HealthStatus struct {
	Status              string    `json:"status"`
	Timestamp           time.Time `json:"timestamp"`
	CacheLastUpdate     time.Time `json:"cache_last_update"`
	CacheStale          bool      `json:"cache_stale"`
	CacheAgeSeconds     float64   `json:"cache_age_seconds"`
	PollerRunning       bool      `json:"poller_running"`
	LastSuccessfulPoll  time.Time `json:"last_successful_poll"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	TotalPolls          uint64    `json:"total_polls"`
	TotalErrors         uint64    `json:"total_errors"`
}


func (s *Server) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	
	// Get cache status
	lastUpdate := s.cache.GetLastUpdateTime()
	cacheAge := now.Sub(lastUpdate)
	maxStaleAge := 5 * time.Minute // Consider cache stale after 5 minutes
	isStale := s.cache.IsStale(maxStaleAge)
	
	// Get poller status
	pollerStatus := s.poller.GetStatus()
	
	// Determine overall health status
	status := "healthy"
	httpStatus := http.StatusOK
	
	// Check for unhealthy conditions
	if !pollerStatus.IsRunning {
		status = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
		s.logger.Warn("Health check failed: poller not running")
	} else if isStale {
		status = "degraded"
		httpStatus = http.StatusOK // Still return 200 for degraded state
		s.logger.WithFields(logrus.Fields{
			"cache_age_seconds": cacheAge.Seconds(),
			"max_stale_age":     maxStaleAge.Seconds(),
		}).Warn("Health check degraded: cache is stale")
	} else if pollerStatus.ConsecutiveFailures >= 5 {
		status = "degraded"
		httpStatus = http.StatusOK // Still return 200 for degraded state
		s.logger.WithField("consecutive_failures", pollerStatus.ConsecutiveFailures).
			Warn("Health check degraded: multiple consecutive polling failures")
	}
	
	// Build health status response
	healthStatus := HealthStatus{
		Status:              status,
		Timestamp:           now,
		CacheLastUpdate:     lastUpdate,
		CacheStale:          isStale,
		CacheAgeSeconds:     cacheAge.Seconds(),
		PollerRunning:       pollerStatus.IsRunning,
		LastSuccessfulPoll:  pollerStatus.LastSuccessfulPoll,
		ConsecutiveFailures: pollerStatus.ConsecutiveFailures,
		TotalPolls:          pollerStatus.TotalPolls,
		TotalErrors:         pollerStatus.TotalErrors,
	}
	
	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	
	// Encode and send JSON response
	if err := json.NewEncoder(w).Encode(healthStatus); err != nil {
		s.logger.WithError(err).Error("Failed to encode health check response")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	
	// Log health check result
	s.logger.WithFields(logrus.Fields{
		"status":               status,
		"cache_age_seconds":    cacheAge.Seconds(),
		"poller_running":       pollerStatus.IsRunning,
		"consecutive_failures": pollerStatus.ConsecutiveFailures,
	}).Debug("Health check completed")
}