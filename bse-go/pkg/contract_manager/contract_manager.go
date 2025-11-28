// Package contract_manager manages F&O Contract Master files.
// It auto-downloads from BSE API and cleans up old files.
package contract_manager

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"bse-go/pkg/contract_fetcher"
)

// ContractInfo holds F&O contract details
type ContractInfo struct {
	Token      string
	Symbol     string // TckrSymb
	Underlying string // underlying from symbol
	InstType   string // instrument type (IF, IO, SF, SO)
	Expiry     string // XpryDt
	ExpiryDisp string // formatted expiry for display
	Strike     string // StrkPric
	OptionType string // OptnTp (CE/PE)
	LotSize    string // MinLot
}

// Manager handles F&O contract file management
type Manager struct {
	fetcher   *contract_fetcher.Fetcher
	dataDir   string
	contracts map[string]ContractInfo // token → info
}

// NewManager creates a new contract manager
func NewManager(apiURL, dataDir string) *Manager {
	if dataDir == "" {
		dataDir = "data/tokens"
	}
	return &Manager{
		fetcher:   contract_fetcher.NewFetcher(apiURL),
		dataDir:   dataDir,
		contracts: make(map[string]ContractInfo),
	}
}

// UpdateContracts fetches latest contract file and cleans up old ones
func (m *Manager) UpdateContracts(date time.Time, keepDays int) error {
	// Build API path
	path, fileName := contract_fetcher.BuildFOPath(date)
	outputPath := filepath.Join(m.dataDir, strings.Replace(fileName, ".csv", "_fetched.csv", 1))

	// Check if file already exists
	if _, err := os.Stat(outputPath); err == nil {
		log.Printf("✅ Contract file already exists: %s", outputPath)
	} else {
		// Download new file
		log.Printf("📥 Downloading F&O contract file: %s/%s", path, fileName)
		if err := m.fetcher.FetchAndSave(path, fileName, outputPath); err != nil {
			return fmt.Errorf("failed to download contract file: %w", err)
		}
		log.Printf("✅ Downloaded: %s", outputPath)
	}

	// Cleanup old files
	m.deleteOldFiles(keepDays)

	// Load contracts into memory
	return m.loadContracts(outputPath)
}

// LoadFromCache loads contracts from existing CSV files without API fetch
func (m *Manager) LoadFromCache() error {
	// Try multiple patterns to find contract files
	patterns := []string{
		filepath.Join(m.dataDir, "BSE_EQD_CONTRACT_*_fetched.csv"),
		filepath.Join(m.dataDir, "BSE_EQD_CONTRACT_*.csv"),
	}

	var allMatches []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		allMatches = append(allMatches, matches...)
	}

	if len(allMatches) == 0 {
		return fmt.Errorf("no contract files found in %s", m.dataDir)
	}

	// Sort to get latest (by filename date)
	sort.Strings(allMatches)
	latestFile := allMatches[len(allMatches)-1]

	log.Printf("📂 Loading contracts from: %s", latestFile)
	return m.loadContracts(latestFile)
}

// loadContracts parses CSV and loads into memory
func (m *Manager) loadContracts(csvPath string) error {
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

	// Required columns for F&O
	// FinInstrmId: token
	// TckrSymb: symbol (e.g., SENSEX25JULFUT, SENSEX25JUL85000CE)
	// XpryDt: expiry date
	// StrkPric: strike price
	// OptnTp: option type (CE/PE/XX)
	// MinLot: lot size

	tokenIdx, ok1 := colIndex["FinInstrmId"]
	symbolIdx, ok2 := colIndex["TckrSymb"]
	expiryIdx, ok3 := colIndex["XpryDt"]
	strikeIdx, ok4 := colIndex["StrkPric"]
	optionIdx, ok5 := colIndex["OptnTp"]
	lotIdx, ok6 := colIndex["MinLot"]

	if !ok1 || !ok2 {
		return fmt.Errorf("required columns (FinInstrmId, TckrSymb) not found in CSV")
	}

	// Clear existing contracts
	m.contracts = make(map[string]ContractInfo)

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

		info := ContractInfo{
			Token:  token,
			Symbol: symbol,
		}

		// Extract optional fields
		if ok3 && len(row) > expiryIdx {
			info.Expiry = strings.TrimSpace(row[expiryIdx])
			info.ExpiryDisp = formatExpiry(info.Expiry)
		}
		if ok4 && len(row) > strikeIdx {
			info.Strike = strings.TrimSpace(row[strikeIdx])
		}
		if ok5 && len(row) > optionIdx {
			info.OptionType = strings.TrimSpace(row[optionIdx])
		}
		if ok6 && len(row) > lotIdx {
			info.LotSize = strings.TrimSpace(row[lotIdx])
		}

		// Parse underlying and instrument type from symbol
		info.Underlying, info.InstType = parseSymbol(symbol)

		m.contracts[token] = info

		// Log first few for verification
		if i < 3 {
			log.Printf("   [%d] Token %s → %s (%s %s %s)",
				i+1, token, info.Underlying, info.OptionType, info.Strike, info.ExpiryDisp)
		}
	}

	log.Printf("✅ Loaded %d F&O contracts", len(m.contracts))
	return nil
}

