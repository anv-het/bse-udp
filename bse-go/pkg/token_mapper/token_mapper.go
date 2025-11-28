// Package token_mapper provides unified token-to-symbol mapping for BSE market data.
// Combines both Equity Cash (BhavCopy) and F&O (Contract Master) tokens.
package token_mapper

import (
	"fmt"
	"log"
	"time"

	"bse-go/pkg/contract_manager"
	"bse-go/pkg/equity_fetcher"
)

// TokenMapper provides unified token→symbol mapping
type TokenMapper struct {
	apiURL          string
	dataDir         string
	equityFetcher   *equity_fetcher.Fetcher
	contractManager *contract_manager.Manager
	tokenMap        map[string]string // token → symbol (unified)
	stats           Stats
}

// Stats holds mapping statistics
type Stats struct {
	EquityTokens int
	FOTokens     int
	TotalTokens  int
	LastUpdate   time.Time
}

// NewTokenMapper creates a new unified token mapper
func NewTokenMapper(apiURL, dataDir string) *TokenMapper {
	if apiURL == "" {
		apiURL = "http://192.168.102.166:2060/v1/sftp-files"
	}
	if dataDir == "" {
		dataDir = "data/tokens"
	}

	return &TokenMapper{
		apiURL:          apiURL,
		dataDir:         dataDir,
		equityFetcher:   equity_fetcher.NewFetcher(apiURL, dataDir),
		contractManager: contract_manager.NewManager(apiURL, dataDir),
		tokenMap:        make(map[string]string),
	}
}

// UpdateAll fetches/updates both Equity and F&O token mappings
func (m *TokenMapper) UpdateAll(keepDays int) error {
	log.Println("🔄 Updating all token mappings...")

	// Use yesterday's date (API typically has previous day's file)
	date := time.Now().AddDate(0, 0, -1)

	successCount := 0

	// Update Equity Cash (BhavCopy)
	log.Println("📊 Updating Equity Cash tokens...")
	if err := m.equityFetcher.UpdateContracts(date, keepDays); err != nil {
		log.Printf("⚠️ Failed to update Equity tokens: %v", err)
	} else {
		successCount++
		m.loadEquityTokens()
	}

	// Update F&O (Contract Master)
	log.Println("📊 Updating F&O tokens...")
	if err := m.contractManager.UpdateContracts(date, keepDays); err != nil {
		log.Printf("⚠️ Failed to update F&O tokens: %v", err)
	} else {
		successCount++
		m.loadFOTokens()
	}

	m.stats.LastUpdate = time.Now()
	m.stats.TotalTokens = len(m.tokenMap)

	log.Printf("✅ Token mapper updated: %d total tokens", m.stats.TotalTokens)
	log.Printf("   Equity: %d | F&O: %d", m.stats.EquityTokens, m.stats.FOTokens)

	if successCount == 0 {
		return fmt.Errorf("failed to update any token source")
	}
	return nil
}

// LoadFromCache loads tokens from existing CSV files (no API fetch)
func (m *TokenMapper) LoadFromCache() error {
	log.Println("📂 Loading tokens from cached CSV files...")

	successCount := 0

	// Load Equity Cash
	if err := m.equityFetcher.LoadFromCache(); err != nil {
		log.Printf("⚠️ Failed to load Equity tokens: %v", err)
	} else {
		successCount++
		m.loadEquityTokens()
	}

	// Load F&O
	if err := m.contractManager.LoadFromCache(); err != nil {
		log.Printf("⚠️ Failed to load F&O tokens: %v", err)
	} else {
		successCount++
		m.loadFOTokens()
	}

	m.stats.TotalTokens = len(m.tokenMap)
	log.Printf("✅ Loaded %d tokens from cache", m.stats.TotalTokens)

	if successCount == 0 {
		return fmt.Errorf("no cached token files found")
	}
	return nil
}

// loadEquityTokens copies equity tokens to unified map
func (m *TokenMapper) loadEquityTokens() {
	count := 0
	for token, info := range m.equityFetcher.GetContracts() {
		// Use symbol with name for display
		m.tokenMap[token] = m.equityFetcher.BuildSymbolDisplay(token)
		_ = info // info already used in BuildSymbolDisplay
		count++
	}
	m.stats.EquityTokens = count
	log.Printf("   Loaded %d equity tokens", count)
}

// loadFOTokens copies F&O tokens to unified map
func (m *TokenMapper) loadFOTokens() {
	count := 0
	for token := range m.contractManager.GetContracts() {
		// Use formatted symbol display
		m.tokenMap[token] = m.contractManager.BuildSymbolDisplay(token)
		count++
	}
	m.stats.FOTokens = count
	log.Printf("   Loaded %d F&O tokens", count)
}

// GetSymbol returns symbol for a token
func (m *TokenMapper) GetSymbol(token string) (string, bool) {
	symbol, ok := m.tokenMap[token]
	return symbol, ok
}

// GetSymbolInt returns symbol for a token (int version)
func (m *TokenMapper) GetSymbolInt(token int) (string, bool) {
	return m.GetSymbol(fmt.Sprintf("%d", token))
}

// GetSymbolsBatch returns symbols for multiple tokens
func (m *TokenMapper) GetSymbolsBatch(tokens []string) map[string]string {
	result := make(map[string]string)
	for _, token := range tokens {
		if symbol, ok := m.GetSymbol(token); ok {
			result[token] = symbol
		}
	}
	return result
}

// IsEquity checks if token is from Equity Cash segment
func (m *TokenMapper) IsEquity(token string) bool {
	_, ok := m.equityFetcher.GetContract(token)
	return ok
}

// IsFO checks if token is from F&O segment
func (m *TokenMapper) IsFO(token string) bool {
	_, ok := m.contractManager.GetContract(token)
	return ok
}

// GetSegment returns segment for a token (EQ, FO, or "")
func (m *TokenMapper) GetSegment(token string) string {
	if m.IsEquity(token) {
		return "EQ"
	}
	if m.IsFO(token) {
		return "FO"
	}
	return ""
}

// Contains checks if token exists in mapper
func (m *TokenMapper) Contains(token string) bool {
	_, ok := m.tokenMap[token]
	return ok
}

// Count returns total number of mapped tokens
func (m *TokenMapper) Count() int {
	return len(m.tokenMap)
}

// GetStats returns mapping statistics
func (m *TokenMapper) GetStats() Stats {
	return m.stats
}

// GetEquityContract returns detailed equity info for a token
func (m *TokenMapper) GetEquityContract(token string) (equity_fetcher.EquityInfo, bool) {
	return m.equityFetcher.GetContract(token)
}

// GetFOContract returns detailed F&O info for a token
func (m *TokenMapper) GetFOContract(token string) (contract_manager.ContractInfo, bool) {
	return m.contractManager.GetContract(token)
}
