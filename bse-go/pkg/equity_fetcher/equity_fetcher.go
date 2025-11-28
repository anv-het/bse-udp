// Package equity_fetcher manages BhavCopy (Equity Cash) files.
// It auto-downloads from BSE API and cleans up old files.
package equity_fetcher

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bse-go/pkg/contract_fetcher"
)

// EquityInfo holds Equity Cash (CM) details
type EquityInfo struct {
	Token  string
	Symbol string // TckrSymb (e.g., RELIANCE)
	Name   string // FinInstrmNm (e.g., RELIANCE INDUSTRIES LTD.)
}

// Fetcher handles BhavCopy file management
type Fetcher struct {
	fetcher   *contract_fetcher.Fetcher
	dataDir   string
	contracts map[string]EquityInfo // token → info
}

// NewFetcher creates a new equity fetcher
func NewFetcher(apiURL, dataDir string) *Fetcher {
	if dataDir == "" {
		dataDir = "data/tokens"
	}
	return &Fetcher{
		fetcher:   contract_fetcher.NewFetcher(apiURL),
		dataDir:   dataDir,
		contracts: make(map[string]EquityInfo),
	}
}

// UpdateContracts fetches latest BhavCopy file and cleans up old ones
func (f *Fetcher) UpdateContracts(date time.Time, keepDays int) error {
	// Build API path
	path, fileName := contract_fetcher.BuildCMPath(date)

	// Output filename: BhavCopy_BSE_CM_YYYYMMDD.csv
	dateStr := date.Format("20060102")
	outputFileName := fmt.Sprintf("BhavCopy_BSE_CM_%s.csv", dateStr)
	outputPath := filepath.Join(f.dataDir, outputFileName)

	// Check if file already exists
	if _, err := os.Stat(outputPath); err == nil {
		log.Printf("✅ BhavCopy file already exists: %s", outputPath)
	} else {
		// Download new file
		log.Printf("📥 Downloading BhavCopy file: %s/%s", path, fileName)
		if err := f.fetcher.FetchAndSave(path, fileName, outputPath); err != nil {
			return fmt.Errorf("failed to download BhavCopy file: %w", err)
		}
		log.Printf("✅ Downloaded: %s", outputPath)
	}

	// Cleanup old files
	f.deleteOldFiles(keepDays)

	// Load contracts into memory
	return f.loadContracts(outputPath)
}

// LoadFromCache loads BhavCopy from existing CSV files without API fetch
func (f *Fetcher) LoadFromCache() error {
	// Find latest BhavCopy file
	pattern := filepath.Join(f.dataDir, "BhavCopy_BSE_CM_*.csv")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob files: %w", err)
	}

	if len(matches) == 0 {
		return fmt.Errorf("no BhavCopy files found in %s", f.dataDir)
	}

	// Sort to get latest (by filename date)
	sort.Strings(matches)
	latestFile := matches[len(matches)-1]

	log.Printf("📂 Loading BhavCopy from: %s", latestFile)
	return f.loadContracts(latestFile)
}

// loadContracts parses CSV and loads into memory
func (f *Fetcher) loadContracts(csvPath string) error {
	file, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read CSV: %w", err)
	}

	if len(records) < 2 {
		return fmt.Errorf("CSV file is empty or has no data rows")
	}

	// Build column index map from header
	header := records[0]
	colIndex := make(map[string]int)
	for i, col := range header {
		colIndex[strings.TrimSpace(col)] = i
	}

	// Required columns for BhavCopy
	// FinInstrmId: token
	// TckrSymb: symbol (e.g., RELIANCE)
	// FinInstrmNm: full name (e.g., RELIANCE INDUSTRIES LTD.)

	tokenIdx, ok1 := colIndex["FinInstrmId"]
	symbolIdx, ok2 := colIndex["TckrSymb"]
	nameIdx, ok3 := colIndex["FinInstrmNm"]

	if !ok1 || !ok2 {
		return fmt.Errorf("required columns (FinInstrmId, TckrSymb) not found in CSV")
	}

	// Clear existing contracts
	f.contracts = make(map[string]EquityInfo)

	// Parse data rows
	for i, row := range records[1:] {
		if len(row) <= tokenIdx || len(row) <= symbolIdx {
			continue
		}

		token := strings.TrimSpace(row[tokenIdx])
		symbol := strings.TrimSpace(row[symbolIdx])

		if token == "" || symbol == "" {
			continue
		}

		info := EquityInfo{
			Token:  token,
			Symbol: symbol,
		}

		// Extract optional name field
		if ok3 && len(row) > nameIdx {
			info.Name = strings.TrimSpace(row[nameIdx])
		}

		f.contracts[token] = info

		// Log first few for verification
		if i < 3 {
			log.Printf("   [%d] Token %s → %s (%s)", i+1, token, symbol, info.Name)
		}
	}

	log.Printf("✅ Loaded %d Equity Cash tokens", len(f.contracts))
	return nil
}

// deleteOldFiles removes BhavCopy files older than keepDays
func (f *Fetcher) deleteOldFiles(keepDays int) {
	pattern := filepath.Join(f.dataDir, "BhavCopy_BSE_CM_*.csv")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		log.Printf("⚠️ Failed to glob for cleanup: %v", err)
		return
	}

	if len(matches) <= keepDays {
		return // Nothing to delete
	}

	// Sort by filename (contains date)
	sort.Strings(matches)

	// Delete oldest files, keep last 'keepDays' files
	toDelete := matches[:len(matches)-keepDays]
	for _, file := range toDelete {
		if err := os.Remove(file); err != nil {
			log.Printf("⚠️ Failed to delete %s: %v", file, err)
		} else {
			log.Printf("🗑️ Deleted old file: %s", filepath.Base(file))
		}
	}
}

// GetContract returns equity info for a token
func (f *Fetcher) GetContract(token string) (EquityInfo, bool) {
	info, ok := f.contracts[token]
	return info, ok
}

// GetContracts returns all contracts
func (f *Fetcher) GetContracts() map[string]EquityInfo {
	return f.contracts
}

// Count returns number of loaded contracts
func (f *Fetcher) Count() int {
	return len(f.contracts)
}

// BuildSymbolDisplay creates a display-friendly symbol
// Format: RELIANCE (RELIANCE INDUSTRIES LTD.)
func (f *Fetcher) BuildSymbolDisplay(token string) string {
	info, ok := f.contracts[token]
	if !ok {
		return fmt.Sprintf("TOKEN_%s", token)
	}

	if info.Name != "" && info.Name != info.Symbol {
		return fmt.Sprintf("%s (%s)", info.Symbol, info.Name)
	}
	return info.Symbol
}
