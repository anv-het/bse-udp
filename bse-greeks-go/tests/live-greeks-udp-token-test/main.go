// BSE Live Greeks Monitor - DIRECT UDP FEED
// ==========================================
//
// Real-time Greeks calculation from LIVE BSE UDP multicast feeds.
// Reads FO token data + Index spot prices directly from UDP (before CSV save).
//
// ================================================================================
// USAGE
// ================================================================================
//
// BUILD:
//   cd d:\bse\bse-greeks-go
//   go build -o bin/live-greeks-udp.exe ./cmd/live-greeks-udp
//
// RUN EXAMPLES:
//
// 1. Monitor SENSEX CE 84900 (Token 1146822):
//    .\bin\live-greeks-udp.exe -token 1146822 -ticks 10
//
// 2. Monitor SENSEX PE 84900 (Token 1149680):
//    .\bin\live-greeks-udp.exe -token 1149680 -ticks 20
//
// 3. Custom multicast IPs and ports:
//    .\bin\live-greeks-udp.exe -token 1146822 -foip 239.1.2.5 -foport 26002 -indexip 239.1.1.5 -indexport 11401 -ticks 50
//
// 4. Custom risk-free rate:
//    .\bin\live-greeks-udp.exe -token 1146822 -rate 0.07 -ticks 10
//
// PARAMETERS:
//   -token int       Token ID to monitor (required, e.g., 1146822)
//   -ticks int       Maximum ticks to capture (default: 100)
//   -rate float      Risk-free rate (default: 0.065 = 6.5%)
//   -foip string     F&O multicast IP (default: "239.1.2.5")
//   -foport int      F&O UDP port (default: 26002)
//   -indexip string  Index multicast IP (default: "239.1.1.5")
//   -indexport int   Index UDP port (default: 11401)
//
// OUTPUT:
//   - Live terminal display with market data + Greeks
//   - CSV file: data/output/YYYYMMDD_HHMMSS_TOKEN_greeks_udp_live.csv
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
	DefaultFOMulticastIP    = "239.1.2.5"
	DefaultIndexMulticastIP = "239.1.2.5" // Same IP as Index server!
	DefaultFOPort           = 26002
	DefaultIndexPort        = 26001
	DefaultRiskFreeRate     = 0.065 // 6.5% Indian T-Bills
	HeaderSize              = 36
	RecordSize              = 264
	BufferSize              = 2048
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
	Timestamp     time.Time
	Token         uint32
	Symbol        string
	Contract      string
	OptionType    string
	Strike        float64
	Expiry        string
	LTP           float64
	Open          float64
	High          float64
	Low           float64
	PrevClose     float64
	ATP           float64
	Volume        int32
	TurnoverLakhs uint32
	SeqNum        uint32
	SpotPrice     float64 // Latest index spot price
	Greeks        OptionGreeks
	CalcTimeMS    float64
}

type TokenInfo struct {
	Symbol     string
	Contract   string
	Expiry     string
	OptionType string
	Strike     float64
}

// ================================================================================
// GLOBAL STATE
// ================================================================================

var (
	TokenMap = make(map[uint32]*TokenInfo)

	// Spot price cache (thread-safe)
	spotPricesMu sync.RWMutex
	spotPrices   = make(map[string]float64)
)

// ================================================================================
// TOKEN MAP LOADING
// ================================================================================

func loadContractMaster() int {
	pattern := "../bse-go-hft/data/tokens/BSE_EQD_CONTRACT_*.csv"
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		return 0
	}

	csvPath := files[len(files)-1]
	file, err := os.Open(csvPath)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	count := 0
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

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

		TokenMap[uint32(tokenID)] = &TokenInfo{
			Symbol:     symbol,
			Contract:   contract,
			Expiry:     expiry,
			OptionType: optType,
			Strike:     strike,
		}
		count++
	}

	return count
}

func getTokenInfo(token uint32) *TokenInfo {
	if info, ok := TokenMap[token]; ok {
		return info
	}
	return nil
}

// ================================================================================
// SPOT PRICE MANAGEMENT
// ================================================================================

func updateSpotPrice(symbol string, price float64) {
	spotPricesMu.Lock()
	spotPrices[symbol] = price
	spotPricesMu.Unlock()
}

