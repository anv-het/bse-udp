// Package contract_fetcher provides BSE API file fetching functionality.
// It downloads CSV files from BSE internal API with Base64 decoding.
package contract_fetcher

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// APIResponse represents the JSON response from BSE API
type APIResponse struct {
	Data struct {
		FileContent string `json:"file_content"` // Base64 encoded CSV
	} `json:"data"`
	Message string `json:"message"`
	Success bool   `json:"success"`
}

// FetchRequest represents the POST payload for file fetch
type FetchRequest struct {
	Path     string `json:"path"`
	FileName string `json:"file_name"`
}

// Fetcher handles file downloads from BSE API
type Fetcher struct {
	apiURL  string
	timeout time.Duration
}

// NewFetcher creates a new BSE API fetcher
func NewFetcher(apiURL string) *Fetcher {
	if apiURL == "" {
		apiURL = "http://192.168.102.166:2060/v1/sftp-files"
	}
	return &Fetcher{
		apiURL:  apiURL,
		timeout: 30 * time.Second,
	}
}

// FetchFile downloads a file from BSE API and returns decoded content
func (f *Fetcher) FetchFile(path, fileName string) ([]byte, error) {
	// Prepare request payload
	payload := FetchRequest{
		Path:     path,
		FileName: fileName,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build URL with api_type parameter
	url := f.apiURL + "?api_type=erp"

	// Create HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Execute request
	client := &http.Client{Timeout: f.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON response
	var apiResp APIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Decode Base64 content
	decoded, err := base64.StdEncoding.DecodeString(apiResp.Data.FileContent)
	if err != nil {
		return nil, fmt.Errorf("failed to decode Base64: %w", err)
	}

	return decoded, nil
}

// FetchAndSave downloads a file and saves it to disk
func (f *Fetcher) FetchAndSave(path, fileName, outputPath string) error {
	content, err := f.FetchFile(path, fileName)
	if err != nil {
		return err
	}

	// Ensure output directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// BuildFOPath builds the API path for F&O Contract Master file
// Format: FNO/Common/{month}-{year}/{date}/BSE_EQD_CONTRACT_{date}.csv
func BuildFOPath(date time.Time) (path, fileName string) {
	// Month name with year (e.g., "NOV-2025")
	monthYear := strings.ToUpper(date.Format("Jan-2006"))

	// Date in DDMMYYYY format
	dateStr := date.Format("02012006")

	path = fmt.Sprintf("FNO/Common/%s/%s", monthYear, dateStr)
	fileName = fmt.Sprintf("BSE_EQD_CONTRACT_%s.csv", dateStr)

	return path, fileName
}

// BuildCMPath builds the API path for BhavCopy (CM) file
// Format: EQ/Common/{month}-{year}/{date}/BhavCopy_BSE_CM_0_0_0_{date}_F_0000.csv
func BuildCMPath(date time.Time) (path, fileName string) {
	// Month name with year (e.g., "NOV-2025")
	monthYear := strings.ToUpper(date.Format("Jan-2006"))

	// Date in YYYYMMDD format
	dateStr := date.Format("20060102")

	// Date folder in DDMMYYYY format
	dateFolderStr := date.Format("02012006")

	path = fmt.Sprintf("EQ/Common/%s/%s", monthYear, dateFolderStr)
	fileName = fmt.Sprintf("BhavCopy_BSE_CM_0_0_0_%s_F_0000.csv", dateStr)

	return path, fileName
}

// GetYesterday returns yesterday's date (API typically has previous day's file)
func GetYesterday() time.Time {
	return time.Now().AddDate(0, 0, -1)
}
