// BSE Go HFT - Live Token Monitor
// ================================
//
// Monitors specific tokens with LIVE tick-by-tick updates.
// Shows real-time price changes and order book updates.
//
// ================================================================================
// USAGE
// ================================================================================
//
// BUILD:
//   cd d:\bse\bse-go-hft
//   go build -o test_live_token.exe ./tests/test_live_token.go
//
// RUN EXAMPLES:
//
// 1. Monitor Reliance (CM - Equity Cash):
//    .\test_live_token.exe -token 500325 -port 26001 -ticks 50
//
// 2. Monitor SENSEX Future (F&O):
//    .\test_live_token.exe -token 873830 -port 26002 -ticks 50
//
// 3. Monitor BANKEX Option (F&O):
//    .\test_live_token.exe -token 880000 -port 26002 -ticks 100
//
// 4. Custom multicast IP:
//    .\test_live_token.exe -token 500325 -port 26001 -ip 239.1.2.5 -ticks 50

// 5. Custom multicast IP:
//    .\test_live_token.exe -token 1102290 -port 26002 -ip 239.1.2.5 -ticks 50
//
// PARAMETERS:
//   -token int     Token ID to monitor (required)
//   -port int      UDP port: 26001=EQ, 26002=FO (default: 26002)
//   -ip string     Multicast IP (default: "239.1.2.5")
//   -ticks int     Max ticks to capture (default: 100)
//
// OUTPUT:
//   - Live tick display with price changes
//   - CSV file: data/processed_csv/YYYYMMDD_TOKEN_SYMBOL_ticks.csv
//
// ================================================================================

package main

import (
	"bufio"
	"encoding/binary"
	"encoding/csv"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/ipv4"
)

// ================================================================================
// CONFIGURATION
// ================================================================================

const (
	DefaultMulticastIP = "239.1.2.5"
	DefaultPort        = 26002
	HeaderSize         = 36
	RecordSize         = 264
	BufferSize         = 2048
)

// ================================================================================
// DATA STRUCTURES
// ================================================================================

type OrderBook struct {
	BidPrices []float64
	BidQtys   []int32
	BidOrders []int32
	AskPrices []float64
	AskQtys   []int32
	AskOrders []int32
}

type TickData struct {
	Timestamp     time.Time
	Token         uint32
	Symbol        string
	LTP           float64
	Open          float64
	High          float64
	Low           float64
	PrevClose     float64
	ATP           float64
	Volume        int32
	TurnoverLakhs uint32
	LotSize       uint32
	SeqNum        uint32
	OrderBook     OrderBook
}

// ================================================================================
// TOKEN MAP - Loaded from Contract Master
// ================================================================================

// TokenInfo holds contract details
type TokenInfo struct {
	Symbol     string
	Contract   string
	Expiry     string
	OptionType string
	Strike     float64
}

// Global token map - loaded from CSV files
var TokenMap = make(map[uint32]*TokenInfo)

