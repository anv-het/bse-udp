// BSE Go HFT - Live SENSEX Market Data Test
// ==========================================
//
// Tests live SENSEX market data with specific target tokens.
// Validates LTP values against expected prices.
//
// ================================================================================
// USAGE
// ================================================================================
//
// BUILD:
//   cd d:\bse\bse-go-hft
//   go build -o test_live_sensex.exe ./tests/test_live_sensex.go
//
// RUN:
//   .\test_live_sensex.exe
//   .\test_live_sensex.exe -duration 60s
//   .\test_live_sensex.exe -duration 2m -port 26002
//
// PARAMETERS:
//   -duration time   Test duration (default: 30s)
//   -port int        UDP port: 26002=FO (default: 26002)
//   -ip string       Multicast IP (default: "239.1.2.5")
//
// OUTPUT:
//   - Live display of target token updates
//   - CSV file: data/test_results/sensex_live_YYYYMMDD_HHMMSS.csv
//   - Summary report with PASS/FAIL status
//
// ================================================================================

package main

import (
	"encoding/binary"
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/net/ipv4"
)

// ================================================================================
// CONFIGURATION
// ================================================================================

const (
	DefaultMulticastIP = "239.1.2.5"
	DefaultFOPort      = 26002
	HeaderSize         = 36
	RecordSize         = 264
	BufferSize         = 2048
)

// ================================================================================
// TARGET TOKENS - SENSEX Contracts
// ================================================================================

type TargetToken struct {
	Name        string
	ExpectedLTP float64 // Expected LTP for validation (set to 0 to skip validation)
}

// These are example target tokens - update with current contract tokens
var TargetTokens = map[uint32]TargetToken{
	// SENSEX Futures
	873830: {Name: "SENSEX DEC FUT", ExpectedLTP: 0}, // December Future
	861384: {Name: "SENSEX NOV FUT", ExpectedLTP: 0}, // November Future
	842364: {Name: "SENSEX OCT FUT", ExpectedLTP: 0}, // October Future

	// SENSEX Options (sample - update with actual tokens)
	878196: {Name: "SENSEX 83900 CE", ExpectedLTP: 0},
	878015: {Name: "SENSEX 83800 PE", ExpectedLTP: 0},
	877845: {Name: "SENSEX 83700 PE", ExpectedLTP: 0},
	877761: {Name: "SENSEX 84000 CE", ExpectedLTP: 0},
}

// ================================================================================
// DATA STRUCTURES
// ================================================================================

type TickUpdate struct {
	Timestamp time.Time
	Token     uint32
	Name      string
	LTP       float64
	Volume    int32
	SeqNum    uint32
}

type TokenStats struct {
	Updates  []TickUpdate
	FirstLTP float64
	LastLTP  float64
	HighLTP  float64
	LowLTP   float64
	TotalVol int64
}

// ================================================================================
// CSV WRITER
// ================================================================================

type TestCSVWriter struct {
	file     *os.File
	writer   *csv.Writer
	filename string
}

func NewTestCSVWriter() (*TestCSVWriter, error) {
	timestamp := time.Now().Format("20060102_150405")

	// Create test_results directory
	dir := "data/test_results"
	os.MkdirAll(dir, 0755)

	filename := filepath.Join(dir, fmt.Sprintf("sensex_live_%s.csv", timestamp))

	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	writer := csv.NewWriter(file)

	// Write header
	headers := []string{
		"timestamp", "token", "name", "ltp", "volume", "seq_num",
		"expected_ltp", "diff_pct", "status",
	}
	writer.Write(headers)

	return &TestCSVWriter{
		file:     file,
		writer:   writer,
		filename: filename,
	}, nil
}

