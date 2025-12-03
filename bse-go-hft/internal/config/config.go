// Package config provides configuration for BSE HFT system
package config

import (
	"encoding/json"
	"os"
	"time"
)

// Constants for BSE multicast feeds
const (
	DefaultMulticastIP = "239.1.2.5"
	DefaultEQPort      = 26001 // Equity Cash
	DefaultFOPort      = 26002 // F&O Derivatives
	DefaultBufferSize  = 65536

	// Internal API for downloading token files
	InternalAPIURL = "http://192.168.102.166:2060/v1/sftp-files"

	// BSE packet structure
	HeaderSize = 36
	RecordSize = 264 // BSE uses 264-byte records
	MaxRecords = 8

	// Message types
	MsgTypeEquity     = 2020
	MsgTypeDerivative = 2021

	// HFT settings
	RingBufferSize    = 1 << 16 // 65536 slots
	MaxLatencySamples = 100000
	SocketRcvBuf      = 16 * 1024 * 1024 // 16MB socket buffer
)

// Config holds all application configuration
type Config struct {
	// Segments to enable
	Segments SegmentsConfig `json:"segments"`

	// Multicast settings
	MulticastCM MulticastConfig `json:"multicast_cm"`
	MulticastFO MulticastConfig `json:"multicast_fo"`

	// API settings
	API APIConfig `json:"api"`

	// Data management
	DataManagement DataManagementConfig `json:"data_management"`

	// Output settings
	Output OutputConfig `json:"output"`
}

// SegmentsConfig configures which feeds to enable
type SegmentsConfig struct {
	CMEnabled bool `json:"cm_enabled"` // Equity Cash (port 26001)
	FOEnabled bool `json:"fo_enabled"` // Derivatives (port 26002)
}

// MulticastConfig holds multicast settings
type MulticastConfig struct {
	IP           string        `json:"ip"`
	Port         int           `json:"port"`
	BufferSize   int           `json:"buffer_size"`
	SocketRcvBuf int           `json:"socket_rcvbuf"`
	ReadTimeout  time.Duration `json:"read_timeout"`
}

// APIConfig holds API settings
type APIConfig struct {
	BaseURL string `json:"base_url"`
	Timeout int    `json:"timeout"`
}

// DataManagementConfig holds data file settings
type DataManagementConfig struct {
	TokensDir   string `json:"tokens_dir"`
	OutputDir   string `json:"output_dir"`
	KeepDays    int    `json:"keep_days"`
	AutoCleanup bool   `json:"auto_cleanup"`
}

// OutputConfig holds output settings
type OutputConfig struct {
	EnableCSV  bool `json:"enable_csv"`
	EnableJSON bool `json:"enable_json"`
}

// DefaultConfig returns configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Segments: SegmentsConfig{
			CMEnabled: true,
			FOEnabled: true,
		},
		MulticastCM: MulticastConfig{
			IP:           DefaultMulticastIP,
			Port:         DefaultEQPort,
			BufferSize:   DefaultBufferSize,
			SocketRcvBuf: SocketRcvBuf,
			ReadTimeout:  100 * time.Millisecond,
		},
		MulticastFO: MulticastConfig{
			IP:           DefaultMulticastIP,
			Port:         DefaultFOPort,
			BufferSize:   DefaultBufferSize,
			SocketRcvBuf: SocketRcvBuf,
			ReadTimeout:  100 * time.Millisecond,
		},
		API: APIConfig{
			BaseURL: InternalAPIURL,
			Timeout: 30,
		},
		DataManagement: DataManagementConfig{
			TokensDir:   "./data/tokens",
			OutputDir:   "./data/processed_csv",
			KeepDays:    7,
			AutoCleanup: true,
		},
		Output: OutputConfig{
			EnableCSV:  true,
			EnableJSON: false,
		},
	}
}

// LoadConfig loads configuration from a JSON file
func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config := DefaultConfig()
	if err := json.NewDecoder(file).Decode(config); err != nil {
		return nil, err
	}

	// Apply defaults for any zero values
	config.applyDefaults()

	return config, nil
}

// LoadOrDefault loads config from file, or returns defaults if file doesn't exist
func LoadOrDefault(path string) *Config {
	config, err := LoadConfig(path)
	if err != nil {
		return DefaultConfig()
	}
	return config
}

// applyDefaults ensures all required fields have values
func (c *Config) applyDefaults() {
	defaults := DefaultConfig()

	if c.MulticastCM.IP == "" {
		c.MulticastCM.IP = defaults.MulticastCM.IP
	}
	if c.MulticastCM.Port == 0 {
		c.MulticastCM.Port = defaults.MulticastCM.Port
	}
	if c.MulticastCM.BufferSize == 0 {
		c.MulticastCM.BufferSize = defaults.MulticastCM.BufferSize
	}
	if c.MulticastCM.SocketRcvBuf == 0 {
		c.MulticastCM.SocketRcvBuf = defaults.MulticastCM.SocketRcvBuf
	}
	if c.MulticastCM.ReadTimeout == 0 {
		c.MulticastCM.ReadTimeout = defaults.MulticastCM.ReadTimeout
	}

	if c.MulticastFO.IP == "" {
		c.MulticastFO.IP = defaults.MulticastFO.IP
	}
	if c.MulticastFO.Port == 0 {
		c.MulticastFO.Port = defaults.MulticastFO.Port
	}
	if c.MulticastFO.BufferSize == 0 {
		c.MulticastFO.BufferSize = defaults.MulticastFO.BufferSize
	}
	if c.MulticastFO.SocketRcvBuf == 0 {
		c.MulticastFO.SocketRcvBuf = defaults.MulticastFO.SocketRcvBuf
	}
	if c.MulticastFO.ReadTimeout == 0 {
		c.MulticastFO.ReadTimeout = defaults.MulticastFO.ReadTimeout
	}

	if c.API.BaseURL == "" {
		c.API.BaseURL = defaults.API.BaseURL
	}
	if c.API.Timeout == 0 {
		c.API.Timeout = defaults.API.Timeout
	}

	if c.DataManagement.TokensDir == "" {
		c.DataManagement.TokensDir = defaults.DataManagement.TokensDir
	}
	if c.DataManagement.OutputDir == "" {
		c.DataManagement.OutputDir = defaults.DataManagement.OutputDir
	}
	if c.DataManagement.KeepDays == 0 {
		c.DataManagement.KeepDays = defaults.DataManagement.KeepDays
	}
}
