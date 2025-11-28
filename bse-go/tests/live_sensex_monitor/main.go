/*
BSE Live SENSEX Monitor - Real-Time Multi-Token Display (Go)
=============================================================

Monitors multiple SENSEX contracts with LIVE tick-by-tick updates.
Watches for futures and popular options contracts.

Usage:
  go run . --ticks 100

Features:
- Monitors multiple SENSEX tokens simultaneously
- Live tick display with summary table
- Per-run CSV file with all captured ticks
- Order book depth display for each tick
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

// TargetContract defines a contract to watch
type TargetContract struct {
	Token       uint32
	Name        string
	Description string
	ExpectedLTP float64 // For validation
}

// SensexTick represents a captured tick
type SensexTick struct {
	Timestamp     time.Time
	Token         uint32
	Name          string
	LTP           float64
	Open          float64
	High          float64
	Low           float64
	PrevClose     float64
	ATP           float64
	Volume        int32
	TurnoverLakhs uint32
	SeqNumber     uint32
	Change        float64
	ChangePct     float64
}

// WatchList of SENSEX contracts - can be updated based on current expiry
var WatchList = []TargetContract{
	// SENSEX Futures (update based on current expiry)
	// Token format: 8xxxxx typically for FO
}

func main() {
	maxTicks := flag.Int("ticks", 100, "Max total ticks to capture")
	multicastIP := flag.String("ip", "239.1.2.5", "Multicast IP")
	port := flag.Int("port", 26002, "Port (26002 for F&O)")
	dataDir := flag.String("data", "data", "Data directory for token files")
	showAll := flag.Bool("all", false, "Show all SENSEX tokens (not just watch list)")
	flag.Parse()

	fmt.Println(strings.Repeat("=", 100))
	fmt.Println("BSE LIVE SENSEX MONITOR (Go)")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("Time:  %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("Port:  %d (F&O)\n", *port)

	// Load FO token mapping
	fmt.Println("\n📋 Loading F&O symbol mapping...")
	collector := data_collector.NewMarketDataCollector("FO", *dataDir)
	stats := collector.GetStats()
	fmt.Printf("   ✅ Loaded %d F&O contracts\n", stats["tokens_loaded"])

	// Create output directory and CSV file
	outputDir := filepath.Join(*dataDir, "processed_csv")
	os.MkdirAll(outputDir, 0755)

	timestamp := time.Now().Format("20060102_150405")
	csvFilename := filepath.Join(outputDir, fmt.Sprintf("sensex_live_%s.csv", timestamp))

	csvFile, err := os.Create(csvFilename)
	if err != nil {
		log.Fatalf("Failed to create CSV file: %v", err)
	}
	defer csvFile.Close()

	csvWriter := csv.NewWriter(csvFile)
	defer csvWriter.Flush()

	// Write CSV header
	csvWriter.Write([]string{
		"timestamp", "token", "symbol", "ltp", "change", "change_pct",
		"open", "high", "low", "prev_close", "atp",
		"volume", "turnover_lakhs", "seq",
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

	conn.SetReadBuffer(1024 * 1024) // 1MB buffer
	fmt.Println("   ✅ Connected!")

	fmt.Printf("\n🚀 STARTING SENSEX MONITOR (Press Ctrl+C to stop)\n")
	if *showAll {
		fmt.Println("   Mode: Watching ALL SENSEX-related tokens")
	} else {
		fmt.Println("   Mode: Watching SENSEX futures and options")
	}
	fmt.Printf("   Max ticks: %d\n", *maxTicks)
	fmt.Println(strings.Repeat("=", 100))

	// Signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	tickCount := 0
	sensexTicks := make(map[uint32]*SensexTick) // Track latest tick per token
	lastSeqs := make(map[uint32]uint32)         // Track sequence numbers
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
				token := binary.LittleEndian.Uint32(recordBytes[0:4])

				// Get symbol info
				symbolInfo := collector.GetTokenInfo(token)
				if symbolInfo == nil {
					continue
				}

				// Check if SENSEX-related
				isSensex := strings.Contains(strings.ToUpper(symbolInfo.Symbol), "SENSEX") ||
					strings.Contains(strings.ToUpper(symbolInfo.Symbol), "SXSPL") ||
					strings.Contains(strings.ToUpper(symbolInfo.Symbol), "BSX")

				if !isSensex && !*showAll {
					continue
				}

				// Decode
				seq := binary.LittleEndian.Uint32(recordBytes[44:48])
				if lastSeq, ok := lastSeqs[token]; ok && seq == lastSeq {
					continue
				}
				lastSeqs[token] = seq

				ltp := float64(int32(binary.LittleEndian.Uint32(recordBytes[36:40]))) / 100.0
				prevClose := float64(int32(binary.LittleEndian.Uint32(recordBytes[8:12]))) / 100.0

				change := ltp - prevClose
				changePct := 0.0
				if prevClose > 0 {
					changePct = (change / prevClose) * 100
				}

				tick := &SensexTick{
					Timestamp:     time.Now(),
					Token:         token,
					Name:          formatSensexName(symbolInfo),
					LTP:           ltp,
					Open:          float64(int32(binary.LittleEndian.Uint32(recordBytes[4:8]))) / 100.0,
					High:          float64(int32(binary.LittleEndian.Uint32(recordBytes[12:16]))) / 100.0,
					Low:           float64(int32(binary.LittleEndian.Uint32(recordBytes[16:20]))) / 100.0,
					PrevClose:     prevClose,
					ATP:           float64(int32(binary.LittleEndian.Uint32(recordBytes[84:88]))) / 100.0,
					Volume:        int32(binary.LittleEndian.Uint32(recordBytes[24:28])),
					TurnoverLakhs: binary.LittleEndian.Uint32(recordBytes[28:32]),
					SeqNumber:     seq,
					Change:        change,
					ChangePct:     changePct,
				}

				tickCount++
				sensexTicks[token] = tick

				// Display
				displaySensexTick(tick, tickCount)

				// Write to CSV
				writeSensexTickToCSV(csvWriter, tick)
				csvWriter.Flush()
			}
		}
	}

	// Summary
	printSummary(sensexTicks, tickCount, csvFilename)
}

func formatSensexName(info *data_collector.ContractInfo) string {
	symbol := info.Symbol
	expiry := info.Expiry
	strike := info.Strike
	optType := info.OptionType

	if strike != "" && optType != "" {
		return fmt.Sprintf("%s %s %s %s", symbol, strike, optType, expiry)
	}
	if expiry != "" {
		return fmt.Sprintf("%s FUT %s", symbol, expiry)
	}
	return symbol
}

func displaySensexTick(tick *SensexTick, tickNum int) {
	now := tick.Timestamp.Format("15:04:05.000")

	// Color indicator
	indicator := "━"
	if tick.Change > 0 {
		indicator = "▲"
	} else if tick.Change < 0 {
		indicator = "▼"
	}

	fmt.Printf("[%s] #%-5d %s %-40s LTP: ₹%10.2f %s %+.2f (%+.2f%%) Vol: %d\n",
		now, tickNum, indicator, tick.Name, tick.LTP, indicator, tick.Change, tick.ChangePct, tick.Volume)
}

func writeSensexTickToCSV(writer *csv.Writer, tick *SensexTick) {
	row := []string{
		tick.Timestamp.Format("2006-01-02 15:04:05.000"),
		strconv.FormatUint(uint64(tick.Token), 10),
		tick.Name,
		fmt.Sprintf("%.2f", tick.LTP),
		fmt.Sprintf("%.2f", tick.Change),
		fmt.Sprintf("%.2f", tick.ChangePct),
		fmt.Sprintf("%.2f", tick.Open),
		fmt.Sprintf("%.2f", tick.High),
		fmt.Sprintf("%.2f", tick.Low),
		fmt.Sprintf("%.2f", tick.PrevClose),
		fmt.Sprintf("%.2f", tick.ATP),
		strconv.Itoa(int(tick.Volume)),
		strconv.FormatUint(uint64(tick.TurnoverLakhs), 10),
		strconv.FormatUint(uint64(tick.SeqNumber), 10),
	}
	writer.Write(row)
}

func printSummary(ticks map[uint32]*SensexTick, totalTicks int, csvFile string) {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 100))
	fmt.Println("📊 SENSEX MONITOR SUMMARY")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("Total Ticks:     %d\n", totalTicks)
	fmt.Printf("Unique Tokens:   %d\n", len(ticks))
	fmt.Printf("CSV File:        %s\n", csvFile)

	if len(ticks) > 0 {
		fmt.Println()
		fmt.Println("Latest Values:")
		fmt.Println(strings.Repeat("-", 100))
		fmt.Printf("%-40s %12s %12s %12s %10s\n", "Contract", "LTP", "Change", "Change%", "Volume")
		fmt.Println(strings.Repeat("-", 100))

		for _, tick := range ticks {
			indicator := ""
			if tick.Change > 0 {
				indicator = "▲"
			} else if tick.Change < 0 {
				indicator = "▼"
			}
			fmt.Printf("%-40s %12.2f %11.2f%s %11.2f%% %10d\n",
				tick.Name, tick.LTP, tick.Change, indicator, tick.ChangePct, tick.Volume)
		}
		fmt.Println(strings.Repeat("-", 100))
	}

	fmt.Println(strings.Repeat("=", 100))
}