func getSpotPrice(symbol string) (float64, bool) {
	spotPricesMu.RLock()
	price, ok := spotPrices[symbol]
	spotPricesMu.RUnlock()
	return price, ok
}

// ================================================================================
// GREEKS CALCULATION (Black-Scholes)
// ================================================================================

// NormalCDF - Cumulative distribution function
func NormalCDF(x float64) float64 {
	return 0.5 * (1.0 + math.Erf(x/math.Sqrt(2.0)))
}

// NormalPDF - Probability density function
func NormalPDF(x float64) float64 {
	return math.Exp(-0.5*x*x) / math.Sqrt(2.0*math.Pi)
}

// CalculateImpliedVolatility - Newton-Raphson method
func CalculateImpliedVolatility(spotPrice, strikePrice, timeToExpiry, riskFreeRate, marketPrice float64, isCall bool) (float64, bool) {
	if marketPrice <= 0 || spotPrice <= 0 || strikePrice <= 0 || timeToExpiry <= 0 {
		return 0, false
	}

	sigma := 0.20
	if timeToExpiry < 5.0/365.0 {
		sigma = 0.12
	}

	maxIterations := 100
	tolerance := 1e-6

	sqrtT := math.Sqrt(timeToExpiry)

	for i := 0; i < maxIterations; i++ {
		// Calculate d1 and d2
		d1 := (math.Log(spotPrice/strikePrice) + (riskFreeRate+0.5*sigma*sigma)*timeToExpiry) / (sigma * sqrtT)
		d2 := d1 - sigma*sqrtT

		// Calculate option price
		var theoreticalPrice float64
		if isCall {
			theoreticalPrice = spotPrice*NormalCDF(d1) - strikePrice*math.Exp(-riskFreeRate*timeToExpiry)*NormalCDF(d2)
		} else {
			theoreticalPrice = strikePrice*math.Exp(-riskFreeRate*timeToExpiry)*NormalCDF(-d2) - spotPrice*NormalCDF(-d1)
		}

		diff := theoreticalPrice - marketPrice

		if math.Abs(diff) < tolerance {
			return sigma, true
		}

		// Vega for Newton-Raphson
		nd1 := math.Exp(-0.5*d1*d1) / math.Sqrt(2*math.Pi)
		vega := spotPrice * nd1 * sqrtT

		if vega < 1e-10 {
			break
		}

		sigma = sigma - (diff / vega)

		// Bounds
		if sigma < 0.01 {
			sigma = 0.01
		}
		if sigma > 3.0 {
			sigma = 3.0
		}
	}

	return 0, false
}

