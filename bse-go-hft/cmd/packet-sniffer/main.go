// BSE Packet Sniffer - Captures and analyzes all message types
// This tool helps reverse engineer unknown message types by:
// 1. Capturing raw UDP packets
// 2. Identifying message types
// 3. Saving hex dumps for analysis
// 4. Generating statistics by message type
//
// Usage:
//   go run ./cmd/packet-sniffer/main.go
//   go run ./cmd/packet-sniffer/main.go -msgtype 2011
//   go run ./cmd/packet-sniffer/main.go -duration 5m -output captures/

package main

import (
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	HeaderSize = 36
)

type PacketCapture struct {
	Timestamp   time.Time
	MessageType uint16
	Length      int
	RawData     []byte
}

type MessageTypeStats struct {
	Count         int
	FirstSeen     time.Time
	LastSeen      time.Time
	MinSize       int
	MaxSize       int
	TotalBytes    int64
	SamplePackets [][]byte // Store first 10 packets for analysis
}

type PacketSniffer struct {
	captures      map[uint16]*MessageTypeStats
	mu            sync.Mutex
	outputDir     string
	filterMsgType uint16 // 0 = capture all
	conn          *net.UDPConn
}

func NewPacketSniffer(outputDir string, filterMsgType uint16) *PacketSniffer {
	return &PacketSniffer{
		captures:      make(map[uint16]*MessageTypeStats),
		outputDir:     outputDir,
		filterMsgType: filterMsgType,
	}
}

func (ps *PacketSniffer) Start(multicastAddr, port string) error {
	// Parse multicast address
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%s", multicastAddr, port))
	if err != nil {
		return fmt.Errorf("failed to resolve address: %w", err)
	}

	// Create UDP connection
	ps.conn, err = net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	// Set receive buffer to 16MB
	if err := ps.conn.SetReadBuffer(16 * 1024 * 1024); err != nil {
		fmt.Printf("Warning: Failed to set buffer size: %v\n", err)
	}

	fmt.Printf("✅ Listening on %s:%s\n", multicastAddr, port)
	fmt.Printf("📁 Output directory: %s\n", ps.outputDir)
	if ps.filterMsgType > 0 {
		fmt.Printf("🔍 Filtering message type: %d\n", ps.filterMsgType)
	} else {
		fmt.Printf("🔍 Capturing all message types\n")
	}
	fmt.Println("Press Ctrl+C to stop and view statistics...")
	fmt.Println()

	return nil
}

func (ps *PacketSniffer) CaptureLoop() {
	buffer := make([]byte, 4096)

	for {
		n, _, err := ps.conn.ReadFromUDP(buffer)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			fmt.Printf("Error reading: %v\n", err)
			return
		}

		if n < HeaderSize {
			continue
		}

		// Extract message type (Little-Endian at offset 8-9)
		msgType := binary.LittleEndian.Uint16(buffer[8:10])

		// Filter if specified
		if ps.filterMsgType > 0 && msgType != ps.filterMsgType {
			continue
		}

		// Store capture
		ps.recordCapture(msgType, buffer[:n])
	}
}

func (ps *PacketSniffer) recordCapture(msgType uint16, data []byte) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats, exists := ps.captures[msgType]
	if !exists {
		stats = &MessageTypeStats{
			FirstSeen:     time.Now(),
			MinSize:       len(data),
			MaxSize:       len(data),
			SamplePackets: make([][]byte, 0, 10),
		}
		ps.captures[msgType] = stats
	}

	stats.Count++
	stats.LastSeen = time.Now()
	stats.TotalBytes += int64(len(data))

	if len(data) < stats.MinSize {
		stats.MinSize = len(data)
	}
	if len(data) > stats.MaxSize {
		stats.MaxSize = len(data)
	}

	// Store first 10 sample packets
	if len(stats.SamplePackets) < 10 {
		packetCopy := make([]byte, len(data))
		copy(packetCopy, data)
		stats.SamplePackets = append(stats.SamplePackets, packetCopy)
	}

	// Print live updates every 100 packets
	if stats.Count%100 == 0 {
		fmt.Printf("\r[MsgType %d] Captured: %d packets, Size: %d-%d bytes",
			msgType, stats.Count, stats.MinSize, stats.MaxSize)
	}
}

