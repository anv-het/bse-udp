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

type DataSaver struct {
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

func NewDataSaver(outputDir string) *DataSaver {
	jsonDir := filepath.Join(outputDir, "processed_json")
	csvDir := filepath.Join(outputDir, "processed_csv")

	os.MkdirAll(jsonDir, 0755)
	os.MkdirAll(csvDir, 0755)

	return &DataSaver{
		outputDir: outputDir,
		jsonDir:   jsonDir,
		csvDir:    csvDir,
	}
}

func (s *DataSaver) SaveQuotes(quotes []data_collector.Quote) {
	if len(quotes) == 0 {
		return
	}

	dateStr := time.Now().Format("20060102")
	timeStr := time.Now().Format("150405")

	// Save to JSON
	if err := s.saveToJSON(quotes, dateStr, timeStr); err != nil {
		log.Printf("JSON save error: %v", err)
	}

	// Save to CSV
	if err := s.saveToCSV(quotes, dateStr); err != nil {
		log.Printf("CSV save error: %v", err)
	}
}

func (s *DataSaver) saveToJSON(quotes []data_collector.Quote, dateStr, timeStr string) error {
	filename := filepath.Join(s.jsonDir, fmt.Sprintf("%s_%s_quotes.json", dateStr, timeStr))

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
	log.Printf("Saved %d quotes to JSON: %s", len(quotes), filename)
	return nil
}

func (s *DataSaver) saveToCSV(quotes []data_collector.Quote, dateStr string) error {
	filename := filepath.Join(s.csvDir, fmt.Sprintf("%s_quotes.csv", dateStr))

	fileExists := fileExists(filename)
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		s.stats.ioErrors++
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header if new file
	if !fileExists {
		header := []string{
			"token", "symbol", "symbol_name", "expiry", "option_type", "strike", "timestamp",
			"open", "high", "low", "close", "ltp", "volume", "prev_close",
			"bid_prices", "bid_qtys", "bid_orders",
			"ask_prices", "ask_qtys", "ask_orders",
		}
		if err := writer.Write(header); err != nil {
			s.stats.ioErrors++
			return err
		}
	}

	// Write quotes
	for _, quote := range quotes {
		row := []string{
			strconv.Itoa(int(quote.Token)),
			quote.Symbol,
			quote.SymbolName,
			quote.Expiry,
			quote.OptionType,
			quote.Strike,
			fmt.Sprintf("=\"%s\"", quote.Timestamp),
			strconv.FormatFloat(quote.Open, 'f', 2, 64),
			strconv.FormatFloat(quote.High, 'f', 2, 64),
			strconv.FormatFloat(quote.Low, 'f', 2, 64),
			strconv.FormatFloat(quote.Close, 'f', 2, 64),
			strconv.FormatFloat(quote.LTP, 'f', 2, 64),
			strconv.Itoa(int(quote.Volume)),
			strconv.FormatFloat(quote.PrevClose, 'f', 2, 64),
			s.flattenLevels(quote.BidLevels, "price"),
			s.flattenLevels(quote.BidLevels, "quantity"),
			s.flattenLevels(quote.BidLevels, "flag"),
			s.flattenLevels(quote.AskLevels, "price"),
			s.flattenLevels(quote.AskLevels, "quantity"),
			s.flattenLevels(quote.AskLevels, "flag"),
		}
		if err := writer.Write(row); err != nil {
			s.stats.ioErrors++
			return err
		}
		s.stats.quotesWrittenCSV++
	}

	s.stats.csvFilesSaved++
	log.Printf("Saved %d quotes to CSV: %s", len(quotes), filename)
	return nil
}

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

	return strings.Join(values, ",")
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}

func (s *DataSaver) GetStats() map[string]int {
	return map[string]int{
		"json_files_saved":    s.stats.jsonFilesSaved,
		"csv_files_saved":     s.stats.csvFilesSaved,
		"quotes_written_json": s.stats.quotesWrittenJSON,
		"quotes_written_csv":  s.stats.quotesWrittenCSV,
		"io_errors":           s.stats.ioErrors,
	}
}
