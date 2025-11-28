package data_collector

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"bse-go/pkg/contract_manager"
	"bse-go/pkg/decompressor"
	"bse-go/pkg/equity_fetcher"
)

// Quote represents a normalized market data quote
type Quote struct {
	Token         uint32       `json:"token"`
	Symbol        string       `json:"symbol"`
	SymbolName    string       `json:"symbol_name"`
	Expiry        string       `json:"expiry"`
	OptionType    string       `json:"option_type"`
	Strike        string       `json:"strike"`
	Timestamp     string       `json:"timestamp"`
	Open          float64      `json:"open"`
	High          float64      `json:"high"`
	Low           float64      `json:"low"`
	Close         float64      `json:"close"`
	LTP           float64      `json:"ltp"`
	Volume        int32        `json:"volume"`
	PrevClose     float64      `json:"prev_close"`
	ATP           float64      `json:"atp"`
	TurnoverLakhs uint32       `json:"turnover_lakhs"`
	LotSize       uint32       `json:"lot_size"`
	SeqNumber     uint32       `json:"seq_number"`
	BidLevels     []OrderLevel `json:"bid_levels,omitempty"`
	AskLevels     []OrderLevel `json:"ask_levels,omitempty"`
}

// OrderLevel represents a single price level in the order book
type OrderLevel struct {
	Price    float64 `json:"price"`
	Quantity int32   `json:"quantity"`
	Flag     int32   `json:"flag"`
}

// ContractInfo stores contract details from CSV files
type ContractInfo struct {
	Symbol         string `json:"symbol"`
	Expiry         string `json:"expiry"`
	OptionType     string `json:"option_type"`
	Strike         string `json:"strike"`
	LotSize        int    `json:"lot_size"`
	InstrumentName string `json:"instrument_name"`
	FullName       string `json:"full_name"`
	Segment        string `json:"segment"` // "EQ" or "FO"
	CompanyName    string `json:"company_name"`
}

// MarketDataCollector handles token resolution and quote normalization
type MarketDataCollector struct {
	segment         string                    // "CM" or "FO"
	tokenMap        map[string]*ContractInfo  // token -> contract info
	apiURL          string                    // API URL for auto-download
	dataDir         string                    // Data directory
	equityFetcher   *equity_fetcher.Fetcher   // For CM segment
	contractManager *contract_manager.Manager // For FO segment
	stats           struct {
		quotesCollected int
		unknownTokens   int
	}
}

// NewMarketDataCollector creates a new collector with loaded token maps
func NewMarketDataCollector(segment string, dataDir string) *MarketDataCollector {
	return NewMarketDataCollectorWithAPI(segment, dataDir, "")
}

// NewMarketDataCollectorWithAPI creates a collector with API auto-download capability
func NewMarketDataCollectorWithAPI(segment, dataDir, apiURL string) *MarketDataCollector {
	if apiURL == "" {
		apiURL = "http://192.168.102.166:2060/v1/sftp-files"
	}

	collector := &MarketDataCollector{
		segment:  segment,
		tokenMap: make(map[string]*ContractInfo),
		apiURL:   apiURL,
		dataDir:  dataDir,
	}

	// Initialize fetchers for auto-download
	tokensDir := filepath.Join(dataDir, "tokens")
	if segment == "CM" {
		collector.equityFetcher = equity_fetcher.NewFetcher(apiURL, tokensDir)
	} else {
		collector.contractManager = contract_manager.NewManager(apiURL, tokensDir)
	}

	// Try to load from cache first, then auto-download if needed
	if segment == "CM" {
		if !collector.loadBhavCopyCSV(dataDir) {
			log.Printf("[%s] No cached files, attempting auto-download...", segment)
			collector.autoDownloadAndLoad()
		}
	} else {
		if !collector.loadContractMasterCSV(dataDir) {
			log.Printf("[%s] No cached files, attempting auto-download...", segment)
			collector.autoDownloadAndLoad()
		}
	}

	log.Printf("[%s] MarketDataCollector initialized with %d tokens", segment, len(collector.tokenMap))
	return collector
}

