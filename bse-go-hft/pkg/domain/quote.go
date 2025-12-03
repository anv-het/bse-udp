// Package domain contains domain models for BSE market data
package domain

import (
	"strconv"
	"strings"
	"time"
)

// Quote represents a market data quote
type Quote struct {
	Timestamp   time.Time `json:"timestamp"`
	Token       uint32    `json:"token"`
	Symbol      string    `json:"symbol"`
	SymbolName  string    `json:"symbol_name"`
	Expiry      string    `json:"expiry"`
	OptionType  string    `json:"option_type"` // CE/PE/FUT or empty for EQ
	StrikePrice float64   `json:"strike_price"`
	LTP         float64   `json:"ltp"`
	Open        float64   `json:"open"`
	High        float64   `json:"high"`
	Low         float64   `json:"low"`
	PrevClose   float64   `json:"prev_close"`
	ATP         float64   `json:"atp"` // Average Traded Price
	Volume      int64     `json:"volume"`
	Turnover    float64   `json:"turnover"` // In Lakhs
	LotSize     int       `json:"lot_size"`
	SequenceNum uint32    `json:"sequence_num"`
	Segment     string    `json:"segment"` // EQ or FO

	// Best 5 Bid/Ask
	BidPrices [5]float64 `json:"bid_prices"`
	BidQtys   [5]int64   `json:"bid_qtys"`
	AskPrices [5]float64 `json:"ask_prices"`
	AskQtys   [5]int64   `json:"ask_qtys"`
}

// TimestampString returns formatted timestamp for CSV
func (q *Quote) TimestampString() string {
	return q.Timestamp.Format("2006-01-02 15:04:05.000")
}

// BidPricesString returns comma-separated bid prices
func (q *Quote) BidPricesString() string {
	return floatArrayToString(q.BidPrices[:])
}

// BidQtysString returns comma-separated bid quantities
func (q *Quote) BidQtysString() string {
	return int64ArrayToString(q.BidQtys[:])
}

// AskPricesString returns comma-separated ask prices
func (q *Quote) AskPricesString() string {
	return floatArrayToString(q.AskPrices[:])
}

// AskQtysString returns comma-separated ask quantities
func (q *Quote) AskQtysString() string {
	return int64ArrayToString(q.AskQtys[:])
}

func floatArrayToString(arr []float64) string {
	parts := make([]string, len(arr))
	for i, v := range arr {
		parts[i] = strconv.FormatFloat(v, 'f', 2, 64)
	}
	return strings.Join(parts, ",")
}

func int64ArrayToString(arr []int64) string {
	parts := make([]string, len(arr))
	for i, v := range arr {
		parts[i] = strconv.FormatInt(v, 10)
	}
	return strings.Join(parts, ",")
}

// OrderBook represents the full order book for a token
type OrderBook struct {
	Token     uint32       `json:"token"`
	Timestamp time.Time    `json:"timestamp"`
	Bids      []PriceLevel `json:"bids"`
	Asks      []PriceLevel `json:"asks"`
}

// PriceLevel represents a single price level in the order book
type PriceLevel struct {
	Price    float64 `json:"price"`
	Quantity int64   `json:"quantity"`
	Orders   int     `json:"orders"`
}
