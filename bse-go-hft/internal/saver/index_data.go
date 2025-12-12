// Package saver provides CSV saving functionality for BSE index data
package saver

import (
	"bse-go-hft/pkg/domain"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// IndexDataSaver saves index data to CSV files
type IndexDataSaver struct {
	writer      *csv.Writer
	file        *os.File
	filename    string
	rowCount    int
	flushBuffer int
	mu          sync.Mutex
}

// NewIndexDataSaver creates a new CSV saver for index data
func NewIndexDataSaver(baseDir string, msgType uint16) (*IndexDataSaver, error) {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Create filename based on message type
	var suffix string
	if msgType == 2011 {
		suffix = "index_critical" // 1 second frequency (SENSEX, BSE100, etc.)
	} else if msgType == 2012 {
		suffix = "index_regular" // 8 second frequency (other indices)
	} else {
		suffix = "index_data"
	}

	filename := filepath.Join(baseDir, fmt.Sprintf("%s_%s.csv",
		time.Now().Format("20060102"), suffix))

	// Open file (append mode if exists, create if not)
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// Check if file is empty (need to write header)
	fileInfo, _ := file.Stat()
	needHeader := fileInfo.Size() == 0

	writer := csv.NewWriter(file)

	// Write CSV header if file is new
	if needHeader {
		header := []string{
			"Timestamp",
			"Message_Type",
			"Index_Code",
			"Index_Name",
			"Index_Value",
			"Net_Change",     // Calculated: Current - PrevClose
			"Percent_Change", // Calculated: (NetChange / PrevClose) * 100
			"Prev_Close",
			"Open",
			"High",
			"Low",
		}
		if err := writer.Write(header); err != nil {
			file.Close()
			return nil, fmt.Errorf("failed to write header: %w", err)
		}
		writer.Flush()
	}

	return &IndexDataSaver{
		writer:      writer,
		file:        file,
		filename:    filename,
		flushBuffer: 50, // Flush every 50 rows (indices are less frequent than quotes)
	}, nil
}

// Save writes an index data record to CSV
func (s *IndexDataSaver) Save(index *domain.IndexData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := []string{
		index.Timestamp.Format("2006-01-02 15:04:05.000"),
		fmt.Sprintf("%d", index.MessageType),
		fmt.Sprintf("%d", index.IndexCode),
		index.GetIndexName(),
		fmt.Sprintf("%.2f", index.IndexValue),
		fmt.Sprintf("%.2f", index.NetChange),
		fmt.Sprintf("%.2f", index.PercentChange),
		fmt.Sprintf("%.2f", index.PrevClose),
		fmt.Sprintf("%.2f", index.OpenValue),
		fmt.Sprintf("%.2f", index.HighValue),
		fmt.Sprintf("%.2f", index.LowValue),
	}

	if err := s.writer.Write(row); err != nil {
		return fmt.Errorf("failed to write row: %w", err)
	}

	s.rowCount++

	// Flush periodically
	if s.rowCount%s.flushBuffer == 0 {
		s.writer.Flush()
		if err := s.writer.Error(); err != nil {
			return fmt.Errorf("flush error: %w", err)
		}
	}

	return nil
}

// SaveBatch writes multiple index records to CSV
func (s *IndexDataSaver) SaveBatch(indices []*domain.IndexData) error {
	for _, index := range indices {
		if err := s.Save(index); err != nil {
			return err
		}
	}
	return nil
}

// GetRowCount returns the number of rows written
func (s *IndexDataSaver) GetRowCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rowCount
}

// GetFilename returns the CSV filename
func (s *IndexDataSaver) GetFilename() string {
	return s.filename
}

// Flush forces write of buffered data
func (s *IndexDataSaver) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.writer.Flush()
	return s.writer.Error()
}

// Close flushes and closes the CSV file
func (s *IndexDataSaver) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.writer.Flush()
	if err := s.writer.Error(); err != nil {
		return err
	}

	return s.file.Close()
}
