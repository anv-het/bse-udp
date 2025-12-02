// BSE Go HFT Server - Complete Production Pipeline
// Features:
// 1. Contract Master download (if not exists)
// 2. Token to Symbol mapping
// 3. Full packet decoding with decompression
// 4. CSV/JSON output
// 5. HFT Benchmark statistics

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/net/ipv4"
)

// ================================================================================
// CONFIGURATION
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

// ================================================================================
// CONTRACT/TOKEN STRUCTURES
// ================================================================================

type Contract struct {
	Token          uint32 `json:"token"`
	Symbol         string `json:"symbol"`
	InstrumentName string `json:"instrument_name"`
	Expiry         string `json:"expiry"`
	OptionType     string `json:"option_type"`
	StrikePrice    string `json:"strike_price"`
	LotSize        int    `json:"lot_size"`
	Segment        string `json:"segment"`
}

type TokenMap struct {
	mu        sync.RWMutex
	contracts map[uint32]*Contract
}

func NewTokenMap() *TokenMap {
	return &TokenMap{
		contracts: make(map[uint32]*Contract),
	}
}

func (tm *TokenMap) Add(token uint32, contract *Contract) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.contracts[token] = contract
}

func (tm *TokenMap) Get(token uint32) (*Contract, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	c, ok := tm.contracts[token]
	return c, ok
}

func (tm *TokenMap) Len() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return len(tm.contracts)
}

// ================================================================================
// QUOTE STRUCTURE
// ================================================================================

type Quote struct {
	Timestamp   string  `json:"timestamp"`
	Token       uint32  `json:"token"`
	Symbol      string  `json:"symbol"`
	SymbolName  string  `json:"symbol_name"`
	Expiry      string  `json:"expiry"`
	OptionType  string  `json:"option_type"`
	StrikePrice string  `json:"strike_price"`
	LTP         float64 `json:"ltp"`
	Open        float64 `json:"open"`
	High        float64 `json:"high"`
	Low         float64 `json:"low"`
	PrevClose   float64 `json:"prev_close"`
	Volume      int64   `json:"volume"`
	LotSize     int     `json:"lot_size"`
	SequenceNum uint32  `json:"sequence_num"`
	BidPrices   string  `json:"bid_prices"`
	BidQtys     string  `json:"bid_qtys"`
	AskPrices   string  `json:"ask_prices"`
	AskQtys     string  `json:"ask_qtys"`
}

// ================================================================================
// LATENCY PERCENTILES
// ================================================================================

type LatencyPercentiles struct {
	P50  float64
	P75  float64
	P90  float64
	P95  float64
	P99  float64
	P999 float64
}

// ================================================================================
// STATISTICS
// ================================================================================

type Stats struct {
	TotalPackets atomic.Uint64
	ValidPackets atomic.Uint64
	TotalRecords atomic.Uint64
	TotalBytes   atomic.Uint64
	QuotesSaved  atomic.Uint64

	DecodeSum   atomic.Int64
	DecodeCount atomic.Int64
	DecodeMin   atomic.Int64
	DecodeMax   atomic.Int64

	decodeSamples []int64
	sampleMu      sync.Mutex

	PeakMemory  atomic.Uint64
	MemorySum   atomic.Uint64
	SampleCount atomic.Uint64

	// Missed token tracking
	MissedTokens     map[uint32]int64 // token -> count of occurrences
	MissedTokensMu   sync.Mutex
	MissedTokenCount atomic.Uint64

	startTime time.Time
}

func NewStats() *Stats {
	s := &Stats{
		startTime:     time.Now(),
		decodeSamples: make([]int64, 0, MaxLatencySamples),
		MissedTokens:  make(map[uint32]int64),
	}
	s.DecodeMin.Store(math.MaxInt64)
	return s
}

// TrackMissedToken records a token that was not found in the token map
func (s *Stats) TrackMissedToken(token uint32) {
	s.MissedTokensMu.Lock()
	s.MissedTokens[token]++
	s.MissedTokensMu.Unlock()
	s.MissedTokenCount.Add(1)
}

// GetMissedTokensSummary returns top N missed tokens by occurrence count
func (s *Stats) GetMissedTokensSummary(topN int) []struct {
	Token uint32
	Count int64
} {
	s.MissedTokensMu.Lock()
	defer s.MissedTokensMu.Unlock()

	type tokenCount struct {
		Token uint32
		Count int64
	}
	var list []tokenCount
	for t, c := range s.MissedTokens {
		list = append(list, tokenCount{t, c})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Count > list[j].Count })

	if len(list) > topN {
		list = list[:topN]
	}

	result := make([]struct {
		Token uint32
		Count int64
	}, len(list))
	for i, tc := range list {
		result[i].Token = tc.Token
		result[i].Count = tc.Count
	}
	return result
}

func (s *Stats) RecordLatency(ns int64) {
	s.DecodeSum.Add(ns)
	s.DecodeCount.Add(1)

	for {
		old := s.DecodeMin.Load()
		if ns >= old || s.DecodeMin.CompareAndSwap(old, ns) {
			break
		}
	}
	for {
		old := s.DecodeMax.Load()
		if ns <= old || s.DecodeMax.CompareAndSwap(old, ns) {
			break
		}
	}

	s.sampleMu.Lock()
	if len(s.decodeSamples) < MaxLatencySamples {
		s.decodeSamples = append(s.decodeSamples, ns)
	}
	s.sampleMu.Unlock()
}

func (s *Stats) GetPercentiles() LatencyPercentiles {
	s.sampleMu.Lock()
	defer s.sampleMu.Unlock()

	if len(s.decodeSamples) == 0 {
		return LatencyPercentiles{}
	}

	sorted := make([]int64, len(s.decodeSamples))
	copy(sorted, s.decodeSamples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	n := len(sorted)
	getP := func(p float64) float64 {
		idx := int(float64(n-1) * p)
		return float64(sorted[idx]) / 1000.0
	}

	return LatencyPercentiles{
		P50:  getP(0.50),
		P75:  getP(0.75),
		P90:  getP(0.90),
		P95:  getP(0.95),
		P99:  getP(0.99),
		P999: getP(0.999),
	}
}

func (s *Stats) SampleMemory() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	for {
		peak := s.PeakMemory.Load()
		if m.Alloc <= peak || s.PeakMemory.CompareAndSwap(peak, m.Alloc) {
			break
		}
	}
	s.MemorySum.Add(m.Alloc)
	s.SampleCount.Add(1)
}

// ================================================================================
// CONTRACT MASTER / BHAVCOPY DOWNLOADER - INTERNAL API
// ================================================================================

// APIResponse represents the response from internal API
type APIResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		FileContent string `json:"file_content"` // Base64 encoded CSV
	} `json:"data"`
}

