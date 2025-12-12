// BSE Live Greeks Calculator from UDP Feed
// Calculates real-time Greeks for selected tokens using live BSE data
//
// Usage:
//   go run cmd/live_greeks_udp/main.go
//   OR
//   go build -o bin/live_greeks_udp.exe cmd/live_greeks_udp/main.go
//   .\bin\live_greeks_udp.exe

package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// ==================== CONFIGURATION ====================

const (
	// BSE UDP Multicast Configuration
	MULTICAST_FO_IP    = "239.1.2.5" // F&O Message Type 2021
	MULTICAST_FO_PORT  = "26002"
	MULTICAST_IDX_IP   = "239.1.2.5"  // Index Message Type 2012
	MULTICAST_IDX_PORT = "26001"
	NETWORK_INTERFACE  = ""            // Empty = use default
	
	// Greeks Calculation
	RISK_FREE_RATE = 0.07  // 7% annual risk-free rate
	
	// Display refresh
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
	packetsReceived atomic.Uint64
	packetsIndex    atomic.Uint64
	packetsFO       atomic.Uint64
	updatesIndex    atomic.Uint64
	updatesFO       atomic.Uint64
	startTime       time.Time
}

// ==================== GLOBAL STATE ====================

var (
	liveDataMutex sync.RWMutex
	liveDataMap   = make(map[uint32]*LiveData)
	
	indexDataMutex sync.RWMutex
	sensexData     *IndexData
	
	stats = &Stats{startTime: time.Now()}
)

// ==================== BLACK-SCHOLES GREEKS ====================

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
	
	// Newton-Raphson IV solver
	sigma := 0.20 // Initial guess
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
	
	// Calculate Implied Volatility
	iv, estimated := calculateImpliedVol(S, K, r, T, marketPrice, isCall)
	greeks.ImpliedVol = iv
	greeks.IVEstimated = estimated
	
	// Use market IV for Greeks calculation
	if T <= 0 {
		return greeks
	}
	
	d1, d2 := calculateD1D2(S, K, r, iv, T)
	sqrtT := math.Sqrt(T)
	
	// Calculate intrinsic and time value
	if isCall {
		greeks.IntrinsicValue = math.Max(S-K, 0)
		greeks.Delta = normalCDF(d1)
	} else {
		greeks.IntrinsicValue = math.Max(K-S, 0)
		greeks.Delta = normalCDF(d1) - 1.0
	}
	greeks.TimeValue = marketPrice - greeks.IntrinsicValue
	
	// Moneyness
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
	
	// Gamma (same for call and put)
	greeks.Gamma = normalPDF(d1) / (S * iv * sqrtT)
	
	// Theta (per day)
	term1 := -(S * normalPDF(d1) * iv) / (2 * sqrtT)
	if isCall {
		term2 := r * K * math.Exp(-r*T) * normalCDF(d2)
		greeks.Theta = (term1 - term2) / 365.0
	} else {
		term2 := r * K * math.Exp(-r*T) * normalCDF(-d2)
		greeks.Theta = (term1 + term2) / 365.0
	}
	
	// Vega (per 1% change in volatility)
	greeks.Vega = S * normalPDF(d1) * sqrtT / 100.0
	
	// Rho (per 1% change in interest rate)
	if isCall {
		greeks.Rho = K * T * math.Exp(-r*T) * normalCDF(d2) / 100.0
	} else {
		greeks.Rho = -K * T * math.Exp(-r*T) * normalCDF(-d2) / 100.0
	}
	
	// Advanced Greeks
	// Vanna: ∂²V/∂S∂σ = -∂Delta/∂σ
	greeks.Vanna = -normalPDF(d1) * d2 / iv
	
	// Vomma: ∂²V/∂σ² = S * φ(d1) * sqrt(T) * d1 * d2 / σ
	greeks.Vomma = S * normalPDF(d1) * sqrtT * d1 * d2 / iv
	
	// Charm: ∂²V/∂S∂t (per day)
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

// ==================== UDP PACKET PROCESSING ====================

