// BSE NFCAST Trade Data Capture Server (Message Type 2017)
// Captures individual trade executions from BSE UDP multicast feed
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"bse-go-hft/internal/config"
	"bse-go-hft/internal/decoder"
	"bse-go-hft/internal/receiver"
	"bse-go-hft/internal/saver"
)

var (
	configFile = flag.String("config", "config.json", "Path to config file")
	duration   = flag.Duration("duration", 0, "Run duration (0 = infinite)")
	outputDir  = flag.String("output", "data/processed_csv", "Output directory for CSV files")
)

func main() {
	flag.Parse()

	fmt.Println("BSE NFCAST Trade Data Capture Server")
	fmt.Println("====================================================================================================")
	fmt.Println("Message Type: 2017 (Trade Data)")
	fmt.Println("Capturing individual trade executions...")
	fmt.Println("====================================================================================================")

	// Load configuration (not used for now, using defaults)
	_, err := config.LoadConfig(*configFile)
	if err != nil {
		// Config load failed, use defaults
		fmt.Println("⚠ Config load failed, using defaults")
	}

	fmt.Printf("Multicast Address: %s:%d\n", "239.1.2.5", 26001)
	fmt.Printf("Output Directory:  %s\n", *outputDir)
	if *duration > 0 {
		fmt.Printf("Capture Duration:  %v\n", *duration)
	} else {
		fmt.Println("Capture Duration:  Infinite (Ctrl+C to stop)")
	}
	fmt.Println("====================================================================================================")
	fmt.Println()

	// Initialize decoder
	dec := decoder.NewDecoder()

	// Initialize trade saver
	tradeSaver, err := saver.NewTradeSaver(*outputDir)
	if err != nil {
		log.Fatalf("❌ Failed to create trade saver: %v", err)
	}
	defer tradeSaver.Close()

	// Packet channel for receiving data
	type packet struct {
		data   []byte
		length int
		time   time.Time
	}
	packetChan := make(chan packet, 100)

	// Create packet handler
	handler := func(data []byte, length int, receiveTime time.Time) {
		// Copy data since buffer is reused
		dataCopy := make([]byte, length)
		copy(dataCopy, data[:length])

		select {
		case packetChan <- packet{dataCopy, length, receiveTime}:
		default:
			// Channel full, skip
		}
	}

	// Initialize multicast receiver
	recvConfig := receiver.DefaultConfig()
	recv := receiver.NewMulticastReceiver(recvConfig, handler)

	if err := recv.Connect(); err != nil {
		log.Fatalf("❌ Failed to connect receiver: %v", err)
	}
	defer recv.Close()

	// Start receive loop in goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := recv.ReceiveLoop(ctx); err != nil && err != context.Canceled {
			log.Printf("⚠ Receive loop error: %v", err)
		}
	}()

	fmt.Println("✓ Connected to BSE NFCAST feed")
	fmt.Println("✓ Trade saver initialized")
	fmt.Println("✓ Listening for Message Type 2017 packets...")
	fmt.Println()

	// Statistics tracking
	var (
		totalPackets  int64
		tradePackets  int64
		totalTrades   int64
		otherPackets  int64
		startTime     = time.Now()
		lastStatsTime = startTime
		statsInterval = 10 * time.Second
		flushInterval = 5 * time.Second
		lastFlushTime = startTime
	)

	// Signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Timeout timer if duration is set
	var timeoutChan <-chan time.Time
	if *duration > 0 {
		timeoutChan = time.After(*duration)
	}

	// Statistics ticker
	statsTicker := time.NewTicker(statsInterval)
	defer statsTicker.Stop()

	// Main packet processing loop
	running := true

	for running {
		select {
		case <-sigChan:
			fmt.Println()
			fmt.Println("⚠ Interrupt received, stopping capture...")
			running = false

		case <-timeoutChan:
			fmt.Println()
			fmt.Println("✓ Duration reached, stopping capture...")
			running = false

		case <-statsTicker.C:
			// Print statistics
			elapsed := time.Since(lastStatsTime)
			lastStatsTime = time.Now()

			packetsPerSec := float64(totalPackets) / elapsed.Seconds()
			tradesPerSec := float64(totalTrades) / elapsed.Seconds()

			fmt.Printf("📊 Stats: %d packets (%.0f pkt/s) | %d trades (%.0f trd/s) | 2017: %d | Other: %d\n",
				totalPackets, packetsPerSec, totalTrades, tradesPerSec, tradePackets, otherPackets)

		case pkt := <-packetChan:
			totalPackets++

			// Check message type
			if len(pkt.data) < 10 {
				continue // Too small
			}

			msgType := uint16(pkt.data[8]) | uint16(pkt.data[9])<<8

			// Only process Message Type 2017
			if msgType != 2017 {
				otherPackets++
				continue
			}

			tradePackets++

			// Decode trade data
			trades, err := dec.DecodeMsgType2017(pkt.data, pkt.length)
			if err != nil {
				log.Printf("⚠ Decode error: %v", err)
				continue
			}

			if len(trades) == 0 {
				continue
			}

			// Save to CSV
			if err := tradeSaver.Save(trades); err != nil {
				log.Printf("❌ Save error: %v", err)
				continue
			}

			totalTrades += int64(len(trades))

			// Periodic flush
			if time.Since(lastFlushTime) >= flushInterval {
				if err := tradeSaver.Flush(); err != nil {
					log.Printf("❌ Flush error: %v", err)
				}
				lastFlushTime = time.Now()
			}
		}
	}

	// Final flush
	fmt.Println()
	fmt.Println("Flushing data to disk...")
	if err := tradeSaver.Flush(); err != nil {
		log.Printf("❌ Final flush error: %v", err)
	}

	// Print final statistics
	elapsed := time.Since(startTime)
	recordCount, _, recordsPerSec := tradeSaver.GetStats()

	fmt.Println()
	fmt.Println("====================================================================================================")
	fmt.Println("TRADE DATA CAPTURE SUMMARY")
	fmt.Println("====================================================================================================")
	fmt.Printf("Duration:        %v\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Total Packets:   %d\n", totalPackets)
	fmt.Printf("Trade Packets:   %d (Message Type 2017)\n", tradePackets)
	fmt.Printf("Other Packets:   %d\n", otherPackets)
	fmt.Printf("Total Trades:    %d\n", totalTrades)
	fmt.Printf("Trades Saved:    %d\n", recordCount)
	fmt.Printf("Average Rate:    %.2f trades/sec\n", recordsPerSec)

	// Print output file location
	filename := fmt.Sprintf("%s_trades.csv", time.Now().Format("20060102"))
	outputPath := filepath.Join(*outputDir, filename)
	fmt.Printf("Output File:     %s\n", outputPath)

	// Check file size
	if stat, err := os.Stat(outputPath); err == nil {
		fmt.Printf("File Size:       %.2f KB (%d bytes)\n", float64(stat.Size())/1024, stat.Size())
	}

	fmt.Println("====================================================================================================")
	fmt.Println()
	fmt.Println("✓ Trade capture complete!")
}
