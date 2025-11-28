package saver

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"bse-go/pkg/data_collector"
)

// DataSaver handles saving market data to JSON and CSV files
type DataSaver struct {
	segment   string // "CM" or "FO"
	outputDir string
	jsonDir   string
	csvDir    string
	stats     struct {
		jsonFilesSaved    int
		csvFilesSaved     int
		quotesWrittenJSON int
		quotesWrittenCSV  int
		ioErrors          int
	}
}

// NewDataSaver creates a new data saver with segment-specific directories
func NewDataSaver(outputDir string, segment string) *DataSaver {
	jsonDir := filepath.Join(outputDir, "processed_json")
	csvDir := filepath.Join(outputDir, "processed_csv")

	os.MkdirAll(jsonDir, 0755)
	os.MkdirAll(csvDir, 0755)

	log.Printf("[%s] DataSaver initialized - JSON: %s, CSV: %s", segment, jsonDir, csvDir)

	return &DataSaver{
		segment:   segment,
		outputDir: outputDir,
		jsonDir:   jsonDir,
		csvDir:    csvDir,
	}
}

// SaveQuotes saves quotes to both JSON and CSV files
func (s *DataSaver) SaveQuotes(quotes []data_collector.Quote) {
	if len(quotes) == 0 {
		return
	}

	dateStr := time.Now().Format("20060102")
	timeStr := time.Now().Format("150405")

	// Save to JSON
	if err := s.saveToJSON(quotes, dateStr, timeStr); err != nil {
		log.Printf("[%s] JSON save error: %v", s.segment, err)
	}

	// Save to CSV
	if err := s.saveToCSV(quotes, dateStr); err != nil {
		log.Printf("[%s] CSV save error: %v", s.segment, err)
	}
}

// saveToJSON saves quotes to newline-delimited JSON file
func (s *DataSaver) saveToJSON(quotes []data_collector.Quote, dateStr, timeStr string) error {
	// Build filename with segment suffix
	filename := filepath.Join(s.jsonDir, fmt.Sprintf("%s_%s_%s_quotes.json", dateStr, timeStr, s.segment))

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		s.stats.ioErrors++
		return err
	}
	defer file.Close()

	for _, quote := range quotes {
		jsonData, err := json.Marshal(quote)
		if err != nil {
			continue
		}
		if _, err := file.WriteString(string(jsonData) + "\n"); err != nil {
			s.stats.ioErrors++
			return err
		}
		s.stats.quotesWrittenJSON++
	}

	s.stats.jsonFilesSaved++
	log.Printf("[%s] Saved %d quotes to JSON: %s", s.segment, len(quotes), filepath.Base(filename))
	return nil
}

