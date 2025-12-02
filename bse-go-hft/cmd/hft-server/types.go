// Package main - BSE HFT Server Types
// This file contains data structures and types
package main

import (
	"sync"
)

// Contract represents a tradeable instrument from either BhavCopy or Contract Master
type Contract struct {
	Token      uint32  // Token/ScripCode
	Symbol     string  // Symbol name
	SymbolName string  // Full symbol name (same as Symbol for EQ)
	Series     string  // Series (EQ, FO, etc.)
	Segment    string  // Segment (EQ, FO)
	StrikeRate float64 // Strike price (for options)
	PrevClose  float64 // Previous close price
	Source     string  // Source file (BhavCopy/Contract)
}

// TokenMap is a thread-safe map of token to contract
type TokenMap struct {
	data sync.Map
}

// NewTokenMap creates a new TokenMap
func NewTokenMap() *TokenMap {
	return &TokenMap{}
}

// Set adds or updates a token mapping
func (tm *TokenMap) Set(token uint32, contract *Contract) {
	tm.data.Store(token, contract)
}

// Get retrieves a contract by token
func (tm *TokenMap) Get(token uint32) (*Contract, bool) {
	if v, ok := tm.data.Load(token); ok {
		return v.(*Contract), true
	}
	return nil, false
}

// Len returns the number of tokens in the map
func (tm *TokenMap) Len() int {
	count := 0
	tm.data.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// Quote represents a market data quote
type Quote struct {
	Token        uint32
	Symbol       string
	SymbolName   string
	Series       string
	Segment      string
	LTP          float64
	Volume       int64
	BidPrice     float64
	BidQty       int64
	AskPrice     float64
	AskQty       int64
	OpenPrice    float64
	HighPrice    float64
	LowPrice     float64
	ClosePrice   float64
	TotalBuyQty  int64
	TotalSellQty int64
	Timestamp    int64
	RecvTime     int64
}

// LatencyPercentiles holds latency percentile values in microseconds
type LatencyPercentiles struct {
	P50  float64
	P90  float64
	P99  float64
	P999 float64
}

// MissedTokenInfo tracks missed token occurrences
type MissedTokenInfo struct {
	Token uint32
	Count int64
}
