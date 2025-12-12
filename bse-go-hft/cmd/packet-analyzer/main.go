// BSE NFCAST Complete Packet Analyzer for Message Types 2020 & 2021
// This tool decodes and displays ALL fields available in equity and derivatives packets
package main

import (
	"context"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"bse-go-hft/internal/decoder"
	"bse-go-hft/internal/receiver"
	"bse-go-hft/pkg/domain"
)

var (
	maxPackets = flag.Int("max", 10, "Maximum number of packets to capture (per message type)")
	timeout    = flag.Duration("timeout", 2*time.Minute, "Capture timeout")
	msgType    = flag.Int("type", 0, "Specific message type to capture (0=both, 2020=EQ, 2021=FO)")
	showHex    = flag.Bool("hex", false, "Show hex dump of packets")
	tokensDir  = flag.String("tokens", "data/tokens", "Directory containing BhavCopy token files")
)

func main() {
	flag.Parse()

	fmt.Println("╔════════════════════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║         BSE NFCAST COMPLETE PACKET ANALYZER - Message Types 2020 & 2021                       ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("This tool shows ALL fields available in BSE packets:")
	fmt.Println("  • Header information (timestamp, message type)")
	fmt.Println("  • Token information (with symbol lookup)")
	fmt.Println("  • Price fields (Open, High, Low, Close, LTP, ATP)")
	fmt.Println("  • Volume and turnover")
	fmt.Println("  • Order book (5 levels of bids and asks)")
	fmt.Println("  • Sequence numbers and metadata")
	fmt.Println()
	fmt.Println("════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("Multicast Address:   239.1.2.5:26001\n")
	fmt.Printf("Max Packets:         %d per message type\n", *maxPackets)
	fmt.Printf("Timeout:             %v\n", *timeout)
	if *msgType == 2020 {
		fmt.Printf("Filter:              Equity (2020) only\n")
	} else if *msgType == 2021 {
		fmt.Printf("Filter:              Derivatives (2021) only\n")
	} else {
		fmt.Printf("Filter:              Both Equity (2020) and Derivatives (2021)\n")
	}
	fmt.Printf("Tokens Directory:    %s\n", *tokensDir)
	fmt.Println("════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Load token map for symbol lookup
	fmt.Println("🔍 Loading token database for symbol lookup...")
	tokenMap := domain.NewTokenMap()

	// Try to load tokens from files
	if err := loadTokensFromDir(*tokensDir, tokenMap); err != nil {
		fmt.Printf("⚠ Warning: Could not load token database: %v\n", err)
		fmt.Println("   Continuing without symbol lookup...\n")
		tokenMap = nil
	} else {
		totalCount := tokenMap.Len()
		fmt.Printf("✓ Token database loaded: %d tokens total\n\n", totalCount)
	}

	// Initialize decoder
	dec := decoder.NewDecoder()

	// Packet channel
	type packet struct {
		data   []byte
		length int
		time   time.Time
	}
	packetChan := make(chan packet, 100)

	// Create packet handler
	handler := func(data []byte, length int, receiveTime time.Time) {
		dataCopy := make([]byte, length)
		copy(dataCopy, data[:length])

		select {
		case packetChan <- packet{dataCopy, length, receiveTime}:
		default:
		}
	}

	// Initialize receiver
	recvConfig := receiver.DefaultConfig()
	recv := receiver.NewMulticastReceiver(recvConfig, handler)

	if err := recv.Connect(); err != nil {
		log.Fatalf("❌ Failed to connect: %v", err)
	}
	defer recv.Close()

	// Start receive loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := recv.ReceiveLoop(ctx); err != nil && err != context.Canceled {
			log.Printf("⚠ Receive error: %v", err)
		}
	}()

	fmt.Println("✓ Connected to BSE NFCAST feed")
	fmt.Println("✓ Listening for packets...")
	fmt.Println()

	// Tracking
	eqPacketsAnalyzed := 0
	foPacketsAnalyzed := 0
	totalPackets := 0

	// Signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Timeout
	timeoutChan := time.After(*timeout)

	// Main loop
	running := true
	for running {
		select {
		case <-sigChan:
			fmt.Println("\n⚠ Interrupt received, stopping...")
			running = false

		case <-timeoutChan:
			fmt.Println("\n⚠ Timeout reached, stopping...")
			running = false

		case pkt := <-packetChan:
			totalPackets++

			if len(pkt.data) < 10 {
				continue
			}

			msgTypeVal := binary.LittleEndian.Uint16(pkt.data[8:10])

			// Filter by message type if specified
			if *msgType != 0 && msgTypeVal != uint16(*msgType) {
				continue
			}

			// Check if we should analyze this packet
			shouldAnalyze := false
			if msgTypeVal == 2020 && eqPacketsAnalyzed < *maxPackets {
				shouldAnalyze = true
				eqPacketsAnalyzed++
			} else if msgTypeVal == 2021 && foPacketsAnalyzed < *maxPackets {
				shouldAnalyze = true
				foPacketsAnalyzed++
			}

			if !shouldAnalyze {
				// Check if we're done
				if eqPacketsAnalyzed >= *maxPackets && foPacketsAnalyzed >= *maxPackets {
					running = false
				}
				continue
			}

			// Analyze this packet
			fmt.Println("╔════════════════════════════════════════════════════════════════════════════════════════════════╗")
			if msgTypeVal == 2020 {
				fmt.Printf("║  EQUITY PACKET #%d (Message Type 2020)                                                       ║\n", eqPacketsAnalyzed)
			} else if msgTypeVal == 2021 {
				fmt.Printf("║  DERIVATIVES PACKET #%d (Message Type 2021)                                                  ║\n", foPacketsAnalyzed)
			}
			fmt.Println("╚════════════════════════════════════════════════════════════════════════════════════════════════╝")

			analyzePacketComplete(pkt.data, pkt.length, pkt.time, dec, tokenMap, *showHex)

			fmt.Println()
		}
	}

	// Summary
	fmt.Println()
	fmt.Println("════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("CAPTURE SUMMARY")
	fmt.Println("════════════════════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("Total Packets:       %d\n", totalPackets)
	fmt.Printf("Equity Analyzed:     %d (Message Type 2020)\n", eqPacketsAnalyzed)
	fmt.Printf("Derivatives Analyzed: %d (Message Type 2021)\n", foPacketsAnalyzed)
	fmt.Println("════════════════════════════════════════════════════════════════════════════════════════════════")
}

func loadTokensFromDir(dir string, tokenMap *domain.TokenMap) error {
	// Try to load latest BhavCopy and ContractMaster files
	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("cannot read directory: %w", err)
	}

	bhavCopyLoaded := false
	contractLoaded := false

	// Find latest files
	for _, file := range files {
		name := file.Name()
		if strings.HasPrefix(name, "BhavCopy_BSE_CM_") && strings.HasSuffix(name, ".csv") {
			path := filepath.Join(dir, name)
			if err := parseBhavCopySimple(path, tokenMap); err == nil {
				bhavCopyLoaded = true
			}
		} else if strings.HasPrefix(name, "BSE_EQD_CONTRACT_") && strings.HasSuffix(name, ".csv") {
			path := filepath.Join(dir, name)
			if err := parseContractMasterSimple(path, tokenMap); err == nil {
				contractLoaded = true
			}
		}
	}

	if !bhavCopyLoaded && !contractLoaded {
		return fmt.Errorf("no token files found")
	}

	return nil
}

func parseBhavCopySimple(path string, tokenMap *domain.TokenMap) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Read() // Skip header

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		if len(record) < 15 {
			continue
		}

		tokenStr := strings.TrimSpace(record[0])
		token, err := strconv.ParseUint(tokenStr, 10, 32)
		if err != nil {
			continue
		}

		tokenMap.Set(uint32(token), &domain.Contract{
			Token:      uint32(token),
			Symbol:     strings.TrimSpace(record[1]),
			SymbolName: strings.TrimSpace(record[2]),
			Source:     "BhavCopy",
		})
	}

	return nil
}

