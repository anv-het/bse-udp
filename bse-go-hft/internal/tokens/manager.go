// Package tokens provides token file management for BSE HFT system
package tokens

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"bse-go-hft/pkg/domain"
)

// Manager handles token file download, caching, and loading
type Manager struct {
	tokensDir  string
	apiURL     string
	maxRetries int
	retryDelay time.Duration
	keepFiles  int
	holidays   map[string]bool
}

// NewManager creates a new token file manager
func NewManager(tokensDir, apiURL string) *Manager {
	// Known BSE holidays for 2025
	holidays := map[string]bool{
		"2025-01-26": true, // Republic Day
		"2025-03-14": true, // Holi
		"2025-03-31": true, // Id-Ul-Fitr
		"2025-04-10": true, // Shri Mahavir Jayanti
		"2025-04-14": true, // Dr. Ambedkar Jayanti
		"2025-04-18": true, // Good Friday
		"2025-05-01": true, // Maharashtra Day
		"2025-08-15": true, // Independence Day
		"2025-08-27": true, // Janmashtami
		"2025-10-02": true, // Gandhi Jayanti
		"2025-10-21": true, // Diwali Laxmi Pujan
		"2025-10-22": true, // Diwali Balipratipada
		"2025-11-05": true, // Prakash Gurpurab
		"2025-11-26": true, // Guru Nanak Jayanti
		"2025-12-25": true, // Christmas
	}

	return &Manager{
		tokensDir:  tokensDir,
		apiURL:     apiURL,
		maxRetries: 3,
		retryDelay: 10 * time.Second,
		keepFiles:  2,
		holidays:   holidays,
	}
}

// APIResponse represents the response from internal API
type apiResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		FileContent string `json:"file_content"` // Base64 encoded CSV
	} `json:"data"`
}

// getMonthName returns month abbreviation
func getMonthName(month time.Month) string {
	months := []string{"", "JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}
	return months[month]
}

// isTradingDay checks if a date is a trading day
func (m *Manager) isTradingDay(date time.Time) bool {
	// Check weekend
	if date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
		return false
	}
	// Check holiday
	dateStr := date.Format("2006-01-02")
	return !m.holidays[dateStr]
}

// getTargetDate returns the target file date (previous trading day)
func (m *Manager) getTargetDate() time.Time {
	target := time.Now().AddDate(0, 0, -1)
	for !m.isTradingDay(target) {
		target = target.AddDate(0, 0, -1)
	}
	return target
}

// getTradingDates returns last N trading dates for fallback
func (m *Manager) getTradingDates(count int) []time.Time {
	var dates []time.Time
	current := m.getTargetDate()

	for len(dates) < count {
		if m.isTradingDay(current) {
			dates = append(dates, current)
		}
		current = current.AddDate(0, 0, -1)
	}

	return dates
}

// LoadBhavCopy loads equity tokens from BhavCopy CSV
func (m *Manager) LoadBhavCopy(tokenMap *domain.TokenMap) error {
	targetDate := m.getTargetDate()
	fmt.Printf("\n📊 Loading BhavCopy (target date: %s)\n", targetDate.Format("02-01-2006"))

	// Try to get file (download if needed)
	csvPath, err := m.getLatestFile("EQ", targetDate)
	if err != nil {
		return fmt.Errorf("failed to get BhavCopy: %w", err)
	}

	fmt.Printf("📂 Loading BhavCopy: %s\n", csvPath)

	return m.parseBhavCopy(csvPath, tokenMap)
}

// LoadContractMaster loads F&O tokens from Contract Master CSV
func (m *Manager) LoadContractMaster(tokenMap *domain.TokenMap) error {
	targetDate := m.getTargetDate()
	fmt.Printf("\n📊 Loading Contract Master (target date: %s)\n", targetDate.Format("02-01-2006"))

	// Try to get file (download if needed)
	csvPath, err := m.getLatestFile("FO", targetDate)
	if err != nil {
		return fmt.Errorf("failed to get Contract Master: %w", err)
	}

	fmt.Printf("📂 Loading Contract Master: %s\n", csvPath)

	return m.parseContractMaster(csvPath, tokenMap)
}

