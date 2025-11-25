package data_collector

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"bse-go/pkg/decompressor"
)

type Quote struct {
	Token      uint32       `json:"token"`
	Symbol     string       `json:"symbol"`
	SymbolName string       `json:"symbol_name"`
	Expiry     string       `json:"expiry"`
	OptionType string       `json:"option_type"`
	Strike     string       `json:"strike"`
	Timestamp  string       `json:"timestamp"`
	Open       float64      `json:"open"`
	High       float64      `json:"high"`
	Low        float64      `json:"low"`
	Close      float64      `json:"close"`
	LTP        float64      `json:"ltp"`
	Volume     int32        `json:"volume"`
	PrevClose  float64      `json:"prev_close"`
	BidLevels  []OrderLevel `json:"bid_levels,omitempty"`
	AskLevels  []OrderLevel `json:"ask_levels,omitempty"`
}

type OrderLevel struct {
	Price    float64 `json:"price"`
	Quantity int32   `json:"quantity"`
	Flag     int32   `json:"flag"`
}

type MarketDataCollector struct {
	tokenMap map[string]map[string]interface{}
	stats    struct {
		quotesCollected int
		unknownTokens   int
	}
}

func NewMarketDataCollector(tokenMap map[string]map[string]interface{}) *MarketDataCollector {
	return &MarketDataCollector{
		tokenMap: tokenMap,
	}
}

func LoadTokenMap(filename string) (map[string]map[string]interface{}, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var tokenMap map[string]map[string]interface{}
	if err := json.NewDecoder(file).Decode(&tokenMap); err != nil {
		return nil, err
	}

	log.Printf("✓ Token map loaded: %d tokens", len(tokenMap))
	return tokenMap, nil
}

func (c *MarketDataCollector) CollectQuote(record decompressor.DecompressedRecord) (*Quote, error) {
	tokenStr := strconv.Itoa(int(record.Token))
	symbolInfo := c.resolveSymbolDetails(tokenStr)

	quote := &Quote{
		Token:      record.Token,
		Symbol:     symbolInfo["symbol"],
		SymbolName: c.formatSymbolName(symbolInfo),
		Expiry:     symbolInfo["expiry"],
		OptionType: symbolInfo["option_type"],
		Strike:     symbolInfo["strike"],
		Timestamp:  record.Timestamp.Format("2006-01-02 15:04:05.000"),
		Open:       record.Open,
		High:       record.High,
		Low:        record.Low,
		Close:      record.Close,
		LTP:        record.LTP,
		Volume:     record.Volume,
		PrevClose:  record.PrevClose,
	}

	// Convert order levels
	quote.BidLevels = make([]OrderLevel, len(record.BidLevels))
	for i, level := range record.BidLevels {
		quote.BidLevels[i] = OrderLevel{
			Price:    level.Price,
			Quantity: level.Quantity,
			Flag:     level.Flag,
		}
	}

	quote.AskLevels = make([]OrderLevel, len(record.AskLevels))
	for i, level := range record.AskLevels {
		quote.AskLevels[i] = OrderLevel{
			Price:    level.Price,
			Quantity: level.Quantity,
			Flag:     level.Flag,
		}
	}

	c.stats.quotesCollected++
	return quote, nil
}