// deleteOldFiles removes contract files older than keepDays
func (m *Manager) deleteOldFiles(keepDays int) {
	pattern := filepath.Join(m.dataDir, "BSE_EQD_CONTRACT_*_fetched.csv")
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
	for _, f := range toDelete {
		if err := os.Remove(f); err != nil {
			log.Printf("⚠️ Failed to delete %s: %v", f, err)
		} else {
			log.Printf("🗑️ Deleted old file: %s", filepath.Base(f))
		}
	}
}

// GetContract returns contract info for a token
func (m *Manager) GetContract(token string) (ContractInfo, bool) {
	info, ok := m.contracts[token]
	return info, ok
}

// GetContracts returns all contracts
func (m *Manager) GetContracts() map[string]ContractInfo {
	return m.contracts
}

// Count returns number of loaded contracts
func (m *Manager) Count() int {
	return len(m.contracts)
}

// parseSymbol extracts underlying and instrument type from F&O symbol
// Examples:
//
//	SENSEX25JULFUT → SENSEX, FUT
//	SENSEX25JUL85000CE → SENSEX, CE
//	BANKEX25NOV57000PE → BANKEX, PE
func parseSymbol(symbol string) (underlying, instType string) {
	// Common underlyings in BSE F&O
	underlyings := []string{"SENSEX", "BANKEX", "SENSEX50"}

	for _, u := range underlyings {
		if strings.HasPrefix(symbol, u) {
			// Check for FUT
			if strings.Contains(symbol, "FUT") {
				return u, "FUT"
			}
			// Check for CE/PE at end
			if strings.HasSuffix(symbol, "CE") {
				return u, "CE"
			}
			if strings.HasSuffix(symbol, "PE") {
				return u, "PE"
			}
			return u, "UNKNOWN"
		}
	}

	// Try regex for other symbols
	// Pattern: (SYMBOL)(YYMMMM)(STRIKE)?(CE|PE|FUT)
	re := regexp.MustCompile(`^([A-Z]+)(\d{2}[A-Z]{3})`)
	matches := re.FindStringSubmatch(symbol)
	if len(matches) >= 2 {
		underlying = matches[1]
		if strings.Contains(symbol, "FUT") {
			return underlying, "FUT"
		}
		if strings.HasSuffix(symbol, "CE") {
			return underlying, "CE"
		}
		if strings.HasSuffix(symbol, "PE") {
			return underlying, "PE"
		}
	}

	return symbol, ""
}

// formatExpiry converts expiry date to display format
// Input: 2025-09-18 or 18-Sep-2025
// Output: 18-Sep-2025
func formatExpiry(expiry string) string {
	// Try different date formats
	formats := []string{
		"2006-01-02",
		"02-01-2006",
		"02-Jan-2006",
	}

	for _, fmt := range formats {
		if t, err := time.Parse(fmt, expiry); err == nil {
			return t.Format("02-Jan-2006")
		}
	}

	return expiry // Return as-is if can't parse
}

// BuildSymbolDisplay creates a display-friendly symbol
func (m *Manager) BuildSymbolDisplay(token string) string {
	info, ok := m.contracts[token]
	if !ok {
		return fmt.Sprintf("TOKEN_%s", token)
	}

	// Futures: SENSEX_FUT_18-Sep-2025
	if info.InstType == "FUT" || strings.Contains(strings.ToUpper(info.OptionType), "XX") {
		return fmt.Sprintf("%s_FUT_%s", info.Underlying, info.ExpiryDisp)
	}

	// Options: SENSEX_CE_85000_18-Sep-2025
	if info.OptionType != "" && info.Strike != "" {
		// Clean up strike (remove decimals if .00)
		strike := info.Strike
		if f, err := strconv.ParseFloat(strike, 64); err == nil {
			if f == float64(int(f)) {
				strike = fmt.Sprintf("%.0f", f)
			}
		}
		return fmt.Sprintf("%s_%s_%s_%s", info.Underlying, info.OptionType, strike, info.ExpiryDisp)
	}

	return info.Symbol
}
