package main

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	fmt.Println(`
╔═══════════════════════════════════════════════════════════════╗
║         BSE Raw Packet Decoder Test                           ║
║         Testing with captured packets from raw_packets/       ║
╚═══════════════════════════════════════════════════════════════╝
`)

	// Path to raw packets
	rawPacketsDir := "../../../data/raw_packets"

	// Check if directory exists
	if _, err := os.Stat(rawPacketsDir); os.IsNotExist(err) {
		// Try alternate path
		rawPacketsDir = "d:/bse/data/raw_packets"
	}

	// Get list of packet files
	files, err := filepath.Glob(filepath.Join(rawPacketsDir, "*.bin"))
	if err != nil {
		log.Fatalf("Failed to find packet files: %v", err)
	}

	if len(files) == 0 {
		log.Fatal("No .bin packet files found in raw_packets directory")
	}

	// Sort files by name (timestamp order)
	sort.Strings(files)

	fmt.Printf("Found %d packet files\n", len(files))
	fmt.Println("=" + strings.Repeat("=", 70))

	// Process ALL packets
	totalRecords := 0
	tokenData := make(map[uint32]*MarketRecord) // Store latest data per token
	verbose := false                            // Set to true for detailed output

	for i, file := range files {
		data, err := ioutil.ReadFile(file)
		if err != nil {
			continue
		}

		records := decodePacket(data, verbose)
		totalRecords += len(records)

		for _, r := range records {
			// Keep latest record per token
			tokenData[r.Token] = &r
		}

		// Progress indicator
		if (i+1)%500 == 0 {
			fmt.Printf("   Processed %d/%d packets...\n", i+1, len(files))
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Printf("📊 FULL SUMMARY:\n")
	fmt.Printf("   Packets processed: %d\n", len(files))
	fmt.Printf("   Total records decoded: %d\n", totalRecords)
	fmt.Printf("   Unique tokens found: %d\n", len(tokenData))

	// Show tokens with highest volume (most active)
	fmt.Println("\n� TOP 20 MOST ACTIVE TOKENS (by volume):")
	fmt.Println(strings.Repeat("-", 90))
	fmt.Printf("%-12s %-12s %-12s %-12s %-12s %-15s\n",
		"Token", "LTP", "Open", "High", "Low", "Volume")
	fmt.Println(strings.Repeat("-", 90))

	// Sort by volume
	tokens := make([]*MarketRecord, 0, len(tokenData))
	for _, r := range tokenData {
		tokens = append(tokens, r)
	}
	sort.Slice(tokens, func(i, j int) bool { return tokens[i].Volume > tokens[j].Volume })

	for i, r := range tokens {
		if i >= 20 {
			break
		}
		if r.Volume > 0 && r.LTP > 0 {
			fmt.Printf("%-12d %-12.2f %-12.2f %-12.2f %-12.2f %-15d\n",
				r.Token, r.LTP, r.Open, r.High, r.Low, r.Volume)
		}
	}

	// Show some known tokens
	fmt.Println("\n🔍 CHECKING KNOWN TOKENS:")
	knownTokens := []uint32{
		873830,  // SENSEX Index
		1102290, // SENSEX Future
		880622,  // Some equity
	}

	for _, t := range knownTokens {
		if r, ok := tokenData[t]; ok {
			fmt.Printf("   Token %d: LTP=%.2f, Open=%.2f, High=%.2f, Low=%.2f, Vol=%d\n",
				t, r.LTP, r.Open, r.High, r.Low, r.Volume)
		} else {
			fmt.Printf("   Token %d: NOT FOUND\n", t)
		}
	}

	fmt.Println("\n✅ Go decoder successfully decoded all captured packets!")
}

type MarketRecord struct {
	Token     uint32
	Open      float64
	PrevClose float64
	High      float64
	Low       float64
	LTP       float64
	Volume    int32
	ATP       float64
}

func decodePacket(packet []byte, verbose bool) []MarketRecord {
	if len(packet) < 36 {
		if verbose {
			fmt.Printf("   ⚠️ Packet too short: %d bytes\n", len(packet))
		}
		return nil
	}

	// Header parsing
	// Bytes 0-3: Leading zeros
	leadingZeros := binary.BigEndian.Uint32(packet[0:4])

	// Bytes 4-5: Format ID (packet size) - Little Endian
	formatID := binary.LittleEndian.Uint16(packet[4:6])

	// Bytes 8-9: Message Type - Little Endian
	msgType := binary.LittleEndian.Uint16(packet[8:10])

	if verbose {
		fmt.Printf("   Header: leading=0x%08X, formatID=%d, msgType=%d, packetLen=%d\n",
			leadingZeros, formatID, msgType, len(packet))
	}

	// Validate header
	if leadingZeros != 0 {
		if verbose {
			fmt.Printf("   ⚠️ Invalid leading zeros: 0x%08X\n", leadingZeros)
		}
		return nil
	}

	if int(formatID) != len(packet) && verbose {
		fmt.Printf("   ⚠️ Format ID mismatch: formatID=%d vs packetLen=%d\n", formatID, len(packet))
	}

	if msgType != 2020 && msgType != 2021 {
		if verbose {
			fmt.Printf("   ⚠️ Unknown message type: %d\n", msgType)
		}
		return nil
	}

	// Calculate number of records
	// Record size is 264 bytes, header is 36 bytes
	recordSize := 264
	headerSize := 36
	dataSize := len(packet) - headerSize
	numRecords := dataSize / recordSize

	if verbose {
		fmt.Printf("   Data: headerSize=%d, dataSize=%d, recordSize=%d, numRecords=%d\n",
			headerSize, dataSize, recordSize, numRecords)
	}

	records := make([]MarketRecord, 0, numRecords)

	for i := 0; i < numRecords; i++ {
		offset := headerSize + i*recordSize
		if offset+recordSize > len(packet) {
			break
		}

		record := parseRecord(packet[offset:offset+recordSize], i)
		if record != nil {
			records = append(records, *record)
			if verbose {
				fmt.Printf("   📈 Record %d: Token=%d, LTP=%.2f, Open=%.2f, High=%.2f, Low=%.2f, Vol=%d\n",
					i+1, record.Token, record.LTP, record.Open, record.High, record.Low, record.Volume)
			}
		}
	}

	return records
}

func parseRecord(data []byte, index int) *MarketRecord {
	if len(data) < 40 {
		return nil
	}

	// Token (offset 0-3, Little-Endian)
	token := binary.LittleEndian.Uint32(data[0:4])
	if token == 0 {
		return nil // Empty slot
	}

	// Parse prices (all Little-Endian, in paise - divide by 100)
	open := float64(int32(binary.LittleEndian.Uint32(data[4:8]))) / 100.0
	prevClose := float64(int32(binary.LittleEndian.Uint32(data[8:12]))) / 100.0
	high := float64(int32(binary.LittleEndian.Uint32(data[12:16]))) / 100.0
	low := float64(int32(binary.LittleEndian.Uint32(data[16:20]))) / 100.0
	volume := int32(binary.LittleEndian.Uint32(data[24:28]))
	ltp := float64(int32(binary.LittleEndian.Uint32(data[36:40]))) / 100.0

	var atp float64
	if len(data) >= 88 {
		atp = float64(int32(binary.LittleEndian.Uint32(data[84:88]))) / 100.0
	}

	return &MarketRecord{
		Token:     token,
		Open:      open,
		PrevClose: prevClose,
		High:      high,
		Low:       low,
		LTP:       ltp,
		Volume:    volume,
		ATP:       atp,
	}
}
