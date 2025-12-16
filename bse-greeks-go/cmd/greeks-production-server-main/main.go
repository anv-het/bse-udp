// BSE Live Greeks Production Server - ALL TOKENS
// ===============================================
//
// Production server that:
// - Processes ALL F&O options from UDP feed
// - Tracks spot prices for SENSEX, BANKEX, SNSX50 from Index feed
// - Calculates Greeks for all options in real-time
// - Saves to CSV continuously
//
// ================================================================================
// USAGE
// ================================================================================
//
// BUILD:
//   cd d:\bse\bse-greeks-go
//   go build -o bin/greeks-production-server.exe ./cmd/greeks-production-server
//
// RUN:
//   .\bin\greeks-production-server.exe -duration 1m
//   .\bin\greeks-production-server.exe -duration 5m
//   .\bin\greeks-production-server.exe -duration 1h
//
// PARAMETERS:
//   -duration string   How long to run (e.g., 1m, 5m, 1h) (default: "5m")
//   -rate float        Risk-free rate (default: 0.065 = 6.5%)
//   -output string     Output CSV file path (default: data/output/greeks_live_TIMESTAMP.csv)
//
// ================================================================================

package main

import (
	"bufio"
	"encoding/binary"
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/ipv4"
)

// ================================================================================
// CONFIGURATION
// ================================================================================

const (
	FOMulticastIP       = "239.1.2.5"
	IndexMulticastIP    = "239.1.2.5"
	FOPort              = 26002
	IndexPort           = 26001
	DefaultRiskFreeRate = 0.065
	HeaderSize          = 36
	RecordSize          = 264
	IndexRecordSize     = 56
	BufferSize          = 2048
)

// ================================================================================
// DATA STRUCTURES
// ================================================================================

type OptionGreeks struct {
	ImpliedVolatility float64
	Delta             float64
	Gamma             float64
	Theta             float64
	Vega              float64
	Rho               float64
	Vanna             float64
	Vomma             float64
	Charm             float64
}

type FOTickData struct {
	Timestamp   time.Time
	Token       uint32
	Symbol      string
	Contract    string
	OptionType  string
	StrikePrice float64
	Expiry      string
	LTP         float64
	SpotPrice   float64
	Open        float64
	High        float64
	Low         float64
	Close       float64
	ATP         float64
	Volume      int64
	Turnover    int64
	Greeks      *OptionGreeks
}

type ContractInfo struct {
	Token       uint32
	Symbol      string
	Contract    string
	OptionType  string
	StrikePrice float64
	Expiry      string
}

// ================================================================================
// GLOBALS
// ================================================================================

var (
	spotPrices     = make(map[string]float64)
	spotPricesMux  sync.RWMutex
	contractsCache = make(map[uint32]*ContractInfo)
	contractsMux   sync.RWMutex
	riskFreeRate   = DefaultRiskFreeRate
	csvWriter      *CSVWriter
	csvMux         sync.Mutex
)

// ================================================================================
// SPOT PRICE CACHE
// ================================================================================

func setSpotPrice(symbol string, price float64) {
	spotPricesMux.Lock()
	defer spotPricesMux.Unlock()
	spotPrices[symbol] = price
}

func getSpotPrice(symbol string) (float64, bool) {
	spotPricesMux.RLock()
	defer spotPricesMux.RUnlock()
	price, ok := spotPrices[symbol]
	return price, ok
}

// ================================================================================
// CONTRACT MASTER LOADING
// ================================================================================

func loadContractMaster() error {
	contractsMux.Lock()
	defer contractsMux.Unlock()

	// Use same approach as live-greeks-udp: glob pattern for BSE_EQD_CONTRACT files
	pattern := "../bse-go-hft/data/tokens/BSE_EQD_CONTRACT_*.csv"
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		return fmt.Errorf("no contract files found with pattern: %s", pattern)
	}

	// Use the latest file
	csvPath := files[len(files)-1]
	file, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("failed to open contract file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	totalLoaded := 0
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip header
		if lineNum == 1 && strings.HasPrefix(line, "FinInstrmId") {
			continue
		}

		fields := strings.Split(line, ",")
		if len(fields) < 20 {
			continue
		}

		tokenID, err := strconv.ParseUint(fields[0], 10, 32)
		if err != nil || tokenID == 0 {
			continue
		}

		symbol := fields[3]
		contract := fields[18]
		expiry := fields[4]
		optType := fields[6]

		strike := 0.0
		if len(fields) > 5 {
			if s, err := strconv.ParseFloat(fields[5], 64); err == nil {
				strike = s / 100.0
			}
		}

		contractsCache[uint32(tokenID)] = &ContractInfo{
			Token:       uint32(tokenID),
			Symbol:      symbol,
			Contract:    contract,
			OptionType:  optType,
			StrikePrice: strike,
			Expiry:      expiry,
		}
		totalLoaded++
	}

	fmt.Printf("   ✅ Loaded %d F&O contracts from %s\n", totalLoaded, filepath.Base(csvPath))
	return nil
}