// autoDownloadAndLoad downloads token files from API and loads them
func (c *MarketDataCollector) autoDownloadAndLoad() {
	if c.segment == "CM" && c.equityFetcher != nil {
		// Try yesterday first (API typically has previous day's file)
		yesterday := time.Now().AddDate(0, 0, -1)
		if err := c.equityFetcher.UpdateContracts(yesterday, 2); err != nil {
			log.Printf("[%s] Failed to download BhavCopy: %v", c.segment, err)
			return
		}
		// Load the downloaded contracts
		for token, info := range c.equityFetcher.GetContracts() {
			c.tokenMap[token] = &ContractInfo{
				Symbol:      info.Symbol,
				Segment:     "EQ",
				CompanyName: info.Name,
			}
		}
		log.Printf("[%s] Auto-downloaded and loaded %d equity tokens", c.segment, len(c.tokenMap))
	} else if c.segment == "FO" && c.contractManager != nil {
		yesterday := time.Now().AddDate(0, 0, -1)
		if err := c.contractManager.UpdateContracts(yesterday, 2); err != nil {
			log.Printf("[%s] Failed to download Contract Master: %v", c.segment, err)
			return
		}
		// Load the downloaded contracts
		for token, info := range c.contractManager.GetContracts() {
			c.tokenMap[token] = &ContractInfo{
				Symbol:         info.Underlying,
				Expiry:         info.ExpiryDisp,
				OptionType:     info.OptionType,
				Strike:         info.Strike,
				InstrumentName: info.InstType,
				Segment:        "FO",
			}
			if ls, err := strconv.Atoi(info.LotSize); err == nil {
				c.tokenMap[token].LotSize = ls
			}
		}
		log.Printf("[%s] Auto-downloaded and loaded %d F&O contracts", c.segment, len(c.tokenMap))
	}
}

// loadBhavCopyCSV loads equity tokens from BhavCopy CSV
// Returns true if tokens were loaded successfully
func (c *MarketDataCollector) loadBhavCopyCSV(dataDir string) bool {
	pattern := filepath.Join(dataDir, "tokens", "BhavCopy_BSE_CM_*.csv")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		log.Printf("[%s] Warning: No BhavCopy CSV files found at %s", c.segment, pattern)
		return false
	}

	// Sort to get latest file
	sort.Strings(files)
	latestFile := files[len(files)-1]
	log.Printf("[%s] Loading BhavCopy from: %s", c.segment, latestFile)

	file, err := os.Open(latestFile)
	if err != nil {
		log.Printf("[%s] Error opening BhavCopy: %v", c.segment, err)
		return false
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		log.Printf("[%s] Error reading BhavCopy header: %v", c.segment, err)
		return false
	}

	// Find column indices
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.TrimSpace(col)] = i
	}

	// BhavCopy CSV columns: FinInstrmId (token), TckrSymb (symbol), FinInstrmNm (company name)
	tokenIdx, hasToken := colIdx["FinInstrmId"]
	symbolIdx, hasSymbol := colIdx["TckrSymb"]
	nameIdx, hasName := colIdx["FinInstrmNm"]

	if !hasToken || !hasSymbol {
		log.Printf("[%s] BhavCopy CSV missing required columns (FinInstrmId, TckrSymb). Found: %v", c.segment, header[:10])
		return false
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

		token := strings.TrimSpace(record[tokenIdx])
		symbol := strings.TrimSpace(record[symbolIdx])
		var name string
		if hasName && nameIdx < len(record) {
			name = strings.TrimSpace(record[nameIdx])
		}

		c.tokenMap[token] = &ContractInfo{
			Symbol:      symbol,
			Segment:     "EQ",
			CompanyName: name,
		}
		count++
	}

	log.Printf("[%s] Loaded %d equity tokens from BhavCopy", c.segment, count)
	return count > 0
}