// getMonthName returns month abbreviation (JAN, FEB, etc.)
func getMonthName(month time.Month) string {
	months := []string{"", "JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}
	return months[month]
}

// ================================================================================
// TOKEN FILE MANAGER - Python-like file management
// ================================================================================

// TokenFileManager manages token file download, caching, and cleanup
// Implements the same logic as Python's contract_manager.py
type TokenFileManager struct {
	tokensDir  string
	maxRetries int
	retryDelay time.Duration
	keepFiles  int
	holidays   map[string]bool // Known holidays (can be extended)
}

// NewTokenFileManager creates a new token file manager
func NewTokenFileManager(tokensDir string) *TokenFileManager {
	// Known BSE holidays for 2025 (can be extended)
	holidays := map[string]bool{
		"2025-01-26": true, // Republic Day
		"2025-03-14": true, // Holi
		"2025-03-31": true, // Id-Ul-Fitr
		"2025-04-10": true, // Shri Mahavir Jayanti
		"2025-04-14": true, // Dr. Ambedkar Jayanti
		"2025-04-18": true, // Good Friday
		"2025-05-01": true, // Maharashtra Day
		"2025-08-15": true, // Independence Day
		"2025-08-27": true, // Janmashtami
		"2025-10-02": true, // Gandhi Jayanti
		"2025-10-21": true, // Diwali Laxmi Pujan
		"2025-10-22": true, // Diwali Balipratipada
		"2025-11-05": true, // Prakash Gurpurab
		"2025-11-26": true, // Guru Nanak Jayanti
		"2025-12-25": true, // Christmas
	}

	return &TokenFileManager{
		tokensDir:  tokensDir,
		maxRetries: 3,
		retryDelay: 10 * time.Second,
		keepFiles:  2, // Keep last 2 files
		holidays:   holidays,
	}
}

// isWeekend checks if a date is Saturday or Sunday
func (m *TokenFileManager) isWeekend(date time.Time) bool {
	return date.Weekday() == time.Saturday || date.Weekday() == time.Sunday
}

// isHoliday checks if a date is a known holiday
func (m *TokenFileManager) isHoliday(date time.Time) bool {
	dateStr := date.Format("2006-01-02")
	return m.holidays[dateStr]
}

// isTradingDay checks if a date is a trading day (not weekend and not holiday)
func (m *TokenFileManager) isTradingDay(date time.Time) bool {
	return !m.isWeekend(date) && !m.isHoliday(date)
}

// getTargetDate returns the target file date (previous trading day)
// Example: If today is 02-12-2025 (Tuesday), target is 01-12-2025 (Monday)
// If today is 01-12-2025 (Monday), target is 28-11-2025 (Friday, skipping weekend)
func (m *TokenFileManager) getTargetDate() time.Time {
	target := time.Now().AddDate(0, 0, -1) // Start with yesterday

	// Keep going back until we find a trading day
	for !m.isTradingDay(target) {
		target = target.AddDate(0, 0, -1)
	}

	return target
}

// getTradingDates returns last N trading dates for fallback
func (m *TokenFileManager) getTradingDates(count int) []time.Time {
	var dates []time.Time
	current := m.getTargetDate()

	for len(dates) < count {
		if m.isTradingDay(current) {
			dates = append(dates, current)
		}
		current = current.AddDate(0, 0, -1)
	}

	return dates
}

// getExpectedFilename returns the expected filename for a given date and feed type
func (m *TokenFileManager) getExpectedFilename(date time.Time, feedType string) string {
	if feedType == "EQ" {
		return fmt.Sprintf("BhavCopy_BSE_CM_%s.csv", date.Format("02012006"))
	}
	return fmt.Sprintf("BSE_EQD_CONTRACT_%s.csv", date.Format("02012006"))
}

// getCachedFile checks if correct file exists in cache (for target date)
func (m *TokenFileManager) getCachedFile(feedType string) (string, time.Time, bool) {
	targetDate := m.getTargetDate()
	expectedFilename := m.getExpectedFilename(targetDate, feedType)
	expectedPath := filepath.Join(m.tokensDir, expectedFilename)

	if info, err := os.Stat(expectedPath); err == nil && info.Size() > 1000 {
		return expectedPath, targetDate, true
	}

	return "", targetDate, false
}

// downloadWithRetries downloads file with retry logic (3 retries with delay)
func (m *TokenFileManager) downloadWithRetries(date time.Time, feedType string) (string, error) {
	var lastErr error

	for retry := 1; retry <= m.maxRetries; retry++ {
		fmt.Printf("   🔄 Attempt %d/%d for %s (%s)...\n", retry, m.maxRetries, date.Format("02-01-2006"), date.Weekday())

		csvData, err := fetchFromInternalAPI(date, feedType)
		if err == nil {
			// Success! Save the file
			filename := m.getExpectedFilename(date, feedType)
			outputPath := filepath.Join(m.tokensDir, filename)

			if err := os.MkdirAll(m.tokensDir, 0755); err != nil {
				return "", fmt.Errorf("create directory failed: %w", err)
			}

			if err := os.WriteFile(outputPath, csvData, 0644); err != nil {
				return "", fmt.Errorf("write file failed: %w", err)
			}

			fmt.Printf("   ✅ Downloaded and saved: %s\n", filename)
			return outputPath, nil
		}

		lastErr = err
		fmt.Printf("   ❌ Attempt %d failed: %v\n", retry, err)

		if retry < m.maxRetries {
			fmt.Printf("   ⏳ Waiting %v before retry...\n", m.retryDelay)
			time.Sleep(m.retryDelay)
		}
	}

	return "", fmt.Errorf("all %d retries failed: %w", m.maxRetries, lastErr)
}

// GetLatestFile gets the latest token file with proper logic:
// 1. Check if correct date file exists in cache
// 2. If not, download with 3 retries
// 3. If download fails, fallback to older cached files
func (m *TokenFileManager) GetLatestFile(feedType string) (string, error) {
	targetDate := m.getTargetDate()
	feedName := "Contract Master"
	if feedType == "EQ" {
		feedName = "BhavCopy"
	}

	fmt.Printf("\n📊 Loading %s (target date: %s)\n", feedName, targetDate.Format("02-01-2006"))

	// Step 1: Check if correct date file exists in cache
	cachedPath, _, exists := m.getCachedFile(feedType)
	if exists {
		fmt.Printf("   ✅ Found cached file for correct date: %s\n", filepath.Base(cachedPath))
		return cachedPath, nil
	}

	// Step 2: Try to download the target date file with retries
	fmt.Printf("   📥 Downloading from API (target: %s)...\n", targetDate.Format("02-01-2006"))
	downloadedPath, err := m.downloadWithRetries(targetDate, feedType)
	if err == nil {
		return downloadedPath, nil
	}

	fmt.Printf("   ⚠️  Download failed after retries: %v\n", err)

	// Step 3: Fallback to older cached files
	fmt.Printf("   🔍 Looking for older cached files as fallback...\n")
	fallbackDates := m.getTradingDates(7) // Try last 7 trading days

	for _, date := range fallbackDates {
		filename := m.getExpectedFilename(date, feedType)
		filePath := filepath.Join(m.tokensDir, filename)

		if info, err := os.Stat(filePath); err == nil && info.Size() > 1000 {
			fmt.Printf("   ✅ Using fallback file: %s (dated %s)\n", filename, date.Format("02-01-2006"))
			return filePath, nil
		}
	}

	// Step 4: Try downloading older dates
	fmt.Printf("   📥 Trying to download older date files...\n")
	for i, date := range fallbackDates {
		if i == 0 {
			continue // Skip target date, already tried
		}

		fmt.Printf("   📥 Trying %s...\n", date.Format("02-01-2006"))
		csvData, err := fetchFromInternalAPI(date, feedType)
		if err == nil {
			filename := m.getExpectedFilename(date, feedType)
			outputPath := filepath.Join(m.tokensDir, filename)

			if err := os.WriteFile(outputPath, csvData, 0644); err != nil {
				continue
			}

			fmt.Printf("   ✅ Downloaded fallback: %s\n", filename)
			return outputPath, nil
		}
	}

	return "", fmt.Errorf("failed to get %s file after all attempts", feedName)
}

// CleanupOldFiles removes old token files, keeping only the latest N files
func (m *TokenFileManager) CleanupOldFiles(feedType string) {
	pattern := "BhavCopy_BSE_CM_*.csv"
	if feedType == "FO" {
		pattern = "BSE_EQD_CONTRACT_*.csv"
	}

	// Find all matching files
	matches, err := filepath.Glob(filepath.Join(m.tokensDir, pattern))
	if err != nil || len(matches) <= m.keepFiles {
		return
	}

	// Sort by modification time (newest first)
	type fileInfo struct {
		path    string
		modTime time.Time
	}
	var files []fileInfo

	for _, path := range matches {
		if info, err := os.Stat(path); err == nil {
			files = append(files, fileInfo{path: path, modTime: info.ModTime()})
		}
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	// Delete old files (keep only m.keepFiles)
	deleted := 0
	for i := m.keepFiles; i < len(files); i++ {
		if err := os.Remove(files[i].path); err == nil {
			fmt.Printf("   🗑️  Deleted old file: %s\n", filepath.Base(files[i].path))
			deleted++
		}
	}

	if deleted > 0 {
		fmt.Printf("   🧹 Cleanup: Deleted %d old files, kept %d\n", deleted, m.keepFiles)
	}
}

// ================================================================================
// LEGACY FUNCTIONS (kept for compatibility)
// ================================================================================

// getLastTradingDate returns the most recent trading date (skipping weekends/holidays)
// If today is Saturday, returns Friday. If Sunday/Monday, returns previous Friday.
func getLastTradingDate() time.Time {
	now := time.Now()

	// If before 9 AM, use previous day (markets not open yet)
	if now.Hour() < 9 {
		now = now.AddDate(0, 0, -1)
	}

	// Skip weekends
	switch now.Weekday() {
	case time.Sunday:
		now = now.AddDate(0, 0, -2) // Sunday -> Friday
	case time.Saturday:
		now = now.AddDate(0, 0, -1) // Saturday -> Friday
	}

	return now
}

// getTradingDates returns last N trading dates for fallback (skipping weekends)
func getTradingDates(count int) []time.Time {
	var dates []time.Time
	current := getLastTradingDate()

	for len(dates) < count {
		// Skip weekends
		if current.Weekday() != time.Saturday && current.Weekday() != time.Sunday {
			dates = append(dates, current)
		}
		current = current.AddDate(0, 0, -1)
	}

	return dates
}

// fetchFromInternalAPI fetches file from internal API (Base64 encoded)
// Returns decoded CSV content
func fetchFromInternalAPI(date time.Time, feedType string) ([]byte, error) {
	// Date formats
	dateDD_MM_YYYY := date.Format("02-01-2006") // DD-MM-YYYY for path
	dateDDMMYYYY := date.Format("02012006")     // DDMMYYYY for filename
	dateYYYYMMDD := date.Format("20060102")     // YYYYMMDD for filename
	monthYear := fmt.Sprintf("%s-%d", getMonthName(date.Month()), date.Year())

	var filePath, fileName string

	if feedType == "EQ" {
		// BhavCopy: EQ/Common/DEC-2025/01-12-2025/BhavCopy_BSE_CM_0_0_0_20251201_F_0000.csv
		filePath = fmt.Sprintf("EQ/Common/%s/%s/BhavCopy_BSE_CM_0_0_0_%s_F_0000.csv", monthYear, dateDD_MM_YYYY, dateYYYYMMDD)
		fileName = fmt.Sprintf("BhavCopy_BSE_CM_0_0_0_%s_F_0000.csv", dateYYYYMMDD)
	} else {
		// Contract Master: FNO/Common/DEC-2025/01-12-2025/BSE_EQD_CONTRACT_01122025.csv
		filePath = fmt.Sprintf("FNO/Common/%s/%s/BSE_EQD_CONTRACT_%s.csv", monthYear, dateDD_MM_YYYY, dateDDMMYYYY)
		fileName = fmt.Sprintf("BSE_EQD_CONTRACT_%s.csv", dateDDMMYYYY)
	}

	fmt.Printf("   📡 API Request: %s\n", fileName)
	fmt.Printf("   📂 Path: %s\n", filePath)

	// Build request
	requestBody := map[string]string{
		"path":      filePath,
		"file_name": fileName,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request failed: %w", err)
	}

	// Make HTTP POST request
	client := &http.Client{Timeout: 30 * time.Second}

	url := InternalAPIURL + "?api_type=erp"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned HTTP %d", resp.StatusCode)
	}

	// Parse response
	var apiResp APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	if apiResp.Status != "success" {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	if apiResp.Data.FileContent == "" {
		return nil, fmt.Errorf("empty file content in response")
	}

	// Decode Base64
	csvBytes, err := base64.StdEncoding.DecodeString(apiResp.Data.FileContent)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}

	fmt.Printf("   ✅ Received %d bytes\n", len(csvBytes))
	return csvBytes, nil
}

// downloadFromInternalAPI downloads file from internal API with date fallbacks
// Tries multiple dates (skipping weekends/holidays) until successful
func downloadFromInternalAPI(outputDir string, feedType string, maxDays int) (string, error) {
	dates := getTradingDates(maxDays)

	fmt.Printf("📥 Fetching %s file from Internal API...\n", feedType)
	fmt.Printf("   API: %s\n", InternalAPIURL)
	fmt.Printf("   Trying last %d trading days (skipping weekends)\n\n", maxDays)

	for i, date := range dates {
		dateStr := date.Format("02-01-2006")
		fmt.Printf("🔄 Attempt %d/%d: %s (%s)\n", i+1, maxDays, dateStr, date.Weekday())

		// Generate output filename
		var outputFilename string
		if feedType == "EQ" {
			outputFilename = fmt.Sprintf("BhavCopy_BSE_CM_%s.csv", date.Format("02012006"))
		} else {
			outputFilename = fmt.Sprintf("BSE_EQD_CONTRACT_%s.csv", date.Format("02012006"))
		}
		outputPath := filepath.Join(outputDir, outputFilename)

		// Check if file already exists locally
		if info, err := os.Stat(outputPath); err == nil && info.Size() > 1000 {
			fmt.Printf("   ✅ Found cached file: %s (%d bytes)\n", outputFilename, info.Size())
			return outputPath, nil
		}

		// Fetch from API
		csvData, err := fetchFromInternalAPI(date, feedType)
		if err != nil {
			fmt.Printf("   ❌ Failed: %v\n\n", err)
			continue
		}

		// Create output directory
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return "", fmt.Errorf("create directory failed: %w", err)
		}

		// Save to file
		if err := os.WriteFile(outputPath, csvData, 0644); err != nil {
			return "", fmt.Errorf("write file failed: %w", err)
		}

		fmt.Printf("   💾 Saved: %s\n", outputPath)
		return outputPath, nil
	}

	return "", fmt.Errorf("failed to fetch file after trying %d dates", maxDays)
}

