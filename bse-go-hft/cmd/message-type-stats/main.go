// Message Type Statistics Tracker
// Monitors BSE NFCAST feed and tracks which message types are received
package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"bse-go-hft/internal/receiver"
)

// Message type names from BSE NFCAST Manual
var messageTypeNames = map[uint16]string{
	2001: "Time Broadcast",
	2030: "Auction Keep Alive",
	2002: "Product State Change",
	2003: "Shortage Auction Session",
	2004: "News Headline",
	2020: "Market Picture (Equity)",
	2021: "Market Picture (Derivative)",
	2017: "Auction Market Picture",
	2027: "Odd-lot Market Picture",
	2033: "Debt Market Picture",
	2011: "Index Change (Equity)",
	2012: "Index Change (Derivative)",
	2034: "Limit Price Protection",
	2035: "Call Auction Cancelled Qty",
	2014: "Close Price",
	2015: "Open Interest",
	2016: "VaR Percentage",
	2022: "RBI Reference Rate",
	2028: "Implied Volatility",
}

// MessageStats tracks statistics for each message type
type MessageStats struct {
	Count      uint64
	TotalBytes uint64
	FirstSeen  time.Time
	LastSeen   time.Time
}

// StatsTracker tracks message type statistics
type StatsTracker struct {
	mu    sync.RWMutex
	stats map[uint16]*MessageStats

	// Global stats
	totalPackets    uint64
	totalBytes      uint64
	unknownMsgTypes uint64
	startTime       time.Time
}

// NewStatsTracker creates a new statistics tracker
func NewStatsTracker() *StatsTracker {
	return &StatsTracker{
		stats:     make(map[uint16]*MessageStats),
		startTime: time.Now(),
	}
}

// RecordMessage records a message type and updates statistics
func (st *StatsTracker) RecordMessage(msgType uint16, length int) {
	st.mu.Lock()
	defer st.mu.Unlock()

	st.totalPackets++
	st.totalBytes += uint64(length)

	// Check if message type is known
	if _, known := messageTypeNames[msgType]; !known {
		st.unknownMsgTypes++
	}

	// Get or create stats for this message type
	stats, exists := st.stats[msgType]
	if !exists {
		stats = &MessageStats{
			FirstSeen: time.Now(),
		}
		st.stats[msgType] = stats
	}

	stats.Count++
	stats.TotalBytes += uint64(length)
	stats.LastSeen = time.Now()
}

// PrintStats prints current statistics
func (st *StatsTracker) PrintStats() {
	st.mu.RLock()
	defer st.mu.RUnlock()

	// Clear screen
	fmt.Print("\033[2J\033[H")

	fmt.Println("╔═══════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    BSE NFCAST MESSAGE TYPE STATISTICS                             ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════════════════════════════╣")

	elapsed := time.Since(st.startTime)
	fmt.Printf("║ Running Time: %-67s ║\n", elapsed.Round(time.Second))
	fmt.Printf("║ Total Packets: %-66d ║\n", st.totalPackets)
	fmt.Printf("║ Total Bytes: %-68s ║\n", formatBytes(st.totalBytes))
	fmt.Printf("║ Unknown Message Types: %-58d ║\n", st.unknownMsgTypes)
	fmt.Println("╠═══════════════════════════════════════════════════════════════════════════════════╣")

	// Sort message types
	msgTypes := make([]uint16, 0, len(st.stats))
	for msgType := range st.stats {
		msgTypes = append(msgTypes, msgType)
	}
	sort.Slice(msgTypes, func(i, j int) bool {
		return msgTypes[i] < msgTypes[j]
	})

	// Print table header
	fmt.Println("║ Type │ Message Name                      │ Count      │ Bytes      │ Pkt/sec   ║")
	fmt.Println("╠══════╪═══════════════════════════════════╪════════════╪════════════╪═══════════╣")

	// Print statistics for each message type
	totalCount := uint64(0)
	totalMsgBytes := uint64(0)

	for _, msgType := range msgTypes {
		stats := st.stats[msgType]
		name := messageTypeNames[msgType]
		if name == "" {
			name = "UNKNOWN"
		}

		pktsPerSec := float64(stats.Count) / elapsed.Seconds()

		fmt.Printf("║ %4d │ %-33s │ %10d │ %10s │ %9.2f ║\n",
			msgType,
			truncate(name, 33),
			stats.Count,
			formatBytes(stats.TotalBytes),
			pktsPerSec,
		)

		totalCount += stats.Count
		totalMsgBytes += stats.TotalBytes
	}

	fmt.Println("╠══════╧═══════════════════════════════════╧════════════╧════════════╧═══════════╣")

	// Print totals
	avgPktsPerSec := float64(st.totalPackets) / elapsed.Seconds()
	fmt.Printf("║ Total Messages: %10d │ Total Bytes: %10s │ Avg Pkt/s: %7.2f ║\n",
		totalCount, formatBytes(totalMsgBytes), avgPktsPerSec)

	fmt.Println("╚═══════════════════════════════════════════════════════════════════════════════════╝")

	// Print message type coverage
	fmt.Println()
	fmt.Println("Message Types Coverage:")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════")

	// All known message types
	allTypes := []uint16{2001, 2030, 2002, 2003, 2004, 2020, 2021, 2017, 2027, 2033, 2011, 2012, 2034, 2035, 2014, 2015, 2016, 2022, 2028}

	receivedCount := 0
	notReceivedCount := 0

	fmt.Println("✅ RECEIVED:")
	for _, msgType := range allTypes {
		if _, exists := st.stats[msgType]; exists {
			fmt.Printf("  • [%4d] %s (Count: %d)\n", msgType, messageTypeNames[msgType], st.stats[msgType].Count)
			receivedCount++
		}
	}

	fmt.Println("\n❌ NOT RECEIVED:")
	for _, msgType := range allTypes {
		if _, exists := st.stats[msgType]; !exists {
			fmt.Printf("  • [%4d] %s\n", msgType, messageTypeNames[msgType])
			notReceivedCount++
		}
	}

	fmt.Printf("\nCoverage: %d/%d message types (%.1f%%)\n",
		receivedCount, len(allTypes), float64(receivedCount)*100/float64(len(allTypes)))
}