// loadContractMasterCSV loads F&O tokens from Contract Master CSV
// Returns true if tokens were loaded successfully
func (c *MarketDataCollector) loadContractMasterCSV(dataDir string) bool {
	pattern := filepath.Join(dataDir, "tokens", "BSE_EQD_CONTRACT_*.csv")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		log.Printf("[%s] Warning: No Contract Master CSV files found at %s", c.segment, pattern)
		return false
	}

	// Sort to get latest file
	sort.Strings(files)
	latestFile := files[len(files)-1]
	log.Printf("[%s] Loading Contract Master from: %s", c.segment, latestFile)

	file, err := os.Open(latestFile)
	if err != nil {
		log.Printf("[%s] Error opening Contract Master: %v", c.segment, err)
		return false
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		log.Printf("[%s] Error reading Contract Master header: %v", c.segment, err)
		return false
	}

	// Find column indices
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.TrimSpace(col)] = i
	}

	// Contract Master CSV columns:
	// FinInstrmId = token
	// TckrSymb = base symbol (e.g., SENSEX, BANKEX)
	// XpryDt = expiry date
	// OptnTp = option type (CE/PE)
	// StrkPric = strike price (in paise)
	// MinLot = lot size
	// FinInstrmNm = instrument name type (IO, IF, etc.)
	// UndrlygAsst = underlying asset name (better for symbol)

	tokenIdx, hasToken := colIdx["FinInstrmId"]
	symbolIdx := colIdx["TckrSymb"]        // Base symbol
	underlyingIdx := colIdx["UndrlygAsst"] // Full underlying name
	expiryIdx := colIdx["XpryDt"]
	optTypeIdx := colIdx["OptnTp"]
	strikeIdx := colIdx["StrkPric"]
	lotSizeIdx := colIdx["MinLot"]
	instTypeIdx := colIdx["FinInstrmNm"] // "IO" for options, "IF" for futures

	if !hasToken {
		log.Printf("[%s] Contract Master CSV missing FinInstrmId column", c.segment)
		return false
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

		token := strings.TrimSpace(record[tokenIdx])
		symbol := ""
		if symbolIdx < len(record) {
			symbol = strings.TrimSpace(record[symbolIdx])
		}
		// Also try underlying asset for better symbol name
		if symbol == "" && underlyingIdx < len(record) {
			symbol = strings.TrimSpace(record[underlyingIdx])
		}
		expiry := ""
		if expiryIdx < len(record) {
			expiry = strings.TrimSpace(record[expiryIdx])
		}
		optType := ""
		if optTypeIdx < len(record) {
			optType = strings.TrimSpace(record[optTypeIdx])
		}
		strike := ""
		if strikeIdx < len(record) {
			// Strike is in paise, convert to rupees
			if strikeVal, err := strconv.ParseFloat(record[strikeIdx], 64); err == nil && strikeVal > 0 {
				strikeRupees := strikeVal / 100.0
				if strikeRupees == float64(int(strikeRupees)) {
					strike = strconv.Itoa(int(strikeRupees))
				} else {
					strike = fmt.Sprintf("%.2f", strikeRupees)
				}
			}
		}
		lotSize := 0
		if lotSizeIdx < len(record) {
			if ls, err := strconv.Atoi(strings.TrimSpace(record[lotSizeIdx])); err == nil {
				lotSize = ls
			}
		}
		instType := ""
		if instTypeIdx < len(record) {
			instType = strings.TrimSpace(record[instTypeIdx])
		}

		c.tokenMap[token] = &ContractInfo{
			Symbol:         symbol,
			Expiry:         expiry,
			OptionType:     optType,
			Strike:         strike,
			LotSize:        lotSize,
			InstrumentName: instType, // "IO" for options, "IF" for futures
			Segment:        "FO",
		}
		count++
	}

	log.Printf("[%s] Loaded %d F&O contracts from Contract Master", c.segment, count)
	return count > 0
}

// LoadTokenMapFromJSON loads legacy token map from JSON file
func LoadTokenMapFromJSON(filename string) (map[string]map[string]interface{}, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var tokenMap map[string]map[string]interface{}
	if err := json.NewDecoder(file).Decode(&tokenMap); err != nil {
		return nil, err
	}

	log.Printf("✓ Token map loaded from JSON: %d tokens", len(tokenMap))
	return tokenMap, nil
}