func getContractInfo(token uint32) *ContractInfo {
	contractsMux.RLock()
	defer contractsMux.RUnlock()
	return contractsCache[token]
}

// ================================================================================
// GREEKS CALCULATION (Same as live-greeks-udp)
// ================================================================================

func normalPDF(x float64) float64 {
	return math.Exp(-0.5*x*x) / math.Sqrt(2*math.Pi)
}

func normalCDF(x float64) float64 {
	return 0.5 * (1.0 + math.Erf(x/math.Sqrt2))
}

func blackScholesPrice(spotPrice, strikePrice, timeToExpiry, riskFreeRate, volatility float64, isCall bool) float64 {
	if timeToExpiry <= 0 || volatility <= 0 {
		if isCall {
			return math.Max(spotPrice-strikePrice, 0)
		}
		return math.Max(strikePrice-spotPrice, 0)
	}

	sqrtT := math.Sqrt(timeToExpiry)
	d1 := (math.Log(spotPrice/strikePrice) + (riskFreeRate+0.5*volatility*volatility)*timeToExpiry) / (volatility * sqrtT)
	d2 := d1 - volatility*sqrtT

	if isCall {
		return spotPrice*normalCDF(d1) - strikePrice*math.Exp(-riskFreeRate*timeToExpiry)*normalCDF(d2)
	}
	return strikePrice*math.Exp(-riskFreeRate*timeToExpiry)*normalCDF(-d2) - spotPrice*normalCDF(-d1)
}

func calculateImpliedVolatility(marketPrice, spotPrice, strikePrice, timeToExpiry, riskFreeRate float64, isCall bool) float64 {
	if timeToExpiry <= 0 {
		return 0
	}

	intrinsicValue := 0.0
	if isCall {
		intrinsicValue = math.Max(spotPrice-strikePrice, 0)
	} else {
		intrinsicValue = math.Max(strikePrice-spotPrice, 0)
	}

	if marketPrice <= intrinsicValue+0.01 {
		return 0.01
	}

	initialGuess := 0.20
	if timeToExpiry < 5.0/365.0 {
		initialGuess = 0.12
	}

	volatility := initialGuess
	maxIterations := 100
	tolerance := 1e-6

	for i := 0; i < maxIterations; i++ {
		price := blackScholesPrice(spotPrice, strikePrice, timeToExpiry, riskFreeRate, volatility, isCall)
		diff := price - marketPrice

		if math.Abs(diff) < tolerance {
			return volatility
		}

		sqrtT := math.Sqrt(timeToExpiry)
		d1 := (math.Log(spotPrice/strikePrice) + (riskFreeRate+0.5*volatility*volatility)*timeToExpiry) / (volatility * sqrtT)
		vega := spotPrice * normalPDF(d1) * sqrtT

		if vega < 1e-10 {
			break
		}

		volatility -= diff / vega

		if volatility <= 0.001 {
			volatility = 0.001
		}
		if volatility > 3.0 {
			volatility = 3.0
		}
	}

	return volatility
}