// downloadWithRetry downloads a file with multiple date fallbacks (BSE website fallback)
func downloadWithRetry(urlPattern string, outputDir string, feedType string, maxDays int) (string, error) {
	dates := getTradingDates(maxDays)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	for _, date := range dates {
		var url, filename string

		if feedType == "EQ" {
			// BhavCopy format: EQ_DDMMYY.CSV inside ZIP
			dateStr := date.Format("020106") // DDMMYY
			url = fmt.Sprintf("https://www.bseindia.com/download/BhavCopy/Equity/EQ%s_CSV.ZIP", dateStr)
			filename = fmt.Sprintf("BhavCopy_BSE_CM_%s.csv", date.Format("02012006"))
		} else {
			// Contract Master (FO) - single file
			url = ContractMasterURL
			filename = fmt.Sprintf("BSE_EQD_CONTRACT_%s.csv", date.Format("02012006"))
		}

		outputPath := filepath.Join(outputDir, filename)

		// Check if already exists
		if _, err := os.Stat(outputPath); err == nil {
			fmt.Printf("✅ Found cached: %s\n", outputPath)
			return outputPath, nil
		}

		fmt.Printf("📥 Trying: %s\n", url)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			continue
		}

		// Add headers to avoid 403
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		req.Header.Set("Accept", "text/csv,application/zip,*/*")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Referer", "https://www.bseindia.com/")

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("   ❌ Network error: %v\n", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			fmt.Printf("   ❌ HTTP %d for date %s\n", resp.StatusCode, date.Format("2006-01-02"))
			continue
		}

		// Create output directory
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			resp.Body.Close()
			return "", fmt.Errorf("create directory failed: %w", err)
		}

		// For BhavCopy (ZIP), we need to extract
		if feedType == "EQ" {
			data, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				continue
			}

			csvData, err := extractCSVFromZip(data)
			if err != nil {
				fmt.Printf("   ❌ ZIP extraction failed: %v\n", err)
				continue
			}

			if err := os.WriteFile(outputPath, csvData, 0644); err != nil {
				return "", fmt.Errorf("write file failed: %w", err)
			}
		} else {
			// Direct CSV download
			out, err := os.Create(outputPath)
			if err != nil {
				resp.Body.Close()
				return "", fmt.Errorf("create file failed: %w", err)
			}

			_, err = io.Copy(out, resp.Body)
			out.Close()
			resp.Body.Close()

			if err != nil {
				os.Remove(outputPath)
				continue
			}
		}

		fmt.Printf("✅ Downloaded: %s\n", outputPath)
		return outputPath, nil
	}

	return "", fmt.Errorf("failed to download after trying %d dates", maxDays)
}

// extractCSVFromZip extracts CSV from BhavCopy ZIP file
func extractCSVFromZip(zipData []byte) ([]byte, error) {
	reader := bytes.NewReader(zipData)
	zipReader, err := zip.NewReader(reader, int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("open zip failed: %w", err)
	}

	for _, file := range zipReader.File {
		if strings.HasSuffix(strings.ToUpper(file.Name), ".CSV") {
			rc, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("open file in zip failed: %w", err)
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("read file in zip failed: %w", err)
			}

			return data, nil
		}
	}

	return nil, fmt.Errorf("no CSV file found in ZIP")
}

// findLatestFile finds the most recent file matching a pattern in a directory
func findLatestFile(dir string, pattern string) (string, error) {
	var latestFile string
	var latestTime time.Time

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			return nil
		}

		matched, _ := filepath.Match(pattern, info.Name())
		if matched {
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestFile = path
			}
		}
		return nil
	})

	if err != nil {
		return "", err
	}
	if latestFile == "" {
		return "", fmt.Errorf("no file matching %s found in %s", pattern, dir)
	}

	return latestFile, nil
}

