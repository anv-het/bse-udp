// Message Type Monitor - Captures and analyzes all BSE NFCAST message types
// This tool listens to the UDP multicast feed for a specified duration and
// logs statistics about all message types received.

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// Multicast configuration (from docs)
	MulticastAddr = "239.1.2.5:26001"
	InterfaceIP   = "0.0.0.0" // Listen on all interfaces

	// Packet constraints
	MaxPacketSize = 2048
	HeaderSize    = 36
)

// MessageTypeStats tracks statistics for a specific message type
type MessageTypeStats struct {
	MessageType   uint16
	PacketCount   int64
	TotalBytes    int64
	FirstSeen     time.Time
	LastSeen      time.Time
	MinSize       int
	MaxSize       int
	AvgSize       float64
	SamplePackets [][]byte // Keep first 3 packets for analysis
}

// Monitor represents the message type monitoring tool
type Monitor struct {
	stats      map[uint16]*MessageTypeStats
	mu         sync.RWMutex
	startTime  time.Time
	totalPkts  int64
	totalBytes int64
}

// NewMonitor creates a new message type monitor
func NewMonitor() *Monitor {
	return &Monitor{
		stats:     make(map[uint16]*MessageTypeStats),
		startTime: time.Now(),
	}
}

// ProcessPacket processes a received packet and updates statistics
func (m *Monitor) ProcessPacket(packet []byte, length int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalPkts++
	m.totalBytes += int64(length)

	// Parse message type (offset 8-9, Little-Endian)
	if length < 10 {
		return
	}

	msgType := binary.LittleEndian.Uint16(packet[8:10])

	// Get or create stats for this message type
	stats, exists := m.stats[msgType]
	if !exists {
		stats = &MessageTypeStats{
			MessageType:   msgType,
			FirstSeen:     time.Now(),
			MinSize:       length,
			MaxSize:       length,
			SamplePackets: make([][]byte, 0, 3),
		}
		m.stats[msgType] = stats
	}

	// Update statistics
	stats.PacketCount++
	stats.TotalBytes += int64(length)
	stats.LastSeen = time.Now()

	if length < stats.MinSize {
		stats.MinSize = length
	}
	if length > stats.MaxSize {
		stats.MaxSize = length
	}

	stats.AvgSize = float64(stats.TotalBytes) / float64(stats.PacketCount)

	// Store sample packets (first 3)
	if len(stats.SamplePackets) < 3 {
		sample := make([]byte, length)
		copy(sample, packet[:length])
		stats.SamplePackets = append(stats.SamplePackets, sample)
	}
}

// GetStats returns a snapshot of current statistics
func (m *Monitor) GetStats() map[uint16]*MessageTypeStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Create a copy
	statsCopy := make(map[uint16]*MessageTypeStats)
	for msgType, stats := range m.stats {
		statsCopy[msgType] = stats
	}

	return statsCopy
}

