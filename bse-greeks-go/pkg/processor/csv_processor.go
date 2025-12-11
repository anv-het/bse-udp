package processor

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"bse-greeks-go/pkg/greeks"
)

// OptionRecord represents a single option quote from BSE
type OptionRecord struct {
	Timestamp   time.Time
	Token       string
	Symbol      string
	Expiry      time.Time
	OptionType  string
	StrikePrice float64
	LTP         float64
	Volume      int64
	BidPrices   []float64
	AskPrices   []float64
}

// EnhancedRecord includes Greeks calculations
type EnhancedRecord struct {
	OptionRecord
	Greeks    greeks.Greeks
	Moneyness string
	Intrinsic float64
	TimeValue float64
}

// CSVProcessor reads BSE FO quotes and calculates Greeks
type CSVProcessor struct {
	calculator *greeks.Calculator
	spotPrices map[string]float64 // Symbol -> Current spot price
	volatility float64
}

// NewCSVProcessor creates a new processor
func NewCSVProcessor(riskFreeRate, volatility float64, spotPrices map[string]float64) *CSVProcessor {
	return &CSVProcessor{
		calculator: greeks.NewCalculator(riskFreeRate),
		spotPrices: spotPrices,
		volatility: volatility,
	}
}

// ProcessFile reads a BSE FO quotes CSV and calculates Greeks
func (p *CSVProcessor) ProcessFile(inputPath string) ([]EnhancedRecord, error) {
	file, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("CSV file has no data rows")
	}

	// Skip header
	results := make([]EnhancedRecord, 0, len(records)-1)

	for i, record := range records {
		if i == 0 {
			continue // Skip header
		}

		enhanced, err := p.processRecord(record)
		if err != nil {
			// Log but continue processing
			fmt.Printf("Warning: Failed to process row %d: %v\n", i, err)
			continue
		}

		if enhanced != nil {
			results = append(results, *enhanced)
		}
	}

	return results, nil
}

// processRecord converts CSV row to EnhancedRecord with Greeks
// CSV Format: timestamp,token,symbol,symbol_name,expiry,option_type,strike_price,ltp,...
func (p *CSVProcessor) processRecord(record []string) (*EnhancedRecord, error) {
	if len(record) < 14 {
		return nil, fmt.Errorf("insufficient columns: %d (need at least 14)", len(record))
	}

	// Parse option record
	opt := OptionRecord{}

	// Timestamp (column 0)
	var err error
	opt.Timestamp, err = time.Parse("2006-01-02 15:04:05.999", record[0])
	if err != nil {
		opt.Timestamp, err = time.Parse("2006-01-02 15:04:05", record[0])
		if err != nil {
			opt.Timestamp = time.Now() // Fallback
		}
	}

	// Token (column 1)
	opt.Token = record[1]

	// Symbol (column 2) - e.g., "SENSEX" or "BANKEX"
	opt.Symbol = strings.TrimSpace(record[2])

	// Expiry date (column 4) - format "24-DEC-2025"
	opt.Expiry, err = time.Parse("02-Jan-2006", record[4])
	if err != nil {
		return nil, fmt.Errorf("invalid expiry date: %s", record[4])
	}

	// Option Type (column 5) - "CE" or "PE"
	opt.OptionType = strings.TrimSpace(record[5])
	if opt.OptionType != "CE" && opt.OptionType != "PE" {
		return nil, fmt.Errorf("invalid option type: %s", opt.OptionType)
	}

	// Strike Price (column 6)
	opt.StrikePrice, err = strconv.ParseFloat(record[6], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid strike price: %s", record[6])
	}

	// LTP (column 7)
	opt.LTP, err = strconv.ParseFloat(record[7], 64)
	if err != nil {
		return nil, fmt.Errorf("invalid LTP: %s", record[7])
	}

	// Volume (column 13)
	if len(record) > 13 {
		opt.Volume, err = strconv.ParseInt(record[13], 10, 64)
		if err != nil {
			opt.Volume = 0
		}
	}

	// Parse bid/ask prices (columns 17, 19)
	if len(record) > 17 {
		opt.BidPrices = parseFloatArray(record[17])
	}
	if len(record) > 19 {
		opt.AskPrices = parseFloatArray(record[19])
	}

	// Get spot price for underlying
	spotPrice, ok := p.spotPrices[opt.Symbol]
	if !ok {
		return nil, fmt.Errorf("no spot price for symbol: %s", opt.Symbol)
	}

	// Calculate Greeks
	g := p.calculator.Calculate(
		opt.OptionType,
		spotPrice,
		opt.StrikePrice,
		opt.Expiry,
		p.volatility,
	)

	// Build enhanced record
	enhanced := &EnhancedRecord{
		OptionRecord: opt,
		Greeks:       g,
		Moneyness:    greeks.Moneyness(opt.OptionType, spotPrice, opt.StrikePrice),
		Intrinsic:    greeks.IntrinsicValue(opt.OptionType, spotPrice, opt.StrikePrice),
		TimeValue:    greeks.TimeValue(opt.LTP, greeks.IntrinsicValue(opt.OptionType, spotPrice, opt.StrikePrice)),
	}

	return enhanced, nil
}

