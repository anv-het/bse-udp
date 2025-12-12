// Package domain contains data structures for BSE index messages
package domain

import "time"

// IndexData represents decoded index information from message types 2011/2012
// Message Type 2011: Critical indices (SENSEX, BSE100) - disseminated every 1 second
// Message Type 2012: Other indices - disseminated every 8 seconds
//
// NOTE: This structure represents ONLY the data available in Type 2011/2012 packets.
// Market statistics (Total_Trades, Total_Volume, Advances, Declines) are NOT included
// in these message types. Net_Change and Percent_Change are calculated client-side.
type IndexData struct {
	MessageType uint16    // 2011 or 2012
	Timestamp   time.Time // System timestamp when received

	// Index identification
	IndexName string // e.g., "SENSEX", "BSE100", "BSE500"
	IndexCode uint32 // Unique index identifier

	// Index values (from UDP feed)
	IndexValue float64 // Current index value (LTP)
	PrevClose  float64 // Previous close value
	OpenValue  float64 // Opening value
	HighValue  float64 // Day's high
	LowValue   float64 // Day's low

	// Calculated values (computed from feed data)
	NetChange     float64 // Calculated: IndexValue - PrevClose
	PercentChange float64 // Calculated: (NetChange / PrevClose) * 100

	// Raw packet data for debugging
	RawData []byte
}

// GetIndexName returns a human-readable name for common index codes
func (i *IndexData) GetIndexName() string {
	// Map common index codes to names
	// These mappings should be loaded from BSE master data
	indexNames := map[uint32]string{
		100: "BSE SENSEX",
		200: "BSE 100",
		300: "BSE 200",
		400: "BSE 500",
		500: "BSE MIDCAP",
		600: "BSE SMALLCAP",
	}

	if name, exists := indexNames[i.IndexCode]; exists {
		return name
	}
	return i.IndexName
}

// IsMessageType2011 returns true if this is a critical index (1 second frequency)
func (i *IndexData) IsMessageType2011() bool {
	return i.MessageType == 2011
}

// IsMessageType2012 returns true if this is a regular index (8 second frequency)
func (i *IndexData) IsMessageType2012() bool {
	return i.MessageType == 2012
}
