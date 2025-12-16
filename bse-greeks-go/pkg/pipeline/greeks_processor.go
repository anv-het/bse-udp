package pipeline

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"bse-greeks-go/pkg/greeks"
)

// FOQuote represents a parsed F&O quote from UDP feed
type FOQuote struct {
	Timestamp   time.Time
	Token       string
	Symbol      string
	SymbolName  string
	Expiry      time.Time
	OptionType  string
	StrikePrice float64
	LTP         float64
	Open        float64
	High        float64
	Low         float64
	PrevClose   float64
	ATP         float64
	Volume      int64
	Turnover    float64
	LotSize     int
	SequenceNum int64
	BidPrices   []float64
	BidQtys     []int
	AskPrices   []float64
	AskQtys     []int
	Segment     string
}

// IndexQuote represents parsed index spot price
type IndexQuote struct {
	Timestamp     time.Time
	MessageType   int
	IndexCode     int
	IndexName     string
	IndexValue    float64
	NetChange     float64
	PercentChange float64
	PrevClose     float64
	Open          float64
	High          float64
	Low           float64
}

// GreeksResult combines FO quote with calculated Greeks
type GreeksResult struct {
	FOQuote
	Greeks    greeks.Greeks
	Moneyness string
	Intrinsic float64
	TimeValue float64
	SpotPrice float64
	CalcTime  time.Time
}

// RealTimeGreeksProcessor calculates Greeks on live UDP data
type RealTimeGreeksProcessor struct {
	calculator     *greeks.Calculator
	spotPrices     map[string]float64 // symbol -> latest spot
	spotPriceMutex sync.RWMutex
	processedCount int64
	greeksChannel  chan GreeksResult
	riskFreeRate   float64
	minVolume      int64
}

// NewRealTimeGreeksProcessor creates a new real-time Greeks processor
func NewRealTimeGreeksProcessor(riskFreeRate float64, minVolume int64) *RealTimeGreeksProcessor {
	return &RealTimeGreeksProcessor{
		calculator:    greeks.NewCalculator(riskFreeRate),
		spotPrices:    make(map[string]float64),
		greeksChannel: make(chan GreeksResult, 1000),
		riskFreeRate:  riskFreeRate,
		minVolume:     minVolume,
	}
}

// UpdateSpotPrice updates the latest spot price for an index
func (p *RealTimeGreeksProcessor) UpdateSpotPrice(symbol string, price float64) {
	p.spotPriceMutex.Lock()
	p.spotPrices[symbol] = price
	p.spotPriceMutex.Unlock()
}

// GetSpotPrice retrieves the latest spot price
func (p *RealTimeGreeksProcessor) GetSpotPrice(symbol string) (float64, bool) {
	p.spotPriceMutex.RLock()
	defer p.spotPriceMutex.RUnlock()
	price, ok := p.spotPrices[symbol]
	return price, ok
}

// ProcessFOQuote calculates Greeks for an FO quote in real-time
func (p *RealTimeGreeksProcessor) ProcessFOQuote(quote FOQuote) (*GreeksResult, error) {
	// Skip low volume options
	if quote.Volume < p.minVolume {
		return nil, nil
	}

	// Get latest spot price
	spotPrice, ok := p.GetSpotPrice(quote.Symbol)
	if !ok {
		// Fallback to default spot prices
		if quote.Symbol == "SENSEX" {
			spotPrice = 85000
		} else if quote.Symbol == "BANKEX" {
			spotPrice = 67000
		} else {
			return nil, fmt.Errorf("no spot price for %s", quote.Symbol)
		}
	}

	// Calculate Greeks using market price (LTP)
	g := p.calculator.CalculateWithIV(
		quote.LTP,
		quote.OptionType,
		spotPrice,
		quote.StrikePrice,
		quote.Expiry,
		quote.Volume,
	)

	// Build result
	result := &GreeksResult{
		FOQuote:   quote,
		Greeks:    g,
		Moneyness: greeks.Moneyness(quote.OptionType, spotPrice, quote.StrikePrice),
		Intrinsic: greeks.IntrinsicValue(quote.OptionType, spotPrice, quote.StrikePrice),
		TimeValue: greeks.TimeValue(quote.LTP, greeks.IntrinsicValue(quote.OptionType, spotPrice, quote.StrikePrice)),
		SpotPrice: spotPrice,
		CalcTime:  time.Now(),
	}

	p.processedCount++
	return result, nil
}

