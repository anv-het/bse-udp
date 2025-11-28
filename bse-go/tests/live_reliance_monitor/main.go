/*
BSE Live Reliance Monitor - Real-Time Equity Display (Go)
==========================================================

Monitors Reliance Industries (Token: 500325) with LIVE tick-by-tick updates.
Shows real-time price changes and order book updates for CM segment.

Usage:
  go run . --ticks 50

Features:
- Dedicated monitor for RELIANCE equity (BSE CM)
- Live tick display with price change highlighting
- Per-run CSV file with all ticks
- Order book depth (Best 5 Bid/Ask)
*/

package main

import (
	"encoding/binary"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"bse-go/pkg/data_collector"
)

// Reliance token on BSE
const RelianceToken uint32 = 500325

// RelianceTick represents a captured tick
type RelianceTick struct {
	Timestamp     time.Time
	Token         uint32
	Symbol        string
	CompanyName   string
	LTP           float64
	Open          float64
	High          float64
	Low           float64
	PrevClose     float64
	ATP           float64
	Volume        int32
	TurnoverLakhs uint32
	SeqNumber     uint32
	BidPrices     []float64
	BidQtys       []int32
	BidOrders     []int32
	AskPrices     []float64
	AskQtys       []int32
	AskOrders     []int32
}

func main() {
	maxTicks := flag.Int("ticks", 100, "Max ticks to capture")
	token := flag.Int("token", int(RelianceToken), "Token to monitor (default: 500325 for Reliance)")
	multicastIP := flag.String("ip", "239.1.2.5", "Multicast IP")
	port := flag.Int("port", 26001, "Port (26001 for CM)")
	dataDir := flag.String("data", "data", "Data directory for token files")
	flag.Parse()

	targetToken := uint32(*token)

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("BSE LIVE RELIANCE/EQUITY MONITOR (Go)")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Time:   %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("Token:  %d\n", targetToken)
	fmt.Printf("Port:   %d (CM - Equity)\n", *port)

	// Load CM token mapping
	fmt.Println("\n📋 Loading CM symbol mapping...")
	collector := data_collector.NewMarketDataCollector("CM", *dataDir)
	stats := collector.GetStats()
	fmt.Printf("   ✅ Loaded %d equity tokens\n", stats["tokens_loaded"])

	// Get symbol info
	symbolInfo := collector.GetTokenInfo(targetToken)
	symbol := fmt.Sprintf("TOKEN_%d", targetToken)
	companyName := symbol
	if symbolInfo != nil {
		symbol = symbolInfo.Symbol
		companyName = symbolInfo.CompanyName
		if companyName == "" {
			companyName = symbol
		}
	}
	fmt.Printf("   📊 Token %d → %s (%s)\n", targetToken, symbol, companyName)

	// Create output directory and CSV file
	outputDir := filepath.Join(*dataDir, "processed_csv")
	os.MkdirAll(outputDir, 0755)

	timestamp := time.Now().Format("20060102_150405")
	safeSymbol := strings.ReplaceAll(strings.ReplaceAll(symbol, " ", "_"), "/", "-")
	csvFilename := filepath.Join(outputDir, fmt.Sprintf("%s_%d_%s_live.csv", timestamp, targetToken, safeSymbol))

	csvFile, err := os.Create(csvFilename)
	if err != nil {
		log.Fatalf("Failed to create CSV file: %v", err)
	}
	defer csvFile.Close()

	csvWriter := csv.NewWriter(csvFile)
	defer csvWriter.Flush()

	// Write CSV header
	csvWriter.Write([]string{
		"timestamp", "token", "symbol", "company_name",
		"ltp", "open", "high", "low", "prev_close", "atp",
		"volume", "turnover_lakhs", "seq",
		"bid_prices", "bid_qtys", "bid_orders",
		"ask_prices", "ask_qtys", "ask_orders",
	})

	fmt.Printf("\n💾 CSV File: %s\n", csvFilename)

	// Connect to multicast
	fmt.Printf("\n📡 Connecting to %s:%d...\n", *multicastIP, *port)

	conn, err := net.ListenMulticastUDP("udp", nil, &net.UDPAddr{
		IP:   net.ParseIP(*multicastIP),
		Port: *port,
	})
	if err != nil {
		log.Fatalf("Failed to join multicast: %v", err)
	}
	defer conn.Close()

	conn.SetReadBuffer(1024 * 1024)
	fmt.Println("   ✅ Connected!")

	fmt.Printf("\n🚀 STARTING %s MONITOR (Press Ctrl+C to stop)\n", symbol)
	fmt.Printf("   Token: %d\n", targetToken)
	fmt.Printf("   Max ticks: %d\n", *maxTicks)
	fmt.Println(strings.Repeat("=", 80))

	// Signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	tickCount := 0
	prevLTP := 0.0
	lastSeq := uint32(0)
	buf := make([]byte, 2048)

monitorLoop:
	for tickCount < *maxTicks {
		select {
		case <-sigChan:
			fmt.Println("\n\n⏹ Stopped by user")
			break monitorLoop
		default:
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				continue
			}

			if n < 300 {
				continue
			}

			// Parse records
			numRecords := (n - 36) / 264

			for i := 0; i < numRecords; i++ {
				offset := 36 + (i * 264)
				if offset+264 > n {
					break
				}

				recordBytes := buf[offset : offset+264]
				recToken := binary.LittleEndian.Uint32(recordBytes[0:4])

				if recToken == targetToken {
					tick := decodeRelianceTick(recordBytes, symbol, companyName)

					// Skip duplicate sequence
					if tick.SeqNumber == lastSeq {
						continue
					}

					lastSeq = tick.SeqNumber
					tickCount++

					// Display live
					displayRelianceTick(tick, tickCount, prevLTP)
					prevLTP = tick.LTP

					// Write to CSV
					writeRelianceTickToCSV(csvWriter, tick)
					csvWriter.Flush()

					fmt.Printf("\n  💾 Saved to CSV (%d ticks)\n", tickCount)
				}
			}
		}
	}

	// Summary
	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("📊 SUMMARY")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("Token:       %d\n", targetToken)
	fmt.Printf("Symbol:      %s\n", symbol)
	fmt.Printf("Company:     %s\n", companyName)
	fmt.Printf("Ticks:       %d\n", tickCount)
	fmt.Printf("CSV File:    %s\n", csvFilename)
	fmt.Println(strings.Repeat("=", 80))
}

