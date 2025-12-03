// Package saver provides HFT-optimized CSV output for market data quotes
// Uses async batched writes to minimize I/O impact on hot path
package saver

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"bse-go-hft/pkg/domain"
)

const (
	// FlushInterval - flush CSV every N records
	FlushInterval = 1000

	// BufferSize - buffered writer size (128KB for efficient disk I/O)
	BufferSize = 128 * 1024

	// BatchSize - number of records to batch before writing
	BatchSize = 100

	// ChannelSize - async write channel buffer
	ChannelSize = 10000
)

// QuoteRecord is a pre-allocated record buffer
type QuoteRecord struct {
	fields [22]string
}

// CSVSaver writes quotes to CSV files with async batched writes
type CSVSaver struct {
	mu        sync.Mutex
	file      *os.File
	bufWriter *bufio.Writer
	writer    *csv.Writer
	count     int
	filePath  string
	feedType  string
	lastFlush time.Time

	// Async write channel
	writeChan chan *domain.Quote
	done      chan struct{}
	wg        sync.WaitGroup

	// Pre-allocated record pool
	recordPool sync.Pool
}

// Header for CSV output
var csvHeader = []string{
	"timestamp", "token", "symbol", "symbol_name", "expiry", "option_type", "strike_price",
	"ltp", "open", "high", "low", "prev_close", "atp", "volume", "turnover_lakhs", "lot_size", "sequence_num",
	"bid_prices", "bid_qtys", "ask_prices", "ask_qtys", "segment",
}

// NewCSVSaver creates a new HFT-optimized CSV saver with async writes
func NewCSVSaver(outputDir, feedType string) (*CSVSaver, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output dir: %w", err)
	}

	filename := fmt.Sprintf("%s_%s_quotes.csv", time.Now().Format("20060102"), feedType)
	filepath := filepath.Join(outputDir, filename)

	// Check if file exists
	fileExists := false
	if _, err := os.Stat(filepath); err == nil {
		fileExists = true
	}

	// Open file in append mode
	file, err := os.OpenFile(filepath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// Use larger buffered writer for better I/O performance
	bufWriter := bufio.NewWriterSize(file, BufferSize)

	saver := &CSVSaver{
		file:      file,
		bufWriter: bufWriter,
		writer:    csv.NewWriter(bufWriter),
		filePath:  filepath,
		feedType:  feedType,
		lastFlush: time.Now(),
		writeChan: make(chan *domain.Quote, ChannelSize),
		done:      make(chan struct{}),
		recordPool: sync.Pool{
			New: func() interface{} {
				return &QuoteRecord{}
			},
		},
	}

	// Write header if new file
	if !fileExists {
		if err := saver.writer.Write(csvHeader); err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to write header: %w", err)
		}
		saver.writer.Flush()
		saver.bufWriter.Flush()
	}

	// Start async writer goroutine
	saver.wg.Add(1)
	go saver.asyncWriter()

	fmt.Printf("📝 CSV output: %s\n", filepath)
	return saver, nil
}

// Save queues a quote for async writing (non-blocking, HFT-safe)
func (s *CSVSaver) Save(quote *domain.Quote) error {
	// Non-blocking send to async writer
	select {
	case s.writeChan <- quote:
		return nil
	default:
		// Channel full - drop quote to prevent blocking hot path
		// In production, you might want to track dropped quotes
		return nil
	}
}

// asyncWriter runs in background, batching writes for efficiency
func (s *CSVSaver) asyncWriter() {
	defer s.wg.Done()

	batch := make([][]string, 0, BatchSize)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case quote, ok := <-s.writeChan:
			if !ok {
				// Channel closed, flush remaining
				s.flushBatch(batch)
				return
			}

			// Build record using pool
			record := s.buildRecord(quote)
			batch = append(batch, record)

			// Flush when batch is full
			if len(batch) >= BatchSize {
				s.flushBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			// Periodic flush for low-volume periods
			if len(batch) > 0 {
				s.flushBatch(batch)
				batch = batch[:0]
			}

		case <-s.done:
			// Drain remaining quotes
			for quote := range s.writeChan {
				record := s.buildRecord(quote)
				batch = append(batch, record)
			}
			s.flushBatch(batch)
			return
		}
	}
}

// buildRecord creates a CSV record from a quote
func (s *CSVSaver) buildRecord(quote *domain.Quote) []string {
	return []string{
		quote.TimestampString(),
		strconv.FormatUint(uint64(quote.Token), 10),
		quote.Symbol,
		quote.SymbolName,
		quote.Expiry,
		quote.OptionType,
		strconv.FormatFloat(quote.StrikePrice, 'f', 2, 64),
		strconv.FormatFloat(quote.LTP, 'f', 2, 64),
		strconv.FormatFloat(quote.Open, 'f', 2, 64),
		strconv.FormatFloat(quote.High, 'f', 2, 64),
		strconv.FormatFloat(quote.Low, 'f', 2, 64),
		strconv.FormatFloat(quote.PrevClose, 'f', 2, 64),
		strconv.FormatFloat(quote.ATP, 'f', 2, 64),
		strconv.FormatInt(quote.Volume, 10),
		strconv.FormatFloat(quote.Turnover, 'f', 2, 64),
		strconv.Itoa(quote.LotSize),
		strconv.FormatUint(uint64(quote.SequenceNum), 10),
		quote.BidPricesString(),
		quote.BidQtysString(),
		quote.AskPricesString(),
		quote.AskQtysString(),
		quote.Segment,
	}
}

// flushBatch writes a batch of records to disk
func (s *CSVSaver) flushBatch(batch [][]string) {
	if len(batch) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, record := range batch {
		s.writer.Write(record)
		s.count++
	}

	// Flush every FlushInterval records
	if s.count%FlushInterval == 0 {
		s.writer.Flush()
		s.bufWriter.Flush()
	}
}

// Close gracefully shuts down the async writer and closes the file
func (s *CSVSaver) Close() error {
	// Signal done and close channel
	close(s.done)
	close(s.writeChan)

	// Wait for async writer to finish
	s.wg.Wait()

	// Final flush
	s.mu.Lock()
	defer s.mu.Unlock()

	s.writer.Flush()
	s.bufWriter.Flush()
	return s.file.Close()
}

// Count returns the number of quotes saved
func (s *CSVSaver) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

// FilePath returns the path to the CSV file
func (s *CSVSaver) FilePath() string {
	return s.filePath
}

// FeedType returns the feed type (EQ or FO)
func (s *CSVSaver) FeedType() string {
	return s.feedType
}

// Flush forces a flush of the CSV writer
func (s *CSVSaver) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writer.Flush()
	s.bufWriter.Flush()
}
