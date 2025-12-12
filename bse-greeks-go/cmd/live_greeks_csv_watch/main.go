// BSE Live Greeks Calculator from HFT Server CSV Files
// Watches CSV files updated by HFT servers and calculates Greeks in real-time
//
// Usage:
//   go run cmd/live_greeks_csv_watch/main.go
//   OR
//   go build -o bin/live_greeks_csv_watch.exe cmd/live_greeks_csv_watch/main.go
//   .\bin\live_greeks_csv_watch.exe

package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// ==================== CONFIGURATION ====================

const (
	// File paths (updated by HFT servers)
	CSV_DIR_DEFAULT = "d:\\bse\\bse-go-hft\\data\\processed_csv"
	
	// Greeks Calculation
	RISK_FREE_RATE = 0.07  // 7% annual risk-free rate
	
	// Watch interval
	WATCH_INTERVAL = 1 * time.Second
	DISPLAY_INTERVAL = 3 * time.Second
)

// Monitored Tokens
var MONITORED_TOKENS = map[uint32]TokenInfo{
	1144708: {
		Symbol:     "SENSEX",
		Expiry:     "18-Dec-2025",
		Strike:     85200.0,
		OptionType: "PE",
	},
	1141880: {
		Symbol:     "SENSEX",
		Expiry:     "18-Dec-2025",
		Strike:     85200.0,
		OptionType: "CE",
	},
}

// ==================== DATA STRUCTURES ====================

type TokenInfo struct {
	Symbol     string
	Expiry     string
	Strike     float64
	OptionType string
}

type LiveData struct {
	Token      uint32
	LTP        float64
	Volume     uint64
	LastUpdate time.Time
	Greeks     Greeks
}

type IndexData struct {
	IndexName  string
	SpotPrice  float64
	Change     float64
	ChangePerc float64
	LastUpdate time.Time
}

type Greeks struct {
	SpotPrice      float64
	ImpliedVol     float64
	IVEstimated    bool
	Delta          float64
	Gamma          float64
	Theta          float64
	Vega           float64
	Rho            float64
	Vanna          float64
	Vomma          float64
	Charm          float64
	Moneyness      string
	IntrinsicValue float64
	TimeValue      float64
	DaysToExpiry   float64
}

type Stats struct {
	indexReads   atomic.Uint64
	foReads      atomic.Uint64
	sensexUpdates atomic.Uint64
	tokenUpdates atomic.Uint64
	startTime    time.Time
}

// ==================== GLOBAL STATE ====================

var (
	liveDataMutex sync.RWMutex
	liveDataMap   = make(map[uint32]*LiveData)
	
	indexDataMutex sync.RWMutex
	sensexData     *IndexData
	
	stats = &Stats{startTime: time.Now()}
	
	csvDir string
)

// ==================== BLACK-SCHOLES GREEKS (Same as before) ====================

func normalCDF(x float64) float64 {
	return 0.5 * (1.0 + math.Erf(x/math.Sqrt(2.0)))
}

func normalPDF(x float64) float64 {
	return math.Exp(-0.5*x*x) / math.Sqrt(2.0*math.Pi)
}

func calculateD1D2(S, K, r, sigma, T float64) (float64, float64) {
	if T <= 0 || sigma <= 0 {
		return 0, 0
	}
	d1 := (math.Log(S/K) + (r+0.5*sigma*sigma)*T) / (sigma * math.Sqrt(T))
	d2 := d1 - sigma*math.Sqrt(T)
	return d1, d2
}

func calculateImpliedVol(S, K, r, T, marketPrice float64, isCall bool) (float64, bool) {
	if T <= 0 || marketPrice <= 0 {
		return 0.15, true
	}
	
	sigma := 0.20
	maxIterations := 50
	tolerance := 0.0001
	
	for i := 0; i < maxIterations; i++ {
		d1, d2 := calculateD1D2(S, K, r, sigma, T)
		
		var price, vega float64
		if isCall {
			price = S*normalCDF(d1) - K*math.Exp(-r*T)*normalCDF(d2)
		} else {
			price = K*math.Exp(-r*T)*normalCDF(-d2) - S*normalCDF(-d1)
		}
		
		vega = S * normalPDF(d1) * math.Sqrt(T)
		
		if math.Abs(vega) < 1e-10 {
			return 0.15, true
		}
		
		diff := price - marketPrice
		if math.Abs(diff) < tolerance {
			if sigma > 0.01 && sigma < 2.0 {
				return sigma, false
			}
			return 0.15, true
		}
		
		sigma = sigma - diff/vega
		
		if sigma < 0.01 {
			sigma = 0.01
		} else if sigma > 2.0 {
			sigma = 2.0
		}
	}
	
	return 0.15, true
}