func (ps *PacketSniffer) PrintSummary() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	fmt.Println("\n\n" + strings.Repeat("=", 80))
	fmt.Println("PACKET CAPTURE SUMMARY")
	fmt.Println(strings.Repeat("=", 80))

	if len(ps.captures) == 0 {
		fmt.Println("No packets captured!")
		return
	}

	fmt.Printf("\nTotal message types captured: %d\n\n", len(ps.captures))

	// Print summary table
	fmt.Println("┌──────────────┬───────────┬────────────┬────────────┬──────────────┐")
	fmt.Println("│ Message Type │   Count   │  Min Size  │  Max Size  │  Total Bytes │")
	fmt.Println("├──────────────┼───────────┼────────────┼────────────┼──────────────┤")

	for msgType, stats := range ps.captures {
		fmt.Printf("│     %-8d │ %9d │ %10d │ %10d │ %12d │\n",
			msgType, stats.Count, stats.MinSize, stats.MaxSize, stats.TotalBytes)
	}

	fmt.Println("└──────────────┴───────────┴────────────┴────────────┴──────────────┘")
}

func (ps *PacketSniffer) SaveSamples() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Create output directory
	if err := os.MkdirAll(ps.outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	fmt.Println("\n\n" + strings.Repeat("=", 80))
	fmt.Println("SAVING SAMPLE PACKETS")
	fmt.Println(strings.Repeat("=", 80))

	for msgType, stats := range ps.captures {
		if len(stats.SamplePackets) == 0 {
			continue
		}

		// Create directory for this message type
		msgDir := filepath.Join(ps.outputDir, fmt.Sprintf("msgtype_%d", msgType))
		if err := os.MkdirAll(msgDir, 0755); err != nil {
			fmt.Printf("⚠️  Failed to create directory for msgtype %d: %v\n", msgType, err)
			continue
		}

		// Save each sample packet
		for i, packet := range stats.SamplePackets {
			// Save as hex dump
			hexFile := filepath.Join(msgDir, fmt.Sprintf("sample_%d.hex", i+1))
			if err := os.WriteFile(hexFile, []byte(hex.Dump(packet)), 0644); err != nil {
				fmt.Printf("⚠️  Failed to write %s: %v\n", hexFile, err)
				continue
			}

			// Save as binary
			binFile := filepath.Join(msgDir, fmt.Sprintf("sample_%d.bin", i+1))
			if err := os.WriteFile(binFile, packet, 0644); err != nil {
				fmt.Printf("⚠️  Failed to write %s: %v\n", binFile, err)
				continue
			}

			// Save analysis
			analysisFile := filepath.Join(msgDir, fmt.Sprintf("sample_%d.txt", i+1))
			analysis := ps.analyzePacket(packet, msgType)
			if err := os.WriteFile(analysisFile, []byte(analysis), 0644); err != nil {
				fmt.Printf("⚠️  Failed to write %s: %v\n", analysisFile, err)
			}
		}

		fmt.Printf("✅ Saved %d samples for message type %d to %s\n",
			len(stats.SamplePackets), msgType, msgDir)
	}

	return nil
}

