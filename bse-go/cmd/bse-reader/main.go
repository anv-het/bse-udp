package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bse-go/pkg/config"
	"bse-go/pkg/connection"
	"bse-go/pkg/data_collector"
	"bse-go/pkg/decoder"
	"bse-go/pkg/decompressor"
	"bse-go/pkg/saver"
)

func main() {
	// Setup logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	log.Println("🚀 BSE UDP Market Data Reader - Go HFT Version")
	log.Println("============================================")

	// Load configuration
	cfg, err := config.LoadConfig("config/config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Load token map
	tokenMap, err := data_collector.LoadTokenMap("data/tokens/token_details.json")
	if err != nil {
		log.Printf("Warning: Failed to load token map: %v", err)
		log.Println("Continuing without token map (symbols will be 'UNKNOWN')")
		tokenMap = make(map[string]map[string]interface{})
	}

	// Create channels for data flow
	rawPackets := make(chan []byte, 100)
	decodedPackets := make(chan decoder.DecodedPacket, 100)
	decompressedRecords := make(chan decompressor.DecompressedRecord, 100)
	quotes := make(chan data_collector.Quote, 100)

	// Create components
	conn := connection.NewBSEConnection(cfg.Multicast.IP, cfg.Multicast.Port, cfg.BufferSize)
	if err := conn.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	dec := decoder.NewPacketDecoder()
	decomp := decompressor.NewNFCASTDecompressor()
	collector := data_collector.NewMarketDataCollector(tokenMap)
	saver := saver.NewDataSaver("data")

	// Start goroutines
	go conn.ReceiveLoop(rawPackets)
	go processPackets(dec, rawPackets, decodedPackets)
	go decompressRecords(decomp, decodedPackets, decompressedRecords)
	go collectQuotes(collector, decompressedRecords, quotes)
	go saveQuotes(saver, quotes)

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("✅ All components started. Press Ctrl+C to stop.")

	// Wait for shutdown signal
	<-sigChan
	log.Println("\n⚠ Shutdown signal received")

	// Signal connection to stop receiving
	conn.Stop()
	time.Sleep(200 * time.Millisecond) // Allow goroutines to finish

	log.Println("👋 BSE Go Reader shutdown complete")
}

func processPackets(dec *decoder.PacketDecoder, rawPackets <-chan []byte, decodedPackets chan<- decoder.DecodedPacket) {
	for packet := range rawPackets {
		decoded, err := dec.DecodePacket(packet)
		if err != nil {
			log.Printf("Decode error: %v", err)
			continue
		}
		if decoded != nil {
			decodedPackets <- *decoded
		}
	}
	close(decodedPackets)
}

func decompressRecords(decomp *decompressor.NFCASTDecompressor, decodedPackets <-chan decoder.DecodedPacket, decompressedRecords chan<- decompressor.DecompressedRecord) {
	for packet := range decodedPackets {
		for _, record := range packet.Records {
			decompressed, err := decomp.DecompressRecord(packet.Header, record)
			if err != nil {
				log.Printf("Decompress error: %v", err)
				continue
			}
			if decompressed != nil {
				decompressedRecords <- *decompressed
			}
		}
	}
	close(decompressedRecords)
}

func collectQuotes(collector *data_collector.MarketDataCollector, decompressedRecords <-chan decompressor.DecompressedRecord, quotes chan<- data_collector.Quote) {
	for record := range decompressedRecords {
		quote, err := collector.CollectQuote(record)
		if err != nil {
			log.Printf("Collect error: %v", err)
			continue
		}
		if quote != nil {
			quotes <- *quote
		}
	}
	close(quotes)
}

func saveQuotes(saver *saver.DataSaver, quotes <-chan data_collector.Quote) {
	var batch []data_collector.Quote
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case quote, ok := <-quotes:
			if !ok {
				// Channel closed, save remaining batch
				if len(batch) > 0 {
					saver.SaveQuotes(batch)
				}
				return
			}
			batch = append(batch, quote)

			// Save in batches of 10 or every second
			if len(batch) >= 10 {
				saver.SaveQuotes(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				saver.SaveQuotes(batch)
				batch = batch[:0]
			}
		}
	}
}
