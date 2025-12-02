// Package decoder provides zero-copy packet decoding for BSE NFCAST protocol
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
	ErrInvalidToken   = errors.New("invalid token")
)

// Constants
const (
	HeaderSize = 36
	RecordSize = 66
	MaxRecords = 8

	// Message types
	MsgTypeEquity     = 2020
	MsgTypeDerivative = 2021
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
	PrevClose   float64 // Rupees
	LTP         float64 // Rupees
	Volume      int64
	SequenceNum uint32
	Timestamp   time.Time

	// Decompressed fields (populated by decompressor)
	Open float64
	High float64
	Low  float64

	// Best 5 levels
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
// Uses pre-allocated records slice to avoid allocations
func (d *Decoder) Decode(packet []byte, length int) ([]Record, int, error) {
	if length < HeaderSize {
		d.stats.DecodeErrors++
		return nil, 0, ErrPacketTooSmall
	}

	// Parse header
	header := d.parseHeader(packet)
	if !d.validateHeader(header) {
		d.stats.InvalidHeaders++
		return nil, 0, ErrInvalidHeader
	}

	// Calculate number of records
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
	// BSE uses mixed endianness - empirically validated
	// FormatID: Big-Endian
	// MessageType, Hour, Minute, Second, Millisecond: Little-Endian

	header := Header{
		FormatID:    binary.BigEndian.Uint16(packet[4:6]),
		MessageType: binary.LittleEndian.Uint16(packet[8:10]),
		Hour:        binary.LittleEndian.Uint16(packet[20:22]),
		Minute:      binary.LittleEndian.Uint16(packet[22:24]),
		Second:      binary.LittleEndian.Uint16(packet[24:26]),
		Millisecond: binary.LittleEndian.Uint16(packet[26:28]),
	}

	// Build timestamp
	now := time.Now()
	header.Timestamp = time.Date(
		now.Year(), now.Month(), now.Day(),
		int(header.Hour), int(header.Minute), int(header.Second),
		int(header.Millisecond)*1e6, now.Location(),
	)

	return header
}

// validateHeader validates the packet header
func (d *Decoder) validateHeader(header Header) bool {
	// Check format ID (0x0234 for NFCAST)
	if header.FormatID != 0x0234 {
		return false
	}

	// Check message type
	if header.MessageType != MsgTypeEquity && header.MessageType != MsgTypeDerivative {
		return false
	}

	// Validate time components
	if header.Hour > 23 || header.Minute > 59 || header.Second > 59 {
		return false
	}

	return true
}

// parseRecord parses a single record from the packet
// Returns false if record should be skipped (empty token)
func (d *Decoder) parseRecord(data []byte, record *Record, timestamp time.Time) bool {
	// Token: Little-Endian (offset 0-3)
	token := binary.LittleEndian.Uint32(data[0:4])

	// Skip empty records (token 0 or 1)
	if token <= 1 {
		return false
	}

	record.Token = token
	record.Timestamp = timestamp

	// PrevClose: Big-Endian (offset 8-11), in paise
	prevClose := int32(binary.BigEndian.Uint32(data[8:12]))
	record.PrevClose = float64(prevClose) / 100.0

	// LTP: Big-Endian (offset 20-23), in paise
	ltp := int32(binary.BigEndian.Uint32(data[20:24]))
	record.LTP = float64(ltp) / 100.0

	// Volume: Big-Endian (offset 24-27)
	volume := int32(binary.BigEndian.Uint32(data[24:28]))
	record.Volume = int64(volume)

	// Sequence number: (offset 44-47) for packet loss detection
	record.SequenceNum = binary.LittleEndian.Uint32(data[44:48])

	return true
}

// GetStats returns decoder statistics
func (d *Decoder) GetStats() DecoderStats {
	return d.stats
}

// ResetStats resets decoder statistics
func (d *Decoder) ResetStats() {
	d.stats = DecoderStats{}
}