func downloadContractMaster(outputPath string) error {
	fmt.Printf("📥 Downloading Contract Master from BSE...\n")
	fmt.Printf("   URL: %s\n", ContractMasterURL)

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", ContractMasterURL, nil)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	// Add headers to avoid 403
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "text/csv,*/*")
	req.Header.Set("Referer", "https://www.bseindia.com/")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	// Create directory if not exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory failed: %w", err)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create file failed: %w", err)
	}
	defer out.Close()

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("write file failed: %w", err)
	}

	fmt.Printf("✅ Downloaded %d bytes to %s\n", written, outputPath)
	return nil
}

func loadContractMaster(csvPath string, tokenMap *TokenMap) error {
	fmt.Printf("📂 Loading Contract Master: %s\n", csvPath)

	file, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("open file failed: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Read header
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read header failed: %w", err)
	}

	// Find column indices
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.TrimSpace(col)] = i
	}

	// Required columns (BSE EQD CONTRACT format)
	// SctyId = Security ID = Token number
	// Contract Master columns from Internal API:
	// FinInstrmId = Token (e.g., 1117307)
	// TckrSymb = Underlying symbol (e.g., APLAPOLLO)
	// FinInstrmNm = Instrument type (e.g., SO = Stock Option, IO = Index Option, SF = Stock Futures, IF = Index Futures)
	// XpryDt = Expiry date (e.g., 24-DEC-2025)
	// StrkPric = Strike price in paise (e.g., 152000 = ₹1520)
	// OptnTp = Option type (CE or PE)
	// SctyLngNm = Full name (e.g., APLAPOLLO25DEC1520CE)
	// MinLot = Lot size
	tokenCol := findColumn(colIdx, "FinInstrmId", "SctyId", "ScripCode", "Token", "token", "InstrmId")
	symbolCol := findColumn(colIdx, "TckrSymb", "Symbol", "symbol") // Underlying symbol
	instNameCol := findColumn(colIdx, "SctyLngNm", "FinInstrmNm", "InstrumentName", "instrument_name")
	expiryCol := findColumn(colIdx, "XpryDt", "Expiry", "expiry")
	optTypeCol := findColumn(colIdx, "OptnTp", "OptionType", "option_type")
	strikeCol := findColumn(colIdx, "StrkPric", "StrikePrice", "strike_price")
	lotSizeCol := findColumn(colIdx, "MinLot", "NewBrdLotQty", "UnitOfMeasr", "LotSize", "lot_size")
	instTypeCol := findColumn(colIdx, "FinInstrmNm", "InstType") // SO, IO, SF, IF

	if tokenCol == -1 || symbolCol == -1 {
		return fmt.Errorf("required columns not found: Token=%d, Symbol=%d", tokenCol, symbolCol)
	}

	fmt.Printf("   Token column: %d (%s)\n", tokenCol, header[tokenCol])
	fmt.Printf("   Symbol column: %d (%s)\n", symbolCol, header[symbolCol])
	if instNameCol >= 0 {
		fmt.Printf("   Name column: %d (%s)\n", instNameCol, header[instNameCol])
	}

	count := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// Parse token
		tokenStr := ""
		if tokenCol < len(record) {
			tokenStr = strings.TrimSpace(record[tokenCol])
		}
		token, err := strconv.ParseUint(tokenStr, 10, 32)
		if err != nil || token == 0 {
			continue
		}

		// Build contract
		contract := &Contract{
			Token:   uint32(token),
			Segment: "FO",
		}

		if symbolCol >= 0 && symbolCol < len(record) {
			contract.Symbol = strings.TrimSpace(record[symbolCol])
		}
		if instNameCol >= 0 && instNameCol < len(record) {
			contract.InstrumentName = strings.TrimSpace(record[instNameCol])
		}
		if expiryCol >= 0 && expiryCol < len(record) {
			contract.Expiry = strings.TrimSpace(record[expiryCol])
		}
		if optTypeCol >= 0 && optTypeCol < len(record) {
			contract.OptionType = strings.TrimSpace(record[optTypeCol])
		}
		if strikeCol >= 0 && strikeCol < len(record) {
			// Strike is in paise, convert to rupees
			strikePaise := strings.TrimSpace(record[strikeCol])
			if strike, err := strconv.ParseFloat(strikePaise, 64); err == nil {
				contract.StrikePrice = fmt.Sprintf("%.2f", strike/100.0)
			} else {
				contract.StrikePrice = strikePaise
			}
		}
		if lotSizeCol >= 0 && lotSizeCol < len(record) {
			if lot, err := strconv.Atoi(strings.TrimSpace(record[lotSizeCol])); err == nil {
				contract.LotSize = lot
			}
		}
		// Get instrument type (SO, IO, SF, IF)
		if instTypeCol >= 0 && instTypeCol < len(record) {
			instType := strings.TrimSpace(record[instTypeCol])
			// Build descriptive symbol if we have underlying
			if contract.Symbol != "" && instType != "" {
				// e.g., SENSEX_CE_85000_04-DEC-2025 or SENSEX_FUT_04-DEC-2025
				if contract.OptionType != "" {
					contract.Symbol = fmt.Sprintf("%s_%s_%s_%s", contract.Symbol, contract.OptionType, contract.StrikePrice, contract.Expiry)
				} else if instType == "SF" || instType == "IF" {
					contract.Symbol = fmt.Sprintf("%s_FUT_%s", contract.Symbol, contract.Expiry)
				}
			}
		}

		tokenMap.Add(uint32(token), contract)
		count++
	}

	fmt.Printf("✅ Loaded %d contracts\n", count)
	return nil
}

func findColumn(colIdx map[string]int, names ...string) int {
	for _, name := range names {
		if idx, ok := colIdx[name]; ok {
			return idx
		}
	}
	return -1
}

// loadTokenDetailsJSON loads token mapping from token_details.json format
func loadTokenDetailsJSON(jsonPath string, tokenMap *TokenMap) error {
	fmt.Printf("📂 Loading Token Details JSON: %s\n", jsonPath)

	file, err := os.Open(jsonPath)
	if err != nil {
		return fmt.Errorf("open file failed: %w", err)
	}
	defer file.Close()

	// Parse JSON: {"token_id": {"symbol": "...", "ticker": "...", "expiry": "...", ...}}
	var data map[string]struct {
		Symbol     string `json:"symbol"`
		Ticker     string `json:"ticker"`
		Expiry     string `json:"expiry"`
		Strike     string `json:"strike"`
		OptionType string `json:"option_type"`
	}

	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return fmt.Errorf("JSON decode failed: %w", err)
	}

	count := 0
	for tokenStr, info := range data {
		token, err := strconv.ParseUint(tokenStr, 10, 32)
		if err != nil || token == 0 {
			continue
		}

		contract := &Contract{
			Token:          uint32(token),
			Symbol:         info.Ticker, // Use ticker as symbol
			InstrumentName: info.Symbol, // Full instrument name
			Expiry:         info.Expiry,
			OptionType:     info.OptionType,
			StrikePrice:    info.Strike,
			Segment:        "FO",
		}

		tokenMap.Add(uint32(token), contract)
		count++
	}

	fmt.Printf("✅ Loaded %d tokens from JSON\n", count)
	return nil
}