// PrintSummary prints a summary of all captured message types
func (m *Monitor) PrintSummary() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	duration := time.Since(m.startTime)

	fmt.Println("\n" + strings.Repeat("=", 100))
	fmt.Println("MESSAGE TYPE MONITORING SUMMARY")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("\nMonitoring Duration: %v\n", duration)
	fmt.Printf("Total Packets:       %d\n", m.totalPkts)
	fmt.Printf("Total Bytes:         %d (%.2f MB)\n", m.totalBytes, float64(m.totalBytes)/(1024*1024))
	fmt.Printf("Average Rate:        %.2f packets/sec\n", float64(m.totalPkts)/duration.Seconds())
	fmt.Printf("Unique Message Types: %d\n", len(m.stats))

	fmt.Println("\n" + strings.Repeat("-", 100))
	fmt.Println("DETAILED MESSAGE TYPE STATISTICS")
	fmt.Println(strings.Repeat("-", 100))

	// Sort message types
	msgTypes := make([]uint16, 0, len(m.stats))
	for msgType := range m.stats {
		msgTypes = append(msgTypes, msgType)
	}
	sort.Slice(msgTypes, func(i, j int) bool {
		return msgTypes[i] < msgTypes[j]
	})

	// Print header
	fmt.Printf("%-15s %-15s %-12s %-15s %-12s %-12s %-15s %-20s\n",
		"Message Type", "Packet Count", "Total Bytes", "Avg Size", "Min Size", "Max Size", "Frequency", "Description")
	fmt.Println(strings.Repeat("-", 100))

	// Print each message type
	for _, msgType := range msgTypes {
		stats := m.stats[msgType]
		frequency := time.Since(stats.FirstSeen).Seconds() / float64(stats.PacketCount)
		description := getMessageTypeDescription(msgType)

		fmt.Printf("%-15d %-15d %-12d %-15.2f %-12d %-12d %-15s %-20s\n",
			msgType,
			stats.PacketCount,
			stats.TotalBytes,
			stats.AvgSize,
			stats.MinSize,
			stats.MaxSize,
			fmt.Sprintf("~%.2fs", frequency),
			description)
	}

	fmt.Println(strings.Repeat("=", 100))
}

// PrintDetailedAnalysis prints detailed analysis of each message type
func (m *Monitor) PrintDetailedAnalysis() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	fmt.Println("\n" + strings.Repeat("=", 100))
	fmt.Println("DETAILED MESSAGE TYPE ANALYSIS")
	fmt.Println(strings.Repeat("=", 100))

	// Sort message types
	msgTypes := make([]uint16, 0, len(m.stats))
	for msgType := range m.stats {
		msgTypes = append(msgTypes, msgType)
	}
	sort.Slice(msgTypes, func(i, j int) bool {
		return msgTypes[i] < msgTypes[j]
	})

	for _, msgType := range msgTypes {
		stats := m.stats[msgType]
		fmt.Printf("\n" + strings.Repeat("-", 100) + "\n")
		fmt.Printf("MESSAGE TYPE: %d (%s)\n", msgType, getMessageTypeDescription(msgType))
		fmt.Printf(strings.Repeat("-", 100) + "\n")
		fmt.Printf("  Total Packets:    %d\n", stats.PacketCount)
		fmt.Printf("  Total Data:       %d bytes (%.2f KB)\n", stats.TotalBytes, float64(stats.TotalBytes)/1024)
		fmt.Printf("  Average Size:     %.2f bytes\n", stats.AvgSize)
		fmt.Printf("  Size Range:       %d - %d bytes\n", stats.MinSize, stats.MaxSize)
		fmt.Printf("  First Seen:       %s\n", stats.FirstSeen.Format("15:04:05.000"))
		fmt.Printf("  Last Seen:        %s\n", stats.LastSeen.Format("15:04:05.000"))

		duration := stats.LastSeen.Sub(stats.FirstSeen)
		if duration.Seconds() > 0 && stats.PacketCount > 1 {
			frequency := duration.Seconds() / float64(stats.PacketCount-1)
			fmt.Printf("  Update Frequency: ~%.2f seconds\n", frequency)
		}

		// Print sample packet header
		if len(stats.SamplePackets) > 0 {
			fmt.Printf("\n  Sample Packet Header (first %d bytes):\n", min(64, len(stats.SamplePackets[0])))
			printHexDump(stats.SamplePackets[0], 64)
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 100))
}

// getMessageTypeDescription returns a human-readable description of message type
func getMessageTypeDescription(msgType uint16) string {
	descriptions := map[uint16]string{
		2011: "Index Broadcast 1 (Critical)",
		2012: "Index Broadcast 2 (Regular)",
		2013: "Market Statistics",
		2014: "News Messages",
		2015: "Circuit Breaker",
		2016: "Order Book Snapshots",
		2017: "Trade Data",
		2018: "Corporate Actions",
		2019: "Reference Data",
		2020: "Equity Quotes (CM)",
		2021: "Derivatives (F&O)",
	}

	if desc, exists := descriptions[msgType]; exists {
		return desc
	}
	return "Unknown"
}