// WriteToCSV writes enhanced records to output CSV
func (p *CSVProcessor) WriteToCSV(records []EnhancedRecord, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	header := []string{
		"timestamp", "token", "symbol", "expiry", "option_type", "strike_price",
		"ltp", "volume", "moneyness",
		"delta", "gamma", "theta", "vega", "rho",
		"intrinsic_value", "time_value",
	}
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write data
	for _, rec := range records {
		row := []string{
			rec.Timestamp.Format("2006-01-02 15:04:05"),
			rec.Token,
			rec.Symbol,
			rec.Expiry.Format("2006-01-02"),
			rec.OptionType,
			fmt.Sprintf("%.2f", rec.StrikePrice),
			fmt.Sprintf("%.2f", rec.LTP),
			fmt.Sprintf("%d", rec.Volume),
			rec.Moneyness,
			fmt.Sprintf("%.4f", rec.Greeks.Delta),
			fmt.Sprintf("%.6f", rec.Greeks.Gamma),
			fmt.Sprintf("%.2f", rec.Greeks.Theta),
			fmt.Sprintf("%.2f", rec.Greeks.Vega),
			fmt.Sprintf("%.2f", rec.Greeks.Rho),
			fmt.Sprintf("%.2f", rec.Intrinsic),
			fmt.Sprintf("%.2f", rec.TimeValue),
		}
		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write row: %w", err)
		}
	}

	return nil
}

// parseFloatArray parses comma-separated float values
func parseFloatArray(s string) []float64 {
	s = strings.Trim(s, "[]\"")
	if s == "" {
		return []float64{}
	}

	parts := strings.Split(s, ",")
	result := make([]float64, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if val, err := strconv.ParseFloat(part, 64); err == nil {
			result = append(result, val)
		}
	}

	return result
}

// PrintSummary prints processing statistics
func PrintSummary(records []EnhancedRecord) {
	if len(records) == 0 {
		fmt.Println("No records processed")
		return
	}

	// Count by symbol
	symbolCounts := make(map[string]int)
	moneyCounts := make(map[string]int)

	for _, rec := range records {
		symbolCounts[rec.Symbol]++
		moneyCounts[rec.Moneyness]++
	}

	fmt.Printf("\n=== Processing Summary ===\n")
	fmt.Printf("Total Options: %d\n\n", len(records))

	fmt.Println("By Symbol:")
	for symbol, count := range symbolCounts {
		fmt.Printf("  %s: %d options\n", symbol, count)
	}

	fmt.Println("\nBy Moneyness:")
	for money, count := range moneyCounts {
		fmt.Printf("  %s: %d options\n", money, count)
	}

	// Sample Greeks
	fmt.Println("\nSample Greeks (first 5 options):")
	for i := 0; i < 5 && i < len(records); i++ {
		rec := records[i]
		fmt.Printf("\n%s %s %.0f %s (Expiry: %s):\n",
			rec.Symbol, rec.OptionType, rec.StrikePrice, rec.Moneyness,
			rec.Expiry.Format("2006-01-02"))
		fmt.Printf("  Delta: %7.4f  Gamma: %.6f\n", rec.Greeks.Delta, rec.Greeks.Gamma)
		fmt.Printf("  Theta: %7.2f  Vega:  %7.2f  Rho: %7.2f\n",
			rec.Greeks.Theta, rec.Greeks.Vega, rec.Greeks.Rho)
	}
}