func calculateGreeks(S, K, r, sigma, T float64, isCall bool, marketPrice float64) Greeks {
	greeks := Greeks{
		SpotPrice:    S,
		DaysToExpiry: T * 365,
	}
	
	iv, estimated := calculateImpliedVol(S, K, r, T, marketPrice, isCall)
	greeks.ImpliedVol = iv
	greeks.IVEstimated = estimated
	
	if T <= 0 {
		return greeks
	}
	
	d1, d2 := calculateD1D2(S, K, r, iv, T)
	sqrtT := math.Sqrt(T)
	
	if isCall {
		greeks.IntrinsicValue = math.Max(S-K, 0)
		greeks.Delta = normalCDF(d1)
	} else {
		greeks.IntrinsicValue = math.Max(K-S, 0)
		greeks.Delta = normalCDF(d1) - 1.0
	}
	greeks.TimeValue = marketPrice - greeks.IntrinsicValue
	
	moneyness := S / K
	if moneyness > 1.02 {
		if isCall {
			greeks.Moneyness = "ITM"
		} else {
			greeks.Moneyness = "OTM"
		}
	} else if moneyness < 0.98 {
		if isCall {
			greeks.Moneyness = "OTM"
		} else {
			greeks.Moneyness = "ITM"
		}
	} else {
		greeks.Moneyness = "ATM"
	}
	
	greeks.Gamma = normalPDF(d1) / (S * iv * sqrtT)
	
	term1 := -(S * normalPDF(d1) * iv) / (2 * sqrtT)
	if isCall {
		term2 := r * K * math.Exp(-r*T) * normalCDF(d2)
		greeks.Theta = (term1 - term2) / 365.0
	} else {
		term2 := r * K * math.Exp(-r*T) * normalCDF(-d2)
		greeks.Theta = (term1 + term2) / 365.0
	}
	
	greeks.Vega = S * normalPDF(d1) * sqrtT / 100.0
	
	if isCall {
		greeks.Rho = K * T * math.Exp(-r*T) * normalCDF(d2) / 100.0
	} else {
		greeks.Rho = -K * T * math.Exp(-r*T) * normalCDF(-d2) / 100.0
	}
	
	greeks.Vanna = -normalPDF(d1) * d2 / iv
	greeks.Vomma = S * normalPDF(d1) * sqrtT * d1 * d2 / iv
	
	if isCall {
		term1 := normalPDF(d1) * (2*r*T - d2*iv*sqrtT) / (2 * T * iv * sqrtT)
		term2 := r * math.Exp(-r*T) * normalCDF(d2)
		greeks.Charm = (term1 - term2) / 365.0
	} else {
		term1 := normalPDF(d1) * (2*r*T - d2*iv*sqrtT) / (2 * T * iv * sqrtT)
		term2 := r * math.Exp(-r*T) * normalCDF(-d2)
		greeks.Charm = (term1 + term2) / 365.0
	}
	
	return greeks
}

// ==================== CSV READING ====================

func readLatestIndexData() {
	// Find latest index CSV file
	pattern := filepath.Join(csvDir, "*_index_regular.csv")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		return
	}
	
	// Get most recent file
	latestFile := files[len(files)-1]
	
	file, err := os.Open(latestFile)
	if err != nil {
		return
	}
	defer file.Close()
	
	stats.indexReads.Add(1)
	
	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil || len(records) < 2 {
		return
	}
	
	// Find SENSEX row (skip header)
	for _, record := range records[1:] {
		if len(record) < 7 {
			continue
		}
		
		indexName := strings.TrimSpace(record[3])
		if indexName != "SENSEX" {
			continue
		}
		
		// Parse values
		spotPrice, _ := strconv.ParseFloat(record[4], 64)
		change, _ := strconv.ParseFloat(record[5], 64)
		changePerc, _ := strconv.ParseFloat(record[6], 64)
		
		if spotPrice > 0 {
			indexDataMutex.Lock()
			sensexData = &IndexData{
				IndexName:  "SENSEX",
				SpotPrice:  spotPrice,
				Change:     change,
				ChangePerc: changePerc,
				LastUpdate: time.Now(),
			}
			indexDataMutex.Unlock()
			
			stats.sensexUpdates.Add(1)
			return
		}
	}
}