// ParseFOCSVLine parses a CSV line from FO feed
func ParseFOCSVLine(line string) (*FOQuote, error) {
	reader := csv.NewReader(strings.NewReader(line))
	record, err := reader.Read()
	if err != nil {
		return nil, err
	}

	if len(record) < 22 {
		return nil, fmt.Errorf("insufficient columns: %d", len(record))
	}

	quote := &FOQuote{}

	// Timestamp (column 0)
	quote.Timestamp, err = time.Parse("2006-01-02 15:04:05.999", record[0])
	if err != nil {
		quote.Timestamp, _ = time.Parse("2006-01-02 15:04:05", record[0])
	}

	// Token (column 1)
	quote.Token = record[1]

	// Symbol (column 2)
	quote.Symbol = strings.TrimSpace(record[2])

	// Symbol Name (column 3)
	quote.SymbolName = strings.TrimSpace(record[3])

	// Expiry (column 4) - format "18-DEC-2025"
	quote.Expiry, err = time.Parse("02-Jan-2006", record[4])
	if err != nil {
		return nil, fmt.Errorf("invalid expiry: %s", record[4])
	}

	// Option Type (column 5)
	quote.OptionType = strings.TrimSpace(record[5])

	// Strike Price (column 6)
	quote.StrikePrice, _ = strconv.ParseFloat(record[6], 64)

	// LTP (column 7)
	quote.LTP, _ = strconv.ParseFloat(record[7], 64)

	// Open, High, Low (columns 8, 9, 10)
	quote.Open, _ = strconv.ParseFloat(record[8], 64)
	quote.High, _ = strconv.ParseFloat(record[9], 64)
	quote.Low, _ = strconv.ParseFloat(record[10], 64)

	// Prev Close (column 11)
	quote.PrevClose, _ = strconv.ParseFloat(record[11], 64)

	// ATP (column 12)
	quote.ATP, _ = strconv.ParseFloat(record[12], 64)

	// Volume (column 13)
	quote.Volume, _ = strconv.ParseInt(record[13], 10, 64)

	// Turnover (column 14)
	quote.Turnover, _ = strconv.ParseFloat(record[14], 64)

	// Lot Size (column 15)
	lotSize, _ := strconv.Atoi(record[15])
	quote.LotSize = lotSize

	// Sequence Number (column 16)
	quote.SequenceNum, _ = strconv.ParseInt(record[16], 10, 64)

	// Bid/Ask prices and quantities (columns 17-20)
	quote.BidPrices = parseFloatArray(record[17])
	quote.BidQtys = parseIntArray(record[18])
	quote.AskPrices = parseFloatArray(record[19])
	quote.AskQtys = parseIntArray(record[20])

	// Segment (column 21)
	quote.Segment = strings.TrimSpace(record[21])

	return quote, nil
}

// ParseIndexCSVLine parses a CSV line from Index feed
func ParseIndexCSVLine(line string) (*IndexQuote, error) {
	reader := csv.NewReader(strings.NewReader(line))
	record, err := reader.Read()
	if err != nil {
		return nil, err
	}

	if len(record) < 11 {
		return nil, fmt.Errorf("insufficient columns: %d", len(record))
	}

	quote := &IndexQuote{}

	// Timestamp (column 0)
	quote.Timestamp, err = time.Parse("2006-01-02 15:04:05.999", record[0])
	if err != nil {
		quote.Timestamp, _ = time.Parse("2006-01-02 15:04:05", record[0])
	}

	// Message Type (column 1)
	quote.MessageType, _ = strconv.Atoi(record[1])

	// Index Code (column 2)
	quote.IndexCode, _ = strconv.Atoi(record[2])

	// Index Name (column 3)
	quote.IndexName = strings.TrimSpace(record[3])

	// Index Value (column 4)
	quote.IndexValue, _ = strconv.ParseFloat(record[4], 64)

	// Net Change (column 5)
	quote.NetChange, _ = strconv.ParseFloat(record[5], 64)

	// Percent Change (column 6)
	quote.PercentChange, _ = strconv.ParseFloat(record[6], 64)

	// Prev Close, Open, High, Low (columns 7-10)
	quote.PrevClose, _ = strconv.ParseFloat(record[7], 64)
	quote.Open, _ = strconv.ParseFloat(record[8], 64)
	quote.High, _ = strconv.ParseFloat(record[9], 64)
	quote.Low, _ = strconv.ParseFloat(record[10], 64)

	return quote, nil
}

// Helper functions
func parseFloatArray(s string) []float64 {
	s = strings.Trim(s, "\"")
	if s == "" {
		return []float64{}
	}
	parts := strings.Split(s, ",")
	result := make([]float64, 0, len(parts))
	for _, part := range parts {
		if val, err := strconv.ParseFloat(strings.TrimSpace(part), 64); err == nil {
			result = append(result, val)
		}
	}
	return result
}

func parseIntArray(s string) []int {
	s = strings.Trim(s, "\"")
	if s == "" {
		return []int{}
	}
	parts := strings.Split(s, ",")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		if val, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
			result = append(result, val)
		}
	}
	return result
}

// GetProcessedCount returns the number of Greeks calculations performed
func (p *RealTimeGreeksProcessor) GetProcessedCount() int64 {
	return p.processedCount
}
