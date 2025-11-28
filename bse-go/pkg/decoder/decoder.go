package decoder

import (
	"encoding/binary"
	"fmt"
	"log"
	"time"
)

// PacketHeader contains parsed BSE packet header fields
type PacketHeader struct {
	FormatID  uint16    `json:"format_id"`
	MsgType   uint16    `json:"msg_type"`
	Timestamp time.Time `json:"timestamp"`
}

// MarketRecord contains parsed market data fields from a single record
type MarketRecord struct {
	Token          uint32     `json:"token"`
	Open           float64    `json:"open"`
	PrevClose      float64    `json:"prev_close"`
	High           float64    `json:"high"`
	Low            float64    `json:"low"`
	Volume         int32      `json:"volume"`
	TurnoverLakhs  uint32     `json:"turnover_lakhs"`
	LotSize        uint32     `json:"lot_size"`
	LTP            float64    `json:"ltp"`
	ATP            float64    `json:"atp"`
	Bid            float64    `json:"bid"`
	Ask            float64    `json:"ask"`
	OrderBook      *OrderBook `json:"order_book,omitempty"`
	SequenceNumber uint32     `json:"sequence_number"`
}

// OrderBook contains 5-level bid/ask depth
type OrderBook struct {
	Bids []OrderLevel `json:"bids"`
	Asks []OrderLevel `json:"asks"`
}

// OrderLevel represents a single price level in the order book
type OrderLevel struct {
	Price    float64 `json:"price"`
	Quantity int32   `json:"quantity"`
	Flag     int32   `json:"flag"`
}

// DecodedPacket contains the fully decoded packet
type DecodedPacket struct {
	Header  PacketHeader   `json:"header"`
	Records []MarketRecord `json:"records"`
}

// PacketDecoder handles BSE NFCAST packet decoding
type PacketDecoder struct {
	segment string // "CM" or "FO"
	stats   struct {
		packetsDecoded int
		decodeErrors   int
		recordsDecoded int
		invalidHeaders int
		emptyRecords   int
	}
}

// NewPacketDecoder creates a new packet decoder
func NewPacketDecoder(segment string) *PacketDecoder {
	return &PacketDecoder{segment: segment}
}

// DecodePacket decodes a raw BSE packet into structured data
func (d *PacketDecoder) DecodePacket(packet []byte) (*DecodedPacket, error) {
	if len(packet) < 36 {
		d.stats.decodeErrors++
		return nil, fmt.Errorf("packet too short: %d bytes", len(packet))
	}

	// Validate leading zeros (offset 0-3 must be 0x00000000)
	if packet[0] != 0 || packet[1] != 0 || packet[2] != 0 || packet[3] != 0 {
		d.stats.invalidHeaders++
		return nil, fmt.Errorf("invalid leading bytes")
	}

	// Format ID (offset 4-5, Little-Endian) - equals packet size
	formatID := binary.LittleEndian.Uint16(packet[4:6])
	if int(formatID) != len(packet) {
		d.stats.invalidHeaders++
		return nil, fmt.Errorf("format ID mismatch: %d vs packet size %d", formatID, len(packet))
	}

	// Message type (offset 8-9, Little-Endian)
	msgType := binary.LittleEndian.Uint16(packet[8:10])

	// Filter for message types 2020 or 2021
	if msgType != 2020 && msgType != 2021 {
		return nil, nil // Not a market data packet
	}

	// Always use system time for timestamp (BSE header timestamps are often invalid)
	timestamp := time.Now()

	header := PacketHeader{
		FormatID:  formatID,
		MsgType:   msgType,
		Timestamp: timestamp,
	}

	// Determine number of records based on packet size
	// Pattern: 36 (header) + N×264 (records)
	recordSize := 264
	headerSize := 36
	dataSize := len(packet) - headerSize
	numRecords := dataSize / recordSize

	records := make([]MarketRecord, 0, numRecords)

	for i := 0; i < numRecords; i++ {
		offset := headerSize + i*recordSize
		if offset+264 > len(packet) {
			break
		}

		record, err := d.parseRecord(packet[offset : offset+recordSize])
		if err != nil {
			continue
		}
		if record != nil {
			records = append(records, *record)
			d.stats.recordsDecoded++
		} else {
			d.stats.emptyRecords++
		}
	}

	d.stats.packetsDecoded++

	return &DecodedPacket{
		Header:  header,
		Records: records,
	}, nil
}

