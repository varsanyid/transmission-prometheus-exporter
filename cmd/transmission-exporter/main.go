package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"transmission-prometheus-exporter/internal/cache"
	"transmission-prometheus-exporter/internal/config"
	"transmission-prometheus-exporter/internal/metrics"
	"transmission-prometheus-exporter/internal/poller"
	"transmission-prometheus-exporter/internal/rpc"
	"transmission-prometheus-exporter/internal/server"
)

// Build information - set at compile time via ldflags
var (
	Version   = "dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
	GoVersion = runtime.Version()
)

func main() {
	// Parse command line flags
	var (
		configPath = flag.String("config", "", "Path to configuration file (optional)")
		version    = flag.Bool("version", false, "Show version information and exit")
		help       = flag.Bool("help", false, "Show help information and exit")
	)
	flag.Parse()

	// Show version information if requested
	if *version {
		fmt.Printf("Transmission Prometheus Exporter\n")
		fmt.Printf("Version: %s\n", Version)
		fmt.Printf("Git Commit: %s\n", GitCommit)
		fmt.Printf("Build Date: %s\n", BuildDate)
		fmt.Printf("Go Version: %s\n", GoVersion)
		os.Exit(0)
	}

	// Show help if requested
	if *help {
		fmt.Printf("Transmission Prometheus Exporter\n\n")
		fmt.Printf("Usage: %s [options]\n\n", os.Args[0])
		fmt.Printf("Options:\n")
		flag.PrintDefaults()
		fmt.Printf("\nEnvironment Variables:\n")
		fmt.Printf("  TRANSMISSION_EXPORTER_TRANSMISSION_HOST     Transmission server hostname\n")
		fmt.Printf("  TRANSMISSION_EXPORTER_TRANSMISSION_PORT     Transmission server port\n")
		fmt.Printf("  TRANSMISSION_EXPORTER_TRANSMISSION_USERNAME Transmission username\n")
		fmt.Printf("  TRANSMISSION_EXPORTER_TRANSMISSION_PASSWORD Transmission password\n")
		fmt.Printf("  TRANSMISSION_EXPORTER_EXPORTER_PORT         Exporter HTTP server port\n")
		fmt.Printf("  TRANSMISSION_EXPORTER_EXPORTER_POLL_INTERVAL Polling interval (e.g., 15s)\n")
		fmt.Printf("  TRANSMISSION_EXPORTER_LOGGING_LEVEL         Log level (debug, info, warn, error)\n")
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	logger := setupLogging(cfg.Logging)

	logger.WithFields(logrus.Fields{
		"version":    Version,
		"git_commit": GitCommit,
		"build_date": BuildDate,
		"go_version": GoVersion,
	}).Info("Starting Transmission Prometheus Exporter")
	configFields := logrus.Fields{
		"config_file":           *configPath,
		"transmission_host":     cfg.Transmission.Host,
		"transmission_port":     cfg.Transmission.Port,
		"transmission_path":     cfg.Transmission.Path,
		"transmission_use_https": cfg.Transmission.UseHTTPS,
		"transmission_timeout":  cfg.Transmission.Timeout,
		"has_basic_auth":        cfg.HasBasicAuth(),
		"exporter_port":         cfg.Exporter.Port,
		"exporter_path":         cfg.Exporter.Path,
		"poll_interval":         cfg.Exporter.PollInterval,
		"max_stale_age":         cfg.Exporter.MaxStaleAge,
		"exporter_instance":     cfg.Exporter.Instance,
		"logging_level":         cfg.Logging.Level,
		"logging_format":        cfg.Logging.Format,
		"common_labels": map[string]string{
			"transmission_host":  cfg.CommonLabels.TransmissionHost,
			"exporter_instance":  cfg.CommonLabels.ExporterInstance,
			"transmission_port":  cfg.CommonLabels.TransmissionPort,
		},
	}
	
	if *configPath != "" {
		logger.WithFields(configFields).Info("Configuration loaded from file")
	} else {
		logger.WithFields(configFields).Info("Configuration loaded from environment variables and defaults")
	}

	if err := cfg.Validate(); err != nil {
		logger.WithFields(logrus.Fields{
			"validation_error": err.Error(),
			"config_source":    *configPath,
		}).Fatal("Configuration validation failed")
	}
	
	logger.Info("Configuration validation completed successfully")

	// Wire together all components with proper dependency injection
	app, err := wireComponents(cfg, logger)
	if err != nil {
		logger.WithError(err).Fatal("Failed to initialize application components")
	}

	// Set up graceful shutdown handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the application
	if err := app.Start(ctx); err != nil {
		logger.WithError(err).Fatal("Failed to start application")
	}

	logger.Info("Application started successfully")

	// Wait for shutdown signal
	sig := <-sigChan
	logger.WithField("signal", sig.String()).Info("Received shutdown signal")

	// Graceful shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := app.Stop(shutdownCtx); err != nil {
		logger.WithError(err).Error("Error during graceful shutdown")
		os.Exit(1)
	}

	logger.Info("Application stopped gracefully")
}