func readLatestFOData() {
	// Find latest FO CSV file
	pattern := filepath.Join(csvDir, "*_FO_quotes.csv")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		return
	}
	
	// Get most recent file
	latestFile := files[len(files)-1]
	
	file, err := os.Open(latestFile)
	if err != nil {
		return
	}
	defer file.Close()
	
	stats.foReads.Add(1)
	
	scanner := bufio.NewScanner(file)
	scanner.Scan() // Skip header
	
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ",")
		
		if len(parts) < 10 {
			continue
		}
		
		// Parse token
		tokenStr := strings.TrimSpace(parts[1])
		token64, err := strconv.ParseUint(tokenStr, 10, 32)
		if err != nil {
			continue
		}
		token := uint32(token64)
		
		// Check if monitored
		if _, isMonitored := MONITORED_TOKENS[token]; !isMonitored {
			continue
		}
		
		// Parse LTP and volume (volume is at index 13, not 8)
		ltp, _ := strconv.ParseFloat(parts[7], 64)
		volume, _ := strconv.ParseUint(parts[13], 10, 64)
		
		if ltp > 0 && volume > 0 {
			liveDataMutex.Lock()
			
			data, exists := liveDataMap[token]
			if !exists {
				data = &LiveData{Token: token}
				liveDataMap[token] = data
			}
			
			data.LTP = ltp
			data.Volume = volume
			data.LastUpdate = time.Now()
			
			liveDataMutex.Unlock()
			
			stats.tokenUpdates.Add(1)
		}
	}
}

// ==================== GREEKS CALCULATION ====================

func calculateLiveGreeks() {
	indexDataMutex.RLock()
	currentSensex := sensexData
	indexDataMutex.RUnlock()
	
	if currentSensex == nil || currentSensex.SpotPrice <= 0 {
		return
	}
	
	expiryDate := time.Date(2025, 12, 18, 15, 30, 0, 0, time.Local)
	now := time.Now()
	daysToExpiry := expiryDate.Sub(now).Hours() / 24.0
	yearsToExpiry := daysToExpiry / 365.0
	
	if yearsToExpiry <= 0 {
		return
	}
	
	liveDataMutex.Lock()
	defer liveDataMutex.Unlock()
	
	for token, data := range liveDataMap {
		info, exists := MONITORED_TOKENS[token]
		if !exists || data.LTP <= 0 {
			continue
		}
		
		isCall := info.OptionType == "CE"
		
		greeks := calculateGreeks(
			currentSensex.SpotPrice,
			info.Strike,
			RISK_FREE_RATE,
			0.15,
			yearsToExpiry,
			isCall,
			data.LTP,
		)
		
		data.Greeks = greeks
	}
}

// ==================== DISPLAY (Same as UDP version) ====================

