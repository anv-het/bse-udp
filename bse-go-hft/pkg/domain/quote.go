// Package domain contains domain models for BSE market data
package domain

import "time"

// Quote represents a market data quote
type Quote struct {
	Token       uint32    `json:"token"`
	Symbol      string    `json:"symbol"`
	Expiry      string    `json:"expiry"`
	StrikePrice float64   `json:"strike_price"`
	OptionType  string    `json:"option_type"` // CE/PE/FUT
	LTP         float64   `json:"ltp"`
	PrevClose   float64   `json:"prev_close"`
	Open        float64   `json:"open"`
	High        float64   `json:"high"`
	Low         float64   `json:"low"`
	Volume      int64     `json:"volume"`
	Timestamp   time.Time `json:"timestamp"`
	SequenceNum uint32    `json:"sequence_num"`

	// Best 5 Bid/Ask
	BidPrices [5]float64 `json:"bid_prices"`
	BidQtys   [5]int64   `json:"bid_qtys"`
	AskPrices [5]float64 `json:"ask_prices"`
	AskQtys   [5]int64   `json:"ask_qtys"`
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
