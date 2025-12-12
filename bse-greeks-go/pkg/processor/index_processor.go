package processor

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// IndexData represents a single index quote from BSE feed
type IndexData struct {
	Timestamp     time.Time
	MessageType   int
	IndexCode     int
	IndexName     string
	IndexValue    float64
	NetChange     float64
	PercentChange float64
	PrevClose     float64
	Open          float64
	High          float64
	Low           float64
}

// IndexProcessor handles index data from BSE UDP feed
// Extracts live spot prices for SENSEX, BANKEX, etc.
type IndexProcessor struct {
	spotPrices map[string]float64   // symbol -> current spot price
	indexData  map[string]IndexData // symbol -> full index data
	lastUpdate time.Time            // Last update timestamp
}

// NewIndexProcessor creates a new index processor
func NewIndexProcessor() *IndexProcessor {
	return &IndexProcessor{
		spotPrices: make(map[string]float64),
		indexData:  make(map[string]IndexData),
	}
}

// ProcessIndexFile reads index CSV and extracts spot prices
func (p *IndexProcessor) ProcessIndexFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open index file: %v", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("failed to read index CSV: %v", err)
	}

	if len(records) < 2 {
		return fmt.Errorf("index file has no data rows")
	}

	// Expected header:
	// Timestamp,Message_Type,Index_Code,Index_Name,Index_Value,Net_Change,
	// Percent_Change,Prev_Close,Open,High,Low

	// Parse data rows (skip header)
	for i, record := range records[1:] {
		if len(record) < 11 {
			continue // Skip malformed rows
		}

		indexData, err := p.parseIndexRecord(record)
		if err != nil {
			// Log warning but continue
			fmt.Printf("Warning: Failed to parse index row %d: %v\n", i+2, err)
			continue
		}

		// Store in maps
		p.indexData[indexData.IndexName] = indexData
		p.spotPrices[indexData.IndexName] = indexData.IndexValue

		// Update last update timestamp
		if indexData.Timestamp.After(p.lastUpdate) {
			p.lastUpdate = indexData.Timestamp
		}
	}

	if len(p.spotPrices) == 0 {
		return fmt.Errorf("no valid index data found in file")
	}

	return nil
}

// parseIndexRecord parses a single CSV row into IndexData
func (p *IndexProcessor) parseIndexRecord(record []string) (IndexData, error) {
	var data IndexData
	var err error

	// Parse timestamp
	data.Timestamp, err = time.Parse("2006-01-02 15:04:05.999", record[0])
	if err != nil {
		// Try alternate format without milliseconds
		data.Timestamp, err = time.Parse("2006-01-02 15:04:05", record[0])
		if err != nil {
			return data, fmt.Errorf("invalid timestamp: %v", err)
		}
	}

	// Parse message type
	data.MessageType, err = strconv.Atoi(record[1])
	if err != nil {
		return data, fmt.Errorf("invalid message type: %v", err)
	}

	// Parse index code
	data.IndexCode, err = strconv.Atoi(record[2])
	if err != nil {
		return data, fmt.Errorf("invalid index code: %v", err)
	}

	// Index name
	data.IndexName = strings.TrimSpace(record[3])

	// Parse numeric fields
	data.IndexValue, err = strconv.ParseFloat(record[4], 64)
	if err != nil {
		return data, fmt.Errorf("invalid index value: %v", err)
	}

	data.NetChange, err = strconv.ParseFloat(record[5], 64)
	if err != nil {
		return data, fmt.Errorf("invalid net change: %v", err)
	}

	data.PercentChange, err = strconv.ParseFloat(record[6], 64)
	if err != nil {
		return data, fmt.Errorf("invalid percent change: %v", err)
	}

	data.PrevClose, err = strconv.ParseFloat(record[7], 64)
	if err != nil {
		return data, fmt.Errorf("invalid prev close: %v", err)
	}

	data.Open, err = strconv.ParseFloat(record[8], 64)
	if err != nil {
		return data, fmt.Errorf("invalid open: %v", err)
	}

	data.High, err = strconv.ParseFloat(record[9], 64)
	if err != nil {
		return data, fmt.Errorf("invalid high: %v", err)
	}

	data.Low, err = strconv.ParseFloat(record[10], 64)
	if err != nil {
		return data, fmt.Errorf("invalid low: %v", err)
	}

	return data, nil
}

// GetSpotPrice returns current spot price for symbol
// Returns 0 if symbol not found
func (p *IndexProcessor) GetSpotPrice(symbol string) float64 {
	return p.spotPrices[symbol]
}

// GetIndexData returns full index data for symbol
func (p *IndexProcessor) GetIndexData(symbol string) (IndexData, bool) {
	data, exists := p.indexData[symbol]
	return data, exists
}

// GetAvailableIndices returns list of available index names
func (p *IndexProcessor) GetAvailableIndices() []string {
	indices := make([]string, 0, len(p.spotPrices))
	for symbol := range p.spotPrices {
		indices = append(indices, symbol)
	}
	return indices
}

// GetLastUpdate returns timestamp of last index update
func (p *IndexProcessor) GetLastUpdate() time.Time {
	return p.lastUpdate
}

// IsStale checks if index data is older than duration
func (p *IndexProcessor) IsStale(maxAge time.Duration) bool {
	return time.Since(p.lastUpdate) > maxAge
}

// GetSpotPriceWithDefault returns spot price with fallback
// Useful when symbol might not exist in index data
func (p *IndexProcessor) GetSpotPriceWithDefault(symbol string, defaultValue float64) float64 {
	if price, exists := p.spotPrices[symbol]; exists && price > 0 {
		return price
	}
	return defaultValue
}

// PrintSummary prints a summary of loaded index data
func (p *IndexProcessor) PrintSummary() {
	fmt.Println("\n📊 Index Data Summary:")
	fmt.Printf("Loaded %d indices\n", len(p.spotPrices))
	fmt.Printf("Last Update: %s\n", p.lastUpdate.Format("2006-01-02 15:04:05"))
	fmt.Println("\nAvailable Indices:")
	for symbol, price := range p.spotPrices {
		data := p.indexData[symbol]
		fmt.Printf("  %-10s: ₹%.2f (%.2f%%)\n",
			symbol, price, data.PercentChange)
	}
	fmt.Println()
}
