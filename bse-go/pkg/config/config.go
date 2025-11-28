package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds the complete application configuration
type Config struct {
	Segments       SegmentsConfig  `json:"segments"`
	Multicast      MulticastConfig `json:"multicast"`
	MulticastCM    MulticastConfig `json:"multicast_cm"`
	MulticastFO    MulticastConfig `json:"multicast_fo"`
	API            APIConfig       `json:"api"`
	DataManagement DataMgmtConfig  `json:"data_management"`
	BufferSize     int             `json:"buffer_size"`
	LoggingLevel   string          `json:"logging_level"`
	Timeout        int             `json:"timeout"`
	StoreLimit     int             `json:"store_limit"`
}

// SegmentsConfig controls which market segments are enabled
type SegmentsConfig struct {
	CMEnabled bool   `json:"cm_enabled"`
	FOEnabled bool   `json:"fo_enabled"`
	Comment   string `json:"comment"`
}

// MulticastConfig holds UDP multicast connection settings
type MulticastConfig struct {
	IP      string `json:"ip"`
	Port    int    `json:"port"`
	Segment string `json:"segment"`
	Env     string `json:"env"`
}

// APIConfig holds BSE API settings for contract master updates
type APIConfig struct {
	BaseURL string `json:"base_url"`
	APIType string `json:"api_type"`
	Timeout int    `json:"timeout"`
}

// DataMgmtConfig holds data retention settings
type DataMgmtConfig struct {
	KeepDays    int  `json:"keep_days"`
	AutoCleanup bool `json:"auto_cleanup"`
}

// LoadConfig loads configuration from a JSON file
func LoadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	var config Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	// Set defaults if not specified
	if config.BufferSize == 0 {
		config.BufferSize = 2048
	}
	if config.Timeout == 0 {
		config.Timeout = 30
	}
	if config.StoreLimit == 0 {
		config.StoreLimit = 100
	}
	if config.DataManagement.KeepDays == 0 {
		config.DataManagement.KeepDays = 2
	}

	return &config, nil
}

// GetMulticastConfig returns the multicast config for a given segment
func (c *Config) GetMulticastConfig(segment string) MulticastConfig {
	switch segment {
	case "CM":
		return c.MulticastCM
	case "FO":
		return c.MulticastFO
	default:
		return c.Multicast
	}
}

// IsCMEnabled returns true if Equity Cash segment is enabled
func (c *Config) IsCMEnabled() bool {
	return c.Segments.CMEnabled
}

// IsFOEnabled returns true if Derivatives segment is enabled
func (c *Config) IsFOEnabled() bool {
	return c.Segments.FOEnabled
}