// saveToCSV saves quotes to CSV file with appropriate headers
func (s *DataSaver) saveToCSV(quotes []data_collector.Quote, dateStr string) error {
	// Build filename with segment suffix: YYYYMMDD_CM_quotes.csv or YYYYMMDD_FO_quotes.csv
	filename := filepath.Join(s.csvDir, fmt.Sprintf("%s_%s_quotes.csv", dateStr, s.segment))

	fileExists := fileExists(filename)
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		s.stats.ioErrors++
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header if new file (segment-specific columns)
	if !fileExists {
		var header []string
		if s.segment == "CM" {
			// CM (Equity): No expiry, option_type, strike, lot_size columns
			header = []string{
				"timestamp", "token", "symbol", "symbol_name",
				"ltp", "open", "high", "low", "prev_close", "atp",
				"volume", "turnover_lakhs", "seq",
				"bid_prices", "bid_qtys", "bid_orders",
				"ask_prices", "ask_qtys", "ask_orders",
			}
		} else {
			// FO (Derivatives): Full columns with expiry, option_type, strike, lot_size
			header = []string{
				"timestamp", "token", "symbol", "symbol_name", "expiry", "option_type", "strike",
				"ltp", "open", "high", "low", "prev_close", "atp",
				"volume", "turnover_lakhs", "lot_size", "seq",
				"bid_prices", "bid_qtys", "bid_orders",
				"ask_prices", "ask_qtys", "ask_orders",
			}
		}
		if err := writer.Write(header); err != nil {
			s.stats.ioErrors++
			return err
		}
	}

	// Write quotes
	for _, quote := range quotes {
		var row []string
		if s.segment == "CM" {
			row = []string{
				quote.Timestamp,
				strconv.Itoa(int(quote.Token)),
				quote.Symbol,
				quote.SymbolName,
				strconv.FormatFloat(quote.LTP, 'f', 2, 64),
				strconv.FormatFloat(quote.Open, 'f', 2, 64),
				strconv.FormatFloat(quote.High, 'f', 2, 64),
				strconv.FormatFloat(quote.Low, 'f', 2, 64),
				strconv.FormatFloat(quote.PrevClose, 'f', 2, 64),
				strconv.FormatFloat(quote.ATP, 'f', 2, 64),
				strconv.Itoa(int(quote.Volume)),
				strconv.Itoa(int(quote.TurnoverLakhs)),
				strconv.Itoa(int(quote.SeqNumber)),
				s.flattenLevels(quote.BidLevels, "price"),
				s.flattenLevels(quote.BidLevels, "quantity"),
				s.flattenLevels(quote.BidLevels, "flag"),
				s.flattenLevels(quote.AskLevels, "price"),
				s.flattenLevels(quote.AskLevels, "quantity"),
				s.flattenLevels(quote.AskLevels, "flag"),
			}
		} else {
			row = []string{
				quote.Timestamp,
				strconv.Itoa(int(quote.Token)),
				quote.Symbol,
				quote.SymbolName,
				quote.Expiry,
				quote.OptionType,
				quote.Strike,
				strconv.FormatFloat(quote.LTP, 'f', 2, 64),
				strconv.FormatFloat(quote.Open, 'f', 2, 64),
				strconv.FormatFloat(quote.High, 'f', 2, 64),
				strconv.FormatFloat(quote.Low, 'f', 2, 64),
				strconv.FormatFloat(quote.PrevClose, 'f', 2, 64),
				strconv.FormatFloat(quote.ATP, 'f', 2, 64),
				strconv.Itoa(int(quote.Volume)),
				strconv.Itoa(int(quote.TurnoverLakhs)),
				strconv.Itoa(int(quote.LotSize)),
				strconv.Itoa(int(quote.SeqNumber)),
				s.flattenLevels(quote.BidLevels, "price"),
				s.flattenLevels(quote.BidLevels, "quantity"),
				s.flattenLevels(quote.BidLevels, "flag"),
				s.flattenLevels(quote.AskLevels, "price"),
				s.flattenLevels(quote.AskLevels, "quantity"),
				s.flattenLevels(quote.AskLevels, "flag"),
			}
		}
		if err := writer.Write(row); err != nil {
			s.stats.ioErrors++
			return err
		}
		s.stats.quotesWrittenCSV++
	}

	s.stats.csvFilesSaved++
	log.Printf("[%s] Saved %d quotes to CSV: %s", s.segment, len(quotes), filepath.Base(filename))
	return nil
}

// flattenLevels converts order book levels to pipe-separated strings
func (s *DataSaver) flattenLevels(levels []data_collector.OrderLevel, field string) string {
	if len(levels) == 0 {
		return ""
	}

	var values []string
	for _, level := range levels {
		switch field {
		case "price":
			values = append(values, strconv.FormatFloat(level.Price, 'f', 2, 64))
		case "quantity":
			values = append(values, strconv.Itoa(int(level.Quantity)))
		case "flag":
			values = append(values, strconv.Itoa(int(level.Flag)))
		}
	}

	return strings.Join(values, "|")
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

// GetStats returns saver statistics
func (s *DataSaver) GetStats() map[string]int {
	return map[string]int{
		"json_files_saved":    s.stats.jsonFilesSaved,
		"csv_files_saved":     s.stats.csvFilesSaved,
		"quotes_written_json": s.stats.quotesWrittenJSON,
		"quotes_written_csv":  s.stats.quotesWrittenCSV,
		"io_errors":           s.stats.ioErrors,
	}
}

// LogStats logs saver statistics
func (s *DataSaver) LogStats() {
	log.Printf("[%s] Saver Stats: JSON=%d, CSV=%d, errors=%d",
		s.segment, s.stats.quotesWrittenJSON, s.stats.quotesWrittenCSV, s.stats.ioErrors)
}
