// BSE Go HFT Server - Configuration
// Contains configuration structures and constants

package main

import (
	"encoding/json"
	"os"
)

// ================================================================================
// CONSTANTS
// ================================================================================

const (
	// BSE Multicast Feed Configuration
	// EQ (Equity/Cash): 239.1.2.5:26001
	// FO (Derivatives): 239.1.2.5:26002
	DefaultMulticastIP = "239.1.2.5"
	DefaultEQPort      = 26001 // Equity feed
	DefaultFOPort      = 26002 // F&O feed
	DefaultPort        = 26002 // Default to F&O
	BufferSize         = 65536

	// Packet structure - BSE NFCAST format
	// Pattern: 36 (header) + N×264 (records) = total packet size
	HeaderSize = 36
	RecordSize = 264 // FIXED: Each record slot is 264 bytes (not 66!)
	MaxRecords = 8

	// Latency sample storage
	MaxLatencySamples = 100000

	// ================================================================================
	// INTERNAL API CONFIGURATION (Primary - Use this for downloading)
	// ================================================================================
	// API endpoint for fetching Contract Master and BhavCopy files
	// Returns Base64 encoded CSV content
	InternalAPIURL = "http://192.168.102.166:2060/v1/sftp-files"

	// Contract Master path pattern: FNO/Common/{MONTH}-{YEAR}/{DD-MM-YYYY}/BSE_EQD_CONTRACT_{DDMMYYYY}.csv
	// BhavCopy path pattern: EQ/Common/{MONTH}-{YEAR}/{DD-MM-YYYY}/BhavCopy_BSE_CM_0_0_0_{YYYYMMDD}_F_0000.csv

	// ================================================================================
	// FALLBACK URLs (BSE Official - May return 403)
	// ================================================================================
	ContractMasterURL = "https://www.bseindia.com/downloads/Help/file/BSE_EQD_CONTRACT.csv"
	BhavCopyURL       = "https://www.bseindia.com/download/BhavCopy/Equity/EQ%s_CSV.ZIP" // Date format: DDMMYY
)

// ================================================================================
// CONFIG FILE STRUCTURE (reads from config.json)
// ================================================================================

// Config represents the application configuration loaded from config.json
type Config struct {
	Segments struct {
		CMEnabled bool `json:"cm_enabled"` // Enable Equity Cash (port 26001)
		FOEnabled bool `json:"fo_enabled"` // Enable Derivatives (port 26002)
	} `json:"segments"`
	MulticastCM struct {
		IP   string `json:"ip"`
		Port int    `json:"port"`
	} `json:"multicast_cm"`
	MulticastFO struct {
		IP   string `json:"ip"`
		Port int    `json:"port"`
	} `json:"multicast_fo"`
	API struct {
		BaseURL string `json:"base_url"`
		Timeout int    `json:"timeout"`
	} `json:"api"`
	DataManagement struct {
		KeepDays    int  `json:"keep_days"`
		AutoCleanup bool `json:"auto_cleanup"`
	} `json:"data_management"`
}

// loadConfig loads configuration from JSON file
func loadConfig(configPath string) (*Config, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, err
	}

	// Set defaults if not specified
	if config.MulticastCM.IP == "" {
		config.MulticastCM.IP = DefaultMulticastIP
	}
	if config.MulticastCM.Port == 0 {
		config.MulticastCM.Port = DefaultEQPort
	}
	if config.MulticastFO.IP == "" {
		config.MulticastFO.IP = DefaultMulticastIP
	}
	if config.MulticastFO.Port == 0 {
		config.MulticastFO.Port = DefaultFOPort
	}

	return &config, nil
}
