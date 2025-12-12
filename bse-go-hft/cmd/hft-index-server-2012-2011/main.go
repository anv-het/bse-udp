// BSE Index Server - Captures and saves index data (Types 2011/2012)
// Runs alongside hft-server to capture index broadcasts from port 26001
//
// Usage:
//   go run ./cmd/hft-index-server/
//   .\hft-index-server.exe
//   .\hft-index-server.exe -duration 1m

package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"bse-go-hft/internal/decoder"
	"bse-go-hft/internal/saver"
)

// Statistics for live monitoring
type Stats struct {
	packets2011 atomic.Uint64
	packets2012 atomic.Uint64
	records2011 atomic.Uint64
	records2012 atomic.Uint64
	errors      atomic.Uint64
	startTime   time.Time
}

func main() {
	// Command-line flags
	duration := flag.Duration("duration", 0, "Run duration (e.g., 10s, 1m, 30m). If 0, run until Ctrl+C")
	outputDir := flag.String("output", "./data/processed_csv", "Output directory for CSV files")
	multicastIP := flag.String("ip", "239.1.2.5", "Multicast IP address")
	port := flag.Int("port", 26001, "UDP port")
	flag.Parse()

	printBanner(*multicastIP, *port, *outputDir, *duration)

	// Initialize decoder
	dec := decoder.NewDecoder()

	// Create index savers
	saver2011, err := saver.NewIndexDataSaver(*outputDir, 2011)
	if err != nil {
		fmt.Printf("❌ Failed to create saver for Type 2011: %v\n", err)
		return
	}
	defer saver2011.Close()

	saver2012, err := saver.NewIndexDataSaver(*outputDir, 2012)
	if err != nil {
		fmt.Printf("❌ Failed to create saver for Type 2012: %v\n", err)
		return
	}
	defer saver2012.Close()

	fmt.Printf("\n✅ Type 2011 CSV: %s\n", saver2011.GetFilename())
	fmt.Printf("✅ Type 2012 CSV: %s\n", saver2012.GetFilename())

	// Initialize statistics
	stats := &Stats{
		startTime: time.Now(),
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Duration timer (if specified)
	var durationTimer <-chan time.Time
	if *duration > 0 {
		durationTimer = time.After(*duration)
		fmt.Printf("\n⏱️  Will run for %v\n", *duration)
	} else {
		fmt.Println("\n⏱️  Running until Ctrl+C")
	}

	// Statistics ticker
	statsTicker := time.NewTicker(5 * time.Second)
	defer statsTicker.Stop()

	// Start UDP receiver
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		runReceiver(ctx, *multicastIP, *port, dec, saver2011, saver2012, stats)
	}()

	fmt.Println("\n================================================================================")
	fmt.Println("📡 RECEIVING INDEX DATA...")
	fmt.Println("================================================================================\n")

	// Main loop
	for {
		select {
		case <-sigChan:
			fmt.Println("\n\n⏹️  Shutting down (Ctrl+C)...")
			cancel()
			wg.Wait()
			printFinalReport(stats, saver2011, saver2012)
			return

		case <-durationTimer:
			fmt.Printf("\n\n⏹️  Duration of %v completed. Shutting down...\n", *duration)
			cancel()
			wg.Wait()
			printFinalReport(stats, saver2012, saver2012)
			return

		case <-statsTicker.C:
			printLiveStats(stats)
		}
	}
}