// loadBhavCopy loads equity token mapping from BhavCopy CSV
// BhavCopy format from Internal API:
// TradDt,BizDt,Sgmt,Src,FinInstrmTp,FinInstrmId,ISIN,TckrSymb,SctySrs,...
// Where: FinInstrmId = Token, TckrSymb = Symbol, FinInstrmNm = Full Name
func loadBhavCopy(csvPath string, tokenMap *TokenMap) error {
	fmt.Printf("📂 Loading BhavCopy: %s\n", csvPath)

	file, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("open file failed: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	// Read header
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("read header failed: %w", err)
	}

	// Find column indices
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.TrimSpace(col)] = i
	}

	// BhavCopy columns (Internal API format)
	// Primary: FinInstrmId (Token), TckrSymb (Symbol), FinInstrmNm (Full Name)
	// Fallback: SC_CODE, SC_NAME for older BSE format
	tokenCol := findColumn(colIdx, "FinInstrmId", "SC_CODE", "ScripCode", "Token", "SCRIP_CD")
	symbolCol := findColumn(colIdx, "TckrSymb", "SC_NAME", "Symbol", "SCRIP_NAME")
	nameCol := findColumn(colIdx, "FinInstrmNm", "SC_NAME", "Name", "SCRIP_NAME")
	groupCol := findColumn(colIdx, "SctySrs", "SC_GROUP", "Group", "SCRIP_GRP")

	if tokenCol == -1 || symbolCol == -1 {
		// Debug: print available columns
		fmt.Printf("   Available columns: %v\n", header)
		return fmt.Errorf("required columns not found: Token=%d, Symbol=%d", tokenCol, symbolCol)
	}

	fmt.Printf("   Token column: %d (%s)\n", tokenCol, header[tokenCol])
	fmt.Printf("   Symbol column: %d (%s)\n", symbolCol, header[symbolCol])

	count := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// Parse token
		tokenStr := ""
		if tokenCol < len(record) {
			tokenStr = strings.TrimSpace(record[tokenCol])
		}
		token, err := strconv.ParseUint(tokenStr, 10, 32)
		if err != nil || token == 0 {
			continue
		}

		// Build contract for equity
		contract := &Contract{
			Token:   uint32(token),
			Segment: "EQ",
		}

		if symbolCol >= 0 && symbolCol < len(record) {
			contract.Symbol = strings.TrimSpace(record[symbolCol])
		}
		if nameCol >= 0 && nameCol < len(record) {
			contract.InstrumentName = strings.TrimSpace(record[nameCol])
		}
		if groupCol >= 0 && groupCol < len(record) {
			contract.OptionType = strings.TrimSpace(record[groupCol]) // Use series/group as identifier
		}

		tokenMap.Add(uint32(token), contract)
		count++
	}

	fmt.Printf("✅ Loaded %d equity scripts from BhavCopy\n", count)

	// Show sample tokens
	if count > 0 {
		fmt.Printf("   Sample tokens loaded:\n")
		sampleCount := 0
		for token, contract := range tokenMap.contracts {
			if contract.Segment == "EQ" && sampleCount < 5 {
				fmt.Printf("      %d → %s (%s)\n", token, contract.Symbol, contract.InstrumentName)
				sampleCount++
			}
		}
	}

	return nil
}

// ================================================================================
// CSV SAVER
// ================================================================================

type CSVSaver struct {
	mu       sync.Mutex
	file     *os.File
	writer   *csv.Writer
	count    int
	filePath string
}

func NewCSVSaver(outputDir string) (*CSVSaver, error) {
	return NewCSVSaverWithFeed(outputDir, "FO")
}

func NewCSVSaverWithFeed(outputDir string, feedType string) (*CSVSaver, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, err
	}

	dateStr := time.Now().Format("20060102")
	filePath := filepath.Join(outputDir, fmt.Sprintf("%s_%s_quotes.csv", dateStr, feedType))

	// Check if file exists
	fileExists := false
	if _, err := os.Stat(filePath); err == nil {
		fileExists = true
	}

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	writer := csv.NewWriter(file)

	// Write header if new file
	if !fileExists {
		header := []string{
			"timestamp", "token", "symbol", "symbol_name", "expiry", "option_type", "strike_price",
			"ltp", "open", "high", "low", "prev_close",
			"volume", "lot_size", "seq",
			"bid_prices", "bid_qtys", "ask_prices", "ask_qtys",
		}
		if err := writer.Write(header); err != nil {
			file.Close()
			return nil, err
		}
		writer.Flush()
	}

	fmt.Printf("📝 CSV output: %s\n", filePath)

	return &CSVSaver{
		file:     file,
		writer:   writer,
		filePath: filePath,
	}, nil
}

func (s *CSVSaver) Save(quote *Quote) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := []string{
		quote.Timestamp,
		strconv.FormatUint(uint64(quote.Token), 10),
		quote.Symbol,
		quote.SymbolName,
		quote.Expiry,
		quote.OptionType,
		quote.StrikePrice,
		fmt.Sprintf("%.2f", quote.LTP),
		fmt.Sprintf("%.2f", quote.Open),
		fmt.Sprintf("%.2f", quote.High),
		fmt.Sprintf("%.2f", quote.Low),
		fmt.Sprintf("%.2f", quote.PrevClose),
		strconv.FormatInt(quote.Volume, 10),
		strconv.Itoa(quote.LotSize),
		strconv.FormatUint(uint64(quote.SequenceNum), 10),
		quote.BidPrices,
		quote.BidQtys,
		quote.AskPrices,
		quote.AskQtys,
	}

	if err := s.writer.Write(row); err != nil {
		return err
	}

	s.count++
	if s.count%100 == 0 {
		s.writer.Flush()
	}

	return nil
}

func (s *CSVSaver) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writer.Flush()
	return s.file.Close()
}

func (s *CSVSaver) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

// ================================================================================
// PACKET DECODER
// ================================================================================

func decodePacket(packet []byte, tokenMap *TokenMap, stats *Stats, timestamp string) ([]*Quote, bool) {
	if len(packet) < HeaderSize {
		return nil, false
	}

	// Parse header - DON'T validate formatID (BSE uses 0x0124, 0x022C, 0x0234, etc.)
	// formatID := binary.BigEndian.Uint16(packet[4:6])
	msgType := binary.LittleEndian.Uint16(packet[8:10])

	// Only validate message type (2020=EQ, 2021=FO)
	if msgType != 2020 && msgType != 2021 {
		return nil, false
	}

	// Calculate records - each record is 264 bytes
	dataLen := len(packet) - HeaderSize
	numRecords := dataLen / RecordSize
	if numRecords > MaxRecords {
		numRecords = MaxRecords
	}
	if numRecords == 0 {
		return nil, false
	}

	var quotes []*Quote
	offset := HeaderSize

	for i := 0; i < numRecords; i++ {
		if offset+RecordSize > len(packet) {
			break
		}

		rec := packet[offset : offset+RecordSize]

		// Token at offset 0-3 (Little-Endian)
		token := binary.LittleEndian.Uint32(rec[0:4])

		if token == 0 {
			offset += RecordSize
			continue
		}

		// CONFIRMED OFFSETS (from Python decoder - all Little-Endian, in paise):
		// Offset 4-7:   Open Price
		// Offset 8-11:  Prev Close
		// Offset 12-15: High Price
		// Offset 16-19: Low Price
		// Offset 24-27: Volume
		// Offset 36-39: LTP (Last Traded Price)
		// Offset 44-47: Sequence Number

		openPrice := float64(int32(binary.LittleEndian.Uint32(rec[4:8]))) / 100.0
		prevClose := float64(int32(binary.LittleEndian.Uint32(rec[8:12]))) / 100.0
		highPrice := float64(int32(binary.LittleEndian.Uint32(rec[12:16]))) / 100.0
		lowPrice := float64(int32(binary.LittleEndian.Uint32(rec[16:20]))) / 100.0
		volume := int64(int32(binary.LittleEndian.Uint32(rec[24:28])))
		ltp := float64(int32(binary.LittleEndian.Uint32(rec[36:40]))) / 100.0
		seqNum := binary.LittleEndian.Uint32(rec[44:48])

		// Build quote
		quote := &Quote{
			Timestamp:   timestamp,
			Token:       token,
			LTP:         ltp,
			Open:        openPrice,
			High:        highPrice,
			Low:         lowPrice,
			PrevClose:   prevClose,
			Volume:      volume,
			SequenceNum: seqNum,
		}

		// Lookup contract details
		if contract, ok := tokenMap.Get(token); ok {
			quote.Symbol = contract.Symbol
			quote.SymbolName = contract.InstrumentName
			quote.Expiry = contract.Expiry
			quote.OptionType = contract.OptionType
			quote.StrikePrice = contract.StrikePrice
			quote.LotSize = contract.LotSize
		} else {
			quote.Symbol = fmt.Sprintf("TOKEN_%d", token)
			quote.SymbolName = ""
			// Track missed token
			if stats != nil {
				stats.TrackMissedToken(token)
			}
		}

		// Best 5 bid/ask levels (starting at offset 104)
		if len(rec) >= 264 {
			bidPrices, bidQtys, askPrices, askQtys := decodeOrderBook(rec[104:264], ltp)
			quote.BidPrices = bidPrices
			quote.BidQtys = bidQtys
			quote.AskPrices = askPrices
			quote.AskQtys = askQtys
		}

		quotes = append(quotes, quote)
		offset += RecordSize
	}

	return quotes, len(quotes) > 0
}

