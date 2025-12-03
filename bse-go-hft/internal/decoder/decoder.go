// Package decoder provides zero-copy packet decoding for BSE NFCAST protocol
// Based on COMPLETE_PACKET_STRUCTURE_ANALYSIS.md - ALL FIELDS ARE LITTLE-ENDIAN
package decoder

import (
	"encoding/binary"
	"errors"
	"time"
)

// Errors
var (
	ErrPacketTooSmall = errors.New("packet too small")
	ErrInvalidHeader  = errors.New("invalid packet header")
)

// Constants - BSE NFCAST Protocol
const (
	HeaderSize = 36
	RecordSize = 264 // BSE uses 264-byte records (NULL padded)
	MaxRecords = 6   // Max 6 records per packet (1620 bytes max)

	// Message types
	MsgTypeEquity     = 2020
	MsgTypeDerivative = 2021
)

// Record field offsets (ALL LITTLE-ENDIAN per BSE spec)
const (
	OffsetToken     = 0   // Token ID (uint32 LE)
	OffsetOpen      = 4   // Open Price (int32 LE, paise)
	OffsetPrevClose = 8   // Previous Close (int32 LE, paise)
	OffsetHigh      = 12  // High Price (int32 LE, paise)
	OffsetLow       = 16  // Low Price (int32 LE, paise)
	OffsetVolume    = 24  // Volume (int32 LE)
	OffsetTurnover  = 28  // Turnover in Lakhs (uint32 LE)
	OffsetLotSize   = 32  // Lot Size (uint32 LE)
	OffsetLTP       = 36  // LTP (int32 LE, paise)
	OffsetSequence  = 44  // Sequence Number (uint32 LE)
	OffsetATP       = 84  // Average Traded Price (int32 LE, paise)
	OffsetOrderBook = 104 // Order Book starts here (160 bytes, 5 levels)
)

// Header represents the decoded packet header
type Header struct {
	FormatID    uint16
	MessageType uint16
	Hour        uint16
	Minute      uint16
	Second      uint16
	Millisecond uint16
	Timestamp   time.Time
}

// Record represents a decoded market data record
type Record struct {
	Token       uint32
	Open        float64 // Rupees
	PrevClose   float64 // Rupees
	High        float64 // Rupees
	Low         float64 // Rupees
	LTP         float64 // Rupees
	ATP         float64 // Average Traded Price (Rupees)
	Volume      int64
	Turnover    float64 // In Lakhs
	LotSize     uint32
	SequenceNum uint32
	Timestamp   time.Time

	// Best 5 levels (interleaved bid/ask)
	BidPrices [5]float64
	BidQtys   [5]int64
	AskPrices [5]float64
	AskQtys   [5]int64
}

// Decoder performs zero-copy packet decoding
type Decoder struct {
	// Pre-allocated records to avoid allocations
	records [MaxRecords]Record

	// Statistics
	stats DecoderStats
}

// DecoderStats holds decoder statistics
type DecoderStats struct {
	PacketsDecoded uint64
	RecordsDecoded uint64
	DecodeErrors   uint64
	InvalidHeaders uint64
	SkippedRecords uint64
}

// NewDecoder creates a new decoder
func NewDecoder() *Decoder {
	return &Decoder{}
}

// Decode decodes a packet and returns records
// BSE packet format: 36-byte header + N×264-byte records
func (d *Decoder) Decode(packet []byte, length int) ([]Record, int, error) {
	if length < HeaderSize {
		d.stats.DecodeErrors++
		return nil, 0, ErrPacketTooSmall
	}

	// Parse header
	header := d.parseHeader(packet)

	// Validate message type (2020=EQ, 2021=FO)
	if header.MessageType != MsgTypeEquity && header.MessageType != MsgTypeDerivative {
		d.stats.InvalidHeaders++
		return nil, 0, ErrInvalidHeader
	}

	// Calculate number of records based on packet size
	dataLen := length - HeaderSize
	numRecords := dataLen / RecordSize
	if numRecords > MaxRecords {
		numRecords = MaxRecords
	}

	// Parse records
	validCount := 0
	offset := HeaderSize

	for i := 0; i < numRecords; i++ {
		if offset+RecordSize > length {
			break
		}

		if d.parseRecord(packet[offset:offset+RecordSize], &d.records[validCount], header.Timestamp) {
			validCount++
		} else {
			d.stats.SkippedRecords++
		}
		offset += RecordSize
	}

	d.stats.PacketsDecoded++
	d.stats.RecordsDecoded += uint64(validCount)

	return d.records[:validCount], validCount, nil
}

// parseHeader parses the packet header with mixed endianness
func (d *Decoder) parseHeader(packet []byte) Header {
	// BSE Header: FormatID=BE, rest=LE (empirically validated)
	header := Header{
		FormatID:    binary.BigEndian.Uint16(packet[4:6]),
		MessageType: binary.LittleEndian.Uint16(packet[8:10]),
		Hour:        binary.LittleEndian.Uint16(packet[20:22]),
		Minute:      binary.LittleEndian.Uint16(packet[22:24]),
		Second:      binary.LittleEndian.Uint16(packet[24:26]),
		Millisecond: binary.LittleEndian.Uint16(packet[26:28]),
	}

	// Use system time for HFT accuracy (BSE header time is often invalid/delayed)
	// This matches the Python decoder behavior which also uses system time
	header.Timestamp = time.Now()

	return header
}