// runReceiver receives and processes index packets
func runReceiver(
	ctx context.Context,
	multicastIP string,
	port int,
	dec *decoder.Decoder,
	saver2011 *saver.IndexDataSaver,
	saver2012 *saver.IndexDataSaver,
	stats *Stats,
) {
	// Resolve multicast address
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", multicastIP, port))
	if err != nil {
		fmt.Printf("❌ Failed to resolve address: %v\n", err)
		return
	}

	// Create multicast connection
	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		fmt.Printf("❌ Failed to create connection: %v\n", err)
		return
	}
	defer conn.Close()

	// Set read buffer
	conn.SetReadBuffer(4 * 1024 * 1024)

	fmt.Println("✅ Connected to BSE NFCAST feed")

	buffer := make([]byte, 2048)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Read packet
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			continue
		}

		// Check message type
		if n < 10 {
			continue
		}

		msgType := binary.LittleEndian.Uint16(buffer[8:10])

		// Only process index messages
		if msgType == 2011 {
			// Decode Type 2011
			indices, err := dec.DecodeMsgType2011(buffer, n)
			if err != nil {
				stats.errors.Add(1)
				continue
			}

			stats.packets2011.Add(1)
			stats.records2011.Add(uint64(len(indices)))

			// Save to CSV
			for _, idx := range indices {
				saver2011.Save(idx)
			}

		} else if msgType == 2012 {
			// Decode Type 2012
			indices, err := dec.DecodeMsgType2012(buffer, n)
			if err != nil {
				stats.errors.Add(1)
				continue
			}

			stats.packets2012.Add(1)
			stats.records2012.Add(uint64(len(indices)))

			// Save to CSV
			for _, idx := range indices {
				saver2012.Save(idx)
			}
		}
		// Ignore other message types (2020, 2021, etc.)
	}
}

// printLiveStats prints live statistics
func printLiveStats(stats *Stats) {
	elapsed := time.Since(stats.startTime).Seconds()

	p2011 := stats.packets2011.Load()
	p2012 := stats.packets2012.Load()
	r2011 := stats.records2011.Load()
	r2012 := stats.records2012.Load()
	errors := stats.errors.Load()

	totalPackets := p2011 + p2012
	totalRecords := r2011 + r2012

	fmt.Printf("[%.0fs] Packets: %d (2011:%d 2012:%d) | Records: %d (2011:%d 2012:%d) | Errors: %d\n",
		elapsed, totalPackets, p2011, p2012, totalRecords, r2011, r2012, errors)
}

// printFinalReport prints final statistics
func printFinalReport(stats *Stats, saver2011, saver2012 *saver.IndexDataSaver) {
	elapsed := time.Since(stats.startTime)

	fmt.Println("\n================================================================================")
	fmt.Println("📊 FINAL REPORT")
	fmt.Println("================================================================================")
	fmt.Printf("Runtime:         %v\n", elapsed)
	fmt.Printf("\n📦 PACKETS:\n")
	fmt.Printf("  Type 2011:     %d\n", stats.packets2011.Load())
	fmt.Printf("  Type 2012:     %d\n", stats.packets2012.Load())
	fmt.Printf("  Total:         %d\n", stats.packets2011.Load()+stats.packets2012.Load())
	fmt.Printf("\n📝 RECORDS:\n")
	fmt.Printf("  Type 2011:     %d\n", stats.records2011.Load())
	fmt.Printf("  Type 2012:     %d\n", stats.records2012.Load())
	fmt.Printf("  Total:         %d\n", stats.records2011.Load()+stats.records2012.Load())
	fmt.Printf("\n❌ ERRORS:       %d\n", stats.errors.Load())
	fmt.Printf("\n📁 OUTPUT FILES:\n")
	fmt.Printf("  [2011] %s (%d rows)\n", saver2011.GetFilename(), saver2011.GetRowCount())
	fmt.Printf("  [2012] %s (%d rows)\n", saver2012.GetFilename(), saver2012.GetRowCount())
	fmt.Println("================================================================================")
}

// printBanner prints the startup banner
func printBanner(multicastIP string, port int, outputDir string, duration time.Duration) {
	durationStr := "Until Ctrl+C"
	if duration > 0 {
		durationStr = duration.String()
	}

	fmt.Println("================================================================================")
	fmt.Println("         BSE INDEX SERVER - INDEX DATA CAPTURE (Types 2011/2012)")
	fmt.Println("================================================================================")
	fmt.Printf("Start Time:      %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("Multicast:       %s:%d\n", multicastIP, port)
	fmt.Printf("Output Dir:      %s\n", outputDir)
	fmt.Printf("Duration:        %s\n", durationStr)
	fmt.Printf("GOMAXPROCS:      %d\n", runtime.GOMAXPROCS(0))
	fmt.Println("================================================================================")
	fmt.Println("\nℹ️  This server captures ONLY index data (Types 2011/2012)")
	fmt.Println("ℹ️  Run hft-server separately for EQ/FO quotes (Types 2020/2021)")
	fmt.Println()
}
