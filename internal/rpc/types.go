package rpc

import (
	"fmt"
)

// SessionToken represents the CSRF-style token required by Transmission
type SessionToken string

// SessionStats represents global Transmission statistics
type SessionStats struct {
	DownloadSpeed        uint64 `json:"downloadSpeed"`
	UploadSpeed          uint64 `json:"uploadSpeed"`
	ActiveTorrentCount   uint32 `json:"activeTorrentCount"`
	PausedTorrentCount   uint32 `json:"pausedTorrentCount"`
	TotalTorrentCount    uint32 `json:"torrentCount"`
	CumulativeDownloaded uint64 `json:"cumulative-stats-downloaded"`
	CumulativeUploaded   uint64 `json:"cumulative-stats-uploaded"`
}

// TorrentStatus represents the status of a torrent
type TorrentStatus int

const (
	TorrentStopped TorrentStatus = iota
	TorrentCheckWait
	TorrentCheck
	TorrentDownloadWait
	TorrentDownload
	TorrentSeedWait
	TorrentSeed
)

// Torrent represents individual torrent data
type Torrent struct {
	ID                  uint32        `json:"id"`
	Status              TorrentStatus `json:"status"`
	DownloadRate        uint64        `json:"rateDownload"`
	UploadRate          uint64        `json:"rateUpload"`
	PercentDone         float64       `json:"percentDone"`
	PeersConnected      uint32        `json:"peersConnected"`
	PeersSendingToUs    uint32        `json:"peersSendingToUs"`
	PeersGettingFromUs  uint32        `json:"peersGettingFromUs"`
	ErrorString         *string       `json:"errorString"`
}

// SessionConfig represents Transmission session configuration
type SessionConfig struct {
	SpeedLimitDown *uint64 `json:"speed-limit-down"`
	SpeedLimitUp   *uint64 `json:"speed-limit-up"`
	FreeSpace      int64   `json:"download-dir-free-space"` // Can be negative (-1 for unlimited/unknown)
	Version        string  `json:"version"`
}

// RPC Request Types

// BaseRequest represents the common structure for all RPC requests
type BaseRequest struct {
	Method string `json:"method"`
	Tag    *int   `json:"tag,omitempty"`
}

// SessionStatsRequest represents a request for session statistics
type SessionStatsRequest struct {
	BaseRequest
}

// TorrentGetRequest represents a request for torrent information
type TorrentGetRequest struct {
	BaseRequest
	Arguments TorrentGetArguments `json:"arguments"`
}

// TorrentGetArguments represents the arguments for torrent-get requests
type TorrentGetArguments struct {
	Fields []string `json:"fields"`
	IDs    []int    `json:"ids,omitempty"` // Optional: specific torrent IDs
}

// SessionGetRequest represents a request for session configuration
type SessionGetRequest struct {
	BaseRequest
}

// RPC Response Types

// BaseResponse represents the common structure for all RPC responses
type BaseResponse struct {
	Result string `json:"result"`
	Tag    *int   `json:"tag,omitempty"`
}

// SessionStatsResponse represents the response from session-stats
type SessionStatsResponse struct {
	BaseResponse
	Arguments *SessionStats `json:"arguments"`
}

// TorrentGetResponse represents the response from torrent-get
type TorrentGetResponse struct {
	BaseResponse
	Arguments *TorrentGetResponseArguments `json:"arguments"`
}

// TorrentGetResponseArguments represents the arguments in torrent-get response
type TorrentGetResponseArguments struct {
	Torrents []*Torrent `json:"torrents"`
}

// SessionGetResponse represents the response from session-get
type SessionGetResponse struct {
	BaseResponse
	Arguments *SessionConfig `json:"arguments"`
}

// Validation methods

// Validate validates a SessionStatsResponse for completeness and correctness
func (r *SessionStatsResponse) Validate() error {
	if r.Result != "success" {
		return fmt.Errorf("RPC call failed: %s", r.Result)
	}
	
	if r.Arguments == nil {
		return fmt.Errorf("no session stats in response")
	}
	
	// Validate that required fields are present and reasonable
	stats := r.Arguments
	if stats.TotalTorrentCount < stats.ActiveTorrentCount+stats.PausedTorrentCount {
		return fmt.Errorf("invalid torrent counts: total=%d, active=%d, paused=%d", 
			stats.TotalTorrentCount, stats.ActiveTorrentCount, stats.PausedTorrentCount)
	}
	
	return nil
}

// Validate validates a TorrentGetResponse for completeness and correctness
func (r *TorrentGetResponse) Validate() error {
	if r.Result != "success" {
		return fmt.Errorf("RPC call failed: %s", r.Result)
	}
	
	if r.Arguments == nil {
		return fmt.Errorf("no torrent data in response")
	}
	
	// Validate each torrent
	for i, torrent := range r.Arguments.Torrents {
		if torrent == nil {
			return fmt.Errorf("torrent at index %d is nil", i)
		}
		
		if err := torrent.Validate(); err != nil {
			return fmt.Errorf("torrent %d validation failed: %w", i, err)
		}
	}
	
	return nil
}

// Validate validates a SessionGetResponse for completeness and correctness
func (r *SessionGetResponse) Validate() error {
	if r.Result != "success" {
		return fmt.Errorf("RPC call failed: %s", r.Result)
	}
	
	if r.Arguments == nil {
		return fmt.Errorf("no session config in response")
	}
	
	// Validate session config
	config := r.Arguments
	if config.Version == "" {
		return fmt.Errorf("missing version in session config")
	}
	
	// Validate speed limits if present
	if config.SpeedLimitDown != nil && *config.SpeedLimitDown > 1000000000 { // 1GB/s sanity check
		return fmt.Errorf("unrealistic download speed limit: %d", *config.SpeedLimitDown)
	}
	
	if config.SpeedLimitUp != nil && *config.SpeedLimitUp > 1000000000 { // 1GB/s sanity check
		return fmt.Errorf("unrealistic upload speed limit: %d", *config.SpeedLimitUp)
	}
	
	return nil
}

