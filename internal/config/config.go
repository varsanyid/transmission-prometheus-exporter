package config

import (
	"fmt"
	"os"
	"strings"
	"time"
	
	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	// Transmission connection settings
	Transmission TransmissionConfig `mapstructure:"transmission"`
	
	// Exporter settings
	Exporter ExporterConfig `mapstructure:"exporter"`
	
	// Logging settings
	Logging LoggingConfig `mapstructure:"logging"`
	
	// Common labels applied to all metrics
	CommonLabels CommonLabelsConfig `mapstructure:"common_labels"`
}

// TransmissionConfig holds Transmission RPC connection parameters
type TransmissionConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Path     string `mapstructure:"path"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	UseHTTPS bool   `mapstructure:"use_https"`
	Timeout  time.Duration `mapstructure:"timeout"`
}

// ExporterConfig holds exporter-specific settings
type ExporterConfig struct {
	Port         int           `mapstructure:"port"`
	Path         string        `mapstructure:"path"`
	PollInterval time.Duration `mapstructure:"poll_interval"`
	MaxStaleAge  time.Duration `mapstructure:"max_stale_age"`
	Instance     string        `mapstructure:"instance"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// CommonLabelsConfig holds configuration for common labels applied to all metrics
type CommonLabelsConfig struct {
	TransmissionHost string `mapstructure:"transmission_host"`
	ExporterInstance string `mapstructure:"exporter_instance"`
	TransmissionPort string `mapstructure:"transmission_port"`
}

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	// Set default values
	setDefaults()
	
	// Set config file path if provided
	if configPath != "" {
		viper.SetConfigFile(configPath)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/transmission-exporter/")
		viper.AddConfigPath("$HOME/.transmission-exporter/")
	}
	
	// Enable environment variable support with automatic key mapping
	viper.AutomaticEnv()
	viper.SetEnvPrefix("TRANSMISSION_EXPORTER")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	
	// Bind all environment variables for comprehensive Docker support
	bindAllEnvironmentVariables()
	
	// Read configuration file (optional)
	if err := viper.ReadInConfig(); err != nil {
		// Config file is optional, only return error for parsing issues
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}
	
	// Unmarshal into config struct
	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// Post-process configuration to set computed values
	config.postProcess()
	
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	return &config, nil
}

// bindAllEnvironmentVariables binds all configuration keys to environment variables
func bindAllEnvironmentVariables() {
	// Transmission configuration
	viper.BindEnv("transmission.host")
	viper.BindEnv("transmission.port")
	viper.BindEnv("transmission.path")
	viper.BindEnv("transmission.username")
	viper.BindEnv("transmission.password")
	viper.BindEnv("transmission.use_https")
	viper.BindEnv("transmission.timeout")
	
	// Exporter configuration
	viper.BindEnv("exporter.port")
	viper.BindEnv("exporter.path")
	viper.BindEnv("exporter.poll_interval")
	viper.BindEnv("exporter.max_stale_age")
	viper.BindEnv("exporter.instance")
	
	// Logging configuration
	viper.BindEnv("logging.level")
	viper.BindEnv("logging.format")
	
	// Common labels configuration
	viper.BindEnv("common_labels.transmission_host")
	viper.BindEnv("common_labels.exporter_instance")
	viper.BindEnv("common_labels.transmission_port")
}

// setDefaults sets default configuration values suitable for containerized deployment
func setDefaults() {
	// Transmission defaults - suitable for Docker deployment
	viper.SetDefault("transmission.host", "localhost")
	viper.SetDefault("transmission.port", 9091)
	viper.SetDefault("transmission.path", "/transmission/rpc")
	viper.SetDefault("transmission.username", "")
	viper.SetDefault("transmission.password", "")
	viper.SetDefault("transmission.use_https", false)
	viper.SetDefault("transmission.timeout", "10s")
	
	// Exporter defaults - suitable for containerized deployment
	viper.SetDefault("exporter.port", 9190)
	viper.SetDefault("exporter.path", "/metrics")
	viper.SetDefault("exporter.poll_interval", "15s")
	viper.SetDefault("exporter.max_stale_age", "5m")
	viper.SetDefault("exporter.instance", getDefaultInstanceName())
	
	// Logging defaults - structured logging for containers
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")
	
	// Common labels defaults - will be computed in postProcess
	viper.SetDefault("common_labels.transmission_host", "")
	viper.SetDefault("common_labels.exporter_instance", "")
	viper.SetDefault("common_labels.transmission_port", "")
}

// getDefaultInstanceName returns a default instance name based on hostname
func getDefaultInstanceName() string {
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	return "default"
}

