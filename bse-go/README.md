# BSE UDP Market Data Reader - Go HFT Version

High-Frequency Trading (HFT) optimized UDP multicast market data reader for BSE NFCAST feed, converted from Python to Go for better performance.

## Features

- **High Performance**: Go's concurrency and low-level networking for HFT
- **UDP Multicast**: Connects to BSE NFCAST production feed (239.1.2.5:26002)
- **Real-time Decoding**: Parses BSE packet formats (300B, 564B, 828B, etc.)
- **Order Book Depth**: Extracts 5-level bid/ask market depth
- **Symbol Resolution**: Maps token IDs to contract symbols using token_details.json
- **Data Output**: Saves to JSON (newline-delimited) and CSV formats
- **Concurrent Processing**: Goroutines for parallel packet processing
- **Graceful Shutdown**: Signal handling for clean termination

## Project Structure

```
bse-go/
├── cmd/bse-reader/          # Main application
│   └── main.go
├── pkg/
│   ├── config/             # Configuration loading
│   ├── connection/         # UDP multicast connection
│   ├── decoder/            # Packet decoding
│   ├── decompressor/       # Data normalization
│   ├── data_collector/     # Quote collection & symbol resolution
│   └── saver/              # JSON/CSV output
├── config/
│   └── config.json         # Configuration file
├── data/
│   ├── tokens/
│   │   └── token_details.json  # Contract master
│   ├── processed_json/     # Output JSON files
│   └── processed_csv/      # Output CSV files
├── tests/                  # Unit tests
├── docs/                   # Documentation
└── go.mod
```

## Requirements

- Go 1.21+
- Network access to BSE multicast (239.1.2.5:26002)
- IGMPv2 multicast support on network interface
- BSE Market hours: 9:00 AM - 3:30 PM IST (Mon-Fri)

## Configuration

Edit `config/config.json`:

```json
{
  "multicast": {
    "ip": "239.1.2.5",
    "port": 26002,
    "segment": "Equity",
    "env": "production"
  },
  "buffer_size": 2048,
  "timeout": 30,
  "store_limit": 100
}
```

## Building

```bash
go mod tidy
go build ./cmd/bse-reader
```

## Running

```bash
./bse-reader
```

## Output Formats

### JSON (processed_json/YYYYMMDD_HHMMSS_quotes.json)
```json
{"token":873830,"symbol":"SENSEX","ltp":84530.0,"volume":10960,"bid_levels":[...]}
```

### CSV (processed_csv/YYYYMMDD_quotes.csv)
Token, Symbol, LTP, Volume, Bid/Ask levels, etc.

## Performance

- Concurrent packet processing with channels
- Zero-copy where possible
- Optimized for low latency HFT applications

## Original Python Version

This is a conversion of the Python BSE reader located in the parent directory.

## License

BSE Integration Team
