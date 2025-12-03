// Package domain contains core domain types for BSE HFT system
package domain

import (
	"sync"
)

// Contract represents a tradeable instrument from BhavCopy or Contract Master
type Contract struct {
	Token          uint32  `json:"token"`
	Symbol         string  `json:"symbol"`
	SymbolName     string  `json:"symbol_name"`
	InstrumentType string  `json:"instrument_type"` // EQ, SO, IO, SF, IF
	Expiry         string  `json:"expiry"`
	OptionType     string  `json:"option_type"` // CE, PE, FUT, or empty for EQ
	StrikePrice    float64 `json:"strike_price"`
	LotSize        int     `json:"lot_size"`
	Segment        string  `json:"segment"` // EQ or FO
	PrevClose      float64 `json:"prev_close"`
	Source         string  `json:"source"` // BhavCopy or ContractMaster
}

// TokenMap is a thread-safe map of token to contract
// Uses sync.Map for lock-free reads (HFT optimized)
type TokenMap struct {
	data  sync.Map
	count int64
	mu    sync.RWMutex // Only for count tracking
}

// NewTokenMap creates a new TokenMap
func NewTokenMap() *TokenMap {
	return &TokenMap{}
}

// Set adds or updates a token mapping
func (tm *TokenMap) Set(token uint32, contract *Contract) {
	_, loaded := tm.data.LoadOrStore(token, contract)
	if !loaded {
		tm.mu.Lock()
		tm.count++
		tm.mu.Unlock()
	} else {
		// Update existing
		tm.data.Store(token, contract)
	}
}

// Get retrieves a contract by token (lock-free read)
func (tm *TokenMap) Get(token uint32) (*Contract, bool) {
	if v, ok := tm.data.Load(token); ok {
		return v.(*Contract), true
	}
	return nil, false
}

// Len returns the number of tokens in the map
func (tm *TokenMap) Len() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return int(tm.count)
}

// Range iterates over all contracts
func (tm *TokenMap) Range(f func(token uint32, contract *Contract) bool) {
	tm.data.Range(func(key, value interface{}) bool {
		return f(key.(uint32), value.(*Contract))
	})
}

// GetSymbol returns the symbol for a token, or empty string if not found
func (tm *TokenMap) GetSymbol(token uint32) string {
	if c, ok := tm.Get(token); ok {
		return c.Symbol
	}
	return ""
}
