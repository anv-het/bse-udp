# BSE UDP Market Data Reader

🚀 **Production-Ready** Real-time market data parser for Bombay Stock Exchange (BSE) via UDP multicast using the **BSE Direct NFCAST protocol**.

## 🎯 Overview

High-performance market data readers in **Python** and **Go** that receive, decode, and normalize BSE market data from UDP multicast feeds.

### Implementations

| Version | Language | Status | Use Case |
|---------|----------|--------|----------|
| **Python** | Python 3.8+ | ✅ Production | Dual-feed, API integration |
| **Go HFT** | Go 1.21+ | 🚧 Development | Low-latency HFT |

### Supported Feeds

| Port | Segment | Data Source | Records |
|------|---------|-------------|---------|
| 26001 | Equity Cash (CM) | BhavCopy CSV | ~4,689 stocks |
| 26002 | Derivatives (F&O) | Contract Master CSV | ~40,000 contracts |

### Key Features

- ✅ **Dual Feed Support** - Simultaneous CM and F&O data collection
- ✅ **Zero JSON Dependency** - Uses BSE API CSV files (no static token_details.json)
- ✅ **Daily Auto-Update** - Fetches token mappings from BSE API daily
- ✅ **Auto File Cleanup** - Keeps only last 2 days of contract files
- ✅ **Full Order Book** - 5-level bid/ask depth with price, quantity, flags
- ✅ **Segment-Specific CSV** - Separate output files for CM and F&O
- ✅ **System Timestamps** - Millisecond precision from system clock
- ✅ **Graceful Shutdown** - Proper Ctrl+C handling on Windows
- 🚧 **Go HFT Version** - Low-latency concurrent processing

---

## 📁 Project Structure

```
bse/
├── config.json                 # ⭐ Main configuration file
├── README.md                   # This file
├── bse_reader.log              # Application log file
│
├── src/                        # 🐍 Python Source Code
│   ├── main.py                 # ⭐ Entry point - dual feed orchestrator
│   ├── connection.py           # UDP multicast socket setup
│   ├── packet_receiver.py      # Packet reception & routing
│   ├── decoder.py              # Binary packet parsing (264-byte records)
│   ├── decompressor.py         # Market depth extraction
│   ├── data_collector.py       # Token→symbol mapping & validation
│   ├── saver.py                # JSON/CSV output writer
│   ├── token_mapper.py         # ⭐ Unified token mapper (EQ + F&O)
│   ├── contract_manager.py     # F&O contract CSV management
│   ├── equity_cash_fetcher.py  # BhavCopy CSV management
│   ├── bse_contract_fetcher.py # Base API fetcher class
│   ├── database.py             # Database utilities (optional)
│   └── benchmark.py            # Performance benchmarking
│
├── bse-go/                     # 🚀 Go HFT Version
│   ├── cmd/bse-reader/         # Main application
│   │   └── main.go             # ⭐ Entry point with goroutines
│   ├── pkg/                    # Go packages
│   │   ├── config/             # Configuration loading
│   │   ├── connection/         # UDP multicast connection
│   │   ├── decoder/            # Packet decoding
│   │   ├── decompressor/       # Data normalization
│   │   ├── data_collector/     # Quote collection & symbol resolution
│   │   └── saver/              # JSON/CSV output
│   ├── config/config.json      # Go-specific configuration
│   ├── go.mod                  # Go module definition
│   └── README.md               # Go version documentation
│
├── tests/                      # Test files (Python)
│   ├── test_connection.py      # Connection tests
│   ├── test_decoder.py         # Decoder tests
│   ├── test_decompressor.py    # Decompressor tests
│   ├── test_packet_receiver.py # Packet receiver tests
│   ├── test_dual_feed.py       # Dual feed integration test
│   ├── test_dual_feed_full.py  # Full dual feed test
│   ├── test_equity_cash_26001.py # Equity cash testing
│   ├── test_live_sensex.py     # SENSEX live test
│   └── test_live_token.py      # ⭐ Live token monitor
│
├── data/                       # Data storage
│   ├── tokens/                 # Contract CSV files (auto-managed)
│   │   ├── BhavCopy_BSE_CM_*.csv      # Equity tokens
│   │   └── BSE_EQD_CONTRACT_*.csv     # F&O tokens
│   ├── processed_csv/          # CSV output
│   │   ├── YYYYMMDD_CM_quotes.csv     # Equity quotes
│   │   └── YYYYMMDD_FO_quotes.csv     # F&O quotes
│   └── processed_json/         # JSON output
│       └── YYYYMMDD_*.json
│
└── docs/                       # Documentation
    ├── ARCHITECTURE_GUIDE.md   # System architecture diagrams
    ├── PROJECT_DOCUMENTATION.md # Complete technical documentation
    ├── COMPLETE_PACKET_STRUCTURE_ANALYSIS.md # Packet format details
    ├── BOLTPLUS Connectivity Manual V1.14.1.pdf
    └── BSE_DIRECT_NFCAST_Manual.pdf
```

---

## � Python Version

### Quick Start

```cmd
# Setup
python -m venv .venv
.venv\Scripts\activate
pip install -r requirements.txt

# Configure segments in config.json
# Run
python src/main.py
```

### Features
- Dual-feed (CM + F&O) simultaneous reception
- Auto-fetch token mappings from BSE API
- Segment-specific CSV output
- Threading-based concurrency

---

## 🚀 Go HFT Version

### Quick Start

```cmd
cd bse-go

# Build
go mod tidy
go build -o bse-reader.exe ./cmd/bse-reader

# Run
./bse-reader.exe
```

### Features
- Goroutine-based concurrent processing
- Channel-based data pipeline
- Low-latency packet processing
- Optimized for HFT applications

