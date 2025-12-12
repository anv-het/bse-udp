package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"net"
	"os"
	"time"
)

func main() {
	duration := flag.Duration("duration", 100*time.Second, "Capture duration")
	msgType := flag.Int("msgtype", 2012, "Message type to capture")
	flag.Parse()

	addr, err := net.ResolveUDPAddr("udp", "239.1.2.5:26001")
	if err != nil {
		fmt.Printf("Error resolving address: %v\n", err)
		os.Exit(1)
	}

	conn, err := net.ListenMulticastUDP("udp", nil, addr)
	if err != nil {
		fmt.Printf("Error listening: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	fmt.Printf("Capturing message type %d packets for %v...\n", *msgType, *duration)
	fmt.Printf("Will show FULL packet structure including all bytes\n\n")

	buffer := make([]byte, 2048)
	endTime := time.Now().Add(*duration)
	packetCount := 0

	for time.Now().Before(endTime) {
		conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			continue
		}

		if n < 10 {
			continue
		}

		// Check message type
		pktMsgType := binary.LittleEndian.Uint16(buffer[8:10])
		if pktMsgType != uint16(*msgType) {
			continue
		}

		packetCount++
		if packetCount > 3 {
			// Only show first 3 packets
			break
		}

		fmt.Printf("%s\n", string(make([]byte, 80, 80)))
		fmt.Printf("Packet #%d - Length: %d bytes\n", packetCount, n)
		fmt.Printf("%s\n\n", string(make([]byte, 80)))

		// Header
		fmt.Println("HEADER (36 bytes):")
		fmt.Printf("  Format ID:    0x%04X\n", binary.BigEndian.Uint16(buffer[0:2]))
		fmt.Printf("  Message Type: %d\n", pktMsgType)
		fmt.Printf("  Sequence:     %d\n", binary.LittleEndian.Uint32(buffer[4:8]))

		// Calculate records
		dataLen := n - 36
		numRecords := dataLen / 40
		fmt.Printf("\nDATA SECTION: %d bytes (%d records × 40 bytes)\n\n", dataLen, numRecords)

		// Print each record
		for i := 0; i < numRecords; i++ {
			offset := 36 + (i * 40)
			recordData := buffer[offset : offset+40]

			indexCode := binary.LittleEndian.Uint32(recordData[0:4])
			if indexCode == 0 {
				continue
			}

			fmt.Printf("RECORD #%d (Index Code: %d):\n", i+1, indexCode)
			fmt.Println("------------------------------------------------------------")

			// Parse all fields
			high := int32(binary.LittleEndian.Uint32(recordData[4:8]))
			prevClose := int32(binary.LittleEndian.Uint32(recordData[8:12]))
			open := int32(binary.LittleEndian.Uint32(recordData[12:16]))
			low := int32(binary.LittleEndian.Uint32(recordData[16:20]))
			ltp := int32(binary.LittleEndian.Uint32(recordData[20:24]))
			name := string(recordData[24:36])

			fmt.Printf("  Offset 00-03: %10d (Index Code)\n", indexCode)
			fmt.Printf("  Offset 04-07: %10d / %10.2f (High)\n", high, float64(high)/100.0)
			fmt.Printf("  Offset 08-11: %10d / %10.2f (Prev Close)\n", prevClose, float64(prevClose)/100.0)
			fmt.Printf("  Offset 12-15: %10d / %10.2f (Open)\n", open, float64(open)/100.0)
			fmt.Printf("  Offset 16-19: %10d / %10.2f (Low)\n", low, float64(low)/100.0)
			fmt.Printf("  Offset 20-23: %10d / %10.2f (LTP)\n", ltp, float64(ltp)/100.0)
			fmt.Printf("  Offset 24-35: %q (Name)\n", name)

			// Last 4 bytes
			reserved := binary.LittleEndian.Uint32(recordData[36:40])
			fmt.Printf("  Offset 36-39: 0x%08X = %d (Reserved/Unknown)\n", reserved, reserved)

			fmt.Println()
		}

		// Check if there are any bytes AFTER all the records
		expectedEnd := 36 + (numRecords * 40)
		if n > expectedEnd {
			extraBytes := n - expectedEnd
			fmt.Printf("⚠️  WARNING: %d EXTRA BYTES after all records!\n", extraBytes)
			fmt.Printf("Extra bytes: %X\n", buffer[expectedEnd:n])
			fmt.Println()
		}
	}

	fmt.Printf("\nCaptured %d packets\n", packetCount)
}