// parseRecord parses a 264-byte market data record
func (d *PacketDecoder) parseRecord(data []byte) (*MarketRecord, error) {
	if len(data) < 40 {
		return nil, fmt.Errorf("record too short: %d bytes", len(data))
	}

	// Token (offset 0-3, Little-Endian)
	token := binary.LittleEndian.Uint32(data[0:4])
	if token == 0 {
		return nil, nil // Empty slot
	}

	// Parse prices and fields (all Little-Endian, in paise)
	// Offset +4: Open Price
	open := float64(int32(binary.LittleEndian.Uint32(data[4:8]))) / 100.0

	// Offset +8: Prev Close
	prevClose := float64(int32(binary.LittleEndian.Uint32(data[8:12]))) / 100.0

	// Offset +12: High Price
	high := float64(int32(binary.LittleEndian.Uint32(data[12:16]))) / 100.0

	// Offset +16: Low Price
	low := float64(int32(binary.LittleEndian.Uint32(data[16:20]))) / 100.0

	// Offset +24: Volume
	volume := int32(binary.LittleEndian.Uint32(data[24:28]))

	// Offset +28: Turnover in Lakhs
	var turnoverLakhs uint32
	if len(data) >= 32 {
		turnoverLakhs = binary.LittleEndian.Uint32(data[28:32])
	}

	// Offset +32: Lot Size
	var lotSize uint32
	if len(data) >= 36 {
		lotSize = binary.LittleEndian.Uint32(data[32:36])
	}

	// Offset +36: LTP - Last Traded Price
	ltp := float64(int32(binary.LittleEndian.Uint32(data[36:40]))) / 100.0

	// Offset +44: Sequence Number
	var sequenceNumber uint32
	if len(data) >= 48 {
		sequenceNumber = binary.LittleEndian.Uint32(data[44:48])
	}

	// Offset +84: ATP - Average Traded Price
	var atp float64
	if len(data) >= 88 {
		atp = float64(int32(binary.LittleEndian.Uint32(data[84:88]))) / 100.0
	}

	// Offset +104: Best Bid Price
	var bid float64
	if len(data) >= 108 {
		bid = float64(int32(binary.LittleEndian.Uint32(data[104:108]))) / 100.0
	}

	// Parse order book if we have full 264 bytes
	var orderBook *OrderBook
	var ask float64
	if len(data) >= 264 {
		orderBook = d.parseOrderBook(data)
		if orderBook != nil && len(orderBook.Asks) > 0 {
			ask = orderBook.Asks[0].Price
		}
	}

	return &MarketRecord{
		Token:          token,
		Open:           open,
		PrevClose:      prevClose,
		High:           high,
		Low:            low,
		Volume:         volume,
		TurnoverLakhs:  turnoverLakhs,
		LotSize:        lotSize,
		LTP:            ltp,
		ATP:            atp,
		Bid:            bid,
		Ask:            ask,
		OrderBook:      orderBook,
		SequenceNumber: sequenceNumber,
	}, nil
}

// parseOrderBook parses 5-level order book depth (interleaved bid/ask)
// Structure: Offset 104-263, each level = 32 bytes (16 bid + 16 ask)
func (d *PacketDecoder) parseOrderBook(data []byte) *OrderBook {
	if len(data) < 264 {
		return nil
	}

	bids := make([]OrderLevel, 0, 5)
	asks := make([]OrderLevel, 0, 5)

	// Parse 5 levels (interleaved bid/ask pairs)
	for i := 0; i < 5; i++ {
		// Each level occupies 32 bytes (16 bid + 16 ask)
		bidBase := 104 + (i * 32)
		askBase := bidBase + 16

		if bidBase+16 > len(data) || askBase+16 > len(data) {
			break
		}

		// Parse BID: [Price 4B][Qty 4B][Flag 4B][Unknown 4B]
		bidPrice := float64(int32(binary.LittleEndian.Uint32(data[bidBase:bidBase+4]))) / 100.0
		bidQty := int32(binary.LittleEndian.Uint32(data[bidBase+4 : bidBase+8]))
		bidFlag := int32(binary.LittleEndian.Uint32(data[bidBase+8 : bidBase+12]))

		if bidQty > 0 && bidPrice > 0 {
			bids = append(bids, OrderLevel{
				Price:    bidPrice,
				Quantity: bidQty,
				Flag:     bidFlag,
			})
		}

		// Parse ASK: [Price 4B][Qty 4B][Flag 4B][Unknown 4B]
		askPrice := float64(int32(binary.LittleEndian.Uint32(data[askBase:askBase+4]))) / 100.0
		askQty := int32(binary.LittleEndian.Uint32(data[askBase+4 : askBase+8]))
		askFlag := int32(binary.LittleEndian.Uint32(data[askBase+8 : askBase+12]))

		if askQty > 0 && askPrice > 0 {
			asks = append(asks, OrderLevel{
				Price:    askPrice,
				Quantity: askQty,
				Flag:     askFlag,
			})
		}
	}

	return &OrderBook{
		Bids: bids,
		Asks: asks,
	}
}

// GetStats returns decoder statistics
func (d *PacketDecoder) GetStats() map[string]int {
	return map[string]int{
		"packets_decoded": d.stats.packetsDecoded,
		"decode_errors":   d.stats.decodeErrors,
		"records_decoded": d.stats.recordsDecoded,
		"invalid_headers": d.stats.invalidHeaders,
		"empty_records":   d.stats.emptyRecords,
	}
}

// LogStats logs decoder statistics
func (d *PacketDecoder) LogStats() {
	log.Printf("[%s] Decoder Stats: packets=%d, records=%d, errors=%d",
		d.segment, d.stats.packetsDecoded, d.stats.recordsDecoded, d.stats.decodeErrors)
}