// getLatestFile gets the token file, downloading if necessary
func (m *Manager) getLatestFile(feedType string, targetDate time.Time) (string, error) {
	// Check if file exists
	filename := m.getExpectedFilename(targetDate, feedType)
	filePath := filepath.Join(m.tokensDir, filename)

	if _, err := os.Stat(filePath); err == nil {
		fmt.Printf("   ✅ Using cached file: %s\n", filename)
		return filePath, nil
	}

	// Download with retries
	fmt.Printf("   📥 Downloading from API (target: %s)...\n", targetDate.Format("02-01-2006"))

	for retry := 1; retry <= m.maxRetries; retry++ {
		fmt.Printf("   🔄 Attempt %d/%d for %s (%s)...\n",
			retry, m.maxRetries, targetDate.Format("02-01-2006"), targetDate.Weekday())

		csvBytes, err := m.fetchFromAPI(targetDate, feedType)
		if err == nil {
			// Save file
			if err := os.MkdirAll(m.tokensDir, 0755); err != nil {
				return "", err
			}
			if err := os.WriteFile(filePath, csvBytes, 0644); err != nil {
				return "", err
			}
			fmt.Printf("   ✅ Downloaded and saved: %s\n", filename)

			// Cleanup old files
			m.cleanupOldFiles(feedType)

			return filePath, nil
		}

		if retry < m.maxRetries {
			fmt.Printf("   ⚠️  Retry %d failed: %v\n", retry, err)
			time.Sleep(m.retryDelay)
		}
	}

	// Fallback to older cached files
	fmt.Printf("   🔍 Looking for older cached files...\n")
	fallbackDates := m.getTradingDates(7)
	for _, date := range fallbackDates {
		fallbackFilename := m.getExpectedFilename(date, feedType)
		fallbackPath := filepath.Join(m.tokensDir, fallbackFilename)
		if _, err := os.Stat(fallbackPath); err == nil {
			fmt.Printf("   📁 Using fallback: %s\n", fallbackFilename)
			return fallbackPath, nil
		}
	}

	return "", fmt.Errorf("failed to get file after all attempts")
}

func (m *Manager) getExpectedFilename(date time.Time, feedType string) string {
	if feedType == "EQ" {
		return fmt.Sprintf("BhavCopy_BSE_CM_%s.csv", date.Format("02012006"))
	}
	return fmt.Sprintf("BSE_EQD_CONTRACT_%s.csv", date.Format("02012006"))
}

func (m *Manager) fetchFromAPI(date time.Time, feedType string) ([]byte, error) {
	dateDD_MM_YYYY := date.Format("02-01-2006")
	dateDDMMYYYY := date.Format("02012006")
	dateYYYYMMDD := date.Format("20060102")
	monthYear := fmt.Sprintf("%s-%d", getMonthName(date.Month()), date.Year())

	var filePath, fileName string

	if feedType == "EQ" {
		fileName = fmt.Sprintf("BhavCopy_BSE_CM_0_0_0_%s_F_0000.csv", dateYYYYMMDD)
		filePath = fmt.Sprintf("EQ/Common/%s/%s/%s", monthYear, dateDD_MM_YYYY, fileName)
	} else {
		fileName = fmt.Sprintf("BSE_EQD_CONTRACT_%s.csv", dateDDMMYYYY)
		filePath = fmt.Sprintf("FNO/Common/%s/%s/%s", monthYear, dateDD_MM_YYYY, fileName)
	}

	fmt.Printf("   📡 API Request: %s\n", fileName)
	fmt.Printf("   📂 Path: %s\n", filePath)

	// Build request
	requestBody := map[string]string{
		"path":      filePath,
		"file_name": fileName,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	url := m.apiURL + "?api_type=erp"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, err
	}

	if apiResp.Status != "success" {
		return nil, fmt.Errorf("API error: %s", apiResp.Message)
	}

	if apiResp.Data.FileContent == "" {
		return nil, fmt.Errorf("empty file content")
	}

	csvBytes, err := base64.StdEncoding.DecodeString(apiResp.Data.FileContent)
	if err != nil {
		return nil, err
	}

	fmt.Printf("   ✅ Received %d bytes\n", len(csvBytes))
	return csvBytes, nil
}

func (m *Manager) parseBhavCopy(csvPath string, tokenMap *domain.TokenMap) error {
	file, err := os.Open(csvPath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		return err
	}

	// Find column indices
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToLower(strings.TrimSpace(col))] = i
	}

	// Primary columns (Internal API format)
	tokenCol := findColumn(colIdx, "fininstrmid", "sc_code", "scripcode")
	symbolCol := findColumn(colIdx, "tckrsymb", "sc_name", "scripname")
	nameCol := findColumn(colIdx, "fininstrmfulnm", "scty_nm", "securityname")

	if tokenCol == -1 || symbolCol == -1 {
		return fmt.Errorf("required columns not found in BhavCopy")
	}

	fmt.Printf("   Token column: %d (%s)\n", tokenCol, header[tokenCol])
	fmt.Printf("   Symbol column: %d (%s)\n", symbolCol, header[symbolCol])

	count := 0
	sampleCount := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		if len(record) <= tokenCol || len(record) <= symbolCol {
			continue
		}

		tokenStr := strings.TrimSpace(record[tokenCol])
		token64, err := strconv.ParseUint(tokenStr, 10, 32)
		if err != nil || token64 == 0 {
			continue
		}
		token := uint32(token64)

		symbol := strings.TrimSpace(record[symbolCol])
		if symbol == "" {
			continue
		}

		symbolName := symbol
		if nameCol != -1 && len(record) > nameCol {
			symbolName = strings.TrimSpace(record[nameCol])
		}

		contract := &domain.Contract{
			Token:      token,
			Symbol:     symbol,
			SymbolName: symbolName,
			Segment:    "EQ",
			Source:     "BhavCopy",
		}

		tokenMap.Set(token, contract)
		count++

		// Show sample
		if sampleCount < 5 {
			fmt.Printf("      %d → %s (%s)\n", token, symbol, symbolName)
			sampleCount++
		}
	}

	fmt.Printf("✅ Loaded %d equity scripts from BhavCopy\n", count)
	return nil
}