func CalculateGreeks(spotPrice, strikePrice, timeToExpiry, riskFreeRate, marketPrice float64, isCall bool) *OptionGreeks {
	greeks := &OptionGreeks{}

	if timeToExpiry <= 0 {
		return greeks
	}

	// Calculate IV
	sigma := calculateImpliedVolatility(marketPrice, spotPrice, strikePrice, timeToExpiry, riskFreeRate, isCall)
	greeks.ImpliedVolatility = sigma * 100.0

	if sigma <= 0.001 {
		return greeks
	}

	sqrtT := math.Sqrt(timeToExpiry)
	d1 := (math.Log(spotPrice/strikePrice) + (riskFreeRate+0.5*sigma*sigma)*timeToExpiry) / (sigma * sqrtT)
	d2 := d1 - sigma*sqrtT

	nd1 := normalPDF(d1)
	Nd1 := normalCDF(d1)
	Nd2 := normalCDF(d2)

	// Delta
	if isCall {
		greeks.Delta = Nd1
	} else {
		greeks.Delta = Nd1 - 1.0
	}

	// Gamma
	greeks.Gamma = nd1 / (spotPrice * sigma * sqrtT)

	// Theta (per day)
	discountFactor := math.Exp(-riskFreeRate * timeToExpiry)
	if isCall {
		greeks.Theta = ((-spotPrice*nd1*sigma)/(2*sqrtT) - riskFreeRate*strikePrice*discountFactor*Nd2) / 365.0
	} else {
		greeks.Theta = ((-spotPrice*nd1*sigma)/(2*sqrtT) + riskFreeRate*strikePrice*discountFactor*normalCDF(-d2)) / 365.0
	}

	// Vega (per 1% change)
	greeks.Vega = (spotPrice * nd1 * sqrtT) / 100.0

	// Rho (per 1% change)
	if isCall {
		greeks.Rho = (strikePrice * timeToExpiry * discountFactor * Nd2) / 100.0
	} else {
		greeks.Rho = (-strikePrice * timeToExpiry * discountFactor * normalCDF(-d2)) / 100.0
	}

	// Advanced Greeks
	greeks.Vanna = -(greeks.Vega / spotPrice) * (d2 / sigma) * 100.0
	greeks.Vomma = greeks.Vega * d1 * d2 / sigma * 100.0

	if timeToExpiry > 1e-6 {
		greeks.Charm = -nd1 * (2*riskFreeRate*timeToExpiry - d2*sigma*sqrtT) / (2 * timeToExpiry * sigma * sqrtT) / 365.0
	} else {
		greeks.Charm = 0.0
	}

	return greeks
}

// ================================================================================
// UDP DECODERS
// ================================================================================

func DecodeFORecord(record []byte) *FOTickData {
	if len(record) < RecordSize {
		return nil
	}

	token := binary.LittleEndian.Uint32(record[0:4])
	info := getContractInfo(token)
	if info == nil {
		return nil
	}

	// Only process SENSEX, BANKEX, SNSX50 options
	if info.Symbol != "SENSEX" && info.Symbol != "BANKEX" && info.Symbol != "SNSX50" {
		return nil
	}

	// ✅ CORRECT OFFSETS from working decoder:
	// LTP at offset 36 (int32, in paise)
	ltp := int32(binary.LittleEndian.Uint32(record[36:40]))
	ltpFloat := float64(ltp) / 100.0
	if ltpFloat <= 0 {
		return nil
	}

	// All other fields with CORRECT offsets
	open := int32(binary.LittleEndian.Uint32(record[4:8]))       // Offset 4
	high := int32(binary.LittleEndian.Uint32(record[12:16]))     // Offset 12
	low := int32(binary.LittleEndian.Uint32(record[16:20]))      // Offset 16
	prevClose := int32(binary.LittleEndian.Uint32(record[8:12])) // Offset 8 (Previous Close, not current Close)
	volume := int32(binary.LittleEndian.Uint32(record[24:28]))   // Offset 24
	turnover := binary.LittleEndian.Uint32(record[28:32])        // Offset 28 (in lakhs)
	atp := int32(binary.LittleEndian.Uint32(record[84:88]))      // Offset 84

	return &FOTickData{
		Timestamp:   time.Now(),
		Token:       token,
		Symbol:      info.Symbol,
		Contract:    info.Contract,
		OptionType:  info.OptionType,
		StrikePrice: info.StrikePrice,
		Expiry:      info.Expiry,
		LTP:         ltpFloat,
		Open:        float64(open) / 100.0,
		High:        float64(high) / 100.0,
		Low:         float64(low) / 100.0,
		Close:       float64(prevClose) / 100.0, // Using Previous Close as Close
		ATP:         float64(atp) / 100.0,
		Volume:      int64(volume),
		Turnover:    int64(turnover),
	}
}

func DecodeIndexRecord(record []byte) (string, float64, bool) {
	if len(record) < IndexRecordSize {
		return "", 0, false
	}

	indexCode := binary.LittleEndian.Uint16(record[0:2])

	// LTP (Last Traded Price) - THE LIVE PRICE AT OFFSET 20:24 (not 8:12!)
	// Offset 20, 4 bytes, int32 Little-Endian, in paise (divide by 100)
	ltp := int32(binary.LittleEndian.Uint32(record[20:24]))
	indexValue := float64(ltp) / 100.0

	if indexValue <= 0 {
		return "", 0, false
	}

	// Map index codes to symbols
	indexName := ""
	switch indexCode {
	case 1:
		indexName = "SENSEX"
	case 12:
		indexName = "BANKEX"
	case 47:
		indexName = "SNSX50"
	default:
		return "", 0, false
	}

	return indexName, indexValue, true
}

