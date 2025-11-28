package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"bse-go/pkg/config"
	"bse-go/pkg/connection"
	"bse-go/pkg/data_collector"
	"bse-go/pkg/decoder"
	"bse-go/pkg/decompressor"
	"bse-go/pkg/saver"
)

// SegmentPipeline holds all components for one segment (CM or FO)
type SegmentPipeline struct {
	Segment             string
	Connection          *connection.BSEConnection
	Decoder             *decoder.PacketDecoder
	Decompressor        *decompressor.NFCASTDecompressor
	Collector           *data_collector.MarketDataCollector
	Saver               *saver.DataSaver
	RawPackets          chan []byte
	DecodedPackets      chan decoder.DecodedPacket
	DecompressedRecords chan decompressor.DecompressedRecord
	Quotes              chan data_collector.Quote
	stopChan            chan struct{}
}

func main() {
	// Setup logging
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	log.Println("🚀 BSE UDP Market Data Reader - Go HFT Dual-Feed Version")
	log.Println("=========================================================")

	// Load configuration
	cfg, err := config.LoadConfig("config/config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create pipelines for enabled segments
	var pipelines []*SegmentPipeline
	var wg sync.WaitGroup

	// CM Segment Pipeline
	if cfg.Segments.CMEnabled {
		cmPipeline := createPipeline("CM", cfg.MulticastCM.IP, cfg.MulticastCM.Port, cfg.BufferSize, "data")
		if cmPipeline != nil {
			pipelines = append(pipelines, cmPipeline)
			log.Printf("📡 CM Pipeline created: %s:%d", cfg.MulticastCM.IP, cfg.MulticastCM.Port)
		}
	}

	// FO Segment Pipeline
	if cfg.Segments.FOEnabled {
		foPipeline := createPipeline("FO", cfg.MulticastFO.IP, cfg.MulticastFO.Port, cfg.BufferSize, "data")
		if foPipeline != nil {
			pipelines = append(pipelines, foPipeline)
			log.Printf("📡 FO Pipeline created: %s:%d", cfg.MulticastFO.IP, cfg.MulticastFO.Port)
		}
	}

	if len(pipelines) == 0 {
		log.Fatal("❌ No segments enabled or all connections failed")
	}

	// Start all pipeline goroutines
	for _, pipeline := range pipelines {
		wg.Add(5)
		go runReceiver(pipeline, &wg)
		go runDecoder(pipeline, &wg)
		go runDecompressor(pipeline, &wg)
		go runCollector(pipeline, &wg)
		go runSaver(pipeline, &wg)
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("=========================================================")
	log.Printf("✅ Started %d segment pipeline(s). Press Ctrl+C to stop.", len(pipelines))
	log.Println("=========================================================")

	// Wait for shutdown signal
	<-sigChan
	log.Println("\n⚠️ Shutdown signal received, stopping all pipelines...")

	// Stop all connections first
	for _, pipeline := range pipelines {
		pipeline.Connection.Stop()
		close(pipeline.stopChan)
	}

	// Give goroutines time to finish processing
	time.Sleep(500 * time.Millisecond)

	// Wait for all goroutines to complete
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("✅ All goroutines completed cleanly")
	case <-time.After(2 * time.Second):
		log.Println("⚠️ Timeout waiting for goroutines, forcing exit")
	}

	// Close connections
	for _, pipeline := range pipelines {
		pipeline.Connection.Close()
	}

	log.Println("👋 BSE Go HFT Reader shutdown complete")
}

// createPipeline creates a complete pipeline for one segment
func createPipeline(segment, ip string, port, bufferSize int, dataDir string) *SegmentPipeline {
	conn := connection.NewBSEConnection(ip, port, bufferSize, segment)
	if err := conn.Connect(); err != nil {
		log.Printf("❌ Failed to connect %s segment: %v", segment, err)
		return nil
	}

	return &SegmentPipeline{
		Segment:             segment,
		Connection:          conn,
		Decoder:             decoder.NewPacketDecoder(segment),
		Decompressor:        decompressor.NewNFCASTDecompressor(segment),
		Collector:           data_collector.NewMarketDataCollector(segment, dataDir),
		Saver:               saver.NewDataSaver(dataDir, segment),
		RawPackets:          make(chan []byte, 1000),
		DecodedPackets:      make(chan decoder.DecodedPacket, 1000),
		DecompressedRecords: make(chan decompressor.DecompressedRecord, 1000),
		Quotes:              make(chan data_collector.Quote, 1000),
		stopChan:            make(chan struct{}),
	}
}

// runReceiver receives raw packets from UDP multicast using ReceiveLoop
func runReceiver(p *SegmentPipeline, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Printf("[%s] 🔌 Receiver goroutine started", p.Segment)

	// Use the connection's ReceiveLoop which handles the raw packets channel
	p.Connection.ReceiveLoop(p.RawPackets)

	log.Printf("[%s] 🔌 Receiver goroutine stopped", p.Segment)
}

// runDecoder decodes raw packets into structured records
func runDecoder(p *SegmentPipeline, wg *sync.WaitGroup) {
	defer wg.Done()
	defer close(p.DecodedPackets)
	log.Printf("[%s] 📝 Decoder goroutine started", p.Segment)

	decodedCount := 0
	errorCount := 0

	for packet := range p.RawPackets {
		decoded, err := p.Decoder.DecodePacket(packet)
		if err != nil {
			errorCount++
			if errorCount%100 == 0 {
				log.Printf("[%s] ⚠️ Decode errors: %d", p.Segment, errorCount)
			}
			continue
		}
		if decoded != nil {
			p.DecodedPackets <- *decoded
			decodedCount++
		}
	}

	log.Printf("[%s] 📝 Decoder goroutine stopped (decoded: %d, errors: %d)", p.Segment, decodedCount, errorCount)
}

// runDecompressor decompresses records
func runDecompressor(p *SegmentPipeline, wg *sync.WaitGroup) {
	defer wg.Done()
	defer close(p.DecompressedRecords)
	log.Printf("[%s] 🔧 Decompressor goroutine started", p.Segment)

	recordCount := 0

	for packet := range p.DecodedPackets {
		for _, record := range packet.Records {
			decompressed, err := p.Decompressor.DecompressRecord(packet.Header, record)
			if err != nil {
				continue
			}
			if decompressed != nil {
				p.DecompressedRecords <- *decompressed
				recordCount++
			}
		}
	}

	log.Printf("[%s] 🔧 Decompressor goroutine stopped (records: %d)", p.Segment, recordCount)
}

// runCollector collects quotes with symbol mapping
func runCollector(p *SegmentPipeline, wg *sync.WaitGroup) {
	defer wg.Done()
	defer close(p.Quotes)
	log.Printf("[%s] 📊 Collector goroutine started", p.Segment)

	quoteCount := 0
	unknownTokens := 0

	for record := range p.DecompressedRecords {
		quote, err := p.Collector.CollectQuote(record)
		if err != nil {
			unknownTokens++
			continue
		}
		if quote != nil {
			p.Quotes <- *quote
			quoteCount++

			// Log first few quotes for verification
			if quoteCount <= 5 {
				log.Printf("[%s] 📈 Quote: %s LTP=%.2f Vol=%d",
					p.Segment, quote.Symbol, quote.LTP, quote.Volume)
			}
		}
	}

	log.Printf("[%s] 📊 Collector goroutine stopped (quotes: %d, unknown: %d)",
		p.Segment, quoteCount, unknownTokens)
}

// runSaver saves quotes to CSV and JSON files
func runSaver(p *SegmentPipeline, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Printf("[%s] 💾 Saver goroutine started", p.Segment)

	var batch []data_collector.Quote
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	savedCount := 0

	for {
		select {
		case quote, ok := <-p.Quotes:
			if !ok {
				// Channel closed, save remaining batch
				if len(batch) > 0 {
					p.Saver.SaveQuotes(batch)
					savedCount += len(batch)
				}
				log.Printf("[%s] 💾 Saver goroutine stopped (saved: %d quotes)", p.Segment, savedCount)
				return
			}
			batch = append(batch, quote)

			// Save in batches of 50
			if len(batch) >= 50 {
				p.Saver.SaveQuotes(batch)
				savedCount += len(batch)
				if savedCount%500 == 0 {
					log.Printf("[%s] 💾 Saved %d quotes total", p.Segment, savedCount)
				}
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				p.Saver.SaveQuotes(batch)
				savedCount += len(batch)
				batch = batch[:0]
			}
		}
	}
}

func init() {
	// Print banner
	fmt.Println(`
╔═══════════════════════════════════════════════════════════════╗
║           BSE UDP Market Data Reader - Go HFT                 ║
║              Dual-Feed Implementation                         ║
║         CM (Equity) + FO (Derivatives) Segments               ║
╚═══════════════════════════════════════════════════════════════╝
	`)
}