func displayLiveData() {
	fmt.Print("\033[H\033[2J")
	
	fmt.Println("╔════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║         BSE LIVE GREEKS CALCULATOR - CSV Watch (Real-time from HFT)          ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	
	runtime := time.Since(stats.startTime)
	fmt.Printf("⏱️  Runtime: %v | CSV Reads: Index=%d, F&O=%d\n",
		runtime.Round(time.Second),
		stats.indexReads.Load(),
		stats.foReads.Load(),
	)
	fmt.Printf("📊 Updates: SENSEX=%d, Tokens=%d\n\n",
		stats.sensexUpdates.Load(),
		stats.tokenUpdates.Load(),
	)
	
	indexDataMutex.RLock()
	if sensexData != nil {
		fmt.Println("┌─ SENSEX Index Data ────────────────────────────────────────────────────────────┐")
		fmt.Printf("│ Spot: ₹%.2f | Change: ₹%.2f (%.2f%%) | Updated: %s ago\n",
			sensexData.SpotPrice,
			sensexData.Change,
			sensexData.ChangePerc,
			time.Since(sensexData.LastUpdate).Round(time.Second),
		)
		fmt.Println("└────────────────────────────────────────────────────────────────────────────────┘")
		fmt.Println()
	} else {
		fmt.Println("⏳ Waiting for SENSEX index data from CSV...")
		fmt.Println()
	}
	indexDataMutex.RUnlock()
	
	liveDataMutex.RLock()
	for token, data := range liveDataMap {
		info := MONITORED_TOKENS[token]
		
		fmt.Printf("┌─ Token: %d ─ %s %s %s %.0f ───────────────────────────────────────────────┐\n",
			token, info.Symbol, info.Expiry, info.OptionType, info.Strike)
		fmt.Println("│ 📊 Market Data                                                                │")
		fmt.Printf("│   LTP:            ₹%-10.2f  Volume:      %12d                    │\n",
			data.LTP, data.Volume)
		fmt.Printf("│   Moneyness:      %-6s       Intrinsic:   ₹%10.2f                    │\n",
			data.Greeks.Moneyness, data.Greeks.IntrinsicValue)
		fmt.Printf("│   Time Value:     ₹%-10.2f  Updated:     %s ago            │\n",
			data.Greeks.TimeValue, time.Since(data.LastUpdate).Round(time.Second))
		fmt.Println("│                                                                               │")
		
		ivStatus := "Calc"
		if data.Greeks.IVEstimated {
			ivStatus = "Est"
		}
		fmt.Printf("│ 💡 Implied Volatility: %.2f%% (%s)                                           │\n",
			data.Greeks.ImpliedVol*100, ivStatus)
		fmt.Println("│                                                                               │")
		fmt.Println("│ 📈 Basic Greeks (First Order)                                                 │")
		fmt.Printf("│   Delta:    %7.4f  │  Gamma:    %8.6f  │  Theta:    %8.2f/day        │\n",
			data.Greeks.Delta, data.Greeks.Gamma, data.Greeks.Theta)
		fmt.Printf("│   Vega:     %7.2f  │  Rho:      %8.2f                                  │\n",
			data.Greeks.Vega, data.Greeks.Rho)
		fmt.Println("│                                                                               │")
		fmt.Println("│ 🎯 Advanced Greeks (Second Order)                                             │")
		fmt.Printf("│   Vanna:    %7.4f  │  Vomma:    %8.2f  │  Charm:    %8.4f/day        │\n",
			data.Greeks.Vanna, data.Greeks.Vomma, data.Greeks.Charm)
		fmt.Println("│                                                                               │")
		fmt.Println("│ 💭 Interpretation:                                                            │")
		
		if info.OptionType == "CE" {
			fmt.Printf("│   • For every ₹100 SENSEX rise, option gains ₹%-5.0f                       │\n",
				math.Abs(data.Greeks.Delta)*100)
		} else {
			fmt.Printf("│   • For every ₹100 SENSEX fall, option gains ₹%-5.0f                       │\n",
				math.Abs(data.Greeks.Delta)*100)
		}
		fmt.Printf("│   • Time decay: Losing ₹%.2f per day (%.0f days to expiry)                    │\n",
			math.Abs(data.Greeks.Theta), data.Greeks.DaysToExpiry)
		fmt.Printf("│   • If volatility rises 1%%, option gains ₹%.2f                               │\n",
			data.Greeks.Vega)
		fmt.Println("└───────────────────────────────────────────────────────────────────────────────┘")
		fmt.Println()
	}
	liveDataMutex.RUnlock()
	
	fmt.Printf("\n🔄 Watching CSV files every %v, display updates every %v. Press Ctrl+C to stop.\n",
		WATCH_INTERVAL, DISPLAY_INTERVAL)
	fmt.Println("════════════════════════════════════════════════════════════════════════════════")
}

// ==================== MAIN ====================

func main() {
	csvDirFlag := flag.String("csv-dir", CSV_DIR_DEFAULT, "Directory containing HFT CSV files")
	flag.Parse()
	
	csvDir = *csvDirFlag
	
	runtime.GOMAXPROCS(runtime.NumCPU())
	
	fmt.Println("╔════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║              BSE LIVE GREEKS CALCULATOR - CSV Watch Mode                      ║")
	fmt.Println("║             Reads CSV files updated by BSE HFT servers                        ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("📂 Watching CSV directory: %s\n", csvDir)
	fmt.Printf("📊 Monitoring %d tokens:\n", len(MONITORED_TOKENS))
	for token, info := range MONITORED_TOKENS {
		fmt.Printf("   • Token %d: %s %s %s %.0f\n",
			token, info.Symbol, info.Expiry, info.OptionType, info.Strike)
	}
	fmt.Println()
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	var wg sync.WaitGroup
	
	// CSV watcher
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(WATCH_INTERVAL)
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				readLatestIndexData()
				readLatestFOData()
			}
		}
	}()
	
	// Greeks calculator
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				calculateLiveGreeks()
			}
		}
	}()
	
	// Display updater
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(DISPLAY_INTERVAL)
		defer ticker.Stop()
		
		time.Sleep(2 * time.Second)
		displayLiveData()
		
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				displayLiveData()
			}
		}
	}()
	
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	<-sigChan
	fmt.Println("\n\n🛑 Shutting down gracefully...")
	cancel()
	
	wg.Wait()
	
	fmt.Println("\n✅ Live Greeks Calculator stopped.")
	fmt.Printf("📊 Final Stats: %d CSV reads (Index: %d, F&O: %d), SENSEX updates: %d, Token updates: %d\n",
		stats.indexReads.Load()+stats.foReads.Load(),
		stats.indexReads.Load(),
		stats.foReads.Load(),
		stats.sensexUpdates.Load(),
		stats.tokenUpdates.Load(),
	)
}