// CalculateGreeks - All Greeks with IV (CORRECTED FORMULAS)
func CalculateGreeks(spotPrice, strikePrice, timeToExpiry, riskFreeRate, sigma float64, isCall bool) OptionGreeks {
	greeks := OptionGreeks{}

	if spotPrice <= 0 || strikePrice <= 0 || timeToExpiry <= 0 || sigma <= 0 {
		return greeks
	}

	sqrtT := math.Sqrt(timeToExpiry)
	d1 := (math.Log(spotPrice/strikePrice) + (riskFreeRate+0.5*sigma*sigma)*timeToExpiry) / (sigma * sqrtT)
	d2 := d1 - sigma*sqrtT

	nd1 := NormalPDF(d1)
	Nd1 := NormalCDF(d1)
	Nd2 := NormalCDF(d2)

	// Discount factor
	discountFactor := math.Exp(-riskFreeRate * timeToExpiry)

	// Delta
	if isCall {
		greeks.Delta = Nd1
	} else {
		greeks.Delta = Nd1 - 1.0
	}

	// Gamma (same for call and put) - CORRECTED
	// Gamma should NOT have sqrtT in denominator for proper scaling
	greeks.Gamma = nd1 / (spotPrice * sigma * sqrtT)

	// Vega (per 1% volatility change) - CORRECTED
	// Vega = S * N'(d1) * sqrt(T) / 100 (for 1% vol change)
	greeks.Vega = spotPrice * nd1 * sqrtT / 100.0

	// Theta (per calendar day) - CORRECTED FORMULA
	// Standard Black-Scholes Theta formula divided by 365
	if isCall {
		greeks.Theta = ((-spotPrice*nd1*sigma)/(2*sqrtT) - riskFreeRate*strikePrice*discountFactor*Nd2) / 365.0
	} else {
		greeks.Theta = ((-spotPrice*nd1*sigma)/(2*sqrtT) + riskFreeRate*strikePrice*discountFactor*NormalCDF(-d2)) / 365.0
	}

	// Rho (per 1% rate change) - CORRECTED
	if isCall {
		greeks.Rho = strikePrice * timeToExpiry * discountFactor * Nd2 / 100.0
	} else {
		greeks.Rho = -strikePrice * timeToExpiry * discountFactor * NormalCDF(-d2) / 100.0
	}

	// Advanced Greeks - CORRECTED FORMULAS (Version 2)

	// Vanna = dDelta/dSigma = dVega/dS
	// Standard formula: Vanna = -(Vega/S) * (d2/sigma)
	// Or: Vanna = -(N'(d1)/sigma) * d2
	greeks.Vanna = -(greeks.Vega / spotPrice) * (d2 / sigma) * 100.0 // Scaled for 1% vol change

	// Vomma (Volga) = dVega/dSigma
	// Standard formula: Vomma = Vega * (d1 * d2) / sigma
	greeks.Vomma = greeks.Vega * d1 * d2 / sigma * 100.0 // Scaled for 1% vol change

	// Charm = dDelta/dTime - CORRECTED with proper index option formula
	// For index options with no dividends (q=0):
	// Charm = -N'(d1) * [2*r*T - d2*sigma*sqrt(T)] / (2*T*sigma*sqrt(T))
	if timeToExpiry > 1e-6 { // Avoid division by zero near expiry
		greeks.Charm = -nd1 * (2*riskFreeRate*timeToExpiry - d2*sigma*sqrtT) / (2 * timeToExpiry * sigma * sqrtT) / 365.0
	} else {
		greeks.Charm = 0.0
	}

	return greeks
}

// ================================================================================
// UDP DECODERS
// ================================================================================

// DecodeFORecord - Decode F&O record (message type 2020/2021)
func DecodeFORecord(record []byte) *FOTickData {
	if len(record) < RecordSize {
		return nil
	}

	token := binary.LittleEndian.Uint32(record[0:4])
	if token == 0 {
		return nil
	}

	info := getTokenInfo(token)
	if info == nil {
		return nil
	}

	var atp float64
	if len(record) >= 88 {
		atp = float64(int32(binary.LittleEndian.Uint32(record[84:88]))) / 100.0
	}

	var seqNum uint32
	if len(record) >= 48 {
		seqNum = binary.LittleEndian.Uint32(record[44:48])
	}

	return &FOTickData{
		Timestamp:     time.Now(),
		Token:         token,
		Symbol:        info.Symbol,
		Contract:      info.Contract,
		OptionType:    info.OptionType,
		Strike:        info.Strike,
		Expiry:        info.Expiry,
		Open:          float64(int32(binary.LittleEndian.Uint32(record[4:8]))) / 100.0,
		PrevClose:     float64(int32(binary.LittleEndian.Uint32(record[8:12]))) / 100.0,
		High:          float64(int32(binary.LittleEndian.Uint32(record[12:16]))) / 100.0,
		Low:           float64(int32(binary.LittleEndian.Uint32(record[16:20]))) / 100.0,
		Volume:        int32(binary.LittleEndian.Uint32(record[24:28])),
		TurnoverLakhs: binary.LittleEndian.Uint32(record[28:32]),
		LTP:           float64(int32(binary.LittleEndian.Uint32(record[36:40]))) / 100.0,
		SeqNum:        seqNum,
		ATP:           atp,
	}
}

