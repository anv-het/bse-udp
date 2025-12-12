// Package decoder provides decoders for BSE NFCAST Index messages (2011/2012)
package decoder

import (
	"bse-go-hft/pkg/domain"
	"encoding/binary"
	"fmt"
	"time"
)

// Message type constants for Index Data
const (
	MsgTypeIndexBroadcast1 = 2011 // Critical indices (1 second frequency)
	MsgTypeIndexBroadcast2 = 2012 // Other indices (8 second frequency)
)

// Index Data Record Structure (based on ACTUAL packet analysis from BSE feed)
// Structure verified by comparing with real market data from BSE website
const (
	// Index record field offsets (CORRECTED based on real BSE market data)
	IndexOffsetCode      = 0  // Index Code (uint32, Little-Endian)
	IndexOffsetHigh      = 4  // High Value (int32, Little-Endian, scaled by 100)
	IndexOffsetLow       = 8  // Low Value (int32, Little-Endian, scaled by 100)
	IndexOffsetOpen      = 12 // Open Value (int32, Little-Endian, scaled by 100)
	IndexOffsetPrevClose = 16 // Previous Close (int32, Little-Endian, scaled by 100)
	IndexOffsetLTP       = 20 // LTP - Last Traded Price (int32, Little-Endian, scaled by 100) - CHANGES EVERY TICK
	IndexOffsetName      = 24 // Index Name (12-byte string, space padded)
	IndexOffsetReserved  = 36 // Reserved/Additional data (4 bytes)

	// Index record size (from packet analysis: 120 bytes / 3 records = 40 bytes)
	IndexRecordSize = 40 // 40 bytes per index record
)

// DecodeMsgType2011 decodes Index Broadcast 1 message (critical indices, 1 sec frequency)
func (d *Decoder) DecodeMsgType2011(packet []byte, length int) ([]*domain.IndexData, error) {
	if length < HeaderSize {
		d.stats.DecodeErrors++
		return nil, ErrPacketTooSmall
	}

	// Parse header
	header := d.parseHeader(packet)

	// Validate message type
	if header.MessageType != MsgTypeIndexBroadcast1 {
		d.stats.InvalidHeaders++
		return nil, fmt.Errorf("invalid message type for 2011: got %d", header.MessageType)
	}

	// Calculate number of index records
	dataLen := length - HeaderSize
	numRecords := dataLen / IndexRecordSize

	if numRecords == 0 {
		return nil, fmt.Errorf("no index records in packet")
	}

	// Parse all index records
	indices := make([]*domain.IndexData, 0, numRecords)
	offset := HeaderSize

	for i := 0; i < numRecords; i++ {
		if offset+IndexRecordSize > length {
			break
		}

		indexData := d.parseIndexRecord(packet[offset:offset+IndexRecordSize], header.Timestamp, 2011)
		if indexData != nil {
			indices = append(indices, indexData)
		}
		offset += IndexRecordSize
	}

	d.stats.PacketsDecoded++
	d.stats.RecordsDecoded += uint64(len(indices))

	return indices, nil
}

// DecodeMsgType2012 decodes Index Broadcast 2 message (other indices, 8 sec frequency)
func (d *Decoder) DecodeMsgType2012(packet []byte, length int) ([]*domain.IndexData, error) {
	if length < HeaderSize {
		d.stats.DecodeErrors++
		return nil, ErrPacketTooSmall
	}

	// Parse header
	header := d.parseHeader(packet)

	// Validate message type
	if header.MessageType != MsgTypeIndexBroadcast2 {
		d.stats.InvalidHeaders++
		return nil, fmt.Errorf("invalid message type for 2012: got %d", header.MessageType)
	}

	// Calculate number of index records
	dataLen := length - HeaderSize
	numRecords := dataLen / IndexRecordSize

	if numRecords == 0 {
		return nil, fmt.Errorf("no index records in packet")
	}

	// Parse all index records
	indices := make([]*domain.IndexData, 0, numRecords)
	offset := HeaderSize

	for i := 0; i < numRecords; i++ {
		if offset+IndexRecordSize > length {
			break
		}

		indexData := d.parseIndexRecord(packet[offset:offset+IndexRecordSize], header.Timestamp, 2012)
		if indexData != nil {
			indices = append(indices, indexData)
		}
		offset += IndexRecordSize
	}

	d.stats.PacketsDecoded++
	d.stats.RecordsDecoded += uint64(len(indices))

	return indices, nil
}