func processIndexPacket(packet []byte) {
	if len(packet) < 36 {
		fmt.Printf("⚠️  Index packet too small: %d bytes\n", len(packet))
		return
	}
	
	// Parse message type
	msgType := binary.LittleEndian.Uint16(packet[8:10])
	if msgType != 2012 {
		fmt.Printf("⚠️  Wrong message type for index: %d (expected 2012)\n", msgType)
		return
	}
	
	stats.packetsIndex.Add(1)
	fmt.Printf("✅ Processing Index packet (Type %d), size: %d bytes\n", msgType, len(packet))
	
	// Parse records (starting at offset 36)
	offset := 36
	recordCount := 0
	for offset+44 <= len(packet) {
		// Index name (10 bytes)
		indexName := strings.TrimSpace(string(packet[offset : offset+10]))
		
		// Spot price (4 bytes, big-endian, in paise)
		spotPaise := int32(binary.BigEndian.Uint32(packet[offset+10 : offset+14]))
		spotPrice := float64(spotPaise) / 100.0
		
		// Change (4 bytes, big-endian, in paise)
		changePaise := int32(binary.BigEndian.Uint32(packet[offset+18 : offset+22]))
		change := float64(changePaise) / 100.0
		
		// Change % (4 bytes, big-endian, basis points)
		changePercBP := int32(binary.BigEndian.Uint32(packet[offset+22 : offset+26]))
		changePerc := float64(changePercBP) / 100.0
		
		recordCount++
		fmt.Printf("   Record %d: Index=%s, Spot=₹%.2f, Change=%.2f (%.2f%%)\n",
			recordCount, indexName, spotPrice, change, changePerc)
		
		if indexName == "SENSEX" && spotPrice > 0 {
			indexDataMutex.Lock()
			sensexData = &IndexData{
				IndexName:  indexName,
				SpotPrice:  spotPrice,
				Change:     change,
				ChangePerc: changePerc,
				LastUpdate: time.Now(),
			}
			indexDataMutex.Unlock()
			
			stats.updatesIndex.Add(1)
			fmt.Printf("   🎯 SENSEX FOUND: ₹%.2f\n", spotPrice)
		}
		
		offset += 44
	}
	fmt.Printf("   Total records in packet: %d\n", recordCount)
}

func processFOPacket(packet []byte) {
	if len(packet) < 36 {
		return
	}
	
	// Parse message type
	msgType := binary.LittleEndian.Uint16(packet[8:10])
	if msgType != 2021 {
		return
	}
	
	stats.packetsFO.Add(1)
	
	// Parse records (starting at offset 36, each 66 bytes)
	offset := 36
	for offset+66 <= len(packet) {
		// Token (4 bytes, little-endian)
		token := binary.LittleEndian.Uint32(packet[offset : offset+4])
		
		// Check if this is a monitored token
		if _, isMonitored := MONITORED_TOKENS[token]; !isMonitored {
			offset += 66
			continue
		}
		
		// LTP (4 bytes, big-endian, in paise)
		ltpPaise := int32(binary.BigEndian.Uint32(packet[offset+20 : offset+24]))
		ltp := float64(ltpPaise) / 100.0
		
		// Volume (4 bytes, big-endian)
		volume := uint64(binary.BigEndian.Uint32(packet[offset+24 : offset+28]))
		
		if ltp > 0 && volume > 0 {
			liveDataMutex.Lock()
			
			// Get or create live data
			data, exists := liveDataMap[token]
			if !exists {
				data = &LiveData{Token: token}
				liveDataMap[token] = data
			}
			
			data.LTP = ltp
			data.Volume = volume
			data.LastUpdate = time.Now()
			
			liveDataMutex.Unlock()
			
			stats.updatesFO.Add(1)
		}
		
		offset += 66
	}
}

// ==================== UDP RECEIVERS ====================

func receiveIndex(ctx context.Context) {
	fmt.Printf("📡 Starting Index receiver (2012): %s:%s\n", MULTICAST_IDX_IP, MULTICAST_IDX_PORT)
	
	addr, err := net.ResolveUDPAddr("udp", MULTICAST_IDX_IP+":"+MULTICAST_IDX_PORT)
	if err != nil {
		fmt.Printf("❌ Failed to resolve index address: %v\n", err)
		return
	}
	
	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		fmt.Printf("❌ Failed to listen index multicast: %v\n", err)
		return
	}
	defer conn.Close()
	
	conn.SetReadBuffer(2 * 1024 * 1024) // 2MB buffer
	
	buffer := make([]byte, 2000)
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, _, err := conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				continue
			}
			
			if n > 0 {
				stats.packetsReceived.Add(1)
				processIndexPacket(buffer[:n])
			}
		}
	}
}