// Application represents the main application with all components
type Application struct {
	logger        *logrus.Logger
	server        *server.Server
	poller        poller.Poller
	cache         cache.Cache
	config        *config.Config
	metricUpdater *metrics.MetricUpdater
	startTime     time.Time
	uptimeCancel  context.CancelFunc
}

// Start starts all application components
func (app *Application) Start(ctx context.Context) error {
	app.logger.Info("Starting application components...")
	
	// Start the background poller
	app.logger.WithField("poll_interval", app.config.Exporter.PollInterval).Info("Starting background poller")
	if err := app.poller.Start(ctx, app.config.Exporter.PollInterval); err != nil {
		app.logger.WithError(err).Error("Failed to start background poller")
		return fmt.Errorf("failed to start background poller: %w", err)
	}

	// Start the HTTP metrics server
	app.logger.WithFields(logrus.Fields{
		"port": app.config.Exporter.Port,
		"path": app.config.Exporter.Path,
	}).Info("Starting HTTP metrics server")
	if err := app.server.Start(); err != nil {
		app.logger.WithError(err).Error("Failed to start HTTP server")
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// Start uptime tracking goroutine
	app.startUptimeTracking(ctx)

	app.logger.Info("All application components started successfully")
	return nil
}

// Stop gracefully stops all application components
func (app *Application) Stop(ctx context.Context) error {
	app.logger.Info("Stopping application components...")

	// Stop uptime tracking
	if app.uptimeCancel != nil {
		app.uptimeCancel()
		app.logger.Info("Uptime tracking stopped")
	}

	// Stop the background poller first
	app.logger.Info("Stopping background poller...")
	if err := app.poller.Stop(); err != nil {
		app.logger.WithError(err).Error("Error stopping background poller")
	} else {
		app.logger.Info("Background poller stopped successfully")
	}

	// Stop the HTTP server
	app.logger.Info("Stopping HTTP server...")
	if err := app.server.Stop(ctx); err != nil {
		app.logger.WithError(err).Error("Error stopping HTTP server")
		return err
	}
	app.logger.Info("HTTP server stopped successfully")

	app.logger.Info("All application components stopped successfully")
	return nil
}

// startUptimeTracking starts a background goroutine to update uptime metrics
func (app *Application) startUptimeTracking(ctx context.Context) {
	uptimeCtx, cancel := context.WithCancel(ctx)
	app.uptimeCancel = cancel
	
	go func() {
		ticker := time.NewTicker(30 * time.Second) // Update uptime every 30 seconds
		defer ticker.Stop()
		
		for {
			select {
			case <-uptimeCtx.Done():
				return
			case <-ticker.C:
				uptime := time.Since(app.startTime).Seconds()
				app.metricUpdater.UpdateUptime(uptime)
			}
		}
	}()
	
	app.logger.Info("Uptime tracking started")
}

// wireComponents creates and wires together all application components with dependency injection
func wireComponents(cfg *config.Config, logger *logrus.Logger) (*Application, error) {
	// Create metric updater for consistent labeling
	metricUpdater := metrics.NewMetricUpdater(
		cfg.CommonLabels.TransmissionHost,
		cfg.CommonLabels.ExporterInstance,
		cfg.Transmission.Port,
	)

	// Create RPC client for Transmission communication with metrics
	rpcClient, err := rpc.NewHTTPClientWithMetrics(
		cfg.Transmission.Host,
		cfg.Transmission.Port,
		cfg.Transmission.Path,
		cfg.Transmission.UseHTTPS,
		cfg.Transmission.Timeout,
		cfg.Transmission.Username,
		cfg.Transmission.Password,
		logger,
		metricUpdater,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create RPC client: %w", err)
	}

	// Create metric cache with common labels
	metricCache := cache.NewMetricCache(
		cfg.CommonLabels.TransmissionHost,
		cfg.CommonLabels.ExporterInstance,
		cfg.Transmission.Port,
		logger,
	)

	metricUpdater.SetBuildInfo(Version, GoVersion, GitCommit, BuildDate)
	metricUpdater.SetConfigInfo(
		cfg.Exporter.PollInterval.String(),
		cfg.Exporter.MaxStaleAge.String(),
		cfg.Transmission.Timeout.String(),
	)
	
	startTime := time.Now()
	metricUpdater.SetStartTime(float64(startTime.Unix()))

	// Create background poller
	backgroundPoller := poller.NewBackgroundPoller(rpcClient, metricCache, metricUpdater, logger)

	// Create HTTP server configuration
	serverConfig := server.Config{
		Port:         cfg.Exporter.Port,
		Path:         cfg.Exporter.Path,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Create HTTP metrics server
	httpServer := server.New(serverConfig, logger, metricCache, backgroundPoller)

	// Log successful component initialization
	logger.WithFields(logrus.Fields{
		"transmission_url": cfg.GetTransmissionURL(),
		"exporter_address": cfg.GetExporterAddress(),
		"has_basic_auth":   cfg.HasBasicAuth(),
	}).Info("All components initialized successfully")

	return &Application{
		logger:        logger,
		server:        httpServer,
		poller:        backgroundPoller,
		cache:         metricCache,
		config:        cfg,
		metricUpdater: metricUpdater,
		startTime:     startTime,
	}, nil
}

// setupLogging configures structured logging with the specified level and format
func setupLogging(cfg config.LoggingConfig) *logrus.Logger {
	logger := logrus.New()

	// Set log level with detailed error information
	level, err := logrus.ParseLevel(cfg.Level)
	if err != nil {
		// Default to info level if parsing fails
		level = logrus.InfoLevel
		logger.WithFields(logrus.Fields{
			"requested_level": cfg.Level,
			"default_level":   "info",
			"valid_levels":    []string{"debug", "info", "warn", "error", "panic", "fatal"},
		}).WithError(err).Warn("Invalid log level specified, defaulting to info")
	}
	logger.SetLevel(level)

	// Set log format with detailed configuration logging
	switch cfg.Format {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
	case "text":
		logger.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: time.RFC3339,
		})
	default:
		// Default to JSON for containerized deployment
		logger.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: time.RFC3339,
		})
		logger.WithFields(logrus.Fields{
			"requested_format": cfg.Format,
			"default_format":   "json",
			"valid_formats":    []string{"json", "text"},
		}).Warn("Invalid log format specified, defaulting to json")
	}

	// Set output to stdout for containerized deployment
	logger.SetOutput(os.Stdout)

	// Log the final logging configuration
	logger.WithFields(logrus.Fields{
		"level":  level.String(),
		"format": cfg.Format,
		"output": "stdout",
	}).Info("Structured logging configured successfully")

	return logger
}