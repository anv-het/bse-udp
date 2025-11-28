// Token Mapper Test Utility
// Tests the auto-download functionality for BSE token files
package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"bse-go/pkg/token_mapper"
)

func main() {
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("BSE UNIFIED TOKEN MAPPER TEST")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Time: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("Data directory: data/tokens\n\n")

	// Initialize mapper
	mapper := token_mapper.NewTokenMapper("", "data/tokens")

	// Try to load from cache first
	fmt.Println("📂 Attempting to load from cache...")
	if err := mapper.LoadFromCache(); err != nil {
		fmt.Printf("⚠️ Cache load failed: %v\n", err)
		fmt.Println("\n🔄 Fetching from API...")
		if err := mapper.UpdateAll(2); err != nil {
			fmt.Printf("❌ Failed to update: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("✅ Loaded %d tokens from cache\n", mapper.Count())
	}

	// Print stats
	stats := mapper.GetStats()
	fmt.Println("\n📊 Stats:")
	fmt.Printf("   Equity tokens: %d\n", stats.EquityTokens)
	fmt.Printf("   F&O tokens: %d\n", stats.FOTokens)
	fmt.Printf("   Total tokens: %d\n", stats.TotalTokens)

	// Test Equity tokens (port 26001)
	equityTokens := []string{"500325", "532540", "500209", "532555", "532762"}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("🔍 EQUITY CASH TOKENS (Port 26001)")
	fmt.Println(strings.Repeat("=", 80))

	for _, token := range equityTokens {
		symbol, found := mapper.GetSymbol(token)
		segment := mapper.GetSegment(token)
		if found {
			fmt.Printf("✅ %s: %s [%s]\n", token, symbol, segment)
		} else {
			fmt.Printf("❌ %s: NOT FOUND\n", token)
		}
	}

	// Test F&O tokens (port 26002)
	foTokens := []string{"873830", "1102290", "842364", "1114300", "1114301"}

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("🔍 F&O TOKENS (Port 26002)")
	fmt.Println(strings.Repeat("=", 80))

	for _, token := range foTokens {
		symbol, found := mapper.GetSymbol(token)
		segment := mapper.GetSegment(token)
		if found {
			fmt.Printf("✅ %s: %s [%s]\n", token, symbol, segment)
		} else {
			fmt.Printf("❌ %s: NOT FOUND\n", token)
		}
	}

	// Test batch lookup
	allTokens := append(equityTokens, foTokens...)
	batchResults := mapper.GetSymbolsBatch(allTokens)
	fmt.Printf("\n📦 Batch lookup: %d/%d tokens found\n", len(batchResults), len(allTokens))

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("TEST COMPLETE")
	fmt.Println(strings.Repeat("=", 80))

	// If cache was empty, try force update
	if stats.TotalTokens == 0 {
		fmt.Println("\n⚠️ No tokens loaded. Attempting API fetch...")
		if err := mapper.UpdateAll(2); err != nil {
			log.Printf("❌ API fetch failed: %v", err)
		}
	}
}
