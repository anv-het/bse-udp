// Package config handles configuration for the BSE HFT system
package config

import (
	"encoding/json"
	"os"
	"time"
)

// Config holds all configuration settings
type Config struct {
	// Network settings
	MulticastIP string `json:"multicast_ip"`
	Port        int    `json:"port"`
	BufferSize  int    `json:"buffer_size"`

	// Ring buffer settings
	RingBufferSize int `json:"ring_buffer_size"`

	// Benchmark settings
	Duration      time.Duration `json:"duration"`
	ReportingRate time.Duration `json:"reporting_rate"`

	// Output settings
	OutputJSON bool   `json:"output_json"`
	OutputCSV  bool   `json:"output_csv"`
	OutputDir  string `json:"output_dir"`

	// Token file
	TokenFile string `json:"token_file"`
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		MulticastIP:    "239.1.2.5", // BSE F&O default
		Port:           26001,
		BufferSize:     65536,
		RingBufferSize: 65536, // 64K entries
		Duration:       0,     // Run forever
		ReportingRate:  time.Second,
		OutputJSON:     false,
		OutputCSV:      false,
		OutputDir:      "./data",
		TokenFile:      "./data/tokens/token_details.json",
	}
}

// LoadConfig loads configuration from file
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, nil // Return defaults if file doesn't exist
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// EQConfig returns configuration for Equity feed
func EQConfig() *Config {
	cfg := DefaultConfig()
	cfg.MulticastIP = "239.1.2.5"
	cfg.Port = 26001
	return cfg
}

// FOConfig returns configuration for F&O feed
func FOConfig() *Config {
	cfg := DefaultConfig()
	cfg.MulticastIP = "239.1.2.5"
	cfg.Port = 26001
	return cfg
}