func receiveFO(ctx context.Context) {
	fmt.Printf("📡 Starting F&O receiver (2021): %s:%s\n", MULTICAST_FO_IP, MULTICAST_FO_PORT)
	
	addr, err := net.ResolveUDPAddr("udp", MULTICAST_FO_IP+":"+MULTICAST_FO_PORT)
	if err != nil {
		fmt.Printf("❌ Failed to resolve F&O address: %v\n", err)
		return
	}
	
	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		fmt.Printf("❌ Failed to listen F&O multicast: %v\n", err)
		return
	}
	defer conn.Close()
	
	conn.SetReadBuffer(2 * 1024 * 1024) // 2MB buffer
	
	buffer := make([]byte, 2000)
	
	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, _, err := conn.ReadFromUDP(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				continue
			}
			
			if n > 0 {
				stats.packetsReceived.Add(1)
				processFOPacket(buffer[:n])
			}
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
	
	// Calculate time to expiry (18-Dec-2025)
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
			0.15, // Will be replaced by IV calculation
			yearsToExpiry,
			isCall,
			data.LTP,
		)
		
		data.Greeks = greeks
	}
}

// ==================== DISPLAY ====================

func displayLiveData() {
	fmt.Print("\033[H\033[2J") // Clear screen
	
	fmt.Println("╔════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║            BSE LIVE GREEKS CALCULATOR - UDP FEED (Real-time)                 ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	
	// Display stats
	runtime := time.Since(stats.startTime)
	fmt.Printf("⏱️  Runtime: %v | Total Packets: %d (Index: %d, F&O: %d)\n",
		runtime.Round(time.Second),
		stats.packetsReceived.Load(),
		stats.packetsIndex.Load(),
		stats.packetsFO.Load(),
	)
	fmt.Printf("📊 Updates: Index: %d, F&O: %d\n\n",
		stats.updatesIndex.Load(),
		stats.updatesFO.Load(),
	)
	
	// Display SENSEX data
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
		fmt.Println("⏳ Waiting for SENSEX index data...")
	}
	indexDataMutex.RUnlock()
	
	// Display live Greeks for monitored tokens
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
	
	fmt.Printf("\n🔄 Auto-refreshing every %v. Press Ctrl+C to stop.\n", DISPLAY_INTERVAL)
	fmt.Println("════════════════════════════════════════════════════════════════════════════════")
}

// ==================== MAIN ====================

func main() {
	flag.Parse()
	
	runtime.GOMAXPROCS(runtime.NumCPU())
	
	fmt.Println("╔════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                   BSE LIVE GREEKS CALCULATOR - UDP FEED                       ║")
	fmt.Println("║                     Real-time Greeks from Live Market Data                    ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("📊 Monitoring %d tokens:\n", len(MONITORED_TOKENS))
	for token, info := range MONITORED_TOKENS {
		fmt.Printf("   • Token %d: %s %s %s %.0f\n",
			token, info.Symbol, info.Expiry, info.OptionType, info.Strike)
	}
	fmt.Println()
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Start UDP receivers
	var wg sync.WaitGroup
	
	wg.Add(1)
	go func() {
		defer wg.Done()
		receiveIndex(ctx)
	}()
	
	wg.Add(1)
	go func() {
		defer wg.Done()
		receiveFO(ctx)
	}()
	
	// Start Greeks calculator
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
	
	// Start display updater
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(DISPLAY_INTERVAL)
		defer ticker.Stop()
		
		// Initial display after 2 seconds
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
	
	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	<-sigChan
	fmt.Println("\n\n🛑 Shutting down gracefully...")
	cancel()
	
	wg.Wait()
	
	fmt.Println("\n✅ Live Greeks Calculator stopped.")
	fmt.Printf("📊 Final Stats: %d packets received, %d index updates, %d F&O updates\n",
		stats.packetsReceived.Load(),
		stats.updatesIndex.Load(),
		stats.updatesFO.Load(),
	)
}