// DecodeIndexRecord - Decode Index record (message type 2012)
func DecodeIndexRecord(record []byte) (string, float64, bool) {
	if len(record) < 40 {
		return "", 0, false
	}

	// Index code (2 bytes at offset 0)
	indexCode := binary.LittleEndian.Uint16(record[0:2])

	// LTP (Last Traded Price) - THE LIVE PRICE THAT CHANGES EVERY TICK!
	// Offset 20, 4 bytes, int32 Little-Endian, in paise (divide by 100)
	ltp := int32(binary.LittleEndian.Uint32(record[20:24]))
	indexValue := float64(ltp) / 100.0

	if indexValue <= 0 {
		return "", 0, false
	}

	// Map index codes to symbols
	indexSymbol := ""
	switch indexCode {
	case 1:
		indexSymbol = "SENSEX"
	case 12: // Corrected from 46
		indexSymbol = "BANKEX"
	case 47:
		indexSymbol = "SNSX50"
	case 2:
		indexSymbol = "BSE100"
	// Add more as needed
	default:
		return "", 0, false
	}

	return indexSymbol, indexValue, true
}

// ================================================================================
// MULTICAST SOCKETS
// ================================================================================

func createMulticastSocket(multicastIP string, port int) (*net.UDPConn, error) {
	addr := fmt.Sprintf("%s:%d", multicastIP, port)
	gaddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenMulticastUDP("udp4", nil, gaddr)
	if err != nil {
		return nil, err
	}

	conn.SetReadBuffer(16 * 1024 * 1024)

	pconn := ipv4.NewPacketConn(conn)
	pconn.SetControlMessage(ipv4.FlagTTL|ipv4.FlagDst, true)

	return conn, nil
}

// ================================================================================
// INDEX FEED LISTENER (Background Goroutine)
// ================================================================================

func listenIndexFeed(multicastIP string, port int, stopCh <-chan struct{}) {
	conn, err := createMulticastSocket(multicastIP, port)
	if err != nil {
		fmt.Printf("❌ Index feed failed: %v\n", err)
		return
	}
	defer conn.Close()

	fmt.Printf("   ✅ Index feed connected (%s:%d)\n", multicastIP, port)

	buffer := make([]byte, BufferSize)

	for {
		select {
		case <-stopCh:
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			continue
		}

		if n < HeaderSize+40 {
			continue
		}

		// Check message type (2012 = Index)
		msgType := binary.LittleEndian.Uint16(buffer[8:10])
		if msgType != 2012 {
			continue
		}

		// Decode index record
		record := buffer[HeaderSize : HeaderSize+40]
		symbol, price, ok := DecodeIndexRecord(record)
		if ok {
			updateSpotPrice(symbol, price)
		}
	}
}

// ================================================================================
// DISPLAY FUNCTIONS
// ================================================================================

func formatChange(current, previous float64) string {
	if previous == 0 {
		return "  NEW"
	}

	change := current - previous
	pct := (change / previous) * 100

	if change > 0 {
		return fmt.Sprintf(" ▲ +%.2f%%", pct)
	} else if change < 0 {
		return fmt.Sprintf(" ▼ %.2f%%", pct)
	}
	return "   ━ 0.00%"
}

func getMoneyness(spotPrice, strikePrice float64, isCall bool) string {
	diff := math.Abs(spotPrice - strikePrice)
	if diff < 100 {
		return "ATM" // Yellow
	}
	if isCall {
		if spotPrice > strikePrice {
			return "ITM" // Green
		}
		return "OTM" // Red
	} else {
		if strikePrice > spotPrice {
			return "ITM" // Green
		}
		return "OTM" // Red
	}
}

