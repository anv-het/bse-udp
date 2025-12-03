// BSE Go HFT Server - Modular Production Pipeline
// A high-performance UDP multicast reader for BSE market data
//
// Features:
// - Dual feed support (EQ + FO)
// - Zero-copy packet decoding
// - Lock-free ring buffer
// - HFT-grade latency tracking
// - Automatic token file management
//
// Usage:
//   go run ./cmd/hft-server/
//   go run ./cmd/hft-server/ -eq -fo
//   go run ./cmd/hft-server/ -fo-only

/*

# Run for specific durations
.\hft-server.exe -duration 10s    # 10 seconds
.\hft-server.exe -duration 1m     # 1 minute
.\hft-server.exe -duration 30m    # 30 minutes
.\hft-server.exe -duration 2h     # 2 hours

# Run until Ctrl+C (default)
.\hft-server.exe

# Combine with other flags
.\hft-server.exe -duration 5m -eq-only    # 5 minutes, only EQ feed
.\hft-server.exe -duration 1m -fo-only    # 1 minute, only FO feed


*/

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"bse-go-hft/internal/buffer"
	"bse-go-hft/internal/config"
	"bse-go-hft/internal/decoder"
	"bse-go-hft/internal/receiver"
	"bse-go-hft/internal/saver"
	"bse-go-hft/internal/stats"
	"bse-go-hft/internal/tokens"
	"bse-go-hft/pkg/domain"
)

