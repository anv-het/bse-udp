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
	inputFile := flag.String("input", "", "Input CSV file path (BSE FO quotes)")
	outputFile := flag.String("output", "", "Output CSV file path (with Greeks)")
	riskFreeRate := flag.Float64("rate", 0.07, "Risk-free rate (default 0.07 = 7%)")
	volatility := flag.Float64("vol", 0.15, "Volatility (default 0.15 = 15%)")
	sensexSpot := flag.Float64("sensex", 84733.0, "SENSEX spot price")
	bankexSpot := flag.Float64("bankex", 67250.0, "BANKEX spot price")

	flag.Parse()

	// Validate input
	if *inputFile == "" {
		fmt.Println("BSE Greeks Calculator")
		fmt.Println("====================")
		fmt.Println("\nUsage:")
		flag.PrintDefaults()
		fmt.Println("\nExample:")
		fmt.Println("  go run cmd/calculator/main.go -input data/input/20251208_FO_quotes.csv -output data/output/greeks.csv")
		os.Exit(1)
	}

	// Check if input file exists
	if _, err := os.Stat(*inputFile); os.IsNotExist(err) {
		log.Fatalf("Input file not found: %s", *inputFile)
	}

	// Generate output filename if not specified
	if *outputFile == "" {
		dir := filepath.Dir(*inputFile)
		base := filepath.Base(*inputFile)
		*outputFile = filepath.Join(dir, fmt.Sprintf("greeks_%s", base))
	}

	// Create output directory if needed
	outputDir := filepath.Dir(*outputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	fmt.Printf("\n=== BSE Greeks Calculator ===\n")
	fmt.Printf("Input:  %s\n", *inputFile)
	fmt.Printf("Output: %s\n\n", *outputFile)
	fmt.Printf("Parameters:\n")
	fmt.Printf("  Risk-Free Rate: %.2f%%\n", *riskFreeRate*100)
	fmt.Printf("  Volatility:     %.2f%%\n", *volatility*100)
	fmt.Printf("  SENSEX Spot:    %.2f\n", *sensexSpot)
	fmt.Printf("  BANKEX Spot:    %.2f\n\n", *bankexSpot)

	// Set up spot prices
	spotPrices := map[string]float64{
		"SENSEX": *sensexSpot,
		"BANKEX": *bankexSpot,
		"BSX":    *sensexSpot, // Alias for SENSEX
		"BNX":    *bankexSpot, // Alias for BANKEX
	}

	// Create processor
	proc := processor.NewCSVProcessor(*riskFreeRate, *volatility, spotPrices)

	// Process file
	fmt.Println("Processing options...")
	startTime := time.Now()

	records, err := proc.ProcessFile(*inputFile)
	if err != nil {
		log.Fatalf("Failed to process file: %v", err)
	}

	processingTime := time.Since(startTime)

	if len(records) == 0 {
		log.Fatal("No records processed successfully")
	}

	// Print summary
	processor.PrintSummary(records)

	// Write output
	fmt.Printf("\nWriting output to %s...\n", *outputFile)
	if err := proc.WriteToCSV(records, *outputFile); err != nil {
		log.Fatalf("Failed to write output: %v", err)
	}

	// Performance stats
	fmt.Printf("\n=== Performance ===\n")
	fmt.Printf("Total Time:    %v\n", processingTime)
	fmt.Printf("Options/sec:   %.0f\n", float64(len(records))/processingTime.Seconds())
	fmt.Printf("µs per option: %.2f\n", processingTime.Microseconds()/int64(len(records)))

	fmt.Println("\n✅ Processing complete!")
}
