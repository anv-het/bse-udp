// Package saver provides CSV output for market data quotes
package saver

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"bse-go-hft/pkg/domain"
)

// CSVSaver writes quotes to CSV files
type CSVSaver struct {
	mu       sync.Mutex
	file     *os.File
	writer   *csv.Writer
	count    int
	filePath string
	feedType string
}

// Header for CSV output
var csvHeader = []string{
	"timestamp", "token", "symbol", "symbol_name", "expiry", "option_type", "strike_price",
	"ltp", "open", "high", "low", "prev_close", "atp", "volume", "turnover_lakhs", "lot_size", "sequence_num",
	"bid_prices", "bid_qtys", "ask_prices", "ask_qtys", "segment",
}

// NewCSVSaver creates a new CSV saver for the given feed type
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

	saver := &CSVSaver{
		file:     file,
		writer:   csv.NewWriter(file),
		filePath: filepath,
		feedType: feedType,
	}

	// Write header if new file
	if !fileExists {
		if err := saver.writer.Write(csvHeader); err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to write header: %w", err)
		}
		saver.writer.Flush()
	}

	fmt.Printf("📝 CSV output: %s\n", filepath)
	return saver, nil
}

// Save writes a quote to the CSV file
func (s *CSVSaver) Save(quote *domain.Quote) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := []string{
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

	if err := s.writer.Write(record); err != nil {
		return err
	}

	s.count++

	// Flush every 100 records for durability
	if s.count%100 == 0 {
		s.writer.Flush()
	}

	return nil
}

// Close flushes and closes the CSV file
func (s *CSVSaver) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.writer.Flush()
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
}
