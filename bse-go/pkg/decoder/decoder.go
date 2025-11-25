package decoder

import (
	"encoding/binary"
	"fmt"
	"log"
	"time"
)

type PacketHeader struct {
	FormatID  uint16    `json:"format_id"`
	MsgType   uint16    `json:"msg_type"`
	Timestamp time.Time `json:"timestamp"`
}

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

type OrderBook struct {
	Bids []OrderLevel `json:"bids"`
	Asks []OrderLevel `json:"asks"`
}

type OrderLevel struct {
	Price    float64 `json:"price"`
	Quantity int32   `json:"quantity"`
	Flag     int32   `json:"flag"`
}

type DecodedPacket struct {
	Header  PacketHeader   `json:"header"`
	Records []MarketRecord `json:"records"`
}

type PacketDecoder struct {
	stats struct {
		packetsDecoded int
		decodeErrors   int
		recordsDecoded int
	}
}

func NewPacketDecoder() *PacketDecoder {
	return &PacketDecoder{}
}

func (d *PacketDecoder) DecodePacket(packet []byte) (*DecodedPacket, error) {
	if len(packet) < 36 {
		return nil, fmt.Errorf("packet too short: %d bytes", len(packet))
	}

	// Validate leading zeros
	if packet[0] != 0 || packet[1] != 0 || packet[2] != 0 || packet[3] != 0 {
		return nil, fmt.Errorf("invalid leading bytes")
	}

	// Format ID (Big-Endian)
	formatID := binary.BigEndian.Uint16(packet[4:6])

	// Message type (Little-Endian)
	msgType := binary.LittleEndian.Uint16(packet[8:10])

	// Timestamp
	hour := binary.BigEndian.Uint16(packet[20:22])
	minute := binary.BigEndian.Uint16(packet[22:24])
	second := binary.BigEndian.Uint16(packet[24:26])

	var timestamp time.Time
	if hour <= 23 && minute <= 59 && second <= 59 {
		now := time.Now()
		timestamp = time.Date(now.Year(), now.Month(), now.Day(), int(hour), int(minute), int(second), now.Nanosecond(), now.Location())
	} else {
		timestamp = time.Now()
	}

	header := PacketHeader{
		FormatID:  formatID,
		MsgType:   msgType,
		Timestamp: timestamp,
	}

	// Determine number of records
	recordSize := 264
	headerSize := 36
	if len(packet) < headerSize {
		return nil, fmt.Errorf("packet too short for records")
	}
	dataSize := len(packet) - headerSize
	numRecords := dataSize / recordSize

	records := make([]MarketRecord, 0, numRecords)

	for i := 0; i < numRecords; i++ {
		offset := headerSize + i*recordSize
		if offset+40 > len(packet) {
			break
		}

		record, err := d.parseRecord(packet[offset:])
		if err != nil {
			log.Printf("Error parsing record %d: %v", i, err)
			continue
		}
		if record != nil {
			records = append(records, *record)
			d.stats.recordsDecoded++
		}
	}

	d.stats.packetsDecoded++

	return &DecodedPacket{
		Header:  header,
		Records: records,
	}, nil
}

func (d *PacketDecoder) parseRecord(data []byte) (*MarketRecord, error) {
	if len(data) < 40 {
		return nil, fmt.Errorf("record too short")
	}

	// Token (Little-Endian)
	token := binary.LittleEndian.Uint32(data[0:4])
	if token == 0 {
		return nil, nil // Empty slot
	}

	// Prices in paise, convert to rupees
	open := float64(int32(binary.LittleEndian.Uint32(data[4:8]))) / 100.0
	prevClose := float64(int32(binary.LittleEndian.Uint32(data[8:12]))) / 100.0
	high := float64(int32(binary.LittleEndian.Uint32(data[12:16]))) / 100.0
	low := float64(int32(binary.LittleEndian.Uint32(data[16:20]))) / 100.0

	volume := int32(binary.LittleEndian.Uint32(data[24:28]))
	turnoverLakhs := binary.LittleEndian.Uint32(data[28:32])
	lotSize := binary.LittleEndian.Uint32(data[32:36])
	ltp := float64(int32(binary.LittleEndian.Uint32(data[36:40]))) / 100.0

	var atp, bid, ask float64
	var orderBook *OrderBook

	if len(data) >= 88 {
		atp = float64(int32(binary.LittleEndian.Uint32(data[84:88]))) / 100.0
	}

	if len(data) >= 108 {
		bid = float64(int32(binary.LittleEndian.Uint32(data[104:108]))) / 100.0
	}

	if len(data) >= 264 {
		orderBook = d.parseOrderBook(data[104:])
		if orderBook != nil && len(orderBook.Asks) > 0 {
			ask = orderBook.Asks[0].Price
		}
	}

	var sequenceNumber uint32
	if len(data) >= 48 {
		sequenceNumber = binary.LittleEndian.Uint32(data[44:48])
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

func (d *PacketDecoder) parseOrderBook(data []byte) *OrderBook {
	if len(data) < 160 {
		return nil
	}

	bids := make([]OrderLevel, 0, 5)
	asks := make([]OrderLevel, 0, 5)

	for i := 0; i < 5; i++ {
		bidOffset := i * 32
		askOffset := bidOffset + 16

		if bidOffset+16 > len(data) || askOffset+16 > len(data) {
			break
		}

		// Bid
		bidPrice := float64(int32(binary.LittleEndian.Uint32(data[bidOffset:bidOffset+4]))) / 100.0
		bidQty := int32(binary.LittleEndian.Uint32(data[bidOffset+4 : bidOffset+8]))
		bidFlag := int32(binary.LittleEndian.Uint32(data[bidOffset+8 : bidOffset+12]))

		if bidQty > 0 && bidPrice > 0 {
			bids = append(bids, OrderLevel{
				Price:    bidPrice,
				Quantity: bidQty,
				Flag:     bidFlag,
			})
		}

		// Ask
		askPrice := float64(int32(binary.LittleEndian.Uint32(data[askOffset:askOffset+4]))) / 100.0
		askQty := int32(binary.LittleEndian.Uint32(data[askOffset+4 : askOffset+8]))
		askFlag := int32(binary.LittleEndian.Uint32(data[askOffset+8 : askOffset+12]))

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

func (d *PacketDecoder) GetStats() map[string]int {
	return map[string]int{
		"packets_decoded": d.stats.packetsDecoded,
		"decode_errors":   d.stats.decodeErrors,
		"records_decoded": d.stats.recordsDecoded,
	}
}