// printHexDump prints a hex dump of the given data
func printHexDump(data []byte, maxBytes int) {
	length := min(maxBytes, len(data))
	for i := 0; i < length; i += 16 {
		// Print offset
		fmt.Printf("    %04X: ", i)

		// Print hex values
		for j := 0; j < 16; j++ {
			if i+j < length {
				fmt.Printf("%02X ", data[i+j])
			} else {
				fmt.Print("   ")
			}
			if j == 7 {
				fmt.Print(" ")
			}
		}

		// Print ASCII values
		fmt.Print(" |")
		for j := 0; j < 16 && i+j < length; j++ {
			b := data[i+j]
			if b >= 32 && b < 127 {
				fmt.Printf("%c", b)
			} else {
				fmt.Print(".")
			}
		}
		fmt.Println("|")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	// Command-line flags
	duration := flag.Duration("duration", 2*time.Minute, "Monitoring duration (e.g., 30s, 2m, 1h)")
	verbose := flag.Bool("verbose", false, "Print detailed packet analysis")
	statsInterval := flag.Duration("stats-interval", 15*time.Second, "Statistics print interval")

	flag.Parse()

	fmt.Println("BSE NFCAST Message Type Monitor")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("Multicast Address: %s\n", MulticastAddr)
	fmt.Printf("Monitoring Duration: %v\n", *duration)
	fmt.Printf("Statistics Interval: %v\n", *statsInterval)
	fmt.Println(strings.Repeat("=", 100))

	// Create monitor
	monitor := NewMonitor()

	// Setup UDP multicast listener
	addr, err := net.ResolveUDPAddr("udp", MulticastAddr)
	if err != nil {
		fmt.Printf("Error resolving address: %v\n", err)
		os.Exit(1)
	}

	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		fmt.Printf("Error creating multicast connection: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Set read buffer
	if err := conn.SetReadBuffer(4 * 1024 * 1024); err != nil {
		fmt.Printf("Warning: failed to set read buffer: %v\n", err)
	}

	fmt.Println("\n✓ Connected to BSE NFCAST feed")
	fmt.Println("✓ Listening for packets...")
	fmt.Printf("\nMonitoring will stop after %v or press Ctrl+C to stop early\n\n", *duration)

	// Setup signal handler for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Setup timer for duration
	timer := time.NewTimer(*duration)
	defer timer.Stop()

	// Setup statistics ticker
	statsTicker := time.NewTicker(*statsInterval)
	defer statsTicker.Stop()

	// Packet processing goroutine
	packetChan := make(chan []byte, 1000)
	go func() {
		buffer := make([]byte, MaxPacketSize)
		for {
			n, _, err := conn.ReadFromUDP(buffer)
			if err != nil {
				return
			}

			// Make a copy of the packet
			packet := make([]byte, n)
			copy(packet, buffer[:n])

			select {
			case packetChan <- packet:
			default:
				// Channel full, drop packet
			}
		}
	}()

	// Main monitoring loop
	running := true
	for running {
		select {
		case <-timer.C:
			fmt.Println("\n⏱ Duration elapsed, stopping monitor...")
			running = false

		case <-sigChan:
			fmt.Println("\n\n⚠ Interrupt received, stopping monitor...")
			running = false

		case <-statsTicker.C:
			// Print interim statistics
			fmt.Printf("\n[%s] Packets captured: %d, Message types: %d\n",
				time.Now().Format("15:04:05"),
				monitor.totalPkts,
				len(monitor.stats))

		case packet := <-packetChan:
			monitor.ProcessPacket(packet, len(packet))
		}
	}

	// Print final summary
	monitor.PrintSummary()

	if *verbose {
		monitor.PrintDetailedAnalysis()
	}

	fmt.Println("\n✓ Monitoring complete!")
}
