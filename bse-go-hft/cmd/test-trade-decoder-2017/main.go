// BSE NFCAST Message Type 2017 Packet Analyzer
// Captures and analyzes trade data packets in detail
package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bse-go-hft/internal/decoder"
	"bse-go-hft/internal/receiver"
)

var (
	maxPackets = flag.Int("max", 5, "Maximum number of trade packets to capture")
	timeout    = flag.Duration("timeout", 2*time.Minute, "Capture timeout")
)

func main() {
	flag.Parse()

	fmt.Println("BSE NFCAST Message Type 2017 Packet Analyzer")
	fmt.Println("====================================================================================================")
	fmt.Println("Capturing and analyzing trade data packets...")
	fmt.Println("====================================================================================================")

	fmt.Printf("Multicast Address:   %s:%d\n", "239.1.2.5", 26001)
	fmt.Printf("Max Packets:         %d\n", *maxPackets)
	fmt.Printf("Timeout:             %v\n", *timeout)
	fmt.Println("====================================================================================================")
	fmt.Println()

	// Initialize decoder
	dec := decoder.NewDecoder()

	// Initialize multicast receiver
	recvConfig := receiver.DefaultConfig()

	packetCount := 0
	var packetChan chan struct {
		data   []byte
		length int
		time   time.Time
	}
	packetChan = make(chan struct {
		data   []byte
		length int
		time   time.Time
	}, 100)

	handler := func(data []byte, length int, receiveTime time.Time) {
		// Copy data since it's reused
		dataCopy := make([]byte, length)
		copy(dataCopy, data[:length])

		select {
		case packetChan <- struct {
			data   []byte
			length int
			time   time.Time
		}{dataCopy, length, receiveTime}:
		default:
			// Channel full, skip packet
		}
	}

	recv := receiver.NewMulticastReceiver(recvConfig, handler)
	recv := receiver.NewMulticastReceiver(recvConfig, handler)
	if err := recv.Start(context.Background()); err != nil {
		log.Fatalf("❌ Failed to start receiver: %v", err)
	}
	defer recv.Stop()

	fmt.Println("✓ Connected to BSE NFCAST feed")
	fmt.Println("✓ Listening for Message Type 2017 packets...")
	fmt.Println()

	// Signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Timeout
	timeoutChan := time.After(*timeout)

	// Tracking
	tradePacketsFound := 0
	totalPackets := 0
	startTime := time.Now()

	// Start receiving
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

			// Check message type
			if len(pkt.data) < 10 {
				continue
			}

			msgType := uint16(pkt.data[8]) | uint16(pkt.data[9])<<8

			// Only process Message Type 2017
			if msgType != 2017 {
				continue
			}

			tradePacketsFound++

			fmt.Printf("════════════════════════════════════════════════════════════════════════════════════════════════\n")
			fmt.Printf("TRADE PACKET #%d (Received at %s)\n", tradePacketsFound, pkt.time.Format("15:04:05.000"))
			fmt.Printf("════════════════════════════════════════════════════════════════════════════════════════════════\n")

			// Display raw packet
			analyzeRawPacket(pkt.data, pkt.length)

			// Decode packet
			fmt.Println()
			fmt.Println("DECODED TRADE DATA:")
			fmt.Println("--------------------------------------------------------------------------------------------")

			trades, err := dec.DecodeMsgType2017(pkt.data, pkt.length)
			if err != nil {
				fmt.Printf("❌ Decode error: %v\n", err)
			} else {
				for i, trade := range trades {
					fmt.Printf("\nTrade #%d:\n", i+1)
					fmt.Printf("  Token:            %d\n", trade.Token)
					fmt.Printf("  Trade Number:     %d\n", trade.TradeNumber)
					fmt.Printf("  Price:            ₹%.2f\n", trade.TradePrice)
					fmt.Printf("  Quantity:         %d\n", trade.TradeQuantity)
					fmt.Printf("  Trade Time:       %s\n", trade.TradeTime.Format("15:04:05.000"))
					fmt.Printf("  Buy Order:        %d\n", trade.BuyOrderNumber)
					fmt.Printf("  Sell Order:       %d\n", trade.SellOrderNumber)
					fmt.Printf("  Trade Type:       %s\n", trade.TradeType)
					fmt.Printf("  Trading Session:  %s\n", trade.TradingSession)
					fmt.Printf("  Sequence:         %d\n", trade.SequenceNumber)
				}
			}

			fmt.Printf("\n════════════════════════════════════════════════════════════════════════════════════════════════\n\n")

			// Check if we've reached max packets
			if tradePacketsFound >= *maxPackets {
				fmt.Printf("✓ Captured %d trade packets, stopping...\n", *maxPackets)
				running = false
			}
		}
	}

	// Summary
	elapsed := time.Since(startTime)
	fmt.Println()
	fmt.Println("====================================================================================================")
	fmt.Println("CAPTURE SUMMARY")
	fmt.Println("====================================================================================================")
	fmt.Printf("Duration:           %v\n", elapsed.Round(time.Millisecond))
	fmt.Printf("Total Packets:      %d\n", totalPackets)
	fmt.Printf("Trade Packets:      %d (Message Type 2017)\n", tradePacketsFound)
	fmt.Printf("Packet Rate:        %.2f pkt/s\n", float64(totalPackets)/elapsed.Seconds())
	fmt.Println("====================================================================================================")

	if tradePacketsFound == 0 {
		fmt.Println()
		fmt.Println("⚠ No Message Type 2017 packets captured!")
		fmt.Println("   - Trades only occur during active market hours (9:15 AM - 3:30 PM IST)")
		fmt.Println("   - Try running during trading hours for better results")
		fmt.Println("   - Low-volume stocks may have very few trades")
	}
}

