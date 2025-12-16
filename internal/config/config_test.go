package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestConfigDefaults(t *testing.T) {
	config, err := Load("")
	if err != nil {
		t.Fatalf("Expected no error loading default config, got: %v", err)
	}
	
	if config.Transmission.Host != "localhost" {
		t.Errorf("Expected default host 'localhost', got: %s", config.Transmission.Host)
	}
	
	if config.Transmission.Port != 9091 {
		t.Errorf("Expected default port 9091, got: %d", config.Transmission.Port)
	}
	
	if config.Exporter.Port != 9190 {
		t.Errorf("Expected default exporter port 9190, got: %d", config.Exporter.Port)
	}
	
	if config.Exporter.PollInterval != 15*time.Second {
		t.Errorf("Expected default poll interval 15s, got: %v", config.Exporter.PollInterval)
	}
	
	if config.CommonLabels.TransmissionHost != "localhost" {
		t.Errorf("Expected common label transmission_host 'localhost', got: %s", config.CommonLabels.TransmissionHost)
	}
	
	if config.CommonLabels.TransmissionPort != "9091" {
		t.Errorf("Expected common label transmission_port '9091', got: %s", config.CommonLabels.TransmissionPort)
	}
	
	expectedURL := "http://localhost:9091/transmission/rpc"
	if config.GetTransmissionURL() != expectedURL {
		t.Errorf("Expected URL %s, got: %s", expectedURL, config.GetTransmissionURL())
	}
	
	if config.GetExporterAddress() != ":9190" {
		t.Errorf("Expected address ':9190', got: %s", config.GetExporterAddress())
	}
	
	if config.HasBasicAuth() {
		t.Error("Expected no basic auth with default config")
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
		errorContains string
	}{
		{
			name: "valid config",
			config: Config{
				Transmission: TransmissionConfig{
					Host:    "localhost",
					Port:    9091,
					Path:    "/transmission/rpc",
					Timeout: 10 * time.Second,
				},
				Exporter: ExporterConfig{
					Port:         9190,
					Path:         "/metrics",
					PollInterval: 15 * time.Second,
					MaxStaleAge:  5 * time.Minute,
					Instance:     "test",
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				CommonLabels: CommonLabelsConfig{
					TransmissionHost: "localhost",
					ExporterInstance: "test",
					TransmissionPort: "9091",
				},
			},
			expectError: false,
		},
		{
			name: "invalid transmission port",
			config: Config{
				Transmission: TransmissionConfig{
					Host:    "localhost",
					Port:    0, // Invalid port
					Path:    "/transmission/rpc",
					Timeout: 10 * time.Second,
				},
				Exporter: ExporterConfig{
					Port:         9190,
					Path:         "/metrics",
					PollInterval: 15 * time.Second,
					MaxStaleAge:  5 * time.Minute,
					Instance:     "test",
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				CommonLabels: CommonLabelsConfig{
					TransmissionHost: "localhost",
					ExporterInstance: "test",
					TransmissionPort: "0",
				},
			},
			expectError: true,
			errorContains: "transmission.port must be between 1 and 65535",
		},
		{
			name: "invalid poll interval",
			config: Config{
				Transmission: TransmissionConfig{
					Host:    "localhost",
					Port:    9091,
					Path:    "/transmission/rpc",
					Timeout: 10 * time.Second,
				},
				Exporter: ExporterConfig{
					Port:         9190,
					Path:         "/metrics",
					PollInterval: 0, // Invalid interval
					MaxStaleAge:  5 * time.Minute,
					Instance:     "test",
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				CommonLabels: CommonLabelsConfig{
					TransmissionHost: "localhost",
					ExporterInstance: "test",
					TransmissionPort: "9091",
				},
			},
			expectError: true,
			errorContains: "exporter.poll_interval must be between 1s and 60s",
		},
		{
			name: "invalid path format",
			config: Config{
				Transmission: TransmissionConfig{
					Host:    "localhost",
					Port:    9091,
					Path:    "transmission/rpc", // Missing leading slash
					Timeout: 10 * time.Second,
				},
				Exporter: ExporterConfig{
					Port:         9190,
					Path:         "/metrics",
					PollInterval: 15 * time.Second,
					MaxStaleAge:  5 * time.Minute,
					Instance:     "test",
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				CommonLabels: CommonLabelsConfig{
					TransmissionHost: "localhost",
					ExporterInstance: "test",
					TransmissionPort: "9091",
				},
			},
			expectError: true,
			errorContains: "transmission.path must start with '/'",
		},
		{
			name: "username without password",
			config: Config{
				Transmission: TransmissionConfig{
					Host:     "localhost",
					Port:     9091,
					Path:     "/transmission/rpc",
					Username: "user",
					Password: "", // Missing password
					Timeout:  10 * time.Second,
				},
				Exporter: ExporterConfig{
					Port:         9190,
					Path:         "/metrics",
					PollInterval: 15 * time.Second,
					MaxStaleAge:  5 * time.Minute,
					Instance:     "test",
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				CommonLabels: CommonLabelsConfig{
					TransmissionHost: "localhost",
					ExporterInstance: "test",
					TransmissionPort: "9091",
				},
			},
			expectError: true,
			errorContains: "transmission.password must be provided when transmission.username is set",
		},
		{
			name: "port conflict",
			config: Config{
				Transmission: TransmissionConfig{
					Host:    "localhost",
					Port:    9190, // Same as exporter port
					Path:    "/transmission/rpc",
					Timeout: 10 * time.Second,
				},
				Exporter: ExporterConfig{
					Port:         9190, // Same as transmission port
					Path:         "/metrics",
					PollInterval: 15 * time.Second,
					MaxStaleAge:  5 * time.Minute,
					Instance:     "test",
				},
				Logging: LoggingConfig{
					Level:  "info",
					Format: "text",
				},
				CommonLabels: CommonLabelsConfig{
					TransmissionHost: "localhost",
					ExporterInstance: "test",
					TransmissionPort: "9190",
				},
			},
			expectError: true,
			errorContains: "transmission.port and exporter.port cannot be the same",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError && err == nil {
				t.Error("Expected validation error, got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no validation error, got: %v", err)
			}
			if tt.expectError && err != nil && tt.errorContains != "" {
				if !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain '%s', got: %v", tt.errorContains, err)
				}
			}
		})
	}
}

func TestEnvironmentVariables(t *testing.T) {
	// Set environment variables for comprehensive Docker support
	envVars := map[string]string{
		"TRANSMISSION_EXPORTER_TRANSMISSION_HOST":     "test-host",
		"TRANSMISSION_EXPORTER_TRANSMISSION_PORT":     "9092",
		"TRANSMISSION_EXPORTER_TRANSMISSION_USERNAME": "testuser",
		"TRANSMISSION_EXPORTER_TRANSMISSION_PASSWORD": "testpass",
		"TRANSMISSION_EXPORTER_TRANSMISSION_USE_HTTPS": "true",
		"TRANSMISSION_EXPORTER_EXPORTER_PORT":         "8080",
		"TRANSMISSION_EXPORTER_EXPORTER_INSTANCE":     "docker-instance",
		"TRANSMISSION_EXPORTER_LOGGING_LEVEL":         "debug",
		"TRANSMISSION_EXPORTER_LOGGING_FORMAT":        "json",
	}
	
	// Set all environment variables
	for key, value := range envVars {
		os.Setenv(key, value)
	}
	
	// Clean up after test
	defer func() {
		for key := range envVars {
			os.Unsetenv(key)
		}
	}()
	
	config, err := Load("")
	if err != nil {
		t.Fatalf("Expected no error loading config with env vars, got: %v", err)
	}
	
	// Verify transmission settings
	if config.Transmission.Host != "test-host" {
		t.Errorf("Expected host from env var 'test-host', got: %s", config.Transmission.Host)
	}
	
	if config.Transmission.Port != 9092 {
		t.Errorf("Expected port from env var 9092, got: %d", config.Transmission.Port)
	}
	
	if config.Transmission.Username != "testuser" {
		t.Errorf("Expected username from env var 'testuser', got: %s", config.Transmission.Username)
	}
	
	if config.Transmission.Password != "testpass" {
		t.Errorf("Expected password from env var 'testpass', got: %s", config.Transmission.Password)
	}
	
	if !config.Transmission.UseHTTPS {
		t.Error("Expected UseHTTPS to be true from env var")
	}
	
	// Verify exporter settings
	if config.Exporter.Port != 8080 {
		t.Errorf("Expected port from env var 8080, got: %d", config.Exporter.Port)
	}
	
	if config.Exporter.Instance != "docker-instance" {
		t.Errorf("Expected instance from env var 'docker-instance', got: %s", config.Exporter.Instance)
	}
	
	// Verify logging settings
	if config.Logging.Level != "debug" {
		t.Errorf("Expected log level from env var 'debug', got: %s", config.Logging.Level)
	}
	
	if config.Logging.Format != "json" {
		t.Errorf("Expected log format from env var 'json', got: %s", config.Logging.Format)
	}
	
	if config.CommonLabels.TransmissionHost != "test-host" {
		t.Errorf("Expected common label transmission_host 'test-host', got: %s", config.CommonLabels.TransmissionHost)
	}
	
	if config.CommonLabels.ExporterInstance != "docker-instance" {
		t.Errorf("Expected common label exporter_instance 'docker-instance', got: %s", config.CommonLabels.ExporterInstance)
	}
	
	if config.CommonLabels.TransmissionPort != "9092" {
		t.Errorf("Expected common label transmission_port '9092', got: %s", config.CommonLabels.TransmissionPort)
	}
	
	// Verify helper methods work with HTTPS
	expectedURL := "https://test-host:9092/transmission/rpc"
	if config.GetTransmissionURL() != expectedURL {
		t.Errorf("Expected URL %s, got: %s", expectedURL, config.GetTransmissionURL())
	}
	
	if !config.HasBasicAuth() {
		t.Error("Expected basic auth to be enabled")
	}
}

func TestConfigHelperMethods(t *testing.T) {
	config := &Config{
		Transmission: TransmissionConfig{
			Host:     "example.com",
			Port:     9091,
			Path:     "/transmission/rpc",
			Username: "user",
			Password: "pass",
			UseHTTPS: true,
		},
		Exporter: ExporterConfig{
			Port: 9190,
		},
	}
	
	expectedURL := "https://example.com:9091/transmission/rpc"
	if config.GetTransmissionURL() != expectedURL {
		t.Errorf("Expected URL %s, got: %s", expectedURL, config.GetTransmissionURL())
	}
	
	expectedAddr := ":9190"
	if config.GetExporterAddress() != expectedAddr {
		t.Errorf("Expected address %s, got: %s", expectedAddr, config.GetExporterAddress())
	}
	
	if !config.HasBasicAuth() {
		t.Error("Expected basic auth to be enabled")
	}
	
	config.Transmission.Username = ""
	config.Transmission.Password = ""
	if config.HasBasicAuth() {
		t.Error("Expected basic auth to be disabled")
	}
}

func TestCommonLabelsCustomization(t *testing.T) {
	os.Setenv("TRANSMISSION_EXPORTER_COMMON_LABELS_TRANSMISSION_HOST", "custom-host")
	os.Setenv("TRANSMISSION_EXPORTER_COMMON_LABELS_EXPORTER_INSTANCE", "custom-instance")
	os.Setenv("TRANSMISSION_EXPORTER_COMMON_LABELS_TRANSMISSION_PORT", "custom-port")
	
	defer func() {
		os.Unsetenv("TRANSMISSION_EXPORTER_COMMON_LABELS_TRANSMISSION_HOST")
		os.Unsetenv("TRANSMISSION_EXPORTER_COMMON_LABELS_EXPORTER_INSTANCE")
		os.Unsetenv("TRANSMISSION_EXPORTER_COMMON_LABELS_TRANSMISSION_PORT")
	}()
	
	config, err := Load("")
	if err != nil {
		t.Fatalf("Expected no error loading config, got: %v", err)
	}
	
	if config.CommonLabels.TransmissionHost != "custom-host" {
		t.Errorf("Expected custom transmission_host 'custom-host', got: %s", config.CommonLabels.TransmissionHost)
	}
	
	if config.CommonLabels.ExporterInstance != "custom-instance" {
		t.Errorf("Expected custom exporter_instance 'custom-instance', got: %s", config.CommonLabels.ExporterInstance)
	}
	
	if config.CommonLabels.TransmissionPort != "custom-port" {
		t.Errorf("Expected custom transmission_port 'custom-port', got: %s", config.CommonLabels.TransmissionPort)
	}
}