// loadContractMaster loads F&O tokens from Contract Master CSV
func loadContractMaster() int {
	// Find latest contract master file
	pattern := "data/tokens/BSE_EQD_CONTRACT_*.csv"
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		return 0
	}

	// Use the latest file (sort by name - date format ensures order)
	csvPath := files[len(files)-1]

	file, err := os.Open(csvPath)
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	count := 0
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

		// Parse token (column 0)
		tokenID, err := strconv.ParseUint(fields[0], 10, 32)
		if err != nil || tokenID == 0 {
			continue
		}

		// Parse contract details
		symbol := fields[3]    // TckrSymb (underlying)
		contract := fields[18] // Contract name (e.g., SENSEX26JAN85500CE)
		expiry := fields[4]    // Expiry date
		optType := fields[6]   // Option type (CE/PE/XX)

		// Parse strike price
		strike := 0.0
		if len(fields) > 5 {
			if s, err := strconv.ParseFloat(fields[5], 64); err == nil {
				strike = s / 100.0 // Convert paise to rupees
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

// loadBhavCopy loads EQ tokens from BhavCopy CSV
func loadBhavCopy() int {
	// Find latest BhavCopy file
	pattern := "data/tokens/BhavCopy_BSE_CM_*.csv"
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

		// Skip header
		if lineNum == 1 {
			continue
		}

		fields := strings.Split(line, ",")
		if len(fields) < 8 {
			continue
		}

		// Parse token (column 5 = FinInstrmId)
		tokenID, err := strconv.ParseUint(fields[5], 10, 32)
		if err != nil || tokenID == 0 {
			continue
		}

		// Parse symbol (column 7 = TckrSymb)
		symbol := fields[7]

		TokenMap[uint32(tokenID)] = &TokenInfo{
			Symbol:   symbol,
			Contract: symbol,
		}
		count++
	}

	return count
}

func getSymbol(token uint32) string {
	if info, ok := TokenMap[token]; ok {
		if info.Contract != "" {
			return info.Contract
		}
		return info.Symbol
	}
	return fmt.Sprintf("TOKEN_%d", token)
}

func getTokenInfo(token uint32) *TokenInfo {
	if info, ok := TokenMap[token]; ok {
		return info
	}
	return nil
}

// ================================================================================
// PACKET DECODING
// ================================================================================

func parseOrderBook(record []byte) OrderBook {
	ob := OrderBook{
		BidPrices: make([]float64, 0, 5),
		BidQtys:   make([]int32, 0, 5),
		BidOrders: make([]int32, 0, 5),
		AskPrices: make([]float64, 0, 5),
		AskQtys:   make([]int32, 0, 5),
		AskOrders: make([]int32, 0, 5),
	}

	if len(record) < 264 {
		return ob
	}

	for i := 0; i < 5; i++ {
		bidBase := 104 + (i * 32)
		askBase := bidBase + 16

		// BID
		bidPrice := float64(int32(binary.LittleEndian.Uint32(record[bidBase:bidBase+4]))) / 100.0
		bidQty := int32(binary.LittleEndian.Uint32(record[bidBase+4 : bidBase+8]))
		bidOrders := int32(binary.LittleEndian.Uint32(record[bidBase+8 : bidBase+12]))

		// ASK
		askPrice := float64(int32(binary.LittleEndian.Uint32(record[askBase:askBase+4]))) / 100.0
		askQty := int32(binary.LittleEndian.Uint32(record[askBase+4 : askBase+8]))
		askOrders := int32(binary.LittleEndian.Uint32(record[askBase+8 : askBase+12]))

		if bidQty > 0 {
			ob.BidPrices = append(ob.BidPrices, bidPrice)
			ob.BidQtys = append(ob.BidQtys, bidQty)
			ob.BidOrders = append(ob.BidOrders, bidOrders)
		}

		if askQty > 0 {
			ob.AskPrices = append(ob.AskPrices, askPrice)
			ob.AskQtys = append(ob.AskQtys, askQty)
			ob.AskOrders = append(ob.AskOrders, askOrders)
		}
	}

	return ob
}

func decodeRecord(record []byte) *TickData {
	if len(record) < RecordSize {
		return nil
	}

	token := binary.LittleEndian.Uint32(record[0:4])
	if token == 0 {
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

	return &TickData{
		Timestamp:     time.Now(),
		Token:         token,
		Symbol:        getSymbol(token),
		Open:          float64(int32(binary.LittleEndian.Uint32(record[4:8]))) / 100.0,
		PrevClose:     float64(int32(binary.LittleEndian.Uint32(record[8:12]))) / 100.0,
		High:          float64(int32(binary.LittleEndian.Uint32(record[12:16]))) / 100.0,
		Low:           float64(int32(binary.LittleEndian.Uint32(record[16:20]))) / 100.0,
		Volume:        int32(binary.LittleEndian.Uint32(record[24:28])),
		TurnoverLakhs: binary.LittleEndian.Uint32(record[28:32]),
		LotSize:       binary.LittleEndian.Uint32(record[32:36]),
		LTP:           float64(int32(binary.LittleEndian.Uint32(record[36:40]))) / 100.0,
		SeqNum:        seqNum,
		ATP:           atp,
		OrderBook:     parseOrderBook(record),
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
		return fmt.Sprintf(" ▲ +%.2f (+%.2f%%)", change, pct)
	} else if change < 0 {
		return fmt.Sprintf(" ▼ %.2f (%.2f%%)", change, pct)
	}
	return "   ━ 0.00 (0.00%)"
}

func displayTick(data *TickData, tickNum int, prevLTP float64) {
	now := data.Timestamp.Format("15:04:05.000")
	ob := data.OrderBook

	changeStr := formatChange(data.LTP, prevLTP)

	fmt.Printf("\n%s\n", strings.Repeat("═", 80))
	fmt.Printf("  TICK #%-6d  │  %s  │  Token: %d  │  %s\n", tickNum, now, data.Token, data.Symbol)
	fmt.Printf("%s\n", strings.Repeat("═", 80))

	fmt.Printf("\n  💰 LTP: ₹%.2f%s\n", data.LTP, changeStr)
	fmt.Printf("  %s\n", strings.Repeat("─", 72))
	fmt.Printf("  Open: ₹%.2f  │  High: ₹%.2f  │  Low: ₹%.2f  │  Prev: ₹%.2f\n",
		data.Open, data.High, data.Low, data.PrevClose)
	fmt.Printf("  ATP:  ₹%.2f  │  Volume: %d  │  Turnover: ₹%dL  │  Seq: %d\n",
		data.ATP, data.Volume, data.TurnoverLakhs, data.SeqNum)

	// Order Book
	fmt.Printf("\n  📚 ORDER BOOK\n")
	fmt.Printf("  %s\n", strings.Repeat("─", 72))
	fmt.Printf("  %-34s │ %-34s\n", "BID", "ASK")
	fmt.Printf("  %12s  %8s  %5s │ %12s  %8s  %5s\n", "Price", "Qty", "Ord", "Price", "Qty", "Ord")
	fmt.Printf("  %s\n", strings.Repeat("─", 72))

	maxLevels := len(ob.BidPrices)
	if len(ob.AskPrices) > maxLevels {
		maxLevels = len(ob.AskPrices)
	}
	if maxLevels > 5 {
		maxLevels = 5
	}

	for i := 0; i < maxLevels; i++ {
		bidStr := fmt.Sprintf("%12s  %8s  %5s", "--", "--", "--")
		askStr := fmt.Sprintf("%12s  %8s  %5s", "--", "--", "--")

		if i < len(ob.BidPrices) {
			bidStr = fmt.Sprintf("₹%10.2f  %8d  %5d", ob.BidPrices[i], ob.BidQtys[i], ob.BidOrders[i])
		}
		if i < len(ob.AskPrices) {
			askStr = fmt.Sprintf("₹%10.2f  %8d  %5d", ob.AskPrices[i], ob.AskQtys[i], ob.AskOrders[i])
		}

		fmt.Printf("  %s │ %s\n", bidStr, askStr)
	}

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
	today := time.Now().Format("20060102")
	safeSymbol := strings.ReplaceAll(symbol, " ", "_")
	safeSymbol = strings.ReplaceAll(safeSymbol, "/", "-")
	if len(safeSymbol) > 20 {
		safeSymbol = safeSymbol[:20]
	}

	// Create directory
	dir := "data/processed_csv"
	os.MkdirAll(dir, 0755)

	filename := filepath.Join(dir, fmt.Sprintf("%s_%d_%s_ticks.csv", today, token, safeSymbol))

	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	writer := csv.NewWriter(file)

	// Write header
	headers := []string{
		"timestamp", "token", "symbol",
		"ltp", "open", "high", "low", "prev_close", "atp",
		"volume", "turnover_lakhs", "lot_size", "seq",
		"bid_prices", "bid_qtys", "bid_orders",
		"ask_prices", "ask_qtys", "ask_orders",
	}
	writer.Write(headers)

	return &CSVWriter{
		file:     file,
		writer:   writer,
		filename: filename,
	}, nil
}

func (cw *CSVWriter) WriteTick(data *TickData) error {
	ob := data.OrderBook

	// Format arrays as pipe-separated strings
	bidPrices := make([]string, len(ob.BidPrices))
	bidQtys := make([]string, len(ob.BidQtys))
	bidOrders := make([]string, len(ob.BidOrders))
	askPrices := make([]string, len(ob.AskPrices))
	askQtys := make([]string, len(ob.AskQtys))
	askOrders := make([]string, len(ob.AskOrders))

	for i, p := range ob.BidPrices {
		bidPrices[i] = fmt.Sprintf("%.2f", p)
	}
	for i, q := range ob.BidQtys {
		bidQtys[i] = fmt.Sprintf("%d", q)
	}
	for i, o := range ob.BidOrders {
		bidOrders[i] = fmt.Sprintf("%d", o)
	}
	for i, p := range ob.AskPrices {
		askPrices[i] = fmt.Sprintf("%.2f", p)
	}
	for i, q := range ob.AskQtys {
		askQtys[i] = fmt.Sprintf("%d", q)
	}
	for i, o := range ob.AskOrders {
		askOrders[i] = fmt.Sprintf("%d", o)
	}

	row := []string{
		data.Timestamp.Format("2006-01-02 15:04:05.000"),
		fmt.Sprintf("%d", data.Token),
		data.Symbol,
		fmt.Sprintf("%.2f", data.LTP),
		fmt.Sprintf("%.2f", data.Open),
		fmt.Sprintf("%.2f", data.High),
		fmt.Sprintf("%.2f", data.Low),
		fmt.Sprintf("%.2f", data.PrevClose),
		fmt.Sprintf("%.2f", data.ATP),
		fmt.Sprintf("%d", data.Volume),
		fmt.Sprintf("%d", data.TurnoverLakhs),
		fmt.Sprintf("%d", data.LotSize),
		fmt.Sprintf("%d", data.SeqNum),
		strings.Join(bidPrices, "|"),
		strings.Join(bidQtys, "|"),
		strings.Join(bidOrders, "|"),
		strings.Join(askPrices, "|"),
		strings.Join(askQtys, "|"),
		strings.Join(askOrders, "|"),
	}

	return cw.writer.Write(row)
}

func (cw *CSVWriter) Close() {
	cw.writer.Flush()
	cw.file.Close()
}

// ================================================================================
// MULTICAST RECEIVER
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

	// Set multicast options
	pconn := ipv4.NewPacketConn(conn)
	pconn.SetControlMessage(ipv4.FlagTTL|ipv4.FlagDst, true)

	return conn, nil
}

// ================================================================================
// MAIN MONITOR
// ================================================================================

func monitorToken(targetToken uint32, port int, multicastIP string, maxTicks int) {
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("BSE GO HFT - LIVE TOKEN MONITOR")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Token: %d\n", targetToken)

	portName := "F&O - Derivatives"
	if port == 26001 {
		portName = "CM - Equity"
	}
	fmt.Printf("Port:  %d (%s)\n", port, portName)
	fmt.Printf("Time:  %s\n", time.Now().Format("2006-01-02 15:04:05"))

	symbol := getSymbol(targetToken)
	fmt.Printf("\n📊 Token %d → %s\n", targetToken, symbol)

	// Show detailed token info if available
	if info := getTokenInfo(targetToken); info != nil {
		fmt.Printf("   Symbol:     %s\n", info.Symbol)
		fmt.Printf("   Contract:   %s\n", info.Contract)
		if info.Expiry != "" {
			fmt.Printf("   Expiry:     %s\n", info.Expiry)
		}
		if info.OptionType != "" && info.OptionType != "XX" {
			fmt.Printf("   Type:       %s\n", info.OptionType)
		}
		if info.Strike > 0 {
			fmt.Printf("   Strike:     ₹%.2f\n", info.Strike)
		}
	}

	// Create CSV writer
	csvWriter, err := NewCSVWriter(targetToken, symbol)
	if err != nil {
		fmt.Printf("❌ Failed to create CSV file: %v\n", err)
		return
	}
	defer csvWriter.Close()
	fmt.Printf("\n💾 CSV File: %s\n", csvWriter.filename)

	// Connect to multicast
	fmt.Printf("\n📡 Connecting to %s:%d...\n", multicastIP, port)
	conn, err := createMulticastSocket(multicastIP, port)
	if err != nil {
		fmt.Printf("❌ Failed to connect: %v\n", err)
		return
	}
	defer conn.Close()
	fmt.Println("   ✅ Connected!")

	fmt.Printf("\n🚀 STARTING LIVE MONITOR (Press Ctrl+C to stop)\n")
	fmt.Printf("   Watching for token %d (%s)\n", targetToken, symbol)
	fmt.Printf("   Max ticks: %d\n", maxTicks)
	fmt.Println(strings.Repeat("=", 80))

	buffer := make([]byte, BufferSize)
	tickCount := 0
	prevLTP := 0.0
	lastSeq := uint32(0)
	packetsReceived := 0

	for tickCount < maxTicks {
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
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

			tickData := decodeRecord(record)
			if tickData == nil {
				continue
			}

			// Skip duplicates
			if tickData.SeqNum <= lastSeq && lastSeq > 0 {
				continue
			}
			lastSeq = tickData.SeqNum

			tickCount++

			// Display tick
			displayTick(tickData, tickCount, prevLTP)
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
	fmt.Printf("Symbol:     %s\n", symbol)
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
	port := flag.Int("port", DefaultPort, "UDP port: 26001=EQ, 26002=FO")
	ip := flag.String("ip", DefaultMulticastIP, "Multicast IP address")
	ticks := flag.Int("ticks", 100, "Max ticks to capture")
	flag.Parse()

	if *token == 0 {
		fmt.Println("ERROR: -token is required")
		fmt.Println()
		fmt.Println("Usage Examples:")
		fmt.Println("  Reliance (CM):    test_live_token.exe -token 500325 -port 26001 -ticks 50")
		fmt.Println("  SENSEX Fut (FO):  test_live_token.exe -token 873830 -port 26002 -ticks 50")
		fmt.Println()
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Load token maps from CSV files
	fmt.Println("\n📂 Loading token maps...")
	foCount := loadContractMaster()
	eqCount := loadBhavCopy()
	fmt.Printf("   ✅ Loaded %d F&O contracts + %d EQ scripts = %d total\n", foCount, eqCount, len(TokenMap))

	monitorToken(uint32(*token), *port, *ip, *ticks)
}