func init() {
	// HFT Optimizations:
	// 1. Set GOGC to 50% (more frequent but smaller GC pauses)
	debug.SetGCPercent(50)

	// 2. Set memory limit to 150MB to prevent runaway memory usage
	debug.SetMemoryLimit(150 * 1024 * 1024)

	// 3. Use all available CPU cores
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	// Command-line flags
	configPath := flag.String("config", "config.json", "Path to config file")
	eqOnly := flag.Bool("eq-only", false, "Enable only Equity (CM) feed")
	foOnly := flag.Bool("fo-only", false, "Enable only F&O (FO) feed")
	duration := flag.Duration("duration", 0, "Run duration (e.g., 10s, 1m, 30m). If 0, run until Ctrl+C")
	flag.Parse()

	// Load configuration
	cfg := config.LoadOrDefault(*configPath)
	fmt.Printf("📂 Loaded config: %s\n", *configPath)

	// Determine which feeds to enable
	enableEQ := cfg.Segments.CMEnabled
	enableFO := cfg.Segments.FOEnabled

	if *eqOnly {
		enableEQ = true
		enableFO = false
	}
	if *foOnly {
		enableEQ = false
		enableFO = true
	}

	// If nothing enabled, enable both
	if !enableEQ && !enableFO {
		enableEQ = true
		enableFO = true
	}

	// Print banner
	printBanner(cfg, enableEQ, enableFO)

	// Initialize token map
	tokenMap := domain.NewTokenMap()

	// Initialize token manager and load tokens
	tokenMgr := tokens.NewManager(cfg.DataManagement.TokensDir, cfg.API.BaseURL)

	fmt.Println("\n================================================================================")
	fmt.Println("📥 TOKEN FILE MANAGEMENT")
	fmt.Println("================================================================================")
	fmt.Printf("   API: %s\n", cfg.API.BaseURL)
	fmt.Printf("   Target Date: Previous trading day\n")
	fmt.Printf("   Retry Logic: 3 attempts with 10s delay\n")
	fmt.Printf("   Cleanup: Keep last 2 files\n")

	// Load BhavCopy (EQ tokens) if EQ enabled
	if enableEQ {
		fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("📊 [1/2] BHAVCOPY (Equity Cash - Port 26001)")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if err := tokenMgr.LoadBhavCopy(tokenMap); err != nil {
			fmt.Printf("⚠️  Warning: Failed to load BhavCopy: %v\n", err)
		}
	}

	// Load Contract Master (FO tokens) if FO enabled
	if enableFO {
		fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("📊 [2/2] CONTRACT MASTER (F&O Derivatives - Port 26002)")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

		if err := tokenMgr.LoadContractMaster(tokenMap); err != nil {
			fmt.Printf("⚠️  Warning: Failed to load Contract Master: %v\n", err)
		}
	}

	// Print token summary
	fmt.Println("\n================================================================================")
	fmt.Println("📋 TOKEN MAPPING SUMMARY")
	fmt.Println("================================================================================")
	fmt.Printf("✅ Total tokens loaded: %d\n", tokenMap.Len())

	// Initialize statistics tracker
	tracker := stats.NewTracker()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handler
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait group for receivers
	var wg sync.WaitGroup

	// Start EQ receiver if enabled
	if enableEQ {
		eqSaver, err := saver.NewCSVSaver(cfg.DataManagement.OutputDir, "EQ")
		if err != nil {
			fmt.Printf("❌ Failed to create EQ saver: %v\n", err)
			return
		}
		defer eqSaver.Close()
		tracker.SetOutputFile("EQ", eqSaver.FilePath())

		wg.Add(1)
		go func() {
			defer wg.Done()
			runReceiver(ctx, cfg.MulticastCM, "EQ", tokenMap, eqSaver, tracker)
		}()
	}

	// Start FO receiver if enabled
	if enableFO {
		foSaver, err := saver.NewCSVSaver(cfg.DataManagement.OutputDir, "FO")
		if err != nil {
			fmt.Printf("❌ Failed to create FO saver: %v\n", err)
			return
		}
		defer foSaver.Close()
		tracker.SetOutputFile("FO", foSaver.FilePath())

		wg.Add(1)
		go func() {
			defer wg.Done()
			runReceiver(ctx, cfg.MulticastFO, "FO", tokenMap, foSaver, tracker)
		}()
	}

	fmt.Println("\n✅ All feeds connected! Receiving packets...")
	if *duration > 0 {
		fmt.Printf("⏱️  Will run for %v\n", *duration)
	} else {
		fmt.Println("⏱️  Press Ctrl+C to stop")
	}
	fmt.Println()

	// Stats ticker
	statsTicker := time.NewTicker(1 * time.Second)
	defer statsTicker.Stop()

	// Memory sampling ticker
	memTicker := time.NewTicker(100 * time.Millisecond)
	defer memTicker.Stop()

	// Duration timer (if specified)
	var durationTimer <-chan time.Time
	if *duration > 0 {
		durationTimer = time.After(*duration)
	}

	// Main loop
	for {
		select {
		case <-sigChan:
			fmt.Println("\n\n⏹️  Shutting down (Ctrl+C)...")
			cancel()
			wg.Wait()
			tracker.PrintFinalReport(tokenMap.Len())
			return

		case <-durationTimer:
			fmt.Printf("\n\n⏹️  Duration of %v completed. Shutting down...\n", *duration)
			cancel()
			wg.Wait()
			tracker.PrintFinalReport(tokenMap.Len())
			return

		case <-statsTicker.C:
			tracker.PrintLiveStats(tokenMap.Len())

		case <-memTicker.C:
			tracker.SampleSystem()
		}
	}
}