func displayTickWithGreeks(data *FOTickData, tickNum int, prevLTP float64) {
	now := data.Timestamp.Format("15:04:05.000")
	changeStr := formatChange(data.LTP, prevLTP)

	fmt.Printf("\n%s\n", strings.Repeat("═", 80))
	fmt.Printf("  TICK #%-6d  │  %s  │  Token: %d  │  %s\n", tickNum, now, data.Token, data.Contract)
	fmt.Printf("%s\n", strings.Repeat("═", 80))

	fmt.Printf("\n  💰 LTP: ₹%.2f%s\n", data.LTP, changeStr)
	fmt.Printf("  %s\n", strings.Repeat("─", 72))
	fmt.Printf("  Open: ₹%.2f  │  High: ₹%.2f  │  Low: ₹%.2f  │  Prev: ₹%.2f\n",
		data.Open, data.High, data.Low, data.PrevClose)
	fmt.Printf("  ATP:  ₹%.2f  │  Volume: %d  │  Turnover: ₹%dL\n",
		data.ATP, data.Volume, data.TurnoverLakhs)

	// Option Details - CORRECTED Days to Expiry calculation
	daysToExpiry := 0
	hoursToExpiry := 0.0
	if data.Expiry != "" {
		// Parse expiry with IST timezone and 15:30:00 time
		loc, _ := time.LoadLocation("Asia/Kolkata")
		expiryDate, err := time.ParseInLocation("02-Jan-2006 15:04:05", data.Expiry+" 15:30:00", loc)
		if err == nil {
			hoursToExpiry = time.Until(expiryDate).Hours()
			daysToExpiry = int(math.Ceil(hoursToExpiry / 24.0)) // Round up to show remaining days
			// If within same day but hours left, show as 1 day
			if hoursToExpiry > 0 && hoursToExpiry < 24 {
				daysToExpiry = 1
			}
		}
	}

	moneyness := getMoneyness(data.SpotPrice, data.Strike, data.OptionType == "CE")
	intrinsicValue := 0.0
	if data.OptionType == "CE" {
		intrinsicValue = math.Max(0, data.SpotPrice-data.Strike)
	} else if data.OptionType == "PE" {
		intrinsicValue = math.Max(0, data.Strike-data.SpotPrice)
	}
	timeValue := data.LTP - intrinsicValue

	fmt.Printf("\n  📋 OPTION DETAILS\n")
	fmt.Printf("  %s\n", strings.Repeat("─", 72))
	fmt.Printf("  Symbol: %s  │  Type: %s  │  Strike: ₹%.2f  │  Expiry: %s\n",
		data.Symbol, data.OptionType, data.Strike, data.Expiry)
	fmt.Printf("  Spot Price: ₹%.2f  │  Moneyness: %s  │  Days to Expiry: %d\n",
		data.SpotPrice, moneyness, daysToExpiry)
	fmt.Printf("  Intrinsic Value: ₹%.2f  │  Time Value: ₹%.2f\n",
		intrinsicValue, timeValue)

	// Greeks
	fmt.Printf("\n  🎯 OPTION GREEKS\n")
	fmt.Printf("  %s\n", strings.Repeat("─", 72))
	fmt.Printf("  Implied Volatility: %.2f%%\n", data.Greeks.ImpliedVolatility*100)
	fmt.Println()
	fmt.Printf("  Delta:         %10.6f   │   Gamma:         %10.6f\n", data.Greeks.Delta, data.Greeks.Gamma)
	fmt.Printf("  Theta:         %10.2f   │   Vega:          %10.2f\n", data.Greeks.Theta, data.Greeks.Vega)
	fmt.Printf("  Rho:           %10.2f\n", data.Greeks.Rho)

	fmt.Printf("\n  🔬 ADVANCED GREEKS\n")
	fmt.Printf("  %s\n", strings.Repeat("─", 72))
	fmt.Printf("  Vanna:  %10.6f   │   Vomma:  %10.2f   │   Charm:  %10.6f\n",
		data.Greeks.Vanna, data.Greeks.Vomma, data.Greeks.Charm)

	fmt.Printf("\n  ⚡ Calculation Time: %.2f ms\n", data.CalcTimeMS)
	fmt.Printf("  %s\n", strings.Repeat("─", 72))
}

// ================================================================================
// CSV WRITER
// ================================================================================

type CSVWriter struct {
	file     *os.File
	writer   *csv.Writer
	filename string
}