// parseIndexRecord parses a single index record (40 bytes)
// All fields are LITTLE-ENDIAN as per BSE specification
// Structure based on actual packet analysis
func (d *Decoder) parseIndexRecord(data []byte, timestamp time.Time, msgType uint16) *domain.IndexData {
	if len(data) < IndexRecordSize {
		return nil
	}

	// Index Code: uint32 Little-Endian at offset 0
	indexCode := binary.LittleEndian.Uint32(data[IndexOffsetCode : IndexOffsetCode+4])

	// Skip empty records (index code 0 indicates NULL slot)
	if indexCode == 0 {
		return nil
	}

	index := &domain.IndexData{
		MessageType: msgType,
		Timestamp:   timestamp,
		IndexCode:   indexCode,
	}

	// High Value: int32 Little-Endian at offset 4 (scaled by 100)
	highValue := int32(binary.LittleEndian.Uint32(data[IndexOffsetHigh : IndexOffsetHigh+4]))
	index.HighValue = float64(highValue) / 100.0

	// Low Value: int32 Little-Endian at offset 8 (scaled by 100)
	lowValue := int32(binary.LittleEndian.Uint32(data[IndexOffsetLow : IndexOffsetLow+4]))
	index.LowValue = float64(lowValue) / 100.0

	// Open Value: int32 Little-Endian at offset 12 (scaled by 100)
	openValue := int32(binary.LittleEndian.Uint32(data[IndexOffsetOpen : IndexOffsetOpen+4]))
	index.OpenValue = float64(openValue) / 100.0

	// Previous Close: int32 Little-Endian at offset 16 (scaled by 100)
	prevClose := int32(binary.LittleEndian.Uint32(data[IndexOffsetPrevClose : IndexOffsetPrevClose+4]))
	index.PrevClose = float64(prevClose) / 100.0

	// LTP (Last Traded Price): int32 Little-Endian at offset 20 (scaled by 100)
	// This is the current/live index value that changes with every tick
	ltp := int32(binary.LittleEndian.Uint32(data[IndexOffsetLTP : IndexOffsetLTP+4]))
	index.IndexValue = float64(ltp) / 100.0

	// Index Name: 12-byte string at offset 24 (space padded)
	nameBytes := data[IndexOffsetName : IndexOffsetName+12]
	// Trim spaces and NULL bytes
	index.IndexName = string(nameBytes)
	// Remove trailing spaces and nulls
	for len(index.IndexName) > 0 && (index.IndexName[len(index.IndexName)-1] == ' ' || index.IndexName[len(index.IndexName)-1] == 0) {
		index.IndexName = index.IndexName[:len(index.IndexName)-1]
	}

	// Calculate Net Change and Percent Change
	// Net_Change = Current Value (LTP) - Previous Close
	index.NetChange = index.IndexValue - index.PrevClose

	// Percent_Change = (Net_Change / Previous Close) * 100
	// Avoid division by zero
	if index.PrevClose != 0 {
		index.PercentChange = (index.NetChange / index.PrevClose) * 100.0
	} else {
		index.PercentChange = 0.0
	}

	// Store raw data for debugging
	index.RawData = make([]byte, IndexRecordSize)
	copy(index.RawData, data[:IndexRecordSize])

	return index
}

// IsIndexPacket returns true if packet is index data (2011 or 2012)
func IsIndexPacket(packet []byte) bool {
	if len(packet) < 10 {
		return false
	}
	msgType := binary.LittleEndian.Uint16(packet[8:10])
	return msgType == MsgTypeIndexBroadcast1 || msgType == MsgTypeIndexBroadcast2
}