// decodeOrderBook decodes the 5-level order book from offset 104
// Structure: 16 bytes per level (Price 4B + Qty 4B + Flag 4B + Unknown 4B)
// Interleaved: Bid1, Ask1, Bid2, Ask2, Bid3, Ask3, Bid4, Ask4, Bid5, Ask5
func decodeOrderBook(data []byte, basePrice float64) (string, string, string, string) {
	if len(data) < 160 {
		return "", "", "", ""
	}

	var bidPrices, bidQtys, askPrices, askQtys []string

	// 5 levels, each level has Bid (16 bytes) + Ask (16 bytes)
	for i := 0; i < 5; i++ {
		bidOffset := i * 32    // Bid at even positions
		askOffset := i*32 + 16 // Ask at odd positions

		// Bid level
		if bidOffset+16 <= len(data) {
			bidPrice := float64(int32(binary.LittleEndian.Uint32(data[bidOffset:bidOffset+4]))) / 100.0
			bidQty := int32(binary.LittleEndian.Uint32(data[bidOffset+4 : bidOffset+8]))
			if bidPrice > 0 {
				bidPrices = append(bidPrices, fmt.Sprintf("%.2f", bidPrice))
				bidQtys = append(bidQtys, strconv.Itoa(int(bidQty)))
			}
		}

		// Ask level
		if askOffset+16 <= len(data) {
			askPrice := float64(int32(binary.LittleEndian.Uint32(data[askOffset:askOffset+4]))) / 100.0
			askQty := int32(binary.LittleEndian.Uint32(data[askOffset+4 : askOffset+8]))
			if askPrice > 0 {
				askPrices = append(askPrices, fmt.Sprintf("%.2f", askPrice))
				askQtys = append(askQtys, strconv.Itoa(int(askQty)))
			}
		}
	}

	return strings.Join(bidPrices, "|"), strings.Join(bidQtys, "|"),
		strings.Join(askPrices, "|"), strings.Join(askQtys, "|")
}

// ================================================================================
// MULTICAST RECEIVER
// ================================================================================

type Receiver struct {
	ip       string
	port     int
	conn     *net.UDPConn
	tokenMap *TokenMap
	saver    *CSVSaver
	stats    *Stats
}

func NewReceiver(ip string, port int, tokenMap *TokenMap, saver *CSVSaver, stats *Stats) *Receiver {
	return &Receiver{
		ip:       ip,
		port:     port,
		tokenMap: tokenMap,
		saver:    saver,
		stats:    stats,
	}
}

func (r *Receiver) Connect() error {
	addr := fmt.Sprintf("%s:%d", r.ip, r.port)
	gaddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return err
	}

	conn, err := net.ListenMulticastUDP("udp4", nil, gaddr)
	if err != nil {
		return err
	}
	r.conn = conn

	// Large receive buffer
	conn.SetReadBuffer(16 * 1024 * 1024)

	// Set multicast options
	pconn := ipv4.NewPacketConn(conn)
	pconn.SetControlMessage(ipv4.FlagTTL|ipv4.FlagDst, true)

	return nil
}

func (r *Receiver) ReceiveLoop(ctx context.Context) {
	buffer := make([]byte, BufferSize)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		r.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, _, err := r.conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}

		r.stats.TotalPackets.Add(1)
		r.stats.TotalBytes.Add(uint64(n))

		if n < HeaderSize {
			continue
		}

		// Timing
		decodeStart := time.Now()
		timestamp := time.Now().Format("2006-01-02 15:04:05.000")

		// Decode (pass stats for missed token tracking)
		quotes, valid := decodePacket(buffer[:n], r.tokenMap, r.stats, timestamp)

		decodeTime := time.Since(decodeStart).Nanoseconds()
		r.stats.RecordLatency(decodeTime)

		if valid {
			r.stats.ValidPackets.Add(1)
			r.stats.TotalRecords.Add(uint64(len(quotes)))

			// Save to CSV
			for _, quote := range quotes {
				if quote.LTP > 0 {
					if err := r.saver.Save(quote); err == nil {
						r.stats.QuotesSaved.Add(1)
					}
				}
			}
		}
	}
}

func (r *Receiver) Close() error {
	if r.conn != nil {
		return r.conn.Close()
	}
	return nil
}

// ================================================================================
// REPORTING
// ================================================================================

func printLiveStats(stats *Stats, tokenMap *TokenMap, saver *CSVSaver, elapsed time.Duration) {
	packets := stats.TotalPackets.Load()
	records := stats.TotalRecords.Load()
	saved := stats.QuotesSaved.Load()
	missed := stats.MissedTokenCount.Load()
	secs := elapsed.Seconds()

	var pps, rps float64
	if secs > 0 {
		pps = float64(packets) / secs
		rps = float64(records) / secs
	}

	fmt.Printf("\r[%s] Pkts: %d (%.0f/s) | Records: %d (%.0f/s) | Saved: %d | Missed: %d | Tokens: %d   ",
		elapsed.Round(time.Second),
		packets, pps,
		records, rps,
		saved,
		missed,
		tokenMap.Len(),
	)
}

func printFinalReport(stats *Stats, tokenMap *TokenMap, saver *CSVSaver) {
	duration := time.Since(stats.startTime)
	secs := duration.Seconds()

	packets := stats.TotalPackets.Load()
	records := stats.TotalRecords.Load()
	saved := stats.QuotesSaved.Load()
	bytes := stats.TotalBytes.Load()

	var pps, rps, mbps float64
	if secs > 0 {
		pps = float64(packets) / secs
		rps = float64(records) / secs
		mbps = float64(bytes) / secs / (1024 * 1024)
	}

	percentiles := stats.GetPercentiles()

	decodeCount := stats.DecodeCount.Load()
	var avgLatency float64
	if decodeCount > 0 {
		avgLatency = float64(stats.DecodeSum.Load()) / float64(decodeCount) / 1000.0
	}

	peakMem := float64(stats.PeakMemory.Load()) / (1024 * 1024)

	fmt.Println("\n")
	fmt.Println("████████████████████████████████████████████████████████████████████████████████")
	fmt.Println("█                    BSE HFT SERVER - FINAL REPORT                            █")
	fmt.Println("████████████████████████████████████████████████████████████████████████████████")
	fmt.Println()

	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ SESSION SUMMARY                                                             │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Duration:              %-54s│\n", duration.Round(time.Millisecond))
	fmt.Printf("│ Total Packets:         %-54d│\n", packets)
	fmt.Printf("│ Total Records:         %-54d│\n", records)
	fmt.Printf("│ Quotes Saved:          %-54d│\n", saved)
	fmt.Printf("│ Tokens in Master:      %-54d│\n", tokenMap.Len())
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ THROUGHPUT                                                                  │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Packets/sec:           %-54.2f│\n", pps)
	fmt.Printf("│ Records/sec:           %-54.2f│\n", rps)
	fmt.Printf("│ Data Rate:             %-54s│\n", fmt.Sprintf("%.3f MB/s", mbps))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ LATENCY (Decode + Save)                                                     │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Average:               %-54s│\n", fmt.Sprintf("%.2f µs", avgLatency))
	fmt.Printf("│ P50 (Median):          %-54s│\n", fmt.Sprintf("%.2f µs", percentiles.P50))
	fmt.Printf("│ P90:                   %-54s│\n", fmt.Sprintf("%.2f µs", percentiles.P90))
	fmt.Printf("│ P99:                   %-54s│\n", fmt.Sprintf("%.2f µs", percentiles.P99))
	fmt.Printf("│ P99.9:                 %-54s│\n", fmt.Sprintf("%.2f µs", percentiles.P999))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ MEMORY                                                                      │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Peak Memory:           %-54s│\n", fmt.Sprintf("%.2f MB", peakMem))
	fmt.Printf("│ GOMAXPROCS:            %-54d│\n", runtime.GOMAXPROCS(0))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ OUTPUT                                                                      │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ CSV File:              %-54s│\n", saver.filePath)
	fmt.Printf("│ Rows Written:          %-54d│\n", saver.Count())
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	// Missed Tokens Section
	missedCount := stats.MissedTokenCount.Load()
	stats.MissedTokensMu.Lock()
	uniqueMissed := len(stats.MissedTokens)
	stats.MissedTokensMu.Unlock()

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ MISSED TOKENS (Not in Token Master)                                         │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Total Missed:          %-54d│\n", missedCount)
	fmt.Printf("│ Unique Tokens:         %-54d│\n", uniqueMissed)
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")

	if uniqueMissed > 0 {
		topMissed := stats.GetMissedTokensSummary(10) // Top 10 missed tokens
		fmt.Println("│ Top Missed Tokens (Token → Count):                                          │")
		for i, mt := range topMissed {
			line := fmt.Sprintf("   %2d. Token %-12d → %d occurrences", i+1, mt.Token, mt.Count)
			fmt.Printf("│ %-75s│\n", line)
		}
	} else {
		fmt.Println("│ ✅ All tokens found in master file!                                         │")
	}
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("████████████████████████████████████████████████████████████████████████████████")
}

