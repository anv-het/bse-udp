package decompressor

import (
	"bse-go/pkg/decoder"
	"log"
	"time"
)

// DecompressedRecord contains normalized market data after decompression
type DecompressedRecord struct {
	Token          uint32       `json:"token"`
	Open           float64      `json:"open"`
	High           float64      `json:"high"`
	Low            float64      `json:"low"`
	Close          float64      `json:"close"`
	LTP            float64      `json:"ltp"`
	Volume         int32        `json:"volume"`
	PrevClose      float64      `json:"prev_close"`
	ATP            float64      `json:"atp"`
	Bid            float64      `json:"bid"`
	Ask            float64      `json:"ask"`
	BidLevels      []OrderLevel `json:"bid_levels"`
	AskLevels      []OrderLevel `json:"ask_levels"`
	TurnoverLakhs  uint32       `json:"turnover_lakhs"`
	LotSize        uint32       `json:"lot_size"`
	SequenceNumber uint32       `json:"sequence_number"`
	Timestamp      time.Time    `json:"timestamp"`
}

// OrderLevel represents a single price level in the order book
type OrderLevel struct {
	Price    float64 `json:"price"`
	Quantity int32   `json:"quantity"`
	Flag     int32   `json:"flag"`
}

// NFCASTDecompressor handles BSE NFCAST packet decompression
type NFCASTDecompressor struct {
	segment string // "CM" or "FO"
	stats   struct {
		recordsDecompressed int
		decompressErrors    int
	}
}

// NewNFCASTDecompressor creates a new decompressor
func NewNFCASTDecompressor(segment string) *NFCASTDecompressor {
	return &NFCASTDecompressor{segment: segment}
}

// DecompressRecord normalizes a decoded market record
func (d *NFCASTDecompressor) DecompressRecord(header decoder.PacketHeader, record decoder.MarketRecord) (*DecompressedRecord, error) {
	// BSE production feed sends uncompressed data, just normalize
	decompressed := &DecompressedRecord{
		Token:          record.Token,
		Open:           record.Open,
		High:           record.High,
		Low:            record.Low,
		Close:          record.LTP, // Close = LTP in real-time
		LTP:            record.LTP,
		Volume:         record.Volume,
		PrevClose:      record.PrevClose,
		ATP:            record.ATP,
		Bid:            record.Bid,
		Ask:            record.Ask,
		TurnoverLakhs:  record.TurnoverLakhs,
		LotSize:        record.LotSize,
		SequenceNumber: record.SequenceNumber,
		Timestamp:      header.Timestamp,
	}

	// Convert order book
	if record.OrderBook != nil {
		decompressed.BidLevels = make([]OrderLevel, len(record.OrderBook.Bids))
		for i, bid := range record.OrderBook.Bids {
			decompressed.BidLevels[i] = OrderLevel{
				Price:    bid.Price,
				Quantity: bid.Quantity,
				Flag:     bid.Flag,
			}
		}

		decompressed.AskLevels = make([]OrderLevel, len(record.OrderBook.Asks))
		for i, ask := range record.OrderBook.Asks {
			decompressed.AskLevels[i] = OrderLevel{
				Price:    ask.Price,
				Quantity: ask.Quantity,
				Flag:     ask.Flag,
			}
		}
	}

	d.stats.recordsDecompressed++
	return decompressed, nil
}

// GetStats returns decompressor statistics
func (d *NFCASTDecompressor) GetStats() map[string]int {
	return map[string]int{
		"records_decompressed": d.stats.recordsDecompressed,
		"decompress_errors":    d.stats.decompressErrors,
	}
}

// LogStats logs decompressor statistics
func (d *NFCASTDecompressor) LogStats() {
	log.Printf("[%s] Decompressor Stats: records=%d, errors=%d",
		d.segment, d.stats.recordsDecompressed, d.stats.decompressErrors)
}
