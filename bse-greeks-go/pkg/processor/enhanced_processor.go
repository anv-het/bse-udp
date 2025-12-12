package processor

import (
	"encoding/csv"
	"fmt"
	"os"
	"time"

	"bse-greeks-go/pkg/greeks"
)

// EnhancedProcessorWithIV combines FO quotes and index data to calculate all 9 Greeks
type EnhancedProcessorWithIV struct {
	calculator     *greeks.Calculator
	indexProcessor *IndexProcessor
	minVolume      int64 // Skip options with volume below this
}

// NewEnhancedProcessorWithIV creates a processor that uses index data
func NewEnhancedProcessorWithIV(riskFreeRate float64, minVolume int64) *EnhancedProcessorWithIV {
	return &EnhancedProcessorWithIV{
		calculator:     greeks.NewCalculator(riskFreeRate),
		indexProcessor: NewIndexProcessor(),
		minVolume:      minVolume,
	}
}

// ProcessWithIndex processes FO quotes with live index data
func (p *EnhancedProcessorWithIV) ProcessWithIndex(foFile, indexFile string) ([]EnhancedRecord, error) {
	// Step 1: Load index data
	fmt.Println("📊 Loading index data...")
	err := p.indexProcessor.ProcessIndexFile(indexFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load index data: %v", err)
	}

	p.indexProcessor.PrintSummary()

	// Check if index data is stale
	if p.indexProcessor.IsStale(5 * time.Minute) {
		fmt.Printf("⚠️  Warning: Index data is stale (last update: %s)\n",
			p.indexProcessor.GetLastUpdate().Format("15:04:05"))
	}

	// Step 2: Process FO quotes
	fmt.Println("📈 Processing FO quotes with IV calculation...")
	return p.processFOFile(foFile)
}

// processFOFile reads FO quotes and calculates all 9 Greeks
func (p *EnhancedProcessorWithIV) processFOFile(inputPath string) ([]EnhancedRecord, error) {
	file, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open FO file: %w", err)
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

	// Process records
	results := make([]EnhancedRecord, 0, len(records)-1)
	skipped := 0
	ivFailed := 0

	for i, record := range records {
		if i == 0 {
			continue // Skip header
		}

		enhanced, err := p.processRecordWithIV(record)
		if err != nil {
			// Skip problematic records
			skipped++
			if i < 10 { // Only log first few errors
				fmt.Printf("  Warning: Row %d - %v\n", i, err)
			}
			continue
		}

		if enhanced != nil {
			if enhanced.Greeks.IVEstimated {
				ivFailed++
			}
			results = append(results, *enhanced)
		}
	}

	fmt.Printf("\n✅ Processed %d options\n", len(results))
	fmt.Printf("⚠️  Skipped %d options (errors)\n", skipped)
	fmt.Printf("📊 IV failed/estimated: %d options (%.1f%%)\n",
		ivFailed, float64(ivFailed)*100/float64(len(results)))

	return results, nil
}