func decodeRelianceTick(data []byte, symbol, companyName string) *RelianceTick {
	tick := &RelianceTick{
		Timestamp:     time.Now(),
		Token:         binary.LittleEndian.Uint32(data[0:4]),
		Symbol:        symbol,
		CompanyName:   companyName,
		Open:          float64(int32(binary.LittleEndian.Uint32(data[4:8]))) / 100.0,
		PrevClose:     float64(int32(binary.LittleEndian.Uint32(data[8:12]))) / 100.0,
		High:          float64(int32(binary.LittleEndian.Uint32(data[12:16]))) / 100.0,
		Low:           float64(int32(binary.LittleEndian.Uint32(data[16:20]))) / 100.0,
		Volume:        int32(binary.LittleEndian.Uint32(data[24:28])),
		TurnoverLakhs: binary.LittleEndian.Uint32(data[28:32]),
		LTP:           float64(int32(binary.LittleEndian.Uint32(data[36:40]))) / 100.0,
		ATP:           float64(int32(binary.LittleEndian.Uint32(data[84:88]))) / 100.0,
		SeqNumber:     binary.LittleEndian.Uint32(data[44:48]),
	}

	// Parse order book
	for i := 0; i < 5; i++ {
		bidBase := 104 + (i * 32)
		askBase := bidBase + 16

		bidPrice := float64(int32(binary.LittleEndian.Uint32(data[bidBase:bidBase+4]))) / 100.0
		bidQty := int32(binary.LittleEndian.Uint32(data[bidBase+4 : bidBase+8]))
		bidOrders := int32(binary.LittleEndian.Uint32(data[bidBase+8 : bidBase+12]))

		askPrice := float64(int32(binary.LittleEndian.Uint32(data[askBase:askBase+4]))) / 100.0
		askQty := int32(binary.LittleEndian.Uint32(data[askBase+4 : askBase+8]))
		askOrders := int32(binary.LittleEndian.Uint32(data[askBase+8 : askBase+12]))

		if bidQty > 0 {
			tick.BidPrices = append(tick.BidPrices, bidPrice)
			tick.BidQtys = append(tick.BidQtys, bidQty)
			tick.BidOrders = append(tick.BidOrders, bidOrders)
		}

		if askQty > 0 {
			tick.AskPrices = append(tick.AskPrices, askPrice)
			tick.AskQtys = append(tick.AskQtys, askQty)
			tick.AskOrders = append(tick.AskOrders, askOrders)
		}
	}

	return tick
}