func (m *Manager) parseContractMaster(csvPath string, tokenMap *domain.TokenMap) error {
	file, err := os.Open(csvPath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	header, err := reader.Read()
	if err != nil {
		return err
	}

	// Find column indices
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToLower(strings.TrimSpace(col))] = i
	}

	tokenCol := findColumn(colIdx, "fininstrmid", "sctyid")
	symbolCol := findColumn(colIdx, "tckrsymb", "undrlyng")
	nameCol := findColumn(colIdx, "sctylngnm", "sctyname")
	expiryCol := findColumn(colIdx, "xprydt", "exprdt")
	strikeCol := findColumn(colIdx, "strkpric", "strkrt")
	optionCol := findColumn(colIdx, "optntp", "opttype")
	instrCol := findColumn(colIdx, "fininstrmnm", "instrtyp")
	lotCol := findColumn(colIdx, "minlot", "lotsize")

	if tokenCol == -1 || symbolCol == -1 {
		return fmt.Errorf("required columns not found in Contract Master")
	}

	fmt.Printf("   Token column: %d (%s)\n", tokenCol, header[tokenCol])
	fmt.Printf("   Symbol column: %d (%s)\n", symbolCol, header[symbolCol])
	if nameCol != -1 {
		fmt.Printf("   Name column: %d (%s)\n", nameCol, header[nameCol])
	}

	count := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		if len(record) <= tokenCol || len(record) <= symbolCol {
			continue
		}

		tokenStr := strings.TrimSpace(record[tokenCol])
		token64, err := strconv.ParseUint(tokenStr, 10, 32)
		if err != nil || token64 == 0 {
			continue
		}
		token := uint32(token64)

		symbol := strings.TrimSpace(record[symbolCol])
		if symbol == "" {
			continue
		}

		contract := &domain.Contract{
			Token:   token,
			Symbol:  symbol,
			Segment: "FO",
			Source:  "ContractMaster",
		}

		if nameCol != -1 && len(record) > nameCol {
			contract.SymbolName = strings.TrimSpace(record[nameCol])
		}
		if expiryCol != -1 && len(record) > expiryCol {
			contract.Expiry = strings.TrimSpace(record[expiryCol])
		}
		if strikeCol != -1 && len(record) > strikeCol {
			if strike, err := strconv.ParseFloat(strings.TrimSpace(record[strikeCol]), 64); err == nil {
				contract.StrikePrice = strike / 100.0 // paise to rupees
			}
		}
		if optionCol != -1 && len(record) > optionCol {
			contract.OptionType = strings.TrimSpace(record[optionCol])
		}
		if instrCol != -1 && len(record) > instrCol {
			contract.InstrumentType = strings.TrimSpace(record[instrCol])
		}
		if lotCol != -1 && len(record) > lotCol {
			if lot, err := strconv.Atoi(strings.TrimSpace(record[lotCol])); err == nil {
				contract.LotSize = lot
			}
		}

		tokenMap.Set(token, contract)
		count++
	}

	fmt.Printf("✅ Loaded %d contracts\n", count)
	return nil
}

func (m *Manager) cleanupOldFiles(feedType string) {
	pattern := "BhavCopy_BSE_CM_*.csv"
	if feedType == "FO" {
		pattern = "BSE_EQD_CONTRACT_*.csv"
	}

	matches, err := filepath.Glob(filepath.Join(m.tokensDir, pattern))
	if err != nil || len(matches) <= m.keepFiles {
		return
	}

	// Sort by modification time
	type fileInfo struct {
		path    string
		modTime time.Time
	}
	var files []fileInfo

	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		files = append(files, fileInfo{path, info.ModTime()})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	deleted := 0
	for i := m.keepFiles; i < len(files); i++ {
		if os.Remove(files[i].path) == nil {
			fmt.Printf("   🗑️  Deleted old file: %s\n", filepath.Base(files[i].path))
			deleted++
		}
	}

	if deleted > 0 {
		fmt.Printf("   🧹 Cleanup: Deleted %d old files, kept %d\n", deleted, m.keepFiles)
	}
}

func findColumn(colIdx map[string]int, names ...string) int {
	for _, name := range names {
		if idx, ok := colIdx[name]; ok {
			return idx
		}
	}
	return -1
}
