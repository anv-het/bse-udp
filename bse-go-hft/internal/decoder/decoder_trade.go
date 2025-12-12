// Package decoder provides trade data decoding for BSE NFCAST Message Type 2017
package decoder

import (
	"encoding/binary"
	"time"

	"bse-go-hft/pkg/domain"
)

// Message Type 2017 - Trade Data Structure
// Based on BSE NFCAST documentation and packet analysis
// Packet size: 132 bytes (36-byte header + 96-byte trade record)

const (
	MsgTypeTrade    = 2017
	TradeRecordSize = 96 // Size of one trade record
)

// Trade record field offsets (ALL LITTLE-ENDIAN)
const (
	TradeOffsetToken          = 0  // Token ID (uint32 LE)
	TradeOffsetTradeNumber    = 4  // Trade Number (uint32 LE)
	TradeOffsetTradePrice     = 8  // Trade Price (int32 LE, in paise)
	TradeOffsetTradeQty       = 12 // Trade Quantity (uint32 LE)
	TradeOffsetTradeTimeHour  = 16 // Trade Time - Hour (uint16 LE)
	TradeOffsetTradeTimeMin   = 18 // Trade Time - Minute (uint16 LE)
	TradeOffsetTradeTimeSec   = 20 // Trade Time - Second (uint16 LE)
	TradeOffsetTradeTimeMS    = 22 // Trade Time - Millisecond (uint16 LE)
	TradeOffsetBuyOrderNum    = 24 // Buy Order Number (uint64 LE)
	TradeOffsetSellOrderNum   = 32 // Sell Order Number (uint64 LE)
	TradeOffsetTradeType      = 40 // Trade Type (uint16 LE)
	TradeOffsetTradingSession = 42 // Trading Session (uint16 LE)
	TradeOffsetSequence       = 44 // Sequence Number (uint32 LE)
	TradeOffsetReserved1      = 48 // Reserved Field 1 (uint32 LE)
	TradeOffsetReserved2      = 52 // Reserved Field 2 (uint32 LE)
)

// Trade type constants
const (
	TradeTypeBuy     = 1
	TradeTypeSell    = 2
	TradeTypeAuction = 3
)

// Trading session constants
const (
	SessionPreOpen   = 1
	SessionNormal    = 2
	SessionClosing   = 3
	SessionPostClose = 4
)

// DecodeMsgType2017 decodes Message Type 2017 (Trade Data) packets
// Packet structure: 36-byte header + 96-byte trade record
func (d *Decoder) DecodeMsgType2017(packet []byte, length int) ([]*domain.TradeData, error) {
	// Validate packet size
	if length < HeaderSize+TradeRecordSize {
		return nil, ErrPacketTooSmall
	}

	// Parse header
	header := d.parseHeader(packet)
	if header.MessageType != MsgTypeTrade {
		return nil, ErrInvalidHeader
	}

	// Calculate number of trade records in packet
	dataLength := length - HeaderSize
	numRecords := dataLength / TradeRecordSize

	// Parse trade records
	trades := make([]*domain.TradeData, 0, numRecords)
	offset := HeaderSize

	for i := 0; i < numRecords; i++ {
		if offset+TradeRecordSize > length {
			break // Incomplete record
		}

		record := packet[offset : offset+TradeRecordSize]
		trade := d.parseTradeRecord(record, header.Timestamp)

		// Skip empty/invalid trades (token = 0)
		if trade.Token != 0 {
			trades = append(trades, trade)
		}

		offset += TradeRecordSize
	}

	return trades, nil
}

// parseTradeRecord parses a single trade record from the packet
func (d *Decoder) parseTradeRecord(record []byte, timestamp time.Time) *domain.TradeData {
	trade := &domain.TradeData{
		MessageType: MsgTypeTrade,
		Timestamp:   timestamp,
	}

	// Parse token
	trade.Token = binary.LittleEndian.Uint32(record[TradeOffsetToken:])

	// Parse trade number
	trade.TradeNumber = binary.LittleEndian.Uint32(record[TradeOffsetTradeNumber:])

	// Parse trade price (in paise, convert to rupees)
	priceInPaise := int32(binary.LittleEndian.Uint32(record[TradeOffsetTradePrice:]))
	trade.TradePrice = float64(priceInPaise) / 100.0

	// Parse trade quantity
	trade.TradeQuantity = binary.LittleEndian.Uint32(record[TradeOffsetTradeQty:])

	// Parse trade time
	hour := binary.LittleEndian.Uint16(record[TradeOffsetTradeTimeHour:])
	minute := binary.LittleEndian.Uint16(record[TradeOffsetTradeTimeMin:])
	second := binary.LittleEndian.Uint16(record[TradeOffsetTradeTimeSec:])
	millisecond := binary.LittleEndian.Uint16(record[TradeOffsetTradeTimeMS:])

	now := time.Now()
	trade.TradeTime = time.Date(
		now.Year(), now.Month(), now.Day(),
		int(hour), int(minute), int(second), int(millisecond)*1000000,
		time.Local,
	)

	// Parse buy order number
	trade.BuyOrderNumber = binary.LittleEndian.Uint64(record[TradeOffsetBuyOrderNum:])

	// Parse sell order number
	trade.SellOrderNumber = binary.LittleEndian.Uint64(record[TradeOffsetSellOrderNum:])

	// Parse trade type
	tradeType := binary.LittleEndian.Uint16(record[TradeOffsetTradeType:])
	trade.TradeType = getTradeTypeString(tradeType)

	// Parse trading session
	tradingSession := binary.LittleEndian.Uint16(record[TradeOffsetTradingSession:])
	trade.TradingSession = getTradingSessionString(tradingSession)

	// Parse sequence number
	trade.SequenceNumber = binary.LittleEndian.Uint32(record[TradeOffsetSequence:])

	// Parse reserved fields
	trade.Reserved1 = binary.LittleEndian.Uint32(record[TradeOffsetReserved1:])
	trade.Reserved2 = binary.LittleEndian.Uint32(record[TradeOffsetReserved2:])

	return trade
}

// getTradeTypeString converts trade type code to string
func getTradeTypeString(tradeType uint16) string {
	switch tradeType {
	case TradeTypeBuy:
		return "BUY"
	case TradeTypeSell:
		return "SELL"
	case TradeTypeAuction:
		return "AUCTION"
	default:
		return "UNKNOWN"
	}
}

// getTradingSessionString converts trading session code to string
func getTradingSessionString(session uint16) string {
	switch session {
	case SessionPreOpen:
		return "PRE_OPEN"
	case SessionNormal:
		return "NORMAL"
	case SessionClosing:
		return "CLOSING"
	case SessionPostClose:
		return "POST_CLOSE"
	default:
		return "UNKNOWN"
	}
}