// CollectQuote builds a normalized quote from a decompressed record
func (c *MarketDataCollector) CollectQuote(record decompressor.DecompressedRecord) (*Quote, error) {
	tokenStr := strconv.Itoa(int(record.Token))
	contractInfo := c.resolveContract(tokenStr)

	// Format symbol name
	symbolName := c.formatSymbolName(contractInfo)

	quote := &Quote{
		Token:         record.Token,
		Symbol:        contractInfo.Symbol,
		SymbolName:    symbolName,
		Expiry:        contractInfo.Expiry,
		OptionType:    contractInfo.OptionType,
		Strike:        contractInfo.Strike,
		Timestamp:     record.Timestamp.Format("2006-01-02 15:04:05.000"),
		Open:          record.Open,
		High:          record.High,
		Low:           record.Low,
		Close:         record.Close,
		LTP:           record.LTP,
		Volume:        record.Volume,
		PrevClose:     record.PrevClose,
		ATP:           record.ATP,
		TurnoverLakhs: record.TurnoverLakhs,
		LotSize:       record.LotSize,
		SeqNumber:     record.SequenceNumber,
	}

	// Convert order levels
	quote.BidLevels = make([]OrderLevel, len(record.BidLevels))
	for i, level := range record.BidLevels {
		quote.BidLevels[i] = OrderLevel{
			Price:    level.Price,
			Quantity: level.Quantity,
			Flag:     level.Flag,
		}
	}

	quote.AskLevels = make([]OrderLevel, len(record.AskLevels))
	for i, level := range record.AskLevels {
		quote.AskLevels[i] = OrderLevel{
			Price:    level.Price,
			Quantity: level.Quantity,
			Flag:     level.Flag,
		}
	}

	c.stats.quotesCollected++
	return quote, nil
}

// resolveContract looks up contract info for a token
func (c *MarketDataCollector) resolveContract(tokenStr string) *ContractInfo {
	if info, exists := c.tokenMap[tokenStr]; exists {
		return info
	}

	c.stats.unknownTokens++

	// Return default info for unknown tokens
	if c.segment == "CM" {
		return &ContractInfo{
			Symbol:  fmt.Sprintf("TOKEN_%s", tokenStr),
			Segment: "EQ",
		}
	}

	return &ContractInfo{
		Symbol:  fmt.Sprintf("TOKEN_%s", tokenStr),
		Segment: "FO",
	}
}

// formatSymbolName creates a readable symbol name
func (c *MarketDataCollector) formatSymbolName(info *ContractInfo) string {
	symbol := info.Symbol
	expiry := info.Expiry
	optType := info.OptionType
	strike := info.Strike

	// For CM (Equity), return company name if available
	if info.Segment == "EQ" {
		if info.CompanyName != "" {
			return info.CompanyName
		}
		return symbol
	}

	// For FO, format as: SYMBOL or SYMBOL_FUT or SYMBOL_STRIKE_CE/PE
	// The InstrumentName field contains the type: "IO" for options, "IF" for futures
	instType := info.InstrumentName // "IO" or "IF"

	// If option type and strike are present, it's an option
	if optType != "" && (optType == "CE" || optType == "PE") && strike != "" {
		return instType // Just return "IO" (Index Option) like Python does
	}

	// For futures
	if instType != "" {
		return instType // "IF" for Index Future
	}

	// Fallback
	if expiry != "" {
		return fmt.Sprintf("%s_FUT", symbol)
	}

	return symbol
}

// GetTokenInfo returns contract info for a specific token
func (c *MarketDataCollector) GetTokenInfo(token uint32) *ContractInfo {
	tokenStr := strconv.Itoa(int(token))
	if info, exists := c.tokenMap[tokenStr]; exists {
		return info
	}
	return nil
}

// GetStats returns collector statistics
func (c *MarketDataCollector) GetStats() map[string]int {
	return map[string]int{
		"quotes_collected": c.stats.quotesCollected,
		"unknown_tokens":   c.stats.unknownTokens,
		"tokens_loaded":    len(c.tokenMap),
	}
}

// LogStats logs collector statistics
func (c *MarketDataCollector) LogStats() {
	log.Printf("[%s] Collector Stats: quotes=%d, unknown=%d, tokens=%d",
		c.segment, c.stats.quotesCollected, c.stats.unknownTokens, len(c.tokenMap))
}