func parseContractMasterSimple(path string, tokenMap *domain.TokenMap) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Read() // Skip header

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		if len(record) < 20 {
			continue
		}

		tokenStr := strings.TrimSpace(record[0])
		token, err := strconv.ParseUint(tokenStr, 10, 32)
		if err != nil {
			continue
		}

		contract := &domain.Contract{
			Token:      uint32(token),
			Symbol:     strings.TrimSpace(record[2]),
			SymbolName: strings.TrimSpace(record[3]),
			Expiry:     strings.TrimSpace(record[7]),
			OptionType: strings.TrimSpace(record[9]),
			Source:     "ContractMaster",
		}

		// Parse strike price
		if strikeStr := strings.TrimSpace(record[8]); strikeStr != "" {
			if strike, err := strconv.ParseFloat(strikeStr, 64); err == nil {
				contract.StrikePrice = strike
			}
		}

		tokenMap.Set(uint32(token), contract)
	}

	return nil
}

func analyzePacketComplete(data []byte, length int, receiveTime time.Time, dec *decoder.Decoder, tokenMap *domain.TokenMap, showHex bool) {
	// Show packet header info
	fmt.Println()
	fmt.Println("┌─ PACKET HEADER (36 bytes) ─────────────────────────────────────────────────────────────────────┐")

	if length >= 36 {
		formatID := binary.BigEndian.Uint16(data[0:2])
		msgType := binary.LittleEndian.Uint16(data[8:10])
		hour := binary.LittleEndian.Uint16(data[10:12])
		minute := binary.LittleEndian.Uint16(data[12:14])
		second := binary.LittleEndian.Uint16(data[14:16])
		millisecond := binary.LittleEndian.Uint16(data[16:18])

		fmt.Printf("│ Received Time:   %s\n", receiveTime.Format("2006-01-02 15:04:05.000"))
		fmt.Printf("│ Format ID:       0x%04X (%d) [Big-Endian]\n", formatID, formatID)
		fmt.Printf("│ Message Type:    %d (%s) [Little-Endian]\n", msgType, getMessageTypeName(msgType))
		fmt.Printf("│ Packet Time:     %02d:%02d:%02d.%03d [Little-Endian]\n", hour, minute, second, millisecond)
		fmt.Printf("│ Packet Length:   %d bytes\n", length)

		dataLen := length - 36
		numRecords := dataLen / 264
		fmt.Printf("│ Data Length:     %d bytes\n", dataLen)
		fmt.Printf("│ Num Records:     %d (264 bytes each)\n", numRecords)
	}

	fmt.Println("└────────────────────────────────────────────────────────────────────────────────────────────────┘")

	// Decode records
	records, count, err := dec.Decode(data, length)
	if err != nil {
		fmt.Printf("❌ Decode error: %v\n", err)
		return
	}

	if count == 0 {
		fmt.Println("\n⚠ No valid records in this packet (all tokens were 0 or 1)")
		return
	}

	// Analyze each record
	for i, record := range records {
		fmt.Println()
		fmt.Printf("┌─ RECORD #%d ────────────────────────────────────────────────────────────────────────────────────┐\n", i+1)

		// Token and symbol
		fmt.Println("│")
		fmt.Printf("│ ╔═ TOKEN INFORMATION ═══════════════════════════════════════════════════════════════════╗\n")
		fmt.Printf("│ ║ Token ID:        %d (0x%08X)\n", record.Token, record.Token)

		if tokenMap != nil {
			contract, found := tokenMap.Get(record.Token)

			if found && contract != nil {
				fmt.Printf("│ ║ Symbol:          %s\n", contract.Symbol)
				fmt.Printf("│ ║ Company Name:    %s\n", contract.SymbolName)
				fmt.Printf("│ ║ Source:          %s\n", contract.Source)

				// Check if it's a derivative (has expiry or strike)
				if contract.Expiry != "" || contract.StrikePrice > 0 || contract.OptionType != "" {
					fmt.Printf("│ ║ Segment:         FO (Derivatives)\n")
					if contract.Expiry != "" {
						fmt.Printf("│ ║ Expiry:          %s\n", contract.Expiry)
					}
					if contract.StrikePrice > 0 {
						fmt.Printf("│ ║ Strike Price:    ₹%.2f\n", contract.StrikePrice)
					}
					if contract.OptionType != "" {
						fmt.Printf("│ ║ Option Type:     %s\n", contract.OptionType)
					}
				} else {
					fmt.Printf("│ ║ Segment:         EQ (Equity)\n")
				}
			} else {
				fmt.Printf("│ ║ Symbol:          ⚠ NOT FOUND IN DATABASE\n")
				fmt.Printf("│ ║ Note:            Update BhavCopy files in data/tokens/\n")
			}
		} else {
			fmt.Printf("│ ║ Symbol:          ⚠ Token database not loaded\n")
		}
		fmt.Printf("│ ╚═══════════════════════════════════════════════════════════════════════════════════════╝\n")

		// Price fields
		fmt.Println("│")
		fmt.Printf("│ ╔═ PRICE FIELDS ════════════════════════════════════════════════════════════════════════╗\n")
		fmt.Printf("│ ║ LTP (Last Traded Price): ₹%.2f\n", record.LTP)
		fmt.Printf("│ ║ Open Price:              ₹%.2f\n", record.Open)
		fmt.Printf("│ ║ High Price:              ₹%.2f\n", record.High)
		fmt.Printf("│ ║ Low Price:               ₹%.2f\n", record.Low)
		fmt.Printf("│ ║ Previous Close:          ₹%.2f\n", record.PrevClose)
		fmt.Printf("│ ║ ATP (Avg Traded Price):  ₹%.2f\n", record.ATP)

		// Calculate change
		if record.PrevClose > 0 {
			change := record.LTP - record.PrevClose
			changePct := (change / record.PrevClose) * 100
			fmt.Printf("│ ║ Change:                  ₹%.2f (%+.2f%%)\n", change, changePct)
		}
		fmt.Printf("│ ╚═══════════════════════════════════════════════════════════════════════════════════════╝\n")

		// Volume and turnover
		fmt.Println("│")
		fmt.Printf("│ ╔═ VOLUME & TURNOVER ═══════════════════════════════════════════════════════════════════╗\n")
		fmt.Printf("│ ║ Volume:          %d shares/contracts\n", record.Volume)
		fmt.Printf("│ ║ Turnover:        ₹%.2f Lakhs (₹%.2f Crores)\n", record.Turnover, record.Turnover/100)
		fmt.Printf("│ ║ Lot Size:        %d\n", record.LotSize)
		fmt.Printf("│ ╚═══════════════════════════════════════════════════════════════════════════════════════╝\n")

		// Order book
		fmt.Println("│")
		fmt.Printf("│ ╔═ ORDER BOOK (5 LEVELS) ═══════════════════════════════════════════════════════════════╗\n")
		fmt.Printf("│ ║\n")
		fmt.Printf("│ ║   Level │      BID PRICE │  BID QTY │      ASK PRICE │  ASK QTY │   SPREAD\n")
		fmt.Printf("│ ║   ──────┼────────────────┼──────────┼────────────────┼──────────┼──────────\n")

		for level := 0; level < 5; level++ {
			bidPrice := record.BidPrices[level]
			bidQty := record.BidQtys[level]
			askPrice := record.AskPrices[level]
			askQty := record.AskQtys[level]

			spread := ""
			if bidPrice > 0 && askPrice > 0 {
				spreadVal := askPrice - bidPrice
				spread = fmt.Sprintf("₹%.2f", spreadVal)
			}

			fmt.Printf("│ ║   %5d │ ₹%12.2f │ %8d │ ₹%12.2f │ %8d │ %8s\n",
				level+1, bidPrice, bidQty, askPrice, askQty, spread)
		}

		fmt.Printf("│ ║\n")

		// Calculate total bid/ask quantities
		totalBidQty := int64(0)
		totalAskQty := int64(0)
		for level := 0; level < 5; level++ {
			totalBidQty += record.BidQtys[level]
			totalAskQty += record.AskQtys[level]
		}

		fmt.Printf("│ ║   TOTAL BID QUANTITY:  %d\n", totalBidQty)
		fmt.Printf("│ ║   TOTAL ASK QUANTITY:  %d\n", totalAskQty)

		if totalBidQty > 0 || totalAskQty > 0 {
			imbalance := float64(totalBidQty-totalAskQty) / float64(totalBidQty+totalAskQty) * 100
			fmt.Printf("│ ║   ORDER IMBALANCE:     %.2f%% ", imbalance)
			if imbalance > 10 {
				fmt.Printf("(BUY PRESSURE)\n")
			} else if imbalance < -10 {
				fmt.Printf("(SELL PRESSURE)\n")
			} else {
				fmt.Printf("(BALANCED)\n")
			}
		}

		fmt.Printf("│ ╚═══════════════════════════════════════════════════════════════════════════════════════╝\n")

		// Metadata
		fmt.Println("│")
		fmt.Printf("│ ╔═ METADATA ════════════════════════════════════════════════════════════════════════════╗\n")
		fmt.Printf("│ ║ Sequence Number: %d\n", record.SequenceNum)
		fmt.Printf("│ ║ Timestamp:       %s\n", record.Timestamp.Format("2006-01-02 15:04:05.000"))
		fmt.Printf("│ ╚═══════════════════════════════════════════════════════════════════════════════════════╝\n")

		fmt.Println("└────────────────────────────────────────────────────────────────────────────────────────────────┘")
	}

	// Show hex dump if requested
	if showHex {
		fmt.Println()
		fmt.Println("┌─ HEX DUMP ─────────────────────────────────────────────────────────────────────────────────────┐")
		fmt.Println(hex.Dump(data[:length]))
		fmt.Println("└────────────────────────────────────────────────────────────────────────────────────────────────┘")
	}
}

func getMessageTypeName(msgType uint16) string {
	switch msgType {
	case 2020:
		return "Equity (Cash Market)"
	case 2021:
		return "Derivatives (F&O)"
	default:
		return "Unknown"
	}
}