### Architecture

```
Raw Packets → Decoder → Decompressor → Collector → Saver
     ↓           ↓            ↓            ↓         ↓
  Channel    Channel      Channel      Channel   JSON/CSV
```

---

## 📊 Output Formats

### CSV Files (Segment-Specific)

**Equity Cash (`YYYYMMDD_CM_quotes.csv`):**
```csv
timestamp,token,symbol,symbol_name,ltp,open,high,low,prev_close,atp,volume,turnover_lakhs,seq,bid_prices,bid_quantities,ask_prices,ask_quantities
2025-11-27 15:02:45.123,500325,RELIANCE,RELIANCE,2456.50,2445.00,2460.00,2440.00,2450.00,2452.30,125000,3069,12345,2456.00|2455.50,100|200,2457.00|2457.50,150|100
```

**Derivatives (`YYYYMMDD_FO_quotes.csv`):**
```csv
timestamp,token,symbol,symbol_name,expiry,option_type,strike,ltp,open,high,low,prev_close,atp,volume,turnover_lakhs,lot_size,seq,bid_prices,bid_quantities,ask_prices,ask_quantities
2025-11-27 15:02:45.456,873870,SENSEX,SENSEX27NOV2025_84100CE,27-NOV-2025,CE,84100,1207.75,1180.00,1250.00,1150.00,1195.00,1205.50,480,58,20,67890,1205.00|1200.00,20|40,1210.00|1215.00,30|50
```

### JSON Files (Streaming Format)

```json
{"timestamp":"2025-11-27 15:02:45.123","token":873870,"symbol":"SENSEX","ltp":1207.75,"volume":480,"order_book":{"bids":[{"price":1205.0,"quantity":20}],"asks":[{"price":1210.0,"quantity":30}]}}
```

---

## 🔧 Configuration

### config.json (Python)

```json
{
  "segments": {
    "cm_enabled": true,
    "fo_enabled": true
  },
  "multicast_cm": {
    "ip": "239.1.2.5",
    "port": 26001
  },
  "multicast_fo": {
    "ip": "239.1.2.5",
    "port": 26002
  },
  "api": {
    "base_url": "http://192.168.102.166:2060/v1/sftp-files"
  },
  "data_management": {
    "keep_days": 2,
    "auto_cleanup": true
  },
  "buffer_size": 2048,
  "timeout": 30
}
```

### bse-go/config/config.json (Go)

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

---

## 🧪 Testing

### Python Tests

```cmd
# All tests
python -m pytest tests/ -v

# Live token monitoring
python tests/test_live_token.py --token 873830 --port 26002

# Dual feed test
python tests/test_dual_feed.py
```

### Go Tests

```cmd
cd bse-go
go test ./...
```

---

## 📦 Packet Structure

### Overview

```
┌─────────────────────────────────────────┐
│ HEADER (36 bytes)                       │
│ • Format ID, Timestamp, Metadata        │
├─────────────────────────────────────────┤
│ RECORD 1 (264 bytes)                    │
│ • Token, OHLC, Volume, Order Book       │
├─────────────────────────────────────────┤
│ RECORD 2 (264 bytes)                    │
├─────────────────────────────────────────┤
│ ... up to 6 records                     │
└─────────────────────────────────────────┘
```

### Valid Packet Sizes

| Size | Formula | Records |
|------|---------|---------|
| 300 | 36 + 1×264 | 1 |
| 564 | 36 + 2×264 | 2 |
| 828 | 36 + 3×264 | 3 |
| 1092 | 36 + 4×264 | 4 |
| 1356 | 36 + 5×264 | 5 |
| 1620 | 36 + 6×264 | 6 |

### Key Field Offsets (Record)

| Offset | Field | Type |
|--------|-------|------|
| 0-3 | Token | uint32 LE |
| 4-7 | Open Price | int32 LE (paise) |
| 8-11 | Prev Close | int32 LE (paise) |
| 12-15 | High Price | int32 LE (paise) |
| 16-19 | Low Price | int32 LE (paise) |
| 24-27 | Volume | int32 LE |
| 28-31 | Turnover (Lakhs) | uint32 LE |
| 32-35 | Lot Size | uint32 LE |
| 36-39 | LTP | int32 LE (paise) |
| 44-47 | Sequence Number | uint32 LE |
| 84-87 | ATP | int32 LE (paise) |
| 104-263 | Order Book (5 levels) | 160 bytes |

---

## ⏰ Market Hours

| Session | Time (IST) |
|---------|------------|
| Pre-Open | 9:00 AM - 9:15 AM |
| Trading | 9:15 AM - 3:30 PM |
| Post-Close | 3:40 PM - 4:00 PM |

**Note:** Only Monday-Friday (excluding market holidays)

---

## 📈 Performance

| Metric | Value |
|--------|-------|
| Packet Rate | 1000+ packets/sec |
| Latency | <10ms packet-to-output |
| Memory | <100MB RAM |
| Storage | ~5MB/hour |

---

## 📚 Documentation

- [Architecture Guide](docs/ARCHITECTURE_GUIDE.md) - System diagrams and data flow
- [Project Documentation](docs/PROJECT_DOCUMENTATION.md) - Complete technical reference
- [Packet Structure Analysis](docs/COMPLETE_PACKET_STRUCTURE_ANALYSIS.md) - Binary format details

---

## 🔗 Data Sources

| Source | API Endpoint | Purpose |
|--------|--------------|---------|
| BhavCopy | `/sftp-files?file_name=BhavCopy_BSE_CM_*.csv` | Equity symbols |
| Contract Master | `/sftp-files?file_name=BSE_EQD_CONTRACT_*.csv` | F&O contracts |

---

**Status:** Production Ready | **Last Updated:** November 2025
