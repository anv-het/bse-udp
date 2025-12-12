package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"bse-greeks-go/pkg/greeks"
	"bse-greeks-go/pkg/processor"
)

// MonitoredToken represents a token to monitor
type MonitoredToken struct {
	Token       string
	Symbol      string
	Expiry      string
	OptionType  string
	StrikePrice float64
}

// TokenSnapshot contains current data for a monitored token
type TokenSnapshot struct {
	Timestamp      time.Time
	Token          string
	Symbol         string
	Expiry         string
	OptionType     string
	StrikePrice    float64
	LTP            float64
	Volume         int64
	SpotPrice      float64
	Moneyness      string
	IntrinsicValue float64
	TimeValue      float64
	Greeks         greeks.Greeks
	UpdateCount    int64
}

func main() {
	// Command-line flags
	foFile := flag.String("fo-file", "", "FO quotes CSV file")
	indexFile := flag.String("index-file", "", "Index CSV file")
	outputFile := flag.String("output", "data/test_results/live_monitor.csv", "Output CSV file")
	refreshSeconds := flag.Int("refresh", 5, "Refresh interval in seconds")
	tokens := flag.String("tokens", "1144708,1141880", "Comma-separated token IDs to monitor")

	flag.Parse()

	// Print banner
	fmt.Println("\n╔═══════════════════════════════════════════════════════╗")
	fmt.Println("║      BSE Live Greeks Monitor - Token Tracker         ║")
	fmt.Println("║   Real-time Greeks calculation for selected tokens   ║")
	fmt.Println("╚═══════════════════════════════════════════════════════╝\n")

	// Validate input
	if *foFile == "" || *indexFile == "" {
		fmt.Println("❌ Error: Both -fo-file and -index-file are required")
		fmt.Println("\nUsage:")
		flag.PrintDefaults()
		fmt.Println("\nExample:")
		fmt.Println("  go run tests/live_token_monitor.go \\")
		fmt.Println("    -fo-file \"d:\\bse\\bse-go-hft\\data\\processed_csv\\20251212_FO_quotes.csv\" \\")
		fmt.Println("    -index-file \"d:\\bse\\bse-go-hft\\data\\processed_csv\\20251212_index_regular.csv\" \\")
		fmt.Println("    -tokens \"1144708,1141880\" \\")
		fmt.Println("    -refresh 5")
		fmt.Println("\nMonitored Tokens:")
		fmt.Println("  1144708 - SENSEX 18-Dec-2025 PE 85200")
		fmt.Println("  1141880 - SENSEX 18-Dec-2025 CE 85200")
		os.Exit(1)
	}

	// Parse token list
	tokenList := parseTokenList(*tokens)
	fmt.Printf("📊 Monitoring %d tokens:\n", len(tokenList))
	for _, tok := range tokenList {
		fmt.Printf("   • Token %s\n", tok)
	}
	fmt.Println()

	// Create output directory
	outputDir := filepath.Dir(*outputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("❌ Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	// Initialize CSV file
	csvFile, csvWriter, err := initializeCSV(*outputFile)
	if err != nil {
		fmt.Printf("❌ Failed to initialize CSV: %v\n", err)
		os.Exit(1)
	}
	defer csvFile.Close()
	defer csvWriter.Flush()

	// Create processor
	proc := processor.NewEnhancedProcessorWithIV(0.065, 0) // No volume filter for monitoring

	// Load initial data
	fmt.Println("📂 Loading initial data...")
	records, err := proc.ProcessWithIndex(*foFile, *indexFile)
	if err != nil {
		fmt.Printf("❌ Failed to load data: %v\n", err)
		os.Exit(1)
	}

	// Filter for monitored tokens
	monitoredRecords := filterMonitoredTokens(records, tokenList)
	if len(monitoredRecords) == 0 {
		fmt.Printf("❌ No data found for monitored tokens\n")
		os.Exit(1)
	}

	fmt.Printf("✅ Found %d/%d monitored tokens in data\n\n", len(monitoredRecords), len(tokenList))

	// Create snapshots
	snapshots := make(map[string]*TokenSnapshot)
	for _, rec := range monitoredRecords {
		snapshot := createSnapshot(rec)
		snapshots[rec.Token] = snapshot
		
		// Write initial data to CSV
		writeSnapshotToCSV(csvWriter, snapshot)
	}
	csvWriter.Flush()

	// Display initial state
	clearScreen()
	displayHeader()
	displaySnapshots(snapshots)

	// Monitoring loop
	updateCount := int64(0)
	ticker := time.NewTicker(time.Duration(*refreshSeconds) * time.Second)
	defer ticker.Stop()

	fmt.Printf("\n🔄 Auto-refreshing every %d seconds. Press Ctrl+C to stop.\n", *refreshSeconds)
	fmt.Println(strings.Repeat("═", 80))

	for range ticker.C {
		updateCount++

		// Reload data
		records, err := proc.ProcessWithIndex(*foFile, *indexFile)
		if err != nil {
			fmt.Printf("\n⚠️  Error reloading data: %v\n", err)
			continue
		}

		// Update monitored tokens
		monitoredRecords = filterMonitoredTokens(records, tokenList)
		
		// Update snapshots
		for _, rec := range monitoredRecords {
			snapshot := createSnapshot(rec)
			snapshot.UpdateCount = updateCount
			snapshots[rec.Token] = snapshot
			
			// Write to CSV
			writeSnapshotToCSV(csvWriter, snapshot)
		}
		csvWriter.Flush()

		// Redisplay
		clearScreen()
		displayHeader()
		fmt.Printf("🔄 Update #%d - %s\n", updateCount, time.Now().Format("15:04:05"))
		fmt.Println(strings.Repeat("═", 80))
		displaySnapshots(snapshots)
		fmt.Println(strings.Repeat("═", 80))
		fmt.Printf("\n💾 Data saved to: %s\n", *outputFile)
	}
}

func parseTokenList(tokens string) []string {
	list := strings.Split(tokens, ",")
	result := make([]string, 0, len(list))
	for _, tok := range list {
		trimmed := strings.TrimSpace(tok)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func filterMonitoredTokens(records []processor.EnhancedRecord, tokenList []string) []processor.EnhancedRecord {
	tokenMap := make(map[string]bool)
	for _, tok := range tokenList {
		tokenMap[tok] = true
	}

	result := make([]processor.EnhancedRecord, 0)
	for _, rec := range records {
		if tokenMap[rec.Token] {
			result = append(result, rec)
		}
	}
	return result
}

func createSnapshot(rec processor.EnhancedRecord) *TokenSnapshot {
	return &TokenSnapshot{
		Timestamp:      rec.Timestamp,
		Token:          rec.Token,
		Symbol:         rec.Symbol,
		Expiry:         rec.Expiry.Format("02-Jan-2006"),
		OptionType:     rec.OptionType,
		StrikePrice:    rec.StrikePrice,
		LTP:            rec.LTP,
		Volume:         rec.Volume,
		Moneyness:      rec.Moneyness,
		IntrinsicValue: rec.Intrinsic,
		TimeValue:      rec.TimeValue,
		Greeks:         rec.Greeks,
		UpdateCount:    0,
	}
}

func initializeCSV(filename string) (*os.File, *csv.Writer, error) {
	file, err := os.Create(filename)
	if err != nil {
		return nil, nil, err
	}

	writer := csv.NewWriter(file)

	// Write header
	header := []string{
		"timestamp", "update_count", "token", "symbol", "expiry", "option_type", "strike_price",
		"ltp", "volume", "moneyness", "intrinsic_value", "time_value",
		"implied_vol", "iv_estimated",
		"delta", "gamma", "theta", "vega", "rho",
		"vanna", "vomma", "charm",
	}

	if err := writer.Write(header); err != nil {
		file.Close()
		return nil, nil, err
	}
	writer.Flush()

	return file, writer, nil
}

func writeSnapshotToCSV(writer *csv.Writer, snapshot *TokenSnapshot) {
	row := []string{
		snapshot.Timestamp.Format("2006-01-02 15:04:05.000"),
		fmt.Sprintf("%d", snapshot.UpdateCount),
		snapshot.Token,
		snapshot.Symbol,
		snapshot.Expiry,
		snapshot.OptionType,
		fmt.Sprintf("%.2f", snapshot.StrikePrice),
		fmt.Sprintf("%.2f", snapshot.LTP),
		fmt.Sprintf("%d", snapshot.Volume),
		snapshot.Moneyness,
		fmt.Sprintf("%.2f", snapshot.IntrinsicValue),
		fmt.Sprintf("%.2f", snapshot.TimeValue),
		fmt.Sprintf("%.4f", snapshot.Greeks.ImpliedVol),
		fmt.Sprintf("%t", snapshot.Greeks.IVEstimated),
		fmt.Sprintf("%.6f", snapshot.Greeks.Delta),
		fmt.Sprintf("%.6f", snapshot.Greeks.Gamma),
		fmt.Sprintf("%.2f", snapshot.Greeks.Theta),
		fmt.Sprintf("%.2f", snapshot.Greeks.Vega),
		fmt.Sprintf("%.2f", snapshot.Greeks.Rho),
		fmt.Sprintf("%.6f", snapshot.Greeks.Vanna),
		fmt.Sprintf("%.2f", snapshot.Greeks.Vomma),
		fmt.Sprintf("%.6f", snapshot.Greeks.Charm),
	}
	writer.Write(row)
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func displayHeader() {
	fmt.Println("\n╔═══════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    BSE LIVE GREEKS MONITOR - TOKEN TRACKER                   ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════════════════╝")
}

func displaySnapshots(snapshots map[string]*TokenSnapshot) {
	for token, snapshot := range snapshots {
		displaySnapshot(token, snapshot)
		fmt.Println()
	}
}

func displaySnapshot(token string, s *TokenSnapshot) {
	// Header
	fmt.Printf("\n┌─ Token: %s ─ %s %s %.0f %s ────────────────────────────────────────┐\n",
		token, s.Symbol, s.Expiry, s.StrikePrice, s.OptionType)

	// Market Data
	fmt.Printf("│ 📊 Market Data                                                                │\n")
	fmt.Printf("│   LTP:            ₹%-10.2f  Volume:        %10d                    │\n",
		s.LTP, s.Volume)
	fmt.Printf("│   Moneyness:      %-10s  Intrinsic:     ₹%10.2f                    │\n",
		s.Moneyness, s.IntrinsicValue)
	fmt.Printf("│   Time Value:     ₹%-10.2f  Last Update:   %s            │\n",
		s.TimeValue, s.Timestamp.Format("15:04:05"))

	// Implied Volatility
	ivStr := fmt.Sprintf("%.2f%%", s.Greeks.ImpliedVol*100)
	if s.Greeks.IVEstimated {
		ivStr += " (Est)"
	}
	fmt.Printf("│                                                                               │\n")
	fmt.Printf("│ 💡 Implied Volatility: %-20s                                   │\n", ivStr)

	// Basic Greeks
	fmt.Printf("│                                                                               │\n")
	fmt.Printf("│ 📈 Basic Greeks (First Order)                                                 │\n")
	fmt.Printf("│   Delta:   %8.4f  │  Gamma:  %10.6f  │  Theta:  %8.2f/day        │\n",
		s.Greeks.Delta, s.Greeks.Gamma, s.Greeks.Theta)
	fmt.Printf("│   Vega:    %8.2f  │  Rho:    %10.2f                                  │\n",
		s.Greeks.Vega, s.Greeks.Rho)

	// Advanced Greeks
	fmt.Printf("│                                                                               │\n")
	fmt.Printf("│ 🎯 Advanced Greeks (Second Order)                                             │\n")
	fmt.Printf("│   Vanna:   %8.4f  │  Vomma:  %10.2f  │  Charm:  %8.4f/day        │\n",
		s.Greeks.Vanna, s.Greeks.Vomma, s.Greeks.Charm)

	// Greeks Interpretation
	fmt.Printf("│                                                                               │\n")
	fmt.Printf("│ 💭 Interpretation:                                                            │\n")
	
	// Delta interpretation
	deltaPercent := s.Greeks.Delta * 100
	if s.OptionType == "CE" {
		fmt.Printf("│   • For every ₹100 SENSEX rise, option gains ₹%.0f                          │\n", 
			deltaPercent)
	} else {
		fmt.Printf("│   • For every ₹100 SENSEX fall, option gains ₹%.0f                          │\n",
			-deltaPercent)
	}

	// Theta interpretation
	daysToExpiry := time.Until(parseExpiry(s.Expiry)).Hours() / 24
	if daysToExpiry > 0 {
		fmt.Printf("│   • Time decay: Losing ₹%.2f per day (%.0f days to expiry)                    │\n",
			-s.Greeks.Theta, daysToExpiry)
	}

	// Vega interpretation
	fmt.Printf("│   • If volatility rises 1%%, option gains ₹%.2f                               │\n",
		s.Greeks.Vega)

	fmt.Printf("└───────────────────────────────────────────────────────────────────────────────┘")
}

func parseExpiry(expiryStr string) time.Time {
	t, _ := time.Parse("02-Jan-2006", expiryStr)
	return t
}