// postProcess performs post-processing on the configuration after loading
func (c *Config) postProcess() {
	// Set common labels if not explicitly configured
	if c.CommonLabels.TransmissionHost == "" {
		c.CommonLabels.TransmissionHost = c.Transmission.Host
	}
	if c.CommonLabels.ExporterInstance == "" {
		c.CommonLabels.ExporterInstance = c.Exporter.Instance
	}
	if c.CommonLabels.TransmissionPort == "" {
		c.CommonLabels.TransmissionPort = fmt.Sprintf("%d", c.Transmission.Port)
	}
}

// Validate validates the configuration values with helpful error messages
func (c *Config) Validate() error {
	var errors []string
	
	// Validate Transmission config
	if c.Transmission.Host == "" {
		errors = append(errors, "transmission.host cannot be empty")
	}
	if c.Transmission.Port < 1 || c.Transmission.Port > 65535 {
		errors = append(errors, fmt.Sprintf("transmission.port must be between 1 and 65535, got %d", c.Transmission.Port))
	}
	if c.Transmission.Path == "" {
		errors = append(errors, "transmission.path cannot be empty")
	}
	if !strings.HasPrefix(c.Transmission.Path, "/") {
		errors = append(errors, "transmission.path must start with '/'")
	}
	if c.Transmission.Timeout < time.Second {
		errors = append(errors, fmt.Sprintf("transmission.timeout must be at least 1 second, got %v", c.Transmission.Timeout))
	}
	if c.Transmission.Timeout > 60*time.Second {
		errors = append(errors, fmt.Sprintf("transmission.timeout should not exceed 60 seconds, got %v", c.Transmission.Timeout))
	}
	
	// Validate authentication - if username is provided, password should also be provided
	if c.Transmission.Username != "" && c.Transmission.Password == "" {
		errors = append(errors, "transmission.password must be provided when transmission.username is set")
	}
	
	// Validate Exporter config
	if c.Exporter.Port < 1 || c.Exporter.Port > 65535 {
		errors = append(errors, fmt.Sprintf("exporter.port must be between 1 and 65535, got %d", c.Exporter.Port))
	}
	if c.Exporter.Path == "" {
		errors = append(errors, "exporter.path cannot be empty")
	}
	if !strings.HasPrefix(c.Exporter.Path, "/") {
		errors = append(errors, "exporter.path must start with '/'")
	}
	if c.Exporter.PollInterval < time.Second || c.Exporter.PollInterval > 60*time.Second {
		errors = append(errors, fmt.Sprintf("exporter.poll_interval must be between 1s and 60s, got %v", c.Exporter.PollInterval))
	}
	if c.Exporter.MaxStaleAge < time.Minute {
		errors = append(errors, fmt.Sprintf("exporter.max_stale_age must be at least 1 minute, got %v", c.Exporter.MaxStaleAge))
	}
	if c.Exporter.Instance == "" {
		errors = append(errors, "exporter.instance cannot be empty")
	}
	
	// Validate port conflict
	if c.Transmission.Port == c.Exporter.Port && c.Transmission.Host == "localhost" {
		errors = append(errors, "transmission.port and exporter.port cannot be the same when transmission.host is localhost")
	}
	
	// Validate Logging config
	validLevels := map[string]bool{
		"debug": true, "info": true, "warn": true, "error": true, "panic": true, "fatal": true,
	}
	if !validLevels[strings.ToLower(c.Logging.Level)] {
		errors = append(errors, fmt.Sprintf("logging.level must be one of [debug, info, warn, error, panic, fatal], got %s", c.Logging.Level))
	}
	
	validFormats := map[string]bool{
		"text": true, "json": true,
	}
	if !validFormats[strings.ToLower(c.Logging.Format)] {
		errors = append(errors, fmt.Sprintf("logging.format must be one of [text, json], got %s", c.Logging.Format))
	}
	
	// Validate Common Labels
	if c.CommonLabels.TransmissionHost == "" {
		errors = append(errors, "common_labels.transmission_host cannot be empty")
	}
	if c.CommonLabels.ExporterInstance == "" {
		errors = append(errors, "common_labels.exporter_instance cannot be empty")
	}
	if c.CommonLabels.TransmissionPort == "" {
		errors = append(errors, "common_labels.transmission_port cannot be empty")
	}
	
	// Return combined error if any validation failed
	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed:\n  - %s", strings.Join(errors, "\n  - "))
	}
	
	return nil
}

// GetTransmissionURL returns the full URL for the Transmission RPC endpoint
func (c *Config) GetTransmissionURL() string {
	scheme := "http"
	if c.Transmission.UseHTTPS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d%s", scheme, c.Transmission.Host, c.Transmission.Port, c.Transmission.Path)
}

// GetExporterAddress returns the address for the exporter HTTP server
func (c *Config) GetExporterAddress() string {
	return fmt.Sprintf(":%d", c.Exporter.Port)
}

// HasBasicAuth returns true if basic authentication is configured
func (c *Config) HasBasicAuth() bool {
	return c.Transmission.Username != "" && c.Transmission.Password != ""
}