func (ps *PacketSniffer) analyzePacket(packet []byte, msgType uint16) string {
	var analysis strings.Builder

	analysis.WriteString(fmt.Sprintf("BSE NFCAST Packet Analysis - Message Type %d\n", msgType))
	analysis.WriteString(strings.Repeat("=", 60) + "\n\n")

	analysis.WriteString(fmt.Sprintf("Packet Length: %d bytes\n", len(packet)))
	analysis.WriteString(fmt.Sprintf("Capture Time: %s\n\n", time.Now().Format("2006-01-02 15:04:05.000")))

	if len(packet) < HeaderSize {
		analysis.WriteString("⚠️  Packet too short (< 36 bytes)\n")
		return analysis.String()
	}

	// Parse header
	analysis.WriteString("HEADER (36 bytes):\n")
	analysis.WriteString(strings.Repeat("-", 60) + "\n")

	formatID := binary.BigEndian.Uint16(packet[4:6])
	messageType := binary.LittleEndian.Uint16(packet[8:10])
	hour := binary.LittleEndian.Uint16(packet[20:22])
	minute := binary.LittleEndian.Uint16(packet[22:24])
	second := binary.LittleEndian.Uint16(packet[24:26])
	millisecond := binary.LittleEndian.Uint16(packet[26:28])

	analysis.WriteString(fmt.Sprintf("  Format ID:    0x%04X (Big-Endian)\n", formatID))
	analysis.WriteString(fmt.Sprintf("  Message Type: %d (Little-Endian)\n", messageType))
	analysis.WriteString(fmt.Sprintf("  Timestamp:    %02d:%02d:%02d.%03d\n", hour, minute, second, millisecond))

	// Data section
	dataLen := len(packet) - HeaderSize
	analysis.WriteString(fmt.Sprintf("\nDATA SECTION: %d bytes\n", dataLen))
	analysis.WriteString(strings.Repeat("-", 60) + "\n")

	// Show first 256 bytes of data in hex
	showLen := dataLen
	if showLen > 256 {
		showLen = 256
		analysis.WriteString("(Showing first 256 bytes)\n\n")
	} else {
		analysis.WriteString("\n")
	}

	analysis.WriteString(hex.Dump(packet[HeaderSize : HeaderSize+showLen]))

	if dataLen > 256 {
		analysis.WriteString(fmt.Sprintf("\n... (%d more bytes)\n", dataLen-256))
	}

	return analysis.String()
}

func (ps *PacketSniffer) Close() {
	if ps.conn != nil {
		ps.conn.Close()
	}
}

func main() {
	// Command-line flags
	multicastIP := flag.String("ip", "239.1.2.5", "Multicast IP address")
	port := flag.String("port", "26001", "UDP port")
	duration := flag.Duration("duration", 0, "Capture duration (e.g., 1m, 5m). 0 = until Ctrl+C")
	outputDir := flag.String("output", "./data/captures", "Output directory for captured packets")
	filterMsgType := flag.Int("msgtype", 0, "Filter specific message type (0 = capture all)")
	flag.Parse()

	fmt.Println(strings.Repeat("=", 80))
	fmt.Println("BSE PACKET SNIFFER")
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	// Create sniffer
	sniffer := NewPacketSniffer(*outputDir, uint16(*filterMsgType))

	// Start listening
	if err := sniffer.Start(*multicastIP, *port); err != nil {
		fmt.Printf("❌ Failed to start: %v\n", err)
		os.Exit(1)
	}
	defer sniffer.Close()

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start capture in goroutine
	go sniffer.CaptureLoop()

	// Wait for duration or Ctrl+C
	if *duration > 0 {
		fmt.Printf("⏱️  Will capture for %s\n", *duration)
		select {
		case <-time.After(*duration):
			fmt.Println("\n\n⏱️  Duration elapsed, stopping...")
		case <-sigChan:
			fmt.Println("\n\n⚠️  Interrupted by user, stopping...")
		}
	} else {
		<-sigChan
		fmt.Println("\n\n⚠️  Interrupted by user, stopping...")
	}

	// Print summary
	sniffer.PrintSummary()

	// Save samples
	fmt.Println()
	if err := sniffer.SaveSamples(); err != nil {
		fmt.Printf("⚠️  Failed to save samples: %v\n", err)
	}

	fmt.Println("\n✅ Capture complete!")
}
