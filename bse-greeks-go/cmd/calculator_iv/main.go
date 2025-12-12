package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"bse-greeks-go/pkg/processor"
)

func main() {
	// Command-line flags
	foFile := flag.String("fo-file", "", "FO quotes CSV file (BSE F&O data)")
	indexFile := flag.String("index-file", "", "Index CSV file (SENSEX/BANKEX spot prices)")
	outputFile := flag.String("output", "", "Output CSV file path (with all 9 Greeks)")
	riskFreeRate := flag.Float64("rate", 0.065, "Risk-free rate (default 0.065 = 6.5%)")
	minVolume := flag.Int64("min-volume", 10, "Minimum volume filter (default 10)")
	calculateIV := flag.Bool("calculate-iv", true, "Calculate Implied Volatility (default true)")

	flag.Parse()

	// Print banner
	fmt.Println("\n╔═══════════════════════════════════════════════════════╗")
	fmt.Println("║   BSE Greeks Calculator with Implied Volatility     ║")
	fmt.Println("║   Calculates all 9 Greeks from live market data     ║")
	fmt.Println("╚═══════════════════════════════════════════════════════╝\n")

	// Validate input
	if *foFile == "" || *indexFile == "" {
		fmt.Println("Usage:")
		flag.PrintDefaults()
		fmt.Println("\nExample:")
		fmt.Println("  go run cmd/calculator_iv/main.go \\")
		fmt.Println("    -fo-file \"d:\\bse\\bse-go-hft\\data\\processed_csv\\20251212_FO_quotes.csv\" \\")
		fmt.Println("    -index-file \"d:\\bse\\bse-go-hft\\data\\processed_csv\\20251212_index_regular.csv\" \\")
		fmt.Println("    -output \"data/output/greeks_iv_20251212.csv\"")
		fmt.Println("\nGreeks Calculated:")
		fmt.Println("  1. Implied Volatility (IV) - from market price")
		fmt.Println("  2. Delta   - Price sensitivity")
		fmt.Println("  3. Gamma   - Delta sensitivity")
		fmt.Println("  4. Theta   - Time decay")
		fmt.Println("  5. Vega    - Volatility sensitivity")
		fmt.Println("  6. Rho     - Interest rate sensitivity")
		fmt.Println("  7. Vanna   - Cross-gamma")
		fmt.Println("  8. Vomma   - Volatility gamma")
		fmt.Println("  9. Charm   - Delta decay\n")
		os.Exit(1)
	}

	// Check if files exist
	if _, err := os.Stat(*foFile); os.IsNotExist(err) {
		log.Fatalf("FO file not found: %s", *foFile)
	}
	if _, err := os.Stat(*indexFile); os.IsNotExist(err) {
		log.Fatalf("Index file not found: %s", *indexFile)
	}

	// Generate output filename if not specified
	if *outputFile == "" {
		base := filepath.Base(*foFile)
		*outputFile = filepath.Join("data/output", fmt.Sprintf("greeks_iv_%s", base))
	}

	// Create output directory
	outputDir := filepath.Dir(*outputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Print configuration
	fmt.Println("📋 Configuration:")
	fmt.Printf("  FO Quotes:      %s\n", *foFile)
	fmt.Printf("  Index Data:     %s\n", *indexFile)
	fmt.Printf("  Output:         %s\n", *outputFile)
	fmt.Printf("  Risk-Free Rate: %.2f%%\n", *riskFreeRate*100)
	fmt.Printf("  Min Volume:     %d\n", *minVolume)
	fmt.Printf("  Calculate IV:   %t\n\n", *calculateIV)

	// Create enhanced processor
	proc := processor.NewEnhancedProcessorWithIV(*riskFreeRate, *minVolume)

	// Start processing
	fmt.Println("🚀 Starting Greeks calculation...")
	startTime := time.Now()

	// Process with index data
	records, err := proc.ProcessWithIndex(*foFile, *indexFile)
	if err != nil {
		log.Fatalf("Failed to process files: %v", err)
	}

	processingTime := time.Since(startTime)

	if len(records) == 0 {
		log.Fatal("❌ No records processed successfully")
	}

	// Print detailed summary
	printDetailedSummary(records, processingTime)

	// Write output with all Greeks
	fmt.Printf("\n💾 Writing results to %s...\n", *outputFile)
	if err := proc.WriteToCSVWithAllGreeks(records, *outputFile); err != nil {
		log.Fatalf("Failed to write output: %v", err)
	}

	// Final summary
	fmt.Println("\n✨ Greeks calculation complete!")
	fmt.Printf("📊 Processed %d options in %v\n", len(records), processingTime)
	fmt.Printf("⚡ Performance: %.0f options/second\n",
		float64(len(records))/processingTime.Seconds())
	fmt.Printf("💾 Output: %s\n\n", *outputFile)
}

func printDetailedSummary(records []processor.EnhancedRecord, duration time.Duration) {
	separator := "============================================================"
	fmt.Println("\n" + separator)
	fmt.Println("📊 GREEKS CALCULATION SUMMARY")
	fmt.Println(separator)

	// Count by symbol
	symbolCounts := make(map[string]int)
	ivFailedCount := 0
	itmCount := 0
	atmCount := 0
	otmCount := 0

	var totalIV, totalDelta, totalGamma, totalTheta, totalVega float64
	var minIV, maxIV float64 = 999, 0

	for _, rec := range records {
		symbolCounts[rec.Symbol]++

		if rec.Greeks.IVEstimated {
			ivFailedCount++
		}

		switch rec.Moneyness {
		case "ITM":
			itmCount++
		case "ATM":
			atmCount++
		case "OTM":
			otmCount++
		}

		// IV statistics
		iv := rec.Greeks.ImpliedVol
		if iv > 0 && iv < 5 { // Reasonable range
			totalIV += iv
			if iv < minIV {
				minIV = iv
			}
			if iv > maxIV {
				maxIV = iv
			}
		}

		// Greeks averages
		totalDelta += rec.Greeks.Delta
		totalGamma += rec.Greeks.Gamma
		totalTheta += rec.Greeks.Theta
		totalVega += rec.Greeks.Vega
	}

	count := float64(len(records))

	fmt.Printf("\n📈 Options Processed:\n")
	fmt.Printf("  Total Options:     %d\n", len(records))
	for symbol, cnt := range symbolCounts {
		fmt.Printf("  %-15s    %d options\n", symbol+":", cnt)
	}

	fmt.Printf("\n💡 Moneyness Distribution:\n")
	fmt.Printf("  ITM (In-The-Money):  %d (%.1f%%)\n", itmCount, float64(itmCount)*100/count)
	fmt.Printf("  ATM (At-The-Money):  %d (%.1f%%)\n", atmCount, float64(atmCount)*100/count)
	fmt.Printf("  OTM (Out-The-Money): %d (%.1f%%)\n", otmCount, float64(otmCount)*100/count)

	fmt.Printf("\n📊 Implied Volatility Statistics:\n")
	fmt.Printf("  Average IV:        %.2f%% (%.4f)\n", totalIV/count*100, totalIV/count)
	fmt.Printf("  Min IV:            %.2f%%\n", minIV*100)
	fmt.Printf("  Max IV:            %.2f%%\n", maxIV*100)
	fmt.Printf("  IV Failed:         %d options (%.1f%%)\n",
		ivFailedCount, float64(ivFailedCount)*100/count)

	fmt.Printf("\n🎯 Average Greeks:\n")
	fmt.Printf("  Delta:   %8.4f\n", totalDelta/count)
	fmt.Printf("  Gamma:   %8.6f\n", totalGamma/count)
	fmt.Printf("  Theta:   %8.2f (per day)\n", totalTheta/count)
	fmt.Printf("  Vega:    %8.2f\n", totalVega/count)

	fmt.Printf("\n⚡ Performance:\n")
	fmt.Printf("  Total Time:        %v\n", duration)
	fmt.Printf("  Options/Second:    %.0f\n", count/duration.Seconds())
	fmt.Printf("  Time/Option:       %.2f µs\n", duration.Seconds()*1e6/count)

	fmt.Println(separator)
}