func analyzeRawPacket(data []byte, length int) {
	fmt.Println()
	fmt.Println("RAW PACKET STRUCTURE:")
	fmt.Println("--------------------------------------------------------------------------------------------")
	fmt.Printf("Packet Length: %d bytes\n", length)
	fmt.Println()

	// Header analysis (36 bytes)
	fmt.Println("HEADER (36 bytes):")
	if length >= 36 {
		formatID := binary.BigEndian.Uint16(data[0:2])
		msgType := binary.LittleEndian.Uint16(data[8:10])
		hour := binary.LittleEndian.Uint16(data[10:12])
		minute := binary.LittleEndian.Uint16(data[12:14])
		second := binary.LittleEndian.Uint16(data[14:16])
		millisecond := binary.LittleEndian.Uint16(data[16:18])

		fmt.Printf("  [0-1]   Format ID:      0x%04X (%d) - Big-Endian\n", formatID, formatID)
		fmt.Printf("  [8-9]   Message Type:   %d - Little-Endian\n", msgType)
		fmt.Printf("  [10-17] Timestamp:      %02d:%02d:%02d.%03d\n", hour, minute, second, millisecond)

		fmt.Println()
		fmt.Printf("  Hex dump (first 36 bytes):\n")
		fmt.Printf("  %s\n", hex.Dump(data[:36]))
	}

	// Trade record analysis (96 bytes)
	if length >= 132 {
		fmt.Println("TRADE RECORD (96 bytes starting at offset 36):")
		record := data[36:]

		token := binary.LittleEndian.Uint32(record[0:4])
		tradeNum := binary.LittleEndian.Uint32(record[4:8])
		priceRaw := int32(binary.LittleEndian.Uint32(record[8:12]))
		price := float64(priceRaw) / 100.0
		qty := binary.LittleEndian.Uint32(record[12:16])

		fmt.Printf("  [36-39]  Token:          %d (0x%08X)\n", token, token)
		fmt.Printf("  [40-43]  Trade Number:   %d\n", tradeNum)
		fmt.Printf("  [44-47]  Price:          %d paise = ₹%.2f\n", priceRaw, price)
		fmt.Printf("  [48-51]  Quantity:       %d\n", qty)

		tradeHour := binary.LittleEndian.Uint16(record[16:18])
		tradeMin := binary.LittleEndian.Uint16(record[18:20])
		tradeSec := binary.LittleEndian.Uint16(record[20:22])
		tradeMS := binary.LittleEndian.Uint16(record[22:24])
		fmt.Printf("  [52-59]  Trade Time:     %02d:%02d:%02d.%03d\n", tradeHour, tradeMin, tradeSec, tradeMS)

		buyOrder := binary.LittleEndian.Uint64(record[24:32])
		sellOrder := binary.LittleEndian.Uint64(record[32:40])
		fmt.Printf("  [60-67]  Buy Order:      %d\n", buyOrder)
		fmt.Printf("  [68-75]  Sell Order:     %d\n", sellOrder)

		tradeType := binary.LittleEndian.Uint16(record[40:42])
		session := binary.LittleEndian.Uint16(record[42:44])
		sequence := binary.LittleEndian.Uint32(record[44:48])
		fmt.Printf("  [76-77]  Trade Type:     %d\n", tradeType)
		fmt.Printf("  [78-79]  Session:        %d\n", session)
		fmt.Printf("  [80-83]  Sequence:       %d\n", sequence)

		fmt.Println()
		fmt.Printf("  Hex dump (trade record 96 bytes):\n")
		if len(record) >= 96 {
			fmt.Printf("  %s\n", hex.Dump(record[:96]))
		} else {
			fmt.Printf("  %s\n", hex.Dump(record))
		}
	}

	// Full packet hex dump
	fmt.Println()
	fmt.Printf("FULL PACKET HEX DUMP (%d bytes):\n", length)
	fmt.Printf("%s\n", hex.Dump(data[:length]))
}