// Validate validates a Torrent for completeness and correctness
func (t *Torrent) Validate() error {
	if t.ID == 0 {
		return fmt.Errorf("torrent ID cannot be zero")
	}
	
	if t.PercentDone < 0 || t.PercentDone > 1 {
		return fmt.Errorf("invalid percent done: %f (must be between 0 and 1)", t.PercentDone)
	}
	
	if int(t.Status) < 0 || int(t.Status) > int(TorrentSeed) {
		return fmt.Errorf("invalid torrent status: %d", t.Status)
	}
	
	// Validate peer counts are reasonable
	if t.PeersConnected < t.PeersSendingToUs+t.PeersGettingFromUs {
		return fmt.Errorf("peer count inconsistency: connected=%d, sending=%d, getting=%d", 
			t.PeersConnected, t.PeersSendingToUs, t.PeersGettingFromUs)
	}
	
	return nil
}

// NewSessionStatsRequest creates a new session-stats request
func NewSessionStatsRequest() *SessionStatsRequest {
	return &SessionStatsRequest{
		BaseRequest: BaseRequest{
			Method: "session-stats",
		},
	}
}

// NewTorrentGetRequest creates a new torrent-get request with specified fields
func NewTorrentGetRequest(fields []string) *TorrentGetRequest {
	return &TorrentGetRequest{
		BaseRequest: BaseRequest{
			Method: "torrent-get",
		},
		Arguments: TorrentGetArguments{
			Fields: fields,
		},
	}
}

// NewSessionGetRequest creates a new session-get request
func NewSessionGetRequest() *SessionGetRequest {
	return &SessionGetRequest{
		BaseRequest: BaseRequest{
			Method: "session-get",
		},
	}
}

// ExporterMetrics represents metrics about the exporter's own operation
type ExporterMetrics struct {
	ScrapeErrorsTotal     uint64  `json:"scrapeErrorsTotal"`
	RPCLatencySeconds     float64 `json:"rpcLatencySeconds"`
	LastSuccessfulScrape  int64   `json:"lastSuccessfulScrape"`  // Unix timestamp
	CacheHits             uint64  `json:"cacheHits"`
	CacheMisses           uint64  `json:"cacheMisses"`
}

// Validate validates ExporterMetrics for correctness
func (e *ExporterMetrics) Validate() error {
	if e.RPCLatencySeconds < 0 {
		return fmt.Errorf("RPC latency cannot be negative: %f", e.RPCLatencySeconds)
	}
	
	if e.LastSuccessfulScrape < 0 {
		return fmt.Errorf("last successful scrape timestamp cannot be negative: %d", e.LastSuccessfulScrape)
	}
	
	return nil
}

// Helper methods for metric conversion

// ToPrometheusFloat64 converts SessionStats fields to float64 for Prometheus metrics
func (s *SessionStats) ToPrometheusFloat64() (downloadSpeed, uploadSpeed, activeTorrents, pausedTorrents, totalTorrents, cumulativeDownloaded, cumulativeUploaded float64) {
	return float64(s.DownloadSpeed),
		   float64(s.UploadSpeed),
		   float64(s.ActiveTorrentCount),
		   float64(s.PausedTorrentCount),
		   float64(s.TotalTorrentCount),
		   float64(s.CumulativeDownloaded),
		   float64(s.CumulativeUploaded)
}

// ToPrometheusFloat64 converts Torrent fields to float64 for Prometheus metrics
func (t *Torrent) ToPrometheusFloat64() (downloadRate, uploadRate, progress, peersConnected, peersSending, peersGetting, status float64) {
	return float64(t.DownloadRate),
		   float64(t.UploadRate),
		   t.PercentDone,
		   float64(t.PeersConnected),
		   float64(t.PeersSendingToUs),
		   float64(t.PeersGettingFromUs),
		   float64(t.Status)
}

// HasError returns true if the torrent has an error condition
func (t *Torrent) HasError() bool {
	return t.ErrorString != nil && *t.ErrorString != ""
}

// GetErrorString returns the error string or empty string if no error
func (t *Torrent) GetErrorString() string {
	if t.ErrorString == nil {
		return ""
	}
	return *t.ErrorString
}

// GetTorrentIDString returns the torrent ID as a string for use in metric labels
func (t *Torrent) GetTorrentIDString() string {
	return fmt.Sprintf("%d", t.ID)
}

// ToPrometheusFloat64 converts SessionConfig fields to float64 for Prometheus metrics
func (c *SessionConfig) ToPrometheusFloat64() (speedLimitDown, speedLimitUp, freeSpace float64) {
	var downLimit, upLimit float64
	
	if c.SpeedLimitDown != nil {
		downLimit = float64(*c.SpeedLimitDown)
	}
	
	if c.SpeedLimitUp != nil {
		upLimit = float64(*c.SpeedLimitUp)
	}
	
	return downLimit, upLimit, float64(c.FreeSpace)
}

// ToPrometheusFloat64 converts ExporterMetrics fields to float64 for Prometheus metrics
func (e *ExporterMetrics) ToPrometheusFloat64() (scrapeErrors, rpcLatency, lastScrape, cacheHits, cacheMisses float64) {
	return float64(e.ScrapeErrorsTotal),
		   e.RPCLatencySeconds,
		   float64(e.LastSuccessfulScrape),
		   float64(e.CacheHits),
		   float64(e.CacheMisses)
}