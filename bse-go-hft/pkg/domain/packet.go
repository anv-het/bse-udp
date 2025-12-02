// Package domain contains packet structures for BSE NFCAST protocol
package domain

// PacketHeader represents the BSE NFCAST packet header (36 bytes)
type PacketHeader struct {
	LeadingZeros uint32 // Big-Endian (0x00000000)
	FormatID     uint16 // Big-Endian (0x0234)
	Reserved1    uint16
	MessageType  uint16 // Little-Endian (2020/2021)
	Reserved2    uint16
	MarketType   uint16
	Reserved3    uint16
	Reserved4    uint32
	Hour         uint16 // Little-Endian
	Minute       uint16 // Little-Endian
	Second       uint16 // Little-Endian
	Millisecond  uint16 // Little-Endian
	Reserved5    uint32
}

// PacketRecord represents a single record in BSE NFCAST packet (66 bytes)
type PacketRecord struct {
	Token       uint32 // Little-Endian (offset 0-3)
	Reserved1   uint32 // offset 4-7
	PrevClose   int32  // Big-Endian (paise) offset 8-11
	Reserved2   uint64 // offset 12-19
	LTP         int32  // Big-Endian (paise) offset 20-23
	Volume      int32  // Big-Endian offset 24-27
	SequenceNum uint32 // offset 44-47 (for packet loss detection)

	// Compressed fields (differential encoding)
	CompressedData []byte
}

// Packet size constants
const (
	HeaderSize       = 36
	RecordSize       = 66
	MaxRecordsPerPkt = 8
	MaxPacketSize    = HeaderSize + (RecordSize * MaxRecordsPerPkt) // 564 bytes

	// Message types
	MsgTypeEquity     = 2020
	MsgTypeDerivative = 2021
)
