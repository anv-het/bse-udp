// Package saver provides CSV file writing for BSE trade data
package saver

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"bse-go-hft/pkg/domain"
)

// TradeSaver writes trade data to CSV files
type TradeSaver struct {
	outputDir   string
	file        *os.File
	writer      *bufio.Writer
	mu          sync.Mutex
	recordCount int64
	startTime   time.Time
}

// NewTradeSaver creates a new trade data CSV saver
func NewTradeSaver(outputDir string) (*TradeSaver, error) {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create timestamped filename
	filename := fmt.Sprintf("%s_trades.csv", time.Now().Format("20060102"))
	filepath := filepath.Join(outputDir, filename)

	// Open file for writing (append if exists)
	file, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open trade CSV file: %w", err)
	}

	// Check if file is empty (new file)
	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat trade CSV file: %w", err)
	}

	writer := bufio.NewWriterSize(file, 256*1024) // 256KB buffer

	// Write header if file is empty
	if stat.Size() == 0 {
		header := domain.TradeCSVHeader() + "\n"
		if _, err := writer.WriteString(header); err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to write trade CSV header: %w", err)
		}
	}

	return &TradeSaver{
		outputDir:   outputDir,
		file:        file,
		writer:      writer,
		recordCount: 0,
		startTime:   time.Now(),
	}, nil
}

// Save writes trade data to CSV
func (s *TradeSaver) Save(trades []*domain.TradeData) error {
	if len(trades) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, trade := range trades {
		row := trade.ToCSVRow() + "\n"
		if _, err := s.writer.WriteString(row); err != nil {
			return fmt.Errorf("failed to write trade CSV row: %w", err)
		}
		s.recordCount++
	}

	return nil
}

// Flush ensures all buffered data is written to disk
func (s *TradeSaver) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writer != nil {
		return s.writer.Flush()
	}
	return nil
}

// Close flushes and closes the CSV file
func (s *TradeSaver) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.writer != nil {
		if err := s.writer.Flush(); err != nil {
			return err
		}
	}

	if s.file != nil {
		return s.file.Close()
	}

	return nil
}

// GetRecordCount returns the number of records written
func (s *TradeSaver) GetRecordCount() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.recordCount
}

// GetStats returns statistics about saved trade data
func (s *TradeSaver) GetStats() (recordCount int64, duration time.Duration, recordsPerSec float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	duration = time.Since(s.startTime)
	recordCount = s.recordCount

	if duration.Seconds() > 0 {
		recordsPerSec = float64(recordCount) / duration.Seconds()
	}

	return
}