// ================================================================================
// CSV WRITER
// ================================================================================

type CSVWriter struct {
	file   *os.File
	writer *csv.Writer
	mux    sync.Mutex
}

func NewCSVWriter(filePath string) (*CSVWriter, error) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	file, err := os.Create(filePath)
	if err != nil {
		return nil, err
	}

	writer := csv.NewWriter(file)

	// Write header
	header := []string{
		"Timestamp", "Token", "Symbol", "Contract", "OptionType", "Strike", "Expiry",
		"LTP", "SpotPrice", "Open", "High", "Low", "Close", "ATP", "Volume", "Turnover",
		"IV", "Delta", "Gamma", "Theta", "Vega", "Rho", "Vanna", "Vomma", "Charm",
		"Moneyness", "IntrinsicValue", "TimeValue", "TimeToExpiry", "DaysToExpiry",
	}
	if err := writer.Write(header); err != nil {
		file.Close()
		return nil, err
	}
	writer.Flush()

	return &CSVWriter{
		file:   file,
		writer: writer,
	}, nil
}

func (w *CSVWriter) WriteTick(data *FOTickData) error {
	w.mux.Lock()
	defer w.mux.Unlock()

	if data.Greeks == nil {
		return nil
	}

	// Calculate additional fields
	moneyness := "ATM"
	if data.OptionType == "CE" {
		if data.SpotPrice > data.StrikePrice+50 {
			moneyness = "ITM"
		} else if data.SpotPrice < data.StrikePrice-50 {
			moneyness = "OTM"
		}
	} else {
		if data.SpotPrice < data.StrikePrice-50 {
			moneyness = "ITM"
		} else if data.SpotPrice > data.StrikePrice+50 {
			moneyness = "OTM"
		}
	}

	intrinsicValue := 0.0
	if data.OptionType == "CE" {
		intrinsicValue = math.Max(data.SpotPrice-data.StrikePrice, 0)
	} else {
		intrinsicValue = math.Max(data.StrikePrice-data.SpotPrice, 0)
	}
	timeValue := data.LTP - intrinsicValue

	// Days to expiry
	daysToExpiry := 0
	hoursToExpiry := 0.0
	if data.Expiry != "" {
		loc, _ := time.LoadLocation("Asia/Kolkata")
		expiryDate, err := time.ParseInLocation("02-Jan-2006 15:04:05", data.Expiry+" 15:30:00", loc)
		if err == nil {
			hoursToExpiry = time.Until(expiryDate).Hours()
			daysToExpiry = int(math.Ceil(hoursToExpiry / 24.0))
			if hoursToExpiry > 0 && hoursToExpiry < 24 {
				daysToExpiry = 1
			}
		}
	}

	timeToExpiry := hoursToExpiry / (24.0 * 365.0)

	row := []string{
		data.Timestamp.Format("2006-01-02 15:04:05.000"),
		fmt.Sprintf("%d", data.Token),
		data.Symbol,
		data.Contract,
		data.OptionType,
		fmt.Sprintf("%.2f", data.StrikePrice),
		data.Expiry,
		fmt.Sprintf("%.2f", data.LTP),
		fmt.Sprintf("%.2f", data.SpotPrice),
		fmt.Sprintf("%.2f", data.Open),
		fmt.Sprintf("%.2f", data.High),
		fmt.Sprintf("%.2f", data.Low),
		fmt.Sprintf("%.2f", data.Close),
		fmt.Sprintf("%.2f", data.ATP),
		fmt.Sprintf("%d", data.Volume),
		fmt.Sprintf("%d", data.Turnover),
		fmt.Sprintf("%.2f", data.Greeks.ImpliedVolatility),
		fmt.Sprintf("%.6f", data.Greeks.Delta),
		fmt.Sprintf("%.6f", data.Greeks.Gamma),
		fmt.Sprintf("%.2f", data.Greeks.Theta),
		fmt.Sprintf("%.2f", data.Greeks.Vega),
		fmt.Sprintf("%.2f", data.Greeks.Rho),
		fmt.Sprintf("%.6f", data.Greeks.Vanna),
		fmt.Sprintf("%.2f", data.Greeks.Vomma),
		fmt.Sprintf("%.6f", data.Greeks.Charm),
		moneyness,
		fmt.Sprintf("%.2f", intrinsicValue),
		fmt.Sprintf("%.2f", timeValue),
		fmt.Sprintf("%.6f", timeToExpiry),
		fmt.Sprintf("%d", daysToExpiry),
	}

	if err := w.writer.Write(row); err != nil {
		return err
	}
	w.writer.Flush()
	return nil
}