func NewCSVWriter(token uint32, symbol string) (*CSVWriter, error) {
	timestamp := time.Now().Format("20060102_150405")
	safeSymbol := strings.ReplaceAll(symbol, " ", "_")
	safeSymbol = strings.ReplaceAll(safeSymbol, "/", "-")

	dir := "data/output"
	os.MkdirAll(dir, 0755)

	filename := filepath.Join(dir, fmt.Sprintf("%s_%d_%s_greeks_udp_live.csv", timestamp, token, safeSymbol))

	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	writer := csv.NewWriter(file)

	// Write header
	headers := []string{
		"timestamp", "token", "symbol", "contract", "option_type", "strike_price", "expiry_date",
		"ltp", "prev_close", "open", "high", "low", "volume", "value", "atp",
		"spot_price", "days_to_expiry", "implied_volatility",
		"delta", "gamma", "theta", "vega", "rho",
		"vanna", "vomma", "charm",
		"intrinsic_value", "time_value", "moneyness", "calc_time_ms",
	}
	writer.Write(headers)

	return &CSVWriter{
		file:     file,
		writer:   writer,
		filename: filename,
	}, nil
}

func (cw *CSVWriter) WriteTick(data *FOTickData) error {
	// CORRECTED Days to Expiry calculation (matching display function)
	daysToExpiry := 0
	if data.Expiry != "" {
		loc, _ := time.LoadLocation("Asia/Kolkata")
		expiryDate, err := time.ParseInLocation("02-Jan-2006 15:04:05", data.Expiry+" 15:30:00", loc)
		if err == nil {
			hoursToExpiry := time.Until(expiryDate).Hours()
			daysToExpiry = int(math.Ceil(hoursToExpiry / 24.0))
			if hoursToExpiry > 0 && hoursToExpiry < 24 {
				daysToExpiry = 1
			}
		}
	}

	moneyness := getMoneyness(data.SpotPrice, data.Strike, data.OptionType == "CE")
	intrinsicValue := 0.0
	if data.OptionType == "CE" {
		intrinsicValue = math.Max(0, data.SpotPrice-data.Strike)
	} else if data.OptionType == "PE" {
		intrinsicValue = math.Max(0, data.Strike-data.SpotPrice)
	}
	timeValue := data.LTP - intrinsicValue

	row := []string{
		data.Timestamp.Format("2006-01-02 15:04:05.000"),
		fmt.Sprintf("%d", data.Token),
		data.Symbol,
		data.Contract,
		data.OptionType,
		fmt.Sprintf("%.2f", data.Strike),
		data.Expiry,
		fmt.Sprintf("%.2f", data.LTP),
		fmt.Sprintf("%.2f", data.PrevClose),
		fmt.Sprintf("%.2f", data.Open),
		fmt.Sprintf("%.2f", data.High),
		fmt.Sprintf("%.2f", data.Low),
		fmt.Sprintf("%d", data.Volume),
		fmt.Sprintf("%d", data.TurnoverLakhs),
		fmt.Sprintf("%.2f", data.ATP),
		fmt.Sprintf("%.2f", data.SpotPrice),
		fmt.Sprintf("%d", daysToExpiry),
		fmt.Sprintf("%.6f", data.Greeks.ImpliedVolatility),
		fmt.Sprintf("%.6f", data.Greeks.Delta),
		fmt.Sprintf("%.6f", data.Greeks.Gamma),
		fmt.Sprintf("%.2f", data.Greeks.Theta),
		fmt.Sprintf("%.2f", data.Greeks.Vega),
		fmt.Sprintf("%.2f", data.Greeks.Rho),
		fmt.Sprintf("%.6f", data.Greeks.Vanna),
		fmt.Sprintf("%.2f", data.Greeks.Vomma),
		fmt.Sprintf("%.6f", data.Greeks.Charm),
		fmt.Sprintf("%.2f", intrinsicValue),
		fmt.Sprintf("%.2f", timeValue),
		moneyness,
		fmt.Sprintf("%.2f", data.CalcTimeMS),
	}

	return cw.writer.Write(row)
}

func (cw *CSVWriter) Close() {
	cw.writer.Flush()
	cw.file.Close()
}

// ================================================================================
// MAIN MONITOR
// ================================================================================