func (cw *TestCSVWriter) WriteUpdate(update TickUpdate, expectedLTP float64) error {
	var diffPct float64
	var status string

	if expectedLTP > 0 {
		diffPct = math.Abs(update.LTP-expectedLTP) / expectedLTP * 100
		if diffPct < 5 {
			status = "PASS"
		} else if diffPct < 10 {
			status = "WARN"
		} else {
			status = "FAIL"
		}
	} else {
		status = "N/A"
	}

	row := []string{
		update.Timestamp.Format("2006-01-02 15:04:05.000"),
		fmt.Sprintf("%d", update.Token),
		update.Name,
		fmt.Sprintf("%.2f", update.LTP),
		fmt.Sprintf("%d", update.Volume),
		fmt.Sprintf("%d", update.SeqNum),
		fmt.Sprintf("%.2f", expectedLTP),
		fmt.Sprintf("%.2f", diffPct),
		status,
	}

	return cw.writer.Write(row)
}

func (cw *TestCSVWriter) Close() {
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

	pconn := ipv4.NewPacketConn(conn)
	pconn.SetControlMessage(ipv4.FlagTTL|ipv4.FlagDst, true)

	return conn, nil
}

// ================================================================================
// MAIN TEST
// ================================================================================

func testLiveSensex(port int, multicastIP string, duration time.Duration) {
	fmt.Println(strings.Repeat("=", 100))
	fmt.Println("LIVE SENSEX MARKET DATA TEST")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("Start Time: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("Duration:   %s\n", duration)
	fmt.Printf("Port:       %d (F&O)\n", port)
	fmt.Println()

	fmt.Println("Target Contracts:")
	for token, info := range TargetTokens {
		if info.ExpectedLTP > 0 {
			fmt.Printf("  %d: %-30s Expected LTP: ₹%8.2f\n", token, info.Name, info.ExpectedLTP)
		} else {
			fmt.Printf("  %d: %-30s (No expected LTP)\n", token, info.Name)
		}
	}
	fmt.Println()

	// Create CSV writer
	csvWriter, err := NewTestCSVWriter()
	if err != nil {
		fmt.Printf("❌ Failed to create CSV file: %v\n", err)
		return
	}
	defer csvWriter.Close()
	fmt.Printf("📝 Output CSV: %s\n", csvWriter.filename)
	fmt.Println()

	// Connect to multicast
	fmt.Printf("📡 Connecting to %s:%d...\n", multicastIP, port)
	conn, err := createMulticastSocket(multicastIP, port)
	if err != nil {
		fmt.Printf("❌ Failed to connect: %v\n", err)
		return
	}
	defer conn.Close()
	fmt.Println("✅ Connected!")
	fmt.Println()

	fmt.Println("Listening for packets... (Press Ctrl+C to stop early)")
	fmt.Println(strings.Repeat("-", 100))

	// Track found tokens
	foundTokens := make(map[uint32]*TokenStats)
	packetsProcessed := 0
	startTime := time.Now()
	buffer := make([]byte, BufferSize)

	for time.Since(startTime) < duration {
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}

		packetsProcessed++

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

			// Only process target tokens
			targetInfo, isTarget := TargetTokens[token]
			if !isTarget {
				continue
			}

			// Decode
			ltp := float64(int32(binary.LittleEndian.Uint32(record[36:40]))) / 100.0
			volume := int32(binary.LittleEndian.Uint32(record[24:28]))
			seqNum := binary.LittleEndian.Uint32(record[44:48])

			if ltp <= 0 {
				continue
			}

			update := TickUpdate{
				Timestamp: time.Now(),
				Token:     token,
				Name:      targetInfo.Name,
				LTP:       ltp,
				Volume:    volume,
				SeqNum:    seqNum,
			}

			// Track stats
			stats, exists := foundTokens[token]
			if !exists {
				stats = &TokenStats{
					FirstLTP: ltp,
					HighLTP:  ltp,
					LowLTP:   ltp,
				}
				foundTokens[token] = stats
			}

			stats.Updates = append(stats.Updates, update)
			stats.LastLTP = ltp
			if ltp > stats.HighLTP {
				stats.HighLTP = ltp
			}
			if ltp < stats.LowLTP {
				stats.LowLTP = ltp
			}
			stats.TotalVol = int64(volume)

			// Write to CSV
			csvWriter.WriteUpdate(update, targetInfo.ExpectedLTP)

			// Print update
			var status string
			if targetInfo.ExpectedLTP > 0 {
				diffPct := math.Abs(ltp-targetInfo.ExpectedLTP) / targetInfo.ExpectedLTP * 100
				if diffPct < 5 {
					status = "✅"
				} else if diffPct < 10 {
					status = "⚠️"
				} else {
					status = "❌"
				}
				fmt.Printf("%s Token %d: %-30s LTP: ₹%8.2f (Expected: ₹%8.2f, Diff: %5.1f%%)\n",
					status, token, targetInfo.Name, ltp, targetInfo.ExpectedLTP, diffPct)
			} else {
				fmt.Printf("📊 Token %d: %-30s LTP: ₹%8.2f\n", token, targetInfo.Name, ltp)
			}
		}

		// Progress every 10 packets
		if packetsProcessed%10 == 0 {
			fmt.Printf("  ... %d packets processed, %d/%d tokens found ...\n",
				packetsProcessed, len(foundTokens), len(TargetTokens))
		}

		// Stop early if we found all tokens with multiple updates
		if len(foundTokens) >= len(TargetTokens) {
			allHaveMultiple := true
			for _, stats := range foundTokens {
				if len(stats.Updates) < 3 {
					allHaveMultiple = false
					break
				}
			}
			if allHaveMultiple {
				fmt.Println()
				fmt.Println("✅ All target tokens found with multiple updates!")
				break
			}
		}
	}

	// Summary
	fmt.Println()
	fmt.Println(strings.Repeat("=", 100))
	fmt.Println("SUMMARY")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("Duration: %.1f seconds\n", time.Since(startTime).Seconds())
	fmt.Printf("Packets processed: %d\n", packetsProcessed)
	fmt.Printf("Tokens found: %d/%d\n", len(foundTokens), len(TargetTokens))
	fmt.Println()

	if len(foundTokens) > 0 {
		fmt.Println("Final Values:")
		fmt.Println(strings.Repeat("-", 100))

		for token, targetInfo := range TargetTokens {
			stats, found := foundTokens[token]
			if found {
				var status string
				var diffPct float64

				if targetInfo.ExpectedLTP > 0 {
					diffPct = math.Abs(stats.LastLTP-targetInfo.ExpectedLTP) / targetInfo.ExpectedLTP * 100
					if diffPct < 5 {
						status = "✅ PASS"
					} else if diffPct < 10 {
						status = "⚠️ WARN"
					} else {
						status = "❌ FAIL"
					}
				} else {
					status = "📊 DATA"
				}

				fmt.Printf("%s %s\n", status, targetInfo.Name)
				fmt.Printf("       Latest LTP: ₹%8.2f\n", stats.LastLTP)
				if targetInfo.ExpectedLTP > 0 {
					fmt.Printf("       Expected:   ₹%8.2f\n", targetInfo.ExpectedLTP)
					fmt.Printf("       Difference: %5.1f%%\n", diffPct)
				}
				fmt.Printf("       High/Low:   ₹%.2f / ₹%.2f\n", stats.HighLTP, stats.LowLTP)
				fmt.Printf("       Updates:    %d\n", len(stats.Updates))
				fmt.Println()
			} else {
				fmt.Printf("❌ MISS %s - NOT FOUND\n\n", targetInfo.Name)
			}
		}
	} else {
		fmt.Println("❌ No target tokens found in captured packets!")
		fmt.Println("   Possible reasons:")
		fmt.Println("   - Market is closed")
		fmt.Println("   - No trading activity for these contracts")
		fmt.Println("   - Incorrect multicast configuration")
		fmt.Println("   - Token IDs may have changed (check Contract Master)")
	}

	fmt.Printf("\n📝 Output saved to: %s\n", csvWriter.filename)
}

// ================================================================================
// MAIN
// ================================================================================

func main() {
	duration := flag.Duration("duration", 30*time.Second, "Test duration (e.g., 30s, 1m, 5m)")
	port := flag.Int("port", DefaultFOPort, "UDP port (default: 26002 for F&O)")
	ip := flag.String("ip", DefaultMulticastIP, "Multicast IP address")
	flag.Parse()

	testLiveSensex(*port, *ip, *duration)
}
