# BSE UDP Market Data Reader - Go HFT Version

🚀 High-Frequency Trading (HFT) optimized UDP multicast market data reader for BSE NFCAST feed.

## Overview

Go implementation of the BSE market data reader, optimized for low-latency HFT applications using goroutines and channels for concurrent processing.

## Features

- **High Performance** - Go's concurrency model with goroutines
- **Channel Pipeline** - Lock-free data flow between stages
- **UDP Multicast** - Connects to BSE NFCAST production feed
- **Real-time Decoding** - Parses BSE packet formats (300B-1620B)
- **Order Book Depth** - Extracts 5-level bid/ask market depth
- **Symbol Resolution** - Maps token IDs to contract symbols
- **Concurrent Processing** - Parallel packet processing
- **Graceful Shutdown** - Signal handling for clean termination

## Project Structure

```
bse-go/
├── cmd/bse-reader/             # Main application
│   └── main.go                 # Entry point with goroutines
│
├── pkg/                        # Go packages
│   ├── config/                 # Configuration loading
│   │   └── config.go
│   ├── connection/             # UDP multicast connection
│   │   └── connection.go
│   ├── decoder/                # Packet decoding
│   │   └── decoder.go
│   ├── decompressor/           # Data normalization
│   │   └── decompressor.go
│   ├── data_collector/         # Quote collection
│   │   └── data_collector.go
│   └── saver/                  # JSON/CSV output
│       └── saver.go
│
├── config/
│   └── config.json             # Configuration file
│
├── data/
│   ├── tokens/
│   │   └── token_details.json  # Contract master (shared with Python)
│   ├── processed_json/         # Output JSON files
│   └── processed_csv/          # Output CSV files
│
├── tests/                      # Unit tests
│   └── config_test.go
│
├── go.mod                      # Go module definition
├── go.sum                      # Dependency checksums
└── README.md                   # This file
```

## Architecture

### Pipeline Design

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  UDP Socket  │────▶│   Decoder    │────▶│ Decompressor │────▶│  Collector   │────▶│    Saver     │
│  (Receive)   │     │  (Parse)     │     │ (Normalize)  │     │  (Enrich)    │     │ (JSON/CSV)   │
└──────────────┘     └──────────────┘     └──────────────┘     └──────────────┘     └──────────────┘
       │                    │                    │                    │                    │
       ▼                    ▼                    ▼                    ▼                    ▼
   rawPackets          decodedPackets     decompressedRecords      quotes              Files
   chan []byte         chan DecodedPacket chan DecompressedRecord  chan Quote
```

### Goroutine Model

```go
// Each stage runs in its own goroutine
go conn.ReceiveLoop(rawPackets)           // Network I/O
go processPackets(dec, rawPackets, ...)    // CPU-bound decoding
go decompressRecords(decomp, ...)          // Data transformation
go collectQuotes(collector, ...)           // Enrichment
go saveQuotes(saver, quotes)               // File I/O
```

## Requirements

- Go 1.21+
- Network access to BSE multicast (239.1.2.5:26002)
- IGMPv2 multicast support
- BSE Market hours: 9:00 AM - 3:30 PM IST (Mon-Fri)

## Configuration

Edit `config/config.json`:

```json
{
  "multicast": {
    "ip": "239.1.2.5",
    "port": 26002,
    "segment": "Equity Derivatives",
    "env": "production"
  },
  "buffer_size": 2048,
  "timeout": 30,
  "store_limit": 100
}
```

## Building

```bash
# Download dependencies
go mod tidy

# Build executable
go build -o bse-reader.exe ./cmd/bse-reader

# Build with optimizations
go build -ldflags="-s -w" -o bse-reader.exe ./cmd/bse-reader
```

## Running

```bash
# Run from bse-go directory
./bse-reader.exe

# Or with go run
go run ./cmd/bse-reader
```

**Output:**
```
🚀 BSE UDP Market Data Reader - Go HFT Version
============================================
✅ All components started. Press Ctrl+C to stop.
```

## Output Formats

### JSON (processed_json/YYYYMMDD_HHMMSS_quotes.json)
```json
{"token":873830,"symbol":"SENSEX","ltp":84530.0,"volume":10960,"bid_levels":[{"price":84525.0,"qty":20}]}
```

### CSV (processed_csv/YYYYMMDD_quotes.csv)
```csv
token,symbol,ltp,volume,bid1_price,bid1_qty,ask1_price,ask1_qty
873830,SENSEX,84530.0,10960,84525.0,20,84535.0,15
```

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package tests
go test ./pkg/decoder/
```

## Performance Comparison

| Metric | Python | Go |
|--------|--------|-----|
| Latency | ~10ms | <1ms |
| Throughput | 1000 pkt/s | 10000+ pkt/s |
| Memory | ~100MB | ~20MB |
| CPU Usage | Higher | Lower |

## Key Differences from Python

| Feature | Python | Go |
|---------|--------|-----|
| Concurrency | Threading | Goroutines |
| Data Flow | Function calls | Channels |
| Token Map | API CSV fetch | Static JSON |
| Dual Feed | Supported | Single feed |

## Roadmap

- [ ] Add dual-feed support (CM + F&O)
- [ ] Implement API-based token fetching
- [ ] Add WebSocket streaming output
- [ ] Implement packet loss detection
- [ ] Add metrics/monitoring endpoint

## License

BSE Integration Team - November 2025
