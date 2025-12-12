// Package domain provides domain models for BSE market data
package domain

import (
	"fmt"
	"time"
)

// TradeData represents Message Type 2017 - Individual Trade Data
// This structure represents actual trades executed on the exchange
type TradeData struct {
	// Header information
	MessageType uint16    // 2017 for Trade Data
	Timestamp   time.Time // When the message was received

	// Trade identification
	Token       uint32 // Instrument/Contract Token
	TradeNumber uint32 // Unique trade identifier

	// Trade execution details
	TradePrice    float64   // Execution price in Rupees (converted from paise)
	TradeQuantity uint32    // Number of shares/contracts traded
	TradeTime     time.Time // Time when trade was executed (HH:MM:SS:MS)

	// Market context
	BuyOrderNumber  uint64 // Buy order number involved in trade
	SellOrderNumber uint64 // Sell order number involved in trade

	// Trade flags
	TradeType      string // "BUY" or "SELL" or "AUCTION"
	TradingSession string // Current trading session

	// Additional metadata
	SequenceNumber uint32 // Sequence number for ordering
	Reserved1      uint32 // Reserved field 1
	Reserved2      uint32 // Reserved field 2
}

// TradeCSVHeader returns the CSV header for trade data
func TradeCSVHeader() string {
	return "Timestamp,Message_Type,Token,Trade_Number,Trade_Price,Trade_Quantity," +
		"Trade_Time,Buy_Order_Number,Sell_Order_Number,Trade_Type,Trading_Session,Sequence_Number"
}

// ToCSVRow converts TradeData to CSV row
func (t *TradeData) ToCSVRow() string {
	return fmt.Sprintf("%s,2017,%d,%d,%.2f,%d,%s,%d,%d,%s,%s,%d",
		t.Timestamp.Format("2006-01-02 15:04:05.000"),
		t.Token,
		t.TradeNumber,
		t.TradePrice,
		t.TradeQuantity,
		t.TradeTime.Format("15:04:05.000"),
		t.BuyOrderNumber,
		t.SellOrderNumber,
		t.TradeType,
		t.TradingSession,
		t.SequenceNumber,
	)
}