func (c *MarketDataCollector) resolveSymbolDetails(tokenStr string) map[string]string {
	if contract, exists := c.tokenMap[tokenStr]; exists {
		// Get ticker/symbol with safe type handling
		ticker := ""
		if t := contract["ticker"]; t != nil {
			ticker = fmt.Sprintf("%v", t)
		} else if s := contract["symbol"]; s != nil {
			ticker = fmt.Sprintf("%v", s)
		} else {
			ticker = "UNKNOWN"
		}

		// Get expiry with safe type handling
		expiry := ""
		if exp := contract["expiry"]; exp != nil {
			expiry = fmt.Sprintf("%v", exp)
		}

		// Get option_type with safe type handling
		optionType := ""
		if ot := contract["option_type"]; ot != nil {
			optionType = fmt.Sprintf("%v", ot)
		}

		// Get strike with safe type handling (can be string or float64)
		strike := ""
		if st := contract["strike"]; st != nil {
			switch v := st.(type) {
			case float64:
				strikeRupees := v / 100.0
				if strikeRupees == float64(int(strikeRupees)) {
					strike = strconv.Itoa(int(strikeRupees))
				} else {
					strike = fmt.Sprintf("%.2f", strikeRupees)
				}
			case string:
				// Already a string, try to parse and convert
				if strikePaisa, err := strconv.ParseFloat(v, 64); err == nil {
					strikeRupees := strikePaisa / 100.0
					if strikeRupees == float64(int(strikeRupees)) {
						strike = strconv.Itoa(int(strikeRupees))
					} else {
						strike = fmt.Sprintf("%.2f", strikeRupees)
					}
				} else {
					strike = v
				}
			default:
				strike = fmt.Sprintf("%v", st)
			}
		}

		return map[string]string{
			"symbol":      ticker,
			"expiry":      expiry,
			"option_type": optionType,
			"strike":      strike,
		}
	}

	// Fallback decoding
	c.stats.unknownTokens++
	return c.decodeTokenFromID(tokenStr)
}

func (c *MarketDataCollector) decodeTokenFromID(tokenStr string) map[string]string {
	token, _ := strconv.Atoi(tokenStr)

	if strings.HasPrefix(tokenStr, "113") {
		expiry := c.decodeExpiryFromToken(token)
		return map[string]string{
			"symbol":      "SENSEX",
			"expiry":      expiry,
			"option_type": "",
			"strike":      "",
		}
	}

	if token >= 820163 && token <= 1159864 {
		expiry := c.decodeExpiryFromToken(token)
		return map[string]string{
			"symbol":      "SENSEX",
			"expiry":      expiry,
			"option_type": "",
			"strike":      "",
		}
	}

	if token >= 861153 && token <= 1129115 {
		expiry := c.decodeExpiryFromToken(token)
		return map[string]string{
			"symbol":      "BANKEX",
			"expiry":      expiry,
			"option_type": "",
			"strike":      "",
		}
	}

	return map[string]string{
		"symbol":      fmt.Sprintf("TOKEN_%d", token),
		"expiry":      "",
		"option_type": "",
		"strike":      "",
	}
}

func (c *MarketDataCollector) decodeExpiryFromToken(token int) string {
	knownFutures := map[int]string{
		861384:  "30-OCT-2025",
		873830:  "27-NOV-2025",
		1102290: "24-DEC-2025",
		861473:  "30-OCT-2025",
		874881:  "27-NOV-2025",
		1104160: "24-DEC-2025",
	}

	if expiry, exists := knownFutures[token]; exists {
		return expiry
	}

	tokenStr := strconv.Itoa(token)
	if strings.HasPrefix(tokenStr, "113") {
		expiryCodes := map[string]string{
			"1132": "31-JAN-2026",
			"1136": "26-MAR-2026",
			"1138": "28-DEC-2028",
		}
		if code := tokenStr[:4]; expiryCodes[code] != "" {
			return expiryCodes[code]
		}
	}

	return ""
}

func (c *MarketDataCollector) formatSymbolName(symbolInfo map[string]string) string {
	symbol := symbolInfo["symbol"]
	expiry := symbolInfo["expiry"]
	optionType := symbolInfo["option_type"]
	strike := symbolInfo["strike"]

	if optionType != "" && strike != "" {
		return fmt.Sprintf("%s%s_%s%s", symbol, c.formatExpiry(expiry), strike, optionType)
	}

	if expiry != "" {
		return fmt.Sprintf("%s%s_FUT", symbol, c.formatExpiry(expiry))
	}

	return fmt.Sprintf("%s_FUT", symbol)
}

func (c *MarketDataCollector) formatExpiry(expiry string) string {
	if expiry == "" {
		return ""
	}
	// Convert "31-JAN-2026" to "31JAN2026"
	parts := strings.Split(expiry, "-")
	if len(parts) == 3 {
		return parts[0] + parts[1] + parts[2]
	}
	return expiry
}

func (c *MarketDataCollector) GetStats() map[string]int {
	return map[string]int{
		"quotes_collected": c.stats.quotesCollected,
		"unknown_tokens":   c.stats.unknownTokens,
	}
}