// ================================================================================
// MAIN
// ================================================================================

func main() {
	// Flags
	configFile := flag.String("config", "", "Config file path (enables dual-feed mode from config.json)")
	multicastIP := flag.String("ip", DefaultMulticastIP, "Multicast IP address")
	port := flag.Int("port", 0, "UDP port (0 = auto-detect based on feed type)")
	feedType := flag.String("feed", "", "Feed type: EQ, FO, or BOTH (default: read from config or BOTH)")
	duration := flag.Duration("duration", 0, "Run duration (0 = until Ctrl+C)")
	dataDir := flag.String("data", "./data", "Data directory")
	contractFile := flag.String("contracts", "", "Contract master CSV or BhavCopy file")
	flag.Parse()

	// Try to load config file
	var config *Config
	configPath := *configFile
	if configPath == "" {
		// Try default locations
		defaultPaths := []string{"config.json", "../config.json", "../../config.json", "./data/config.json"}
		for _, p := range defaultPaths {
			if _, err := os.Stat(p); err == nil {
				configPath = p
				break
			}
		}
	}

	if configPath != "" {
		var err error
		config, err = loadConfig(configPath)
		if err != nil {
			fmt.Printf("⚠️  Could not load config file: %v\n", err)
			config = nil
		} else {
			fmt.Printf("📂 Loaded config: %s\n", configPath)
		}
	}

	// Determine which feeds to enable
	enableEQ := false
	enableFO := false

	feedTypeName := strings.ToUpper(*feedType)

	if feedTypeName == "BOTH" || feedTypeName == "" {
		// Check config or default to both
		if config != nil {
			enableEQ = config.Segments.CMEnabled
			enableFO = config.Segments.FOEnabled
		} else {
			// Default: enable both if no feed specified
			enableEQ = true
			enableFO = true
		}
	} else if feedTypeName == "EQ" {
		enableEQ = true
	} else if feedTypeName == "FO" {
		enableFO = true
	} else {
		enableFO = true // Default to FO
	}

	// Get multicast settings
	eqIP := *multicastIP
	eqPort := DefaultEQPort
	foIP := *multicastIP
	foPort := DefaultFOPort

	if config != nil {
		if config.MulticastCM.IP != "" {
			eqIP = config.MulticastCM.IP
		}
		if config.MulticastCM.Port != 0 {
			eqPort = config.MulticastCM.Port
		}
		if config.MulticastFO.IP != "" {
			foIP = config.MulticastFO.IP
		}
		if config.MulticastFO.Port != 0 {
			foPort = config.MulticastFO.Port
		}
	}

	// Override with command line if specified
	if *port != 0 {
		if enableEQ && !enableFO {
			eqPort = *port
		} else {
			foPort = *port
		}
	}

	// Determine mode string
	modeStr := ""
	if enableEQ && enableFO {
		modeStr = "BOTH (EQ + FO)"
	} else if enableEQ {
		modeStr = "EQ (Equity/Cash)"
	} else {
		modeStr = "FO (F&O Derivatives)"
	}

	fmt.Println("================================================================================")
	fmt.Println("         BSE GO HFT SERVER - DUAL FEED PRODUCTION PIPELINE                     ")
	fmt.Println("================================================================================")
	fmt.Printf("Start Time:      %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("Mode:            %s\n", modeStr)
	if enableEQ {
		fmt.Printf("EQ Multicast:    %s:%d (Equity Cash)\n", eqIP, eqPort)
	}
	if enableFO {
		fmt.Printf("FO Multicast:    %s:%d (F&O Derivatives)\n", foIP, foPort)
	}
	fmt.Printf("Data Directory:  %s\n", *dataDir)
	fmt.Printf("GOMAXPROCS:      %d\n", runtime.GOMAXPROCS(0))
	if *duration > 0 {
		fmt.Printf("Duration:        %s\n", *duration)
	} else {
		fmt.Printf("Duration:        Until Ctrl+C\n")
	}
	fmt.Println("================================================================================")
	fmt.Println()

	// 1. TOKEN MAP (shared between both feeds)
	tokenMap := NewTokenMap()

	// 2. DOWNLOAD BOTH TOKEN FILES (using TokenFileManager)
	contractPath := *contractFile
	tokensDir := filepath.Join(*dataDir, "tokens")
	os.MkdirAll(tokensDir, 0755)

	// Initialize TokenFileManager (Python-like logic)
	tokenMgr := NewTokenFileManager(tokensDir)

	fmt.Println()
	fmt.Println("================================================================================")
	fmt.Println("📥 TOKEN FILE MANAGEMENT (Python-like logic)")
	fmt.Println("================================================================================")
	fmt.Printf("   API: %s\n", InternalAPIURL)
	fmt.Printf("   Target Date: %s (previous trading day)\n", tokenMgr.getTargetDate().Format("02-01-2006"))
	fmt.Printf("   Retry Logic: %d attempts with %v delay\n", tokenMgr.maxRetries, tokenMgr.retryDelay)
	fmt.Printf("   Cleanup: Keep last %d files\n", tokenMgr.keepFiles)
	fmt.Println()

	// DOWNLOAD BHAVCOPY (EQ)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📊 [1/2] BHAVCOPY (Equity Cash - Port 26001)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	eqLoaded := false
	if eqPath, err := tokenMgr.GetLatestFile("EQ"); err == nil {
		if err := loadBhavCopy(eqPath, tokenMap); err == nil {
			eqLoaded = true
		} else {
			fmt.Printf("   ⚠️  Failed to parse BhavCopy: %v\n", err)
		}
	} else {
		fmt.Printf("   ⚠️  BhavCopy not available: %v\n", err)
	}
	// Cleanup old BhavCopy files
	tokenMgr.CleanupOldFiles("EQ")
	fmt.Println()

	// DOWNLOAD CONTRACT MASTER (FO)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("📊 [2/2] CONTRACT MASTER (F&O Derivatives - Port 26002)")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	foLoaded := false
	// First check if user specified a contract file
	if contractPath != "" {
		if err := loadContractMaster(contractPath, tokenMap); err == nil {
			foLoaded = true
			fmt.Printf("   ✅ Loaded user-specified Contract Master: %s\n", contractPath)
		} else {
			fmt.Printf("   ⚠️  Failed to load user-specified file: %v\n", err)
		}
	}

	// If not loaded yet, use TokenFileManager
	if !foLoaded {
		if foPath, err := tokenMgr.GetLatestFile("FO"); err == nil {
			if err := loadContractMaster(foPath, tokenMap); err == nil {
				foLoaded = true
			} else {
				fmt.Printf("   ⚠️  Failed to parse Contract Master: %v\n", err)
			}
		} else {
			fmt.Printf("   ⚠️  Contract Master not available: %v\n", err)
		}
	}
	// Cleanup old Contract Master files
	tokenMgr.CleanupOldFiles("FO")

	fmt.Println()
	fmt.Println("================================================================================")
	fmt.Println("📋 TOKEN MAPPING SUMMARY")
	fmt.Println("================================================================================")
	fmt.Printf("✅ Total tokens loaded: %d\n", tokenMap.Len())
	if eqLoaded {
		fmt.Printf("   ✅ BhavCopy (EQ): Loaded\n")
	} else {
		fmt.Printf("   ❌ BhavCopy (EQ): Not available\n")
	}
	if foLoaded {
		fmt.Printf("   ✅ Contract Master (FO): Loaded\n")
	} else {
		fmt.Printf("   ❌ Contract Master (FO): Not available\n")
	}
	fmt.Println()

	// 3. CREATE CSV SAVERS AND RECEIVERS
	csvDir := filepath.Join(*dataDir, "processed_csv")

	// Shared stats
	stats := NewStats()

	// Context
	ctx, cancel := context.WithCancel(context.Background())
	if *duration > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), *duration)
	}
	defer cancel()

	// Signal handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	var eqSaver, foSaver *CSVSaver
	var eqReceiver, foReceiver *Receiver

	// Create EQ receiver if enabled
	if enableEQ {
		var err error
		eqSaver, err = NewCSVSaverWithFeed(csvDir, "EQ")
		if err != nil {
			log.Fatalf("❌ Failed to create EQ CSV saver: %v", err)
		}
		defer eqSaver.Close()

		eqReceiver = NewReceiver(eqIP, eqPort, tokenMap, eqSaver, stats)
		fmt.Printf("🔌 Connecting to EQ feed (%s:%d)...\n", eqIP, eqPort)
		if err := eqReceiver.Connect(); err != nil {
			log.Fatalf("❌ Failed to connect to EQ feed: %v", err)
		}
		defer eqReceiver.Close()
		fmt.Println("   ✅ EQ Connected!")
	}

	// Create FO receiver if enabled
	if enableFO {
		var err error
		foSaver, err = NewCSVSaverWithFeed(csvDir, "FO")
		if err != nil {
			log.Fatalf("❌ Failed to create FO CSV saver: %v", err)
		}
		defer foSaver.Close()

		foReceiver = NewReceiver(foIP, foPort, tokenMap, foSaver, stats)
		fmt.Printf("🔌 Connecting to FO feed (%s:%d)...\n", foIP, foPort)
		if err := foReceiver.Connect(); err != nil {
			log.Fatalf("❌ Failed to connect to FO feed: %v", err)
		}
		defer foReceiver.Close()
		fmt.Println("   ✅ FO Connected!")
	}

	fmt.Println()
	fmt.Println("✅ All feeds connected! Receiving packets...")
	fmt.Println()

	// Start receivers in goroutines
	if enableEQ {
		go eqReceiver.ReceiveLoop(ctx)
	}
	if enableFO {
		go foReceiver.ReceiveLoop(ctx)
	}

	// Handle interrupt
	go func() {
		<-sigChan
		fmt.Println("\n\n⚠️  Interrupt received, generating report...")
		cancel()
	}()

	// Stats ticker
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			// Print final report
			printDualFeedReport(stats, tokenMap, eqSaver, foSaver, enableEQ, enableFO)
			return

		case <-ticker.C:
			stats.SampleMemory()
			printDualLiveStats(stats, tokenMap, eqSaver, foSaver, enableEQ, enableFO, time.Since(startTime))
		}
	}
}