// ExportStats exports statistics to a file
func (st *StatsTracker) ExportStats(filename string) error {
	st.mu.RLock()
	defer st.mu.RUnlock()

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	elapsed := time.Since(st.startTime)

	fmt.Fprintf(f, "BSE NFCAST Message Type Statistics\n")
	fmt.Fprintf(f, "===================================\n")
	fmt.Fprintf(f, "Export Time: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(f, "Running Time: %s\n", elapsed.Round(time.Second))
	fmt.Fprintf(f, "Total Packets: %d\n", st.totalPackets)
	fmt.Fprintf(f, "Total Bytes: %s\n", formatBytes(st.totalBytes))
	fmt.Fprintf(f, "Unknown Message Types: %d\n\n", st.unknownMsgTypes)

	// Sort message types
	msgTypes := make([]uint16, 0, len(st.stats))
	for msgType := range st.stats {
		msgTypes = append(msgTypes, msgType)
	}
	sort.Slice(msgTypes, func(i, j int) bool {
		return st.stats[msgTypes[i]].Count > st.stats[msgTypes[j]].Count
	})

	fmt.Fprintf(f, "Message Type Details:\n")
	fmt.Fprintf(f, "%-6s %-35s %-12s %-12s %-12s %-20s %-20s\n",
		"Type", "Name", "Count", "Bytes", "Pkt/sec", "First Seen", "Last Seen")
	fmt.Fprintf(f, "%s\n", "─────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────")

	for _, msgType := range msgTypes {
		stats := st.stats[msgType]
		name := messageTypeNames[msgType]
		if name == "" {
			name = "UNKNOWN"
		}

		pktsPerSec := float64(stats.Count) / elapsed.Seconds()

		fmt.Fprintf(f, "%-6d %-35s %-12d %-12s %-12.2f %-20s %-20s\n",
			msgType,
			name,
			stats.Count,
			formatBytes(stats.TotalBytes),
			pktsPerSec,
			stats.FirstSeen.Format("15:04:05"),
			stats.LastSeen.Format("15:04:05"),
		)
	}

	fmt.Fprintf(f, "\nMessage Type Coverage:\n")
	fmt.Fprintf(f, "======================\n")

	allTypes := []uint16{2001, 2030, 2002, 2003, 2004, 2020, 2021, 2017, 2027, 2033, 2011, 2012, 2034, 2035, 2014, 2015, 2016, 2022, 2028}

	receivedCount := 0

	fmt.Fprintf(f, "\nReceived Message Types:\n")
	for _, msgType := range allTypes {
		if _, exists := st.stats[msgType]; exists {
			fmt.Fprintf(f, "  ✓ [%4d] %s (Count: %d)\n", msgType, messageTypeNames[msgType], st.stats[msgType].Count)
			receivedCount++
		}
	}

	fmt.Fprintf(f, "\nNot Received Message Types:\n")
	for _, msgType := range allTypes {
		if _, exists := st.stats[msgType]; !exists {
			fmt.Fprintf(f, "  ✗ [%4d] %s\n", msgType, messageTypeNames[msgType])
		}
	}

	fmt.Fprintf(f, "\nCoverage: %d/%d message types (%.1f%%)\n",
		receivedCount, len(allTypes), float64(receivedCount)*100/float64(len(allTypes)))

	return nil
}

// formatBytes formats bytes into human-readable format
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// truncate truncates a string to maxLen
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func main() {
	log.Println("BSE NFCAST Message Type Statistics Tracker")
	log.Println("===========================================")
	log.Println()

	// Create statistics tracker
	tracker := NewStatsTracker()

	// Create receiver
	config := receiver.DefaultConfig()
	log.Printf("Connecting to: %s:%d", config.MulticastIP, config.Port)

	rcvr := receiver.NewMulticastReceiver(config, func(data []byte, length int, receiveTime time.Time) {
		// Parse message type
		// BSE NFCAST packet structure (from decoder.go):
		// Bytes 0-3: Unknown/Reserved
		// Bytes 4-5: Format ID (Big Endian)
		// Bytes 6-7: Unknown/Reserved
		// Bytes 8-9: Message Type (Little Endian uint16)
		if length < 10 {
			return
		}

		// Message type is at offset 8-9 in Little Endian
		msgType := binary.LittleEndian.Uint16(data[8:10])

		tracker.RecordMessage(msgType, length)
	})

	// Connect
	if err := rcvr.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer rcvr.Close()

	log.Println("✓ Connected successfully")
	log.Println("✓ Receiving packets...")
	log.Println()
	log.Println("Press Ctrl+C to stop and save statistics")
	log.Println()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start receiving in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := rcvr.ReceiveLoop(ctx); err != nil && err != context.Canceled {
			log.Printf("Receiver error: %v", err)
		}
	}()

	// Print stats every 2 seconds
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tracker.PrintStats()
		case <-sigChan:
			log.Println("\n\nShutting down...")

			// Export final statistics
			filename := fmt.Sprintf("message_type_stats_%s.txt", time.Now().Format("20060102_150405"))
			if err := tracker.ExportStats(filename); err != nil {
				log.Printf("Failed to export stats: %v", err)
			} else {
				log.Printf("Statistics exported to: %s", filename)
			}

			// Print final stats
			tracker.PrintStats()
			return
		}
	}
}