func (w *CSVWriter) Close() {
	w.mux.Lock()
	defer w.mux.Unlock()
	w.writer.Flush()
	w.file.Close()
}

// ================================================================================
// MULTICAST SOCKET CREATION
// ================================================================================

func createMulticastSocket(multicastIP string, port int) (*net.UDPConn, error) {
	addr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: port,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}

	p := ipv4.NewPacketConn(conn)
	ifi, err := net.InterfaceByName("Ethernet")
	if err != nil {
		ifi = nil
	}

	group := net.ParseIP(multicastIP)
	if err := p.JoinGroup(ifi, &net.UDPAddr{IP: group}); err != nil {
		conn.Close()
		return nil, err
	}

	return conn, nil
}

// ================================================================================
// INDEX FEED MONITOR (Background)
// ================================================================================

func monitorIndexFeed(stopChan chan struct{}) {
	conn, err := createMulticastSocket(IndexMulticastIP, IndexPort)
	if err != nil {
		fmt.Printf("❌ Index feed failed: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Printf("   ✅ Index feed connected (239.1.2.5:26001)\n")

	buffer := make([]byte, BufferSize)

	for {
		select {
		case <-stopChan:
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			continue
		}

		if n < HeaderSize+IndexRecordSize {
			continue
		}

		// Validate message type (2012 for Index)
		msgType := binary.LittleEndian.Uint16(buffer[8:10])
		if msgType != 2012 {
			continue
		}

		// Parse records
		numRecords := (n - HeaderSize) / IndexRecordSize
		if numRecords > 20 {
			numRecords = 20
		}

		for i := 0; i < numRecords; i++ {
			offset := HeaderSize + (i * IndexRecordSize)
			if offset+IndexRecordSize > n {
				break
			}

			record := buffer[offset : offset+IndexRecordSize]
			indexName, indexValue, ok := DecodeIndexRecord(record)
			if ok && indexValue > 0 {
				setSpotPrice(indexName, indexValue)
			}
		}
	}
}

// ================================================================================
// F&O FEED MONITOR (Main)
// ================================================================================

func monitorFOFeed(stopChan chan struct{}, stats *Stats) {
	conn, err := createMulticastSocket(FOMulticastIP, FOPort)
	if err != nil {
		fmt.Printf("❌ F&O feed failed: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Printf("   ✅ F&O feed connected (239.1.2.5:26002)\n")

	buffer := make([]byte, BufferSize)
	seenTokens := make(map[uint32]bool)

	for {
		select {
		case <-stopChan:
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			continue
		}

		stats.PacketsReceived++

		if n < HeaderSize+RecordSize {
			continue
		}

		// Validate message type (2020/2021 for F&O)
		msgType := binary.LittleEndian.Uint16(buffer[8:10])
		if msgType != 2020 && msgType != 2021 {
			continue
		}

		// Parse records
		numRecords := (n - HeaderSize) / RecordSize
		if numRecords > 8 {
			numRecords = 8
		}

		for i := 0; i < numRecords; i++ {
			offset := HeaderSize + (i * RecordSize)
			if offset+RecordSize > n {
				break
			}

			record := buffer[offset : offset+RecordSize]
			tickData := DecodeFORecord(record)
			if tickData == nil {
				continue
			}

			// Get spot price
			spotPrice, ok := getSpotPrice(tickData.Symbol)
			if !ok {
				continue
			}
			tickData.SpotPrice = spotPrice

			// Calculate time to expiry
			timeToExpiry := 0.0
			if tickData.Expiry != "" {
				loc, _ := time.LoadLocation("Asia/Kolkata")
				expiryDate, err := time.ParseInLocation("02-Jan-2006 15:04:05", tickData.Expiry+" 15:30:00", loc)
				if err == nil {
					timeToExpiry = time.Until(expiryDate).Hours() / (24.0 * 365.0)
				}
			}

			if timeToExpiry <= 0 {
				continue
			}

			// Calculate Greeks
			isCall := tickData.OptionType == "CE"
			greeks := CalculateGreeks(
				tickData.SpotPrice,
				tickData.StrikePrice,
				timeToExpiry,
				riskFreeRate,
				tickData.LTP,
				isCall,
			)
			tickData.Greeks = greeks

			// Save to CSV
			csvMux.Lock()
			if csvWriter != nil {
				csvWriter.WriteTick(tickData)
			}
			csvMux.Unlock()

			// Track unique tokens
			if !seenTokens[tickData.Token] {
				seenTokens[tickData.Token] = true
				stats.UniqueTokens++
			}
			stats.TicksProcessed++

			// Print summary every 100 ticks
			if stats.TicksProcessed%100 == 0 {
				fmt.Printf("\r⏱️  Processed: %d ticks | %d unique tokens | %d packets",
					stats.TicksProcessed, stats.UniqueTokens, stats.PacketsReceived)
			}
		}
	}
}

// ================================================================================
// STATS
// ================================================================================

type Stats struct {
	TicksProcessed  int
	UniqueTokens    int
	PacketsReceived int
}

// ================================================================================
// MAIN
// ================================================================================

func main() {
	// Parse flags
	durationFlag := flag.String("duration", "5m", "Duration to run (e.g., 1m, 5m, 1h)")
	rateFlag := flag.Float64("rate", DefaultRiskFreeRate, "Risk-free rate (default: 0.065)")
	outputFlag := flag.String("output", "", "Output CSV file path")
	flag.Parse()

	riskFreeRate = *rateFlag

	// Parse duration
	duration, err := time.ParseDuration(*durationFlag)
	if err != nil {
		fmt.Printf("❌ Invalid duration: %v\n", err)
		return
	}

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("BSE LIVE GREEKS PRODUCTION SERVER - ALL OPTIONS")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Duration:   %v\n", duration)
	fmt.Printf("Risk Rate:  %.2f%%\n", riskFreeRate*100)
	fmt.Printf("Date/Time:  %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(strings.Repeat("=", 80))

	// Load contract master
	fmt.Printf("\n📂 Loading contract master...\n")
	if err := loadContractMaster(); err != nil {
		fmt.Printf("❌ Failed to load contracts: %v\n", err)
		return
	}

	// Create CSV writer
	outputPath := *outputFlag
	if outputPath == "" {
		outputPath = fmt.Sprintf("data/output/greeks_live_%s.csv", time.Now().Format("20060102_150405"))
	}

	fmt.Printf("\n💾 CSV File: %s\n", outputPath)
	csvWriter, err = NewCSVWriter(outputPath)
	if err != nil {
		fmt.Printf("❌ Failed to create CSV: %v\n", err)
		return
	}
	defer csvWriter.Close()

	// Start feeds
	fmt.Printf("\n📡 Connecting to feeds...\n")
	stopChan := make(chan struct{})
	stats := &Stats{}

	// Start index feed (background)
	go monitorIndexFeed(stopChan)
	time.Sleep(500 * time.Millisecond)

	// Wait for initial spot prices
	fmt.Printf("\n⏳ Waiting for spot prices...\n")
	for i := 0; i < 50; i++ {
		sensex, sensexOK := getSpotPrice("SENSEX")
		bankex, bankexOK := getSpotPrice("BANKEX")
		snsx50, snsx50OK := getSpotPrice("SNSX50")

		if sensexOK {
			fmt.Printf("   ✅ SENSEX: ₹%.2f\n", sensex)
		}
		if bankexOK {
			fmt.Printf("   ✅ BANKEX: ₹%.2f\n", bankex)
		}
		if snsx50OK {
			fmt.Printf("   ✅ SNSX50: ₹%.2f\n", snsx50)
		}

		if sensexOK || bankexOK || snsx50OK {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	fmt.Printf("\n🚀 STARTING LIVE GREEKS SERVER\n")
	fmt.Printf("   Processing ALL F&O options for SENSEX, BANKEX, SNSX50\n")
	fmt.Printf("   Duration: %v\n", duration)
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	// Start F&O feed
	go monitorFOFeed(stopChan, stats)

	// Run for specified duration
	time.Sleep(duration)

	// Stop
	close(stopChan)
	time.Sleep(500 * time.Millisecond)

	// Final stats
	fmt.Println()
	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("📊 FINAL STATISTICS")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Ticks Processed:  %d\n", stats.TicksProcessed)
	fmt.Printf("Unique Tokens:    %d\n", stats.UniqueTokens)
	fmt.Printf("Packets Received: %d\n", stats.PacketsReceived)
	fmt.Printf("CSV File:         %s\n", outputPath)
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("✅ Server stopped gracefully")
}