// printDualLiveStats prints live stats for dual-feed mode
func printDualLiveStats(stats *Stats, tokenMap *TokenMap, eqSaver, foSaver *CSVSaver, enableEQ, enableFO bool, elapsed time.Duration) {
	packets := stats.TotalPackets.Load()
	records := stats.TotalRecords.Load()
	missed := stats.MissedTokenCount.Load()
	secs := elapsed.Seconds()

	var pps, rps float64
	if secs > 0 {
		pps = float64(packets) / secs
		rps = float64(records) / secs
	}

	eqCount := 0
	foCount := 0
	if eqSaver != nil {
		eqCount = eqSaver.Count()
	}
	if foSaver != nil {
		foCount = foSaver.Count()
	}

	fmt.Printf("\r[%s] Pkts: %d (%.0f/s) | Records: %d (%.0f/s) | EQ: %d | FO: %d | Missed: %d | Tokens: %d   ",
		elapsed.Round(time.Second),
		packets, pps,
		records, rps,
		eqCount, foCount,
		missed,
		tokenMap.Len(),
	)
}

// printDualFeedReport prints final report for dual-feed mode
func printDualFeedReport(stats *Stats, tokenMap *TokenMap, eqSaver, foSaver *CSVSaver, enableEQ, enableFO bool) {
	duration := time.Since(stats.startTime)
	secs := duration.Seconds()

	packets := stats.TotalPackets.Load()
	records := stats.TotalRecords.Load()
	saved := stats.QuotesSaved.Load()
	bytes := stats.TotalBytes.Load()

	var pps, rps, mbps float64
	if secs > 0 {
		pps = float64(packets) / secs
		rps = float64(records) / secs
		mbps = float64(bytes) / secs / (1024 * 1024)
	}

	percentiles := stats.GetPercentiles()

	decodeCount := stats.DecodeCount.Load()
	var avgLatency float64
	if decodeCount > 0 {
		avgLatency = float64(stats.DecodeSum.Load()) / float64(decodeCount) / 1000.0
	}

	peakMem := float64(stats.PeakMemory.Load()) / (1024 * 1024)

	eqCount := 0
	foCount := 0
	eqFile := "N/A"
	foFile := "N/A"
	if eqSaver != nil {
		eqCount = eqSaver.Count()
		eqFile = eqSaver.filePath
	}
	if foSaver != nil {
		foCount = foSaver.Count()
		foFile = foSaver.filePath
	}

	fmt.Println("\n")
	fmt.Println("████████████████████████████████████████████████████████████████████████████████")
	fmt.Println("█                BSE HFT SERVER - DUAL FEED FINAL REPORT                      █")
	fmt.Println("████████████████████████████████████████████████████████████████████████████████")
	fmt.Println()

	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ SESSION SUMMARY                                                             │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Duration:              %-54s│\n", duration.Round(time.Millisecond))
	fmt.Printf("│ Total Packets:         %-54d│\n", packets)
	fmt.Printf("│ Total Records:         %-54d│\n", records)
	fmt.Printf("│ Quotes Saved:          %-54d│\n", saved)
	fmt.Printf("│ Tokens in Master:      %-54d│\n", tokenMap.Len())
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ FEED BREAKDOWN                                                              │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	if enableEQ {
		fmt.Printf("│ EQ (Equity Cash):      %-54s│\n", fmt.Sprintf("%d quotes", eqCount))
	}
	if enableFO {
		fmt.Printf("│ FO (F&O Derivatives):  %-54s│\n", fmt.Sprintf("%d quotes", foCount))
	}
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ THROUGHPUT                                                                  │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Packets/sec:           %-54.2f│\n", pps)
	fmt.Printf("│ Records/sec:           %-54.2f│\n", rps)
	fmt.Printf("│ Data Rate:             %-54s│\n", fmt.Sprintf("%.3f MB/s", mbps))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ LATENCY (Decode + Save)                                                     │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Average:               %-54s│\n", fmt.Sprintf("%.2f µs", avgLatency))
	fmt.Printf("│ P50 (Median):          %-54s│\n", fmt.Sprintf("%.2f µs", percentiles.P50))
	fmt.Printf("│ P90:                   %-54s│\n", fmt.Sprintf("%.2f µs", percentiles.P90))
	fmt.Printf("│ P99:                   %-54s│\n", fmt.Sprintf("%.2f µs", percentiles.P99))
	fmt.Printf("│ P99.9:                 %-54s│\n", fmt.Sprintf("%.2f µs", percentiles.P999))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ MEMORY                                                                      │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Peak Memory:           %-54s│\n", fmt.Sprintf("%.2f MB", peakMem))
	fmt.Printf("│ GOMAXPROCS:            %-54d│\n", runtime.GOMAXPROCS(0))
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ OUTPUT FILES                                                                │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	if enableEQ {
		fmt.Printf("│ EQ CSV:                %-54s│\n", filepath.Base(eqFile))
		fmt.Printf("│ EQ Rows:               %-54d│\n", eqCount)
	}
	if enableFO {
		fmt.Printf("│ FO CSV:                %-54s│\n", filepath.Base(foFile))
		fmt.Printf("│ FO Rows:               %-54d│\n", foCount)
	}
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	// Missed Tokens Section
	missedCount := stats.MissedTokenCount.Load()
	stats.MissedTokensMu.Lock()
	uniqueMissed := len(stats.MissedTokens)
	stats.MissedTokensMu.Unlock()

	fmt.Println()
	fmt.Println("┌─────────────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│ MISSED TOKENS (Not in Token Master)                                         │")
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")
	fmt.Printf("│ Total Missed:          %-54d│\n", missedCount)
	fmt.Printf("│ Unique Tokens:         %-54d│\n", uniqueMissed)
	fmt.Println("├─────────────────────────────────────────────────────────────────────────────┤")

	if uniqueMissed > 0 {
		topMissed := stats.GetMissedTokensSummary(10) // Top 10 missed tokens
		fmt.Println("│ Top Missed Tokens (Token → Count):                                          │")
		for i, mt := range topMissed {
			line := fmt.Sprintf("   %2d. Token %-12d → %d occurrences", i+1, mt.Token, mt.Count)
			fmt.Printf("│ %-75s│\n", line)
		}
	} else {
		fmt.Println("│ ✅ All tokens found in master file!                                         │")
	}
	fmt.Println("└─────────────────────────────────────────────────────────────────────────────┘")

	fmt.Println()
	fmt.Println("████████████████████████████████████████████████████████████████████████████████")
}