// parseRecord parses a single 264-byte record
// ALL fields are LITTLE-ENDIAN per BSE NFCAST specification
// Returns false if record should be skipped (empty token)
func (d *Decoder) parseRecord(data []byte, record *Record, timestamp time.Time) bool {
	// Token: Little-Endian uint32 at offset 0
	token := binary.LittleEndian.Uint32(data[OffsetToken : OffsetToken+4])

	// Skip empty records (token 0 or 1 indicates NULL slot)
	if token <= 1 {
		return false
	}

	record.Token = token
	record.Timestamp = timestamp

	// Open: Little-Endian int32 at offset 4 (paise → rupees)
	open := int32(binary.LittleEndian.Uint32(data[OffsetOpen : OffsetOpen+4]))
	record.Open = float64(open) / 100.0

	// Previous Close: Little-Endian int32 at offset 8 (paise → rupees)
	prevClose := int32(binary.LittleEndian.Uint32(data[OffsetPrevClose : OffsetPrevClose+4]))
	record.PrevClose = float64(prevClose) / 100.0

	// High: Little-Endian int32 at offset 12 (paise → rupees)
	high := int32(binary.LittleEndian.Uint32(data[OffsetHigh : OffsetHigh+4]))
	record.High = float64(high) / 100.0

	// Low: Little-Endian int32 at offset 16 (paise → rupees)
	low := int32(binary.LittleEndian.Uint32(data[OffsetLow : OffsetLow+4]))
	record.Low = float64(low) / 100.0

	// Volume: Little-Endian int32 at offset 24
	volume := int32(binary.LittleEndian.Uint32(data[OffsetVolume : OffsetVolume+4]))
	record.Volume = int64(volume)

	// Turnover: Little-Endian uint32 at offset 28 (in lakhs)
	turnover := binary.LittleEndian.Uint32(data[OffsetTurnover : OffsetTurnover+4])
	record.Turnover = float64(turnover)

	// Lot Size: Little-Endian uint32 at offset 32
	record.LotSize = binary.LittleEndian.Uint32(data[OffsetLotSize : OffsetLotSize+4])

	// LTP: Little-Endian int32 at offset 36 (paise → rupees)
	ltp := int32(binary.LittleEndian.Uint32(data[OffsetLTP : OffsetLTP+4]))
	record.LTP = float64(ltp) / 100.0

	// Sequence Number: Little-Endian uint32 at offset 44
	record.SequenceNum = binary.LittleEndian.Uint32(data[OffsetSequence : OffsetSequence+4])

	// ATP: Little-Endian int32 at offset 84 (paise → rupees)
	atp := int32(binary.LittleEndian.Uint32(data[OffsetATP : OffsetATP+4]))
	record.ATP = float64(atp) / 100.0

	// Parse order book (5 levels, interleaved bid/ask)
	d.parseOrderBook(data, record)

	return true
}

// parseOrderBook parses the 5-level order book (160 bytes at offset 104)
// Structure: Each level = 32 bytes (16-byte Bid + 16-byte Ask)
// Each block: Price(4B) + Qty(4B) + Flag(4B) + Reserved(4B) = 16 bytes
// ALL LITTLE-ENDIAN
func (d *Decoder) parseOrderBook(data []byte, record *Record) {
	for level := 0; level < 5; level++ {
		// Calculate offsets for this level
		// Level N: Bid at 104 + (N*32), Ask at 104 + (N*32) + 16
		bidOffset := OffsetOrderBook + (level * 32)
		askOffset := bidOffset + 16

		// Ensure we have enough data
		if askOffset+16 > len(data) {
			break
		}

		// Bid: Price (offset+0) and Quantity (offset+4) - Little-Endian
		bidPrice := int32(binary.LittleEndian.Uint32(data[bidOffset : bidOffset+4]))
		bidQty := int32(binary.LittleEndian.Uint32(data[bidOffset+4 : bidOffset+8]))
		record.BidPrices[level] = float64(bidPrice) / 100.0
		record.BidQtys[level] = int64(bidQty)

		// Ask: Price (offset+0) and Quantity (offset+4) - Little-Endian
		askPrice := int32(binary.LittleEndian.Uint32(data[askOffset : askOffset+4]))
		askQty := int32(binary.LittleEndian.Uint32(data[askOffset+4 : askOffset+8]))
		record.AskPrices[level] = float64(askPrice) / 100.0
		record.AskQtys[level] = int64(askQty)
	}
}

// GetStats returns decoder statistics
func (d *Decoder) GetStats() DecoderStats {
	return d.stats
}

// ResetStats resets decoder statistics
func (d *Decoder) ResetStats() {
	d.stats = DecoderStats{}
}

// GetMessageType returns the message type from a packet header
func GetMessageType(packet []byte) uint16 {
	if len(packet) < 10 {
		return 0
	}
	return binary.LittleEndian.Uint16(packet[8:10])
}

// IsEquityPacket returns true if packet is equity (2020)
func IsEquityPacket(packet []byte) bool {
	return GetMessageType(packet) == MsgTypeEquity
}

// IsDerivativePacket returns true if packet is derivative (2021)
func IsDerivativePacket(packet []byte) bool {
	return GetMessageType(packet) == MsgTypeDerivative
}