func monitorLiveGreeks(targetToken uint32, foIP string, foPort int, indexIP string, indexPort int, riskFreeRate float64, maxTicks int) {
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("BSE LIVE GREEKS MONITOR - DIRECT UDP FEED")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Token:      %d\n", targetToken)
	fmt.Printf("Risk Rate:  %.2f%%\n", riskFreeRate*100)
	fmt.Printf("Date/Time:  %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(strings.Repeat("=", 80))

	// Get token info
	info := getTokenInfo(targetToken)
	if info == nil {
		fmt.Printf("❌ Token %d not found in contract master\n", targetToken)
		return
	}

	fmt.Printf("\n📊 Token %d → %s\n", targetToken, info.Contract)
	fmt.Printf("   Symbol:     %s\n", info.Symbol)
	if info.Expiry != "" {
		fmt.Printf("   Expiry:     %s\n", info.Expiry)
	}
	if info.OptionType != "" && info.OptionType != "XX" {
		fmt.Printf("   Type:       %s\n", info.OptionType)
	}
	if info.Strike > 0 {
		fmt.Printf("   Strike:     ₹%.2f\n", info.Strike)
	}

	// Create CSV writer
	csvWriter, err := NewCSVWriter(targetToken, info.Contract)
	if err != nil {
		fmt.Printf("❌ Failed to create CSV file: %v\n", err)
		return
	}
	defer csvWriter.Close()
	fmt.Printf("\n💾 CSV File: %s\n", csvWriter.filename)

	// Start index feed listener (background)
	stopCh := make(chan struct{})
	defer close(stopCh)

	fmt.Printf("\n📡 Connecting to feeds...\n")
	fmt.Printf("   Index Feed: %s:%d\n", indexIP, indexPort)
	fmt.Printf("   F&O Feed:   %s:%d\n", foIP, foPort)

	go listenIndexFeed(indexIP, indexPort, stopCh)
	time.Sleep(1 * time.Second) // Let index feed start

	// Connect to F&O multicast
	foConn, err := createMulticastSocket(foIP, foPort)
	if err != nil {
		fmt.Printf("❌ F&O feed failed: %v\n", err)
		return
	}
	defer foConn.Close()
	fmt.Printf("   ✅ F&O feed connected\n")

	// Wait for spot price
	fmt.Printf("\n⏳ Waiting for %s spot price from index feed...\n", info.Symbol)
	spotFound := false
	for i := 0; i < 50; i++ {
		if spot, ok := getSpotPrice(info.Symbol); ok {
			fmt.Printf("   ✅ %s spot price: ₹%.2f\n", info.Symbol, spot)
			spotFound = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if !spotFound {
		fmt.Printf("   ⚠️  No spot price yet, will use when available\n")
	}

	fmt.Printf("\n🚀 STARTING LIVE GREEKS MONITOR (Press Ctrl+C to stop)\n")
	fmt.Printf("   Watching for token %d (%s)\n", targetToken, info.Contract)
	fmt.Printf("   Max ticks: %d\n", maxTicks)
	fmt.Println(strings.Repeat("=", 80))

	buffer := make([]byte, BufferSize)
	tickCount := 0
	prevLTP := 0.0
	lastSeq := uint32(0)
	packetsReceived := 0

	for tickCount < maxTicks {
		foConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, _, err := foConn.ReadFromUDP(buffer)
		if err != nil {
			continue
		}

		packetsReceived++

		if n < HeaderSize+RecordSize {
			continue
		}

		// Validate message type
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
			token := binary.LittleEndian.Uint32(record[0:4])

			// Only process target token
			if token != targetToken {
				continue
			}

			tickData := DecodeFORecord(record)
			if tickData == nil {
				continue
			}

			// Skip duplicates
			if tickData.SeqNum <= lastSeq && lastSeq > 0 {
				continue
			}
			lastSeq = tickData.SeqNum

			// Get latest spot price
			spotPrice, spotOK := getSpotPrice(tickData.Symbol)
			if !spotOK {
				// Try again
				time.Sleep(10 * time.Millisecond)
				spotPrice, spotOK = getSpotPrice(tickData.Symbol)
				if !spotOK {
					fmt.Printf("\n⚠️  No spot price for %s yet, skipping tick\n", tickData.Symbol)
					continue
				}
			}
			tickData.SpotPrice = spotPrice

			// Calculate time to expiry
			timeToExpiry := 0.0
			// if tickData.Expiry != "" {
			// 	if expiry, err := time.Parse("02-Jan-2006", tickData.Expiry); err == nil {
			// 		timeToExpiry = time.Until(expiry).Hours() / (24.0 * 365.0)
			// 	}
			// }
			loc, _ := time.LoadLocation("Asia/Kolkata")

			expiryDate, err := time.ParseInLocation(
				"02-Jan-2006 15:04:05",
				tickData.Expiry+" 15:30:00",
				loc,
			)
			if err == nil {
				timeToExpiry = time.Until(expiryDate).Seconds() / (365.0 * 24 * 60 * 60)
			}

			if timeToExpiry <= 0 {
				fmt.Printf("\n⚠️  Invalid expiry, skipping tick\n")
				continue
			}

			// Calculate Greeks
			startCalc := time.Now()

			// Step 1: Calculate IV from market price
			isCall := (tickData.OptionType == "CE")
			iv, ivOK := CalculateImpliedVolatility(spotPrice, tickData.Strike, timeToExpiry, riskFreeRate, tickData.LTP, isCall)

			if !ivOK {
				fmt.Printf("\n⚠️  IV did not converge, skipping tick\n")
				continue
			}

			// tickData.Greeks.ImpliedVolatility = iv

			// // Step 2: Calculate all Greeks with IV
			// greeks := CalculateGreeks(spotPrice, tickData.Strike, timeToExpiry, riskFreeRate, iv, isCall)
			// tickData.Greeks = greeks
			// tickData.Greeks.ImpliedVolatility = iv // Preserve IV

			greeks := CalculateGreeks(spotPrice, tickData.Strike, timeToExpiry, riskFreeRate, iv, isCall)
			greeks.ImpliedVolatility = iv
			tickData.Greeks = greeks

			tickData.CalcTimeMS = time.Since(startCalc).Seconds() * 1000

			tickCount++

			// Display tick with Greeks
			displayTickWithGreeks(tickData, tickCount, prevLTP)
			prevLTP = tickData.LTP

			// Write to CSV
			csvWriter.WriteTick(tickData)

			if tickCount >= maxTicks {
				break
			}
		}

		// Show progress every 100 packets if no ticks yet
		if tickCount == 0 && packetsReceived%100 == 0 {
			fmt.Printf("\r   ⏳ Searching... %d packets scanned, waiting for token %d...", packetsReceived, targetToken)
		}
	}

	// Summary
	fmt.Printf("\n\n%s\n", strings.Repeat("=", 80))
	fmt.Println("📊 SUMMARY")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Token:      %d\n", targetToken)
	fmt.Printf("Symbol:     %s\n", info.Contract)
	fmt.Printf("Ticks:      %d\n", tickCount)
	fmt.Printf("Packets:    %d\n", packetsReceived)
	fmt.Printf("CSV File:   %s\n", csvWriter.filename)
	fmt.Println(strings.Repeat("=", 80))
}

// ================================================================================
// MAIN
// ================================================================================

func main() {
	token := flag.Uint("token", 0, "Token ID to monitor (required)")
	ticks := flag.Int("ticks", 100, "Max ticks to capture")
	rate := flag.Float64("rate", DefaultRiskFreeRate, "Risk-free rate (0.065 = 6.5%)")
	foIP := flag.String("foip", DefaultFOMulticastIP, "F&O multicast IP")
	foPort := flag.Int("foport", DefaultFOPort, "F&O UDP port")
	indexIP := flag.String("indexip", DefaultIndexMulticastIP, "Index multicast IP")
	indexPort := flag.Int("indexport", DefaultIndexPort, "Index UDP port")
	flag.Parse()

	if *token == 0 {
		fmt.Println("ERROR: -token is required")
		fmt.Println()
		fmt.Println("Usage Examples:")
		fmt.Println("  SENSEX CE 84900:  live-greeks-udp.exe -token 1146822 -ticks 10")
		fmt.Println("  SENSEX PE 84900:  live-greeks-udp.exe -token 1149680 -ticks 20")
		fmt.Println()
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Load token maps from CSV files
	fmt.Println("\n📂 Loading contract master...")
	count := loadContractMaster()
	fmt.Printf("   ✅ Loaded %d F&O contracts\n", count)

	monitorLiveGreeks(uint32(*token), *foIP, *foPort, *indexIP, *indexPort, *rate, *ticks)
}
