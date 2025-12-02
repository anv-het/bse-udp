# BSE Go HFT - High-Frequency Trading System

Complete production-ready pipeline for BSE (Bombay Stock Exchange) NFCAST market data.

## 🚀 Quick Start

### Option 1: Run Directly (Like Python - No Build Needed!)
```powershell
cd d:\bse\bse-go-hft

# Run HFT server (like python main.py)
go run ./cmd/hft-server/main.go

# Run benchmark
go run ./cmd/benchmark/main.go
```

### Option 2: Build First, Then Run
```powershell
cd d:\bse\bse-go-hft

# Build
go build -o benchmark.exe ./cmd/benchmark/
go build -o hft-server.exe ./cmd/hft-server/

# Run
.\hft-server.exe
.\benchmark.exe
```

### Option 3: Using Makefile
```powershell
cd d:\bse\bse-go-hft

make run            # Uses go run (no build)
make run-benchmark  # Benchmark via go run
make server         # Uses compiled exe
make benchmark      # Benchmark compiled exe
```

## 📖 Full Documentation

**See [docs/COMPLETE_HFT_GUIDE.md](docs/COMPLETE_HFT_GUIDE.md) for:**
- Complete pipeline flow with diagrams
- Why we use Ring Buffer (lock-free, zero-copy)
- Technical deep dive (mixed endianness, compression)
- Statistics & metrics explained
- What's implemented vs missing
- Troubleshooting guide

## 📦 Two Executables

| Executable | Purpose | CSV Output |
|------------|---------|------------|
| `hft-server.exe` | Complete pipeline (CM + Socket + Decode + Save) | ✅ Yes |
| `benchmark.exe` | Statistics only (Latency, Throughput, Memory) | ❌ No |

## 🔧 Command Line Options

### HFT Server
```powershell
# Using go run (like Python)
go run ./cmd/hft-server/main.go [options]

# Using compiled exe
.\hft-server.exe [options]

Options:
  -ip string        Multicast IP (default "239.1.2.5" for F&O)
  -port int         UDP port (default 26001)
  -duration string  Run duration, e.g., "5m", "1h" (default: until Ctrl+C)
  -data string      Data directory (default "./data")
  -contracts string Contract master CSV path (auto-download if empty)
```

### Benchmark
```powershell
# Using go run (like Python)
go run ./cmd/benchmark/main.go [options]

# Using compiled exe
.\benchmark.exe [options]

Options:
  -ip string        Multicast IP (default "239.1.2.5" for F&O)
  -port int         UDP port (default 26001)
  -duration string  Run duration (default: until Ctrl+C)
```

## 📊 Usage Examples

### Run HFT Server with F&O Feed (Default)
```powershell
go run ./cmd/hft-server/main.go
# or
.\hft-server.exe
```

### Run HFT Server with Equity Feed
```powershell
go run ./cmd/hft-server/main.go -ip 239.1.2.5
# or
.\hft-server.exe -ip 239.1.2.5
```

### Run for 5 Minutes and Stop
```powershell
go run ./cmd/hft-server/main.go -duration 5m
# or
.\hft-server.exe -duration 5m
```

### Run with Custom Contract File
```powershell
go run ./cmd/hft-server/main.go -contracts d:\bse\data\tokens\BSE_EQD_CONTRACT_27112025_fetched.csv
```

### Run Benchmark for 60 Seconds
```powershell
go run ./cmd/benchmark/main.go -duration 60s
# or
.\benchmark.exe -duration 60s
```

## 📁 Output Files

### CSV Output Location
```
data/
└── processed_csv/
    └── 20251128_FO_quotes.csv    ← Daily quotes file
```

### CSV Columns
```
timestamp, token, symbol, symbol_name, expiry, option_type, strike_price,
ltp, open, high, low, prev_close, volume, lot_size, seq,
bid_prices, bid_qtys, ask_prices, ask_qtys
```

### Contract Master Location
```
data/
└── tokens/
    └── BSE_EQD_CONTRACT_20251128.csv    ← Auto-downloaded
```

## 🏗️ Complete Pipeline Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        HFT SERVER PIPELINE                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐                │
│  │ 1. Contract  │────▶│ 2. Multicast │────▶│ 3. Packet    │                │
│  │    Master    │     │   Connect    │     │   Decode     │                │
│  │   Download   │     │  (UDP/IGMP)  │     │  (NFCAST)    │                │
│  └──────────────┘     └──────────────┘     └──────────────┘                │
│        │                    │                    │                          │
│        ▼                    ▼                    ▼                          │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐                │
│  │ Token Map    │     │ Raw Packets  │     │ Quotes       │                │
│  │ (29k tokens) │     │ (564 bytes)  │     │ (Normalized) │                │
│  └──────────────┘     └──────────────┘     └──────────────┘                │
│                                                   │                          │
│                            ┌─────────────────────┼─────────────────────┐    │
│                            ▼                     ▼                     ▼    │
│                     ┌──────────────┐     ┌──────────────┐     ┌────────┐   │
│                     │ 4. CSV Save  │     │ 5. Stats     │     │ 6.Live │   │
│                     │  (Append)    │     │  Collector   │     │ Output │   │
│                     └──────────────┘     └──────────────┘     └────────┘   │
│                            │                     │                          │
│                            ▼                     ▼                          │
│                     ┌──────────────┐     ┌──────────────┐                   │
│                     │ Daily CSV    │     │ Final Report │                   │
│                     │ quotes.csv   │     │ (Latency,    │                   │
│                     │              │     │  Memory, etc)│                   │
│                     └──────────────┘     └──────────────┘                   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 📈 Sample HFT Server Output

