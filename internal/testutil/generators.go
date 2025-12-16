package testutil

import (
	"time"
	
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	
	"transmission-prometheus-exporter/internal/rpc"
)

// SessionStatsGen generates random SessionStats for property-based testing
func SessionStatsGen() gopter.Gen {
	return gopter.CombineGens(
		gen.UInt64(),                    // DownloadSpeed
		gen.UInt64(),                    // UploadSpeed
		gen.UInt32Range(0, 100),        // ActiveTorrentCount
		gen.UInt32Range(0, 100),        // PausedTorrentCount
		gen.UInt64(),                    // CumulativeDownloaded
		gen.UInt64(),                    // CumulativeUploaded
	).Map(func(values []interface{}) *rpc.SessionStats {
		activeTorrents := values[2].(uint32)
		pausedTorrents := values[3].(uint32)
		totalTorrents := activeTorrents + pausedTorrents
		
		return &rpc.SessionStats{
			DownloadSpeed:        values[0].(uint64),
			UploadSpeed:          values[1].(uint64),
			ActiveTorrentCount:   activeTorrents,
			PausedTorrentCount:   pausedTorrents,
			TotalTorrentCount:    totalTorrents,
			CumulativeDownloaded: values[4].(uint64),
			CumulativeUploaded:   values[5].(uint64),
		}
	})
}

// TorrentGen generates random Torrent for property-based testing
func TorrentGen() gopter.Gen {
	return gopter.CombineGens(
		gen.UInt32Range(1, 999999),                     // ID (non-zero)
		gen.IntRange(0, 6),                             // Status (0-6 for TorrentStatus enum)
		gen.UInt64(),                                   // DownloadRate
		gen.UInt64(),                                   // UploadRate
		gen.Float64Range(0.0, 1.0),                    // PercentDone
		gen.UInt32(),                                   // PeersConnected
		gen.UInt32Range(0, 100),                       // PeersSendingToUs
		gen.UInt32Range(0, 100),                       // PeersGettingFromUs
		gen.Bool(),                                     // HasError
		gen.AlphaString(),                             // ErrorString
	).Map(func(values []interface{}) *rpc.Torrent {
		hasError := values[8].(bool)
		errorString := values[9].(string)
		
		var errorPtr *string
		if hasError && errorString != "" {
			errorPtr = &errorString
		}
		
		// Ensure peer counts are consistent
		peersConnected := values[5].(uint32)
		peersSending := values[6].(uint32)
		peersGetting := values[7].(uint32)
		
		// Adjust peer counts to be consistent
		if peersSending+peersGetting > peersConnected {
			if peersConnected == 0 {
				peersSending = 0
				peersGetting = 0
			} else {
				// Scale down to fit within connected peers
				total := peersSending + peersGetting
				peersSending = (peersSending * peersConnected) / total
				peersGetting = (peersGetting * peersConnected) / total
			}
		}
		
		return &rpc.Torrent{
			ID:                 values[0].(uint32),
			Status:             rpc.TorrentStatus(values[1].(int)),
			DownloadRate:       values[2].(uint64),
			UploadRate:         values[3].(uint64),
			PercentDone:        values[4].(float64),
			PeersConnected:     peersConnected,
			PeersSendingToUs:   peersSending,
			PeersGettingFromUs: peersGetting,
			ErrorString:        errorPtr,
		}
	})
}

// TorrentSliceGen generates random slice of Torrents
func TorrentSliceGen() gopter.Gen {
	return gen.SliceOf(TorrentGen())
}

// SessionConfigGen generates random SessionConfig for property-based testing
func SessionConfigGen() gopter.Gen {
	return gopter.CombineGens(
		gen.Bool(),              // HasSpeedLimitDown
		gen.UInt64Range(0, 1000000), // SpeedLimitDown value
		gen.Bool(),              // HasSpeedLimitUp
		gen.UInt64Range(0, 1000000), // SpeedLimitUp value
		gen.Int64Range(-1, 1000000000), // FreeSpace (can be negative for unlimited/unknown)
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // Version (non-empty)
	).Map(func(values []interface{}) *rpc.SessionConfig {
		var speedLimitDown *uint64
		var speedLimitUp *uint64
		
		if values[0].(bool) {
			downLimit := values[1].(uint64)
			speedLimitDown = &downLimit
		}
		
		if values[2].(bool) {
			upLimit := values[3].(uint64)
			speedLimitUp = &upLimit
		}
		
		return &rpc.SessionConfig{
			SpeedLimitDown: speedLimitDown,
			SpeedLimitUp:   speedLimitUp,
			FreeSpace:      values[4].(int64),
			Version:        values[5].(string),
		}
	})
}

// ValidConfigGen generates valid configuration for property-based testing
func ValidConfigGen() gopter.Gen {
	return gopter.CombineGens(
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // Host
		gen.IntRange(1, 65535),                                                 // Port
		gen.AlphaString().SuchThat(func(s string) bool { return len(s) > 0 }), // Path
		gen.Bool(),                                                             // UseHTTPS
		gen.IntRange(1, 60),                                                    // PollInterval (seconds)
		gen.IntRange(8080, 9999),                                              // Exporter Port
	).Map(func(values []interface{}) map[string]interface{} {
		return map[string]interface{}{
			"transmission.host":         values[0].(string),
			"transmission.port":         values[1].(int),
			"transmission.path":         values[2].(string),
			"transmission.use_https":    values[3].(bool),
			"transmission.timeout":      "10s",
			"exporter.port":            values[5].(int),
			"exporter.path":            "/metrics",
			"exporter.poll_interval":   time.Duration(values[4].(int)) * time.Second,
			"exporter.max_stale_age":   5 * time.Minute,
			"logging.level":            "info",
			"logging.format":           "text",
		}
	})
}