func displayRelianceTick(tick *RelianceTick, tickNum int, prevLTP float64) {
	now := tick.Timestamp.Format("15:04:05.000")

	// Calculate change
	change := tick.LTP - tick.PrevClose
	changePct := 0.0
	if tick.PrevClose > 0 {
		changePct = (change / tick.PrevClose) * 100
	}

	// Tick change
	tickChange := tick.LTP - prevLTP
	tickIndicator := "━"
	if prevLTP > 0 {
		if tickChange > 0 {
			tickIndicator = "▲"
		} else if tickChange < 0 {
			tickIndicator = "▼"
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("═", 80))
	fmt.Printf("  TICK #%-6d  │  %s  │  Token: %d\n", tickNum, now, tick.Token)
	fmt.Printf("  %s (%s)\n", tick.Symbol, tick.CompanyName)
	fmt.Println(strings.Repeat("═", 80))

	fmt.Printf("\n  💰 LTP: ₹%.2f  %s %+.2f (%+.2f%% from prev close)\n",
		tick.LTP, tickIndicator, change, changePct)
	if prevLTP > 0 {
		fmt.Printf("     Tick Change: %+.2f (from ₹%.2f)\n", tickChange, prevLTP)
	}

	fmt.Println("  " + strings.Repeat("─", 74))
	fmt.Printf("  Open: ₹%.2f  │  High: ₹%.2f  │  Low: ₹%.2f  │  Prev: ₹%.2f\n",
		tick.Open, tick.High, tick.Low, tick.PrevClose)
	fmt.Printf("  ATP:  ₹%.2f  │  Volume: %d  │  Turnover: ₹%dL  │  Seq: %d\n",
		tick.ATP, tick.Volume, tick.TurnoverLakhs, tick.SeqNumber)

	// Order Book
	fmt.Println("\n  📚 ORDER BOOK")
	fmt.Println("  " + strings.Repeat("─", 72))
	fmt.Printf("  %34s │ %34s\n", "BID", "ASK")
	fmt.Printf("  %12s  %8s  %5s │ %12s  %8s  %5s\n", "Price", "Qty", "Ord", "Price", "Qty", "Ord")
	fmt.Println("  " + strings.Repeat("─", 72))

	maxLevels := len(tick.BidPrices)
	if len(tick.AskPrices) > maxLevels {
		maxLevels = len(tick.AskPrices)
	}
	if maxLevels > 5 {
		maxLevels = 5
	}

	for i := 0; i < maxLevels; i++ {
		bidStr := fmt.Sprintf("%12s  %8s  %5s", "--", "--", "--")
		askStr := fmt.Sprintf("%12s  %8s  %5s", "--", "--", "--")

		if i < len(tick.BidPrices) {
			bidStr = fmt.Sprintf("₹%10.2f  %8d  %5d", tick.BidPrices[i], tick.BidQtys[i], tick.BidOrders[i])
		}

		if i < len(tick.AskPrices) {
			askStr = fmt.Sprintf("₹%10.2f  %8d  %5d", tick.AskPrices[i], tick.AskQtys[i], tick.AskOrders[i])
		}

		fmt.Printf("  %s │ %s\n", bidStr, askStr)
	}

	fmt.Println("  " + strings.Repeat("─", 72))
}

func writeRelianceTickToCSV(writer *csv.Writer, tick *RelianceTick) {
	bidPrices := make([]string, len(tick.BidPrices))
	bidQtys := make([]string, len(tick.BidQtys))
	bidOrders := make([]string, len(tick.BidOrders))
	askPrices := make([]string, len(tick.AskPrices))
	askQtys := make([]string, len(tick.AskQtys))
	askOrders := make([]string, len(tick.AskOrders))

	for i, p := range tick.BidPrices {
		bidPrices[i] = fmt.Sprintf("%.2f", p)
	}
	for i, q := range tick.BidQtys {
		bidQtys[i] = strconv.Itoa(int(q))
	}
	for i, o := range tick.BidOrders {
		bidOrders[i] = strconv.Itoa(int(o))
	}
	for i, p := range tick.AskPrices {
		askPrices[i] = fmt.Sprintf("%.2f", p)
	}
	for i, q := range tick.AskQtys {
		askQtys[i] = strconv.Itoa(int(q))
	}
	for i, o := range tick.AskOrders {
		askOrders[i] = strconv.Itoa(int(o))
	}

	row := []string{
		tick.Timestamp.Format("2006-01-02 15:04:05.000"),
		strconv.FormatUint(uint64(tick.Token), 10),
		tick.Symbol,
		tick.CompanyName,
		fmt.Sprintf("%.2f", tick.LTP),
		fmt.Sprintf("%.2f", tick.Open),
		fmt.Sprintf("%.2f", tick.High),
		fmt.Sprintf("%.2f", tick.Low),
		fmt.Sprintf("%.2f", tick.PrevClose),
		fmt.Sprintf("%.2f", tick.ATP),
		strconv.Itoa(int(tick.Volume)),
		strconv.FormatUint(uint64(tick.TurnoverLakhs), 10),
		strconv.FormatUint(uint64(tick.SeqNumber), 10),
		strings.Join(bidPrices, "|"),
		strings.Join(bidQtys, "|"),
		strings.Join(bidOrders, "|"),
		strings.Join(askPrices, "|"),
		strings.Join(askQtys, "|"),
		strings.Join(askOrders, "|"),
	}

	writer.Write(row)
}