// processRecordWithIV converts CSV row to EnhancedRecord with all 9 Greeks
func (p *EnhancedProcessorWithIV) processRecordWithIV(record []string) (*EnhancedRecord, error) {
	if len(record) < 14 {
		return nil, fmt.Errorf("insufficient columns")
	}

	// Parse basic option data (reuse existing parser)
	basicProcessor := NewCSVProcessor(0.065, 0.15, nil)
	opt, err := basicProcessor.parseBasicOption(record)
	if err != nil {
		return nil, err
	}

	// Filter low volume options
	if opt.Volume < p.minVolume {
		return nil, nil // Skip silently
	}

	// Get live spot price from index data
	spotPrice := p.indexProcessor.GetSpotPrice(opt.Symbol)
	if spotPrice <= 0 {
		// Fallback to defaults if index not available
		if opt.Symbol == "SENSEX" {
			spotPrice = 85000 // Default
		} else if opt.Symbol == "BANKEX" {
			spotPrice = 67000 // Default
		} else {
			return nil, fmt.Errorf("no spot price for %s", opt.Symbol)
		}
	}

	// Calculate all 9 Greeks using IV from market price
	g := p.calculator.CalculateWithIV(
		opt.LTP,
		opt.OptionType,
		spotPrice,
		opt.StrikePrice,
		opt.Expiry,
		opt.Volume,
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

// WriteToCSVWithAllGreeks writes enhanced records with all 9 Greeks to CSV
func (p *EnhancedProcessorWithIV) WriteToCSVWithAllGreeks(records []EnhancedRecord, outputPath string) error {
	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Enhanced header with all 9 Greeks
	header := []string{
		"timestamp", "token", "symbol", "expiry", "option_type", "strike_price",
		"ltp", "volume", "moneyness", "intrinsic_value", "time_value",
		// Implied Volatility
		"implied_vol", "iv_estimated",
		// Basic Greeks (First-order)
		"delta", "gamma", "theta", "vega", "rho",
		// Advanced Greeks (Second-order)
		"vanna", "vomma", "charm",
	}

	if err := writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write data rows
	for _, rec := range records {
		row := []string{
			rec.Timestamp.Format("2006-01-02 15:04:05.000"),
			rec.Token,
			rec.Symbol,
			rec.Expiry.Format("02-Jan-2006"),
			rec.OptionType,
			fmt.Sprintf("%.2f", rec.StrikePrice),
			fmt.Sprintf("%.2f", rec.LTP),
			fmt.Sprintf("%d", rec.Volume),
			rec.Moneyness,
			fmt.Sprintf("%.2f", rec.Intrinsic),
			fmt.Sprintf("%.2f", rec.TimeValue),
			// IV
			fmt.Sprintf("%.4f", rec.Greeks.ImpliedVol),
			fmt.Sprintf("%t", rec.Greeks.IVEstimated),
			// Basic Greeks
			fmt.Sprintf("%.6f", rec.Greeks.Delta),
			fmt.Sprintf("%.6f", rec.Greeks.Gamma),
			fmt.Sprintf("%.2f", rec.Greeks.Theta),
			fmt.Sprintf("%.2f", rec.Greeks.Vega),
			fmt.Sprintf("%.2f", rec.Greeks.Rho),
			// Advanced Greeks
			fmt.Sprintf("%.6f", rec.Greeks.Vanna),
			fmt.Sprintf("%.2f", rec.Greeks.Vomma),
			fmt.Sprintf("%.6f", rec.Greeks.Charm),
		}

		if err := writer.Write(row); err != nil {
			return fmt.Errorf("failed to write row: %w", err)
		}
	}

	fmt.Printf("✅ Wrote %d records to %s\n", len(records), outputPath)
	return nil
}

// parseBasicOption is a helper to parse option record from CSV row
func (p *CSVProcessor) parseBasicOption(record []string) (OptionRecord, error) {
	opt := OptionRecord{}

	// Timestamp (column 0)
	var err error
	opt.Timestamp, err = time.Parse("2006-01-02 15:04:05.999", record[0])
	if err != nil {
		opt.Timestamp, err = time.Parse("2006-01-02 15:04:05", record[0])
		if err != nil {
			opt.Timestamp = time.Now()
		}
	}

	// Token (column 1)
	opt.Token = record[1]

	// Symbol (column 2)
	opt.Symbol = record[2]

	// Expiry (column 4)
	opt.Expiry, err = time.Parse("02-Jan-2006", record[4])
	if err != nil {
		return opt, fmt.Errorf("invalid expiry: %s", record[4])
	}

	// Option Type (column 5)
	opt.OptionType = record[5]
	if opt.OptionType != "CE" && opt.OptionType != "PE" {
		return opt, fmt.Errorf("invalid option type: %s", opt.OptionType)
	}

	// Strike Price (column 6)
	_, err = fmt.Sscanf(record[6], "%f", &opt.StrikePrice)
	if err != nil {
		return opt, fmt.Errorf("invalid strike: %s", record[6])
	}

	// LTP (column 7)
	_, err = fmt.Sscanf(record[7], "%f", &opt.LTP)
	if err != nil {
		return opt, fmt.Errorf("invalid LTP: %s", record[7])
	}

	// Volume (column 13)
	if len(record) > 13 {
		_, _ = fmt.Sscanf(record[13], "%d", &opt.Volume)
	}

	// Parse bid/ask prices
	if len(record) > 17 {
		opt.BidPrices = parseFloatArray(record[17])
	}
	if len(record) > 19 {
		opt.AskPrices = parseFloatArray(record[19])
	}

	return opt, nil
}