// runReceiver runs a multicast receiver for a specific segment
func runReceiver(
	ctx context.Context,
	mcConfig config.MulticastConfig,
	segment string,
	tokenMap *domain.TokenMap,
	csvSaver *saver.CSVSaver,
	tracker *stats.Tracker,
) {
	// Create ring buffer for lock-free packet passing
	ringBuf := buffer.NewRingBuffer(buffer.DefaultRingSize, buffer.DefaultPacketSize)

	// Create decoder
	dec := decoder.NewDecoder()

	// Packet handler: push to ring buffer
	handler := func(data []byte, length int, _ time.Time) {
		ringBuf.TryPush(data, length)
	}

	// Create receiver
	rcvConfig := receiver.Config{
		MulticastIP:  mcConfig.IP,
		Port:         mcConfig.Port,
		BufferSize:   mcConfig.BufferSize,
		SocketRcvBuf: mcConfig.SocketRcvBuf,
		ReadTimeout:  mcConfig.ReadTimeout,
	}

	rcv := receiver.NewMulticastReceiver(rcvConfig, handler)

	fmt.Printf("🔌 Connecting to %s feed (%s:%d)...\n", segment, mcConfig.IP, mcConfig.Port)

	if err := rcv.Connect(); err != nil {
		fmt.Printf("❌ Failed to connect %s: %v\n", segment, err)
		return
	}
	defer rcv.Close()

	fmt.Printf("   ✅ %s Connected!\n", segment)

	// Start receiver in goroutine
	go func() {
		rcv.ReceiveLoop(ctx)
	}()

	// Process packets from ring buffer
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		data, length, ok := ringBuf.TryPop()
		if !ok {
			// Buffer empty, yield CPU
			runtime.Gosched()
			continue
		}

		// Measure decode latency
		decodeStart := time.Now()

		records, count, err := dec.Decode(data, length)
		if err != nil {
			continue
		}

		decodeLatency := time.Since(decodeStart).Nanoseconds()
		tracker.RecordDecodeLatency(decodeLatency)
		tracker.RecordPacket(segment, length, count)

		// Process each record
		processStart := time.Now()

		for i := 0; i < count; i++ {
			rec := &records[i]

			// Track sequence
			tracker.TrackSequence(rec.Token, rec.SequenceNum)

			// Look up token
			contract, found := tokenMap.Get(rec.Token)
			if !found {
				tracker.TrackMissedToken(rec.Token)
				continue
			}

			// Create quote
			quote := &domain.Quote{
				Timestamp:   rec.Timestamp,
				Token:       rec.Token,
				Symbol:      contract.Symbol,
				SymbolName:  contract.SymbolName,
				Expiry:      contract.Expiry,
				OptionType:  contract.OptionType,
				StrikePrice: contract.StrikePrice,
				LTP:         rec.LTP,
				Open:        rec.Open,
				High:        rec.High,
				Low:         rec.Low,
				PrevClose:   rec.PrevClose,
				ATP:         rec.ATP,
				Volume:      rec.Volume,
				Turnover:    rec.Turnover,
				LotSize:     int(rec.LotSize),
				SequenceNum: rec.SequenceNum,
				Segment:     segment,
				BidPrices:   rec.BidPrices,
				BidQtys:     rec.BidQtys,
				AskPrices:   rec.AskPrices,
				AskQtys:     rec.AskQtys,
			}

			// Save to CSV with latency tracking
			saveStart := time.Now()
			if err := csvSaver.Save(quote); err == nil {
				tracker.RecordSaveLatency(time.Since(saveStart).Nanoseconds())
				tracker.RecordQuote(segment)
			}
		}

		processLatency := time.Since(processStart).Nanoseconds()
		tracker.RecordProcessLatency(processLatency)
	}
}

// printBanner prints the startup banner
func printBanner(cfg *config.Config, enableEQ, enableFO bool) {
	mode := "BOTH (EQ + FO)"
	if enableEQ && !enableFO {
		mode = "EQ Only"
	} else if !enableEQ && enableFO {
		mode = "FO Only"
	}

	fmt.Println("================================================================================")
	fmt.Println("         BSE GO HFT SERVER - DUAL FEED PRODUCTION PIPELINE")
	fmt.Println("================================================================================")
	fmt.Printf("Start Time:      %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("Mode:            %s\n", mode)
	fmt.Printf("EQ Multicast:    %s:%d (Equity Cash)\n", cfg.MulticastCM.IP, cfg.MulticastCM.Port)
	fmt.Printf("FO Multicast:    %s:%d (F&O Derivatives)\n", cfg.MulticastFO.IP, cfg.MulticastFO.Port)
	fmt.Printf("Data Directory:  %s\n", cfg.DataManagement.OutputDir)
	fmt.Printf("GOMAXPROCS:      %d\n", runtime.GOMAXPROCS(0))
	fmt.Printf("Duration:        Until Ctrl+C\n")
	fmt.Println("================================================================================")
}