```
================================================================================
         BSE GO HFT SERVER - COMPLETE PRODUCTION PIPELINE                      
================================================================================
Start Time:      2025-11-28 15:30:00
Multicast:       239.1.2.5:26001
Data Directory:  ./data
GOMAXPROCS:      8
Duration:        Until Ctrl+C
================================================================================

📥 Downloading Contract Master from BSE...
   URL: https://www.bseindia.com/downloads/Help/file/BSE_EQD_CONTRACT.csv
✅ Downloaded 2456789 bytes to ./data/tokens/BSE_EQD_CONTRACT_20251128.csv

📂 Loading Contract Master: ./data/tokens/BSE_EQD_CONTRACT_20251128.csv
✅ Loaded 29432 contracts

📝 CSV output: ./data/processed_csv/20251128_FO_quotes.csv

🔌 Connecting to multicast feed...
✅ Connected! Receiving packets...

[00:01:30] Pkts: 12543 (139.4/s) | Records: 45231 (502.6/s) | Saved: 38421 | Tokens: 29432
```

## 📊 Sample Final Report

```
████████████████████████████████████████████████████████████████████████████████
█                    BSE HFT SERVER - FINAL REPORT                            █
████████████████████████████████████████████████████████████████████████████████

┌─────────────────────────────────────────────────────────────────────────────┐
│ SESSION SUMMARY                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ Duration:              5m0.001s                                             │
│ Total Packets:         41,532                                               │
│ Total Records:         151,234                                              │
│ Quotes Saved:          128,421                                              │
│ Tokens in Master:      29,432                                               │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ LATENCY (Decode + Save)                                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│ Average:               1.23 µs                                              │
│ P50 (Median):          0.89 µs                                              │
│ P90:                   1.56 µs                                              │
│ P99:                   8.92 µs                                              │
│ P99.9:                 45.23 µs                                             │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ OUTPUT                                                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│ CSV File:              ./data/processed_csv/20251128_FO_quotes.csv          │
│ Rows Written:          128,421                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

## 🏛️ Project Structure

```
bse-go-hft/
├── benchmark.exe            ← Benchmark executable
├── hft-server.exe           ← HFT Server executable
├── go.mod
├── go.sum
├── Makefile
├── README.md
│
├── cmd/
│   ├── benchmark/
│   │   └── main.go          # Statistics-only benchmark
│   └── hft-server/
│       └── main.go          # Complete pipeline server
│
├── config/
│   └── config.go            # Configuration handling
│
├── data/                    ← Created at runtime
│   ├── processed_csv/
│   │   └── YYYYMMDD_FO_quotes.csv
│   └── tokens/
│       └── BSE_EQD_CONTRACT_YYYYMMDD.csv
│
├── docs/
│   └── HFT_ARCHITECTURE.md  # Architecture documentation
│
├── internal/
│   ├── buffer/
│   │   └── ring.go          # Lock-free ring buffer
│   ├── decoder/
│   │   └── decoder.go       # Zero-copy packet decoder
│   ├── metrics/
│   │   ├── latency.go       # Latency tracking
│   │   └── system.go        # System stats
│   └── receiver/
│       └── multicast.go     # Multicast receiver
│
└── pkg/
    └── domain/
        ├── packet.go        # Packet structures
        └── quote.go         # Quote model
```

## 🎯 Feed Configurations

| Feed | Multicast IP | Port | Message Type |
|------|--------------|------|--------------|
| F&O (Derivatives) | 239.1.2.5 | 26001 | 2021 |
| Equity (CM) | 239.1.2.5 | 26001 | 2020 |

## 📋 Requirements

- Go 1.21+
- Windows/Linux
- Network access to BSE multicast feeds
- IGMPv2 capable network infrastructure

## 🛠️ Make Commands

```powershell
# BUILD
make build              # Build all executables
make build-release      # Build optimized binaries
make clean              # Clean build artifacts

# GO RUN (Like Python - No Build Needed)
make run                # go run hft-server (like python main.py)
make run-fo             # go run hft-server with F&O feed
make run-eq             # go run hft-server with Equity feed
make run-5m             # go run hft-server for 5 minutes
make run-benchmark      # go run benchmark
make run-benchmark-60s  # go run benchmark for 60 seconds

# HFT SERVER (Using Compiled EXE)
make server             # Run HFT server (F&O)
make server-eq          # Run HFT server (Equity)
make server-5m          # Run HFT server for 5 minutes

# BENCHMARK (Using Compiled EXE)
make benchmark          # Run benchmark (F&O)
make benchmark-eq       # Run benchmark (Equity)
make benchmark-60s      # Run benchmark for 60 seconds

# DEVELOPMENT
make fmt                # Format Go code
make vet                # Run Go vet
make test               # Run tests
make tidy               # Tidy dependencies

make help               # Show all commands
```

## 📝 License

Internal use only - BSE Market Data Systems
