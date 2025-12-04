# BSE C++ HFT Server - Build & Run Guide

## 📋 Table of Contents

1. [Prerequisites](#prerequisites)
2. [Build Instructions](#build-instructions)
3. [Running the Server](#running-the-server)
4. [Command Line Options](#command-line-options)
5. [Configuration](#configuration)
6. [Output Files](#output-files)
7. [Troubleshooting](#troubleshooting)

---

## Prerequisites

### Required Software

| Software | Version | Purpose |
|----------|---------|---------|
| Visual Studio 2022/2026 | With C++ Desktop Development | C++ Compiler (MSVC) |
| CMake | 3.16+ (Optional) | Build system |
| Windows 10/11 | 64-bit | Operating System |

### Visual Studio Components Required

When installing Visual Studio, ensure these components are selected:
- ✅ **Desktop development with C++**
- ✅ MSVC Build Tools for x64/x86
- ✅ Windows 11 SDK (10.0.26100.0 or later)
- ✅ C++ CMake tools for Windows (optional)

---

## Build Instructions

### Method 1: Using build_manual.bat (Recommended)

This is the simplest method - no Developer Command Prompt required!

```cmd
# Navigate to project directory
cd D:\bse\bse-cpp-hft

# Run the build script
build_manual.bat
```

**Output:**
```
============================================
BSE HFT C++ Build Script
============================================

[1/3] Verifying compiler...
Microsoft (R) C/C++ Optimizing Compiler Version 19.50.35719 for x64

[2/3] Compiling source files...
main.cpp
Generating code
Finished generating code

[3/3] Build complete!
============================================
Executable: bin\bse-hft-cpp.exe
============================================
```

### Method 2: Using Developer Command Prompt

```cmd
# Open "Developer Command Prompt for VS 2022" or "x64 Native Tools Command Prompt"

cd D:\bse\bse-cpp-hft

# Create bin directory
mkdir bin

# Compile with optimizations
cl /nologo /std:c++17 /O2 /Oi /Ot /GL /GF /EHsc /MT ^
   /D_WIN32_WINNT=0x0A00 /DNOMINMAX /DNDEBUG ^
   /I"include" ^
   /Fe"bin\bse-hft-cpp.exe" ^
   src\main.cpp ^
   /link /LTCG /OPT:REF /OPT:ICF ws2_32.lib
```

### Method 3: Using CMake

```cmd
# Create and enter build directory
mkdir build
cd build

# Configure with CMake
cmake .. -G "Visual Studio 17 2022" -A x64

# Build
cmake --build . --config Release

# Executable will be at: build\Release\bse-hft-cpp.exe
```

### Method 4: Using MinGW (Alternative)

```cmd
# If you have MinGW-w64 installed
make

# Or directly with g++
g++ -std=c++17 -O3 -march=native -o bin/bse-hft-cpp.exe src/main.cpp -Iinclude -lws2_32
```

---

## Running the Server

### Basic Usage

```cmd
# Run with default settings (both EQ and FO feeds)
.\bin\bse-hft-cpp.exe

# Run for specific duration
.\bin\bse-hft-cpp.exe -duration 60        # 60 seconds
.\bin\bse-hft-cpp.exe -duration 300       # 5 minutes
.\bin\bse-hft-cpp.exe -duration 3600      # 1 hour

# Run specific feed only
.\bin\bse-hft-cpp.exe -eq-only            # Equity only
.\bin\bse-hft-cpp.exe -fo-only            # F&O only

# Save quotes to CSV files
.\bin\bse-hft-cpp.exe -duration 60 -save
.\bin\bse-hft-cpp.exe -duration 60 -save -outdir data/processed_csv

# Combine options
.\bin\bse-hft-cpp.exe -duration 60 -eq -fo -save
```

### Test Tools

```cmd
# Build test tools
cl.exe /EHsc /std:c++17 /O2 /I include tests\benchmark.cpp /Fe:bin\benchmark.exe ws2_32.lib
cl.exe /EHsc /std:c++17 /O2 /I include tests\test_live_sensex.cpp /Fe:bin\test_live_sensex.exe ws2_32.lib
cl.exe /EHsc /std:c++17 /O2 /I include tests\test_live_token.cpp /Fe:bin\test_live_token.exe ws2_32.lib

# Run benchmark
.\bin\benchmark.exe -port 26002 -duration 30

# Monitor SENSEX
.\bin\test_live_sensex.exe -duration 60

# Monitor specific token
.\bin\test_live_token.exe -token 1102290 -port 26002 -ticks 50
```

### Sample Output

```
================================================================================
                    BSE HFT SERVER - C++ IMPLEMENTATION
                         High-Frequency Trading
================================================================================
  Features:
    * Zero-copy packet parsing
    * Lock-free ring buffers (16K slots)
    * SIMD-optimized decoding
    * Async batched CSV writing
    * Sub-microsecond latency
================================================================================

[CONFIG] Loading configuration...
   [OK] Loaded: config.json
   EQ Feed: 239.1.2.5:26001
   FO Feed: 239.1.2.5:26002

[TOKENS] Loading token mappings...
   📂 Looking for BhavCopy files...
   📄 Loading: BhavCopy_BSE_CM_20251202.csv
   ✅ Loaded 4757 EQ tokens

   📂 Looking for Contract Master files...
   📄 Loading: BSE_EQD_CONTRACT_02122025.csv
   ✅ Loaded 33840 F&O tokens
   [OK] Total tokens loaded: 38597

[FEEDS] Starting feed processors...
   [OK] EQ receiver connected
   [OK] FO receiver connected

[RUNNING] Receiving market data (Press Ctrl+C to stop)...

[10s] EQ: 3500 pkts, 7500 recs (750/s) | FO: 3500 pkts, 7500 recs (750/s) | Drops: EQ=0 FO=0
```

---

## Command Line Options

| Option | Description | Default |
|--------|-------------|---------|
| `-duration <seconds>` | Run duration in seconds (0 = unlimited) | 0 (Ctrl+C to stop) |
| `-eq-only` or `-eq` | Enable only Equity Cash feed | false |
| `-fo-only` or `-fo` | Enable only F&O Derivatives feed | false |
| `-save` | Enable CSV saving | false |
| `-outdir <path>` | Output directory for CSV files | data/processed_csv |
| `-config <path>` | Path to config file | config.json |
| `-help` | Show help message | - |

### Examples

```cmd
# Run for 10 seconds with both feeds
.\bin\bse-hft-cpp.exe -duration 10

# Run EQ feed only for 5 minutes
.\bin\bse-hft-cpp.exe -duration 300 -eq-only

# Run with CSV saving
.\bin\bse-hft-cpp.exe -duration 60 -save -outdir data/processed_csv

# Run with custom config
.\bin\bse-hft-cpp.exe -config my_config.json

# Show help
.\bin\bse-hft-cpp.exe -help
```

---

## Configuration

Edit `config.json` to customize settings:

```json
{
  "multicast_eq": {
    "ip": "239.1.2.5",
    "port": 26001,
    "buffer_size": 2048,
    "socket_buffer": 33554432
  },
  "multicast_fo": {
    "ip": "239.1.2.5",
    "port": 26002,
    "buffer_size": 2048,
    "socket_buffer": 33554432
  },
  "data_management": {
    "output_dir": "./data/processed_csv",
    "tokens_dir": "./data/tokens"
  },
  "ring_buffer": {
    "size": 16384,
    "packet_size": 2048
  },
  "csv_writer": {
    "batch_size": 100,
    "buffer_size": 131072,
    "queue_size": 10000
  }
}
```

### Configuration Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `socket_buffer` | OS socket receive buffer (bytes) | 32 MB |
| `ring_buffer.size` | Ring buffer slots | 16384 |
| `csv_writer.batch_size` | Records per batch write | 100 |
| `csv_writer.buffer_size` | File buffer size | 128 KB |

---

## Output Files

### CSV Output Location

```
data/
└── processed_csv/
    ├── 20251204_EQ_quotes.csv    # Equity quotes
    └── 20251204_FO_quotes.csv    # F&O quotes
```

### CSV Format

```csv
timestamp,token,symbol,symbol_name,expiry,option_type,strike_price,ltp,open,high,low,prev_close,atp,volume,turnover_lakhs,lot_size,sequence_num,bid_prices,bid_qtys,ask_prices,ask_qtys,segment
2025-12-04 14:18:00.231,500331,PIDILITIND,A,,,0.00,1480.05,1478.00,1486.10,1475.00,1478.95,1478.25,9405,139.00,7,1449324869,"1478.90,1478.85,1478.80,1478.75,1478.70","15,23,24,1,23","1480.15,1480.20,1480.25,1480.30,1480.35","11,5,2,22,3",EQ
```

---

## Troubleshooting

### Common Issues

#### 1. "cl is not recognized"
**Solution:** Run from Developer Command Prompt or use `build_manual.bat` which sets up the environment automatically.

#### 2. "Cannot connect to multicast"
**Solution:** 
- Check firewall settings
- Ensure you're on the correct network
- Verify multicast IP/Port in config.json

#### 3. "No token files found"
**Solution:** 
- Place BhavCopy and Contract Master CSV files in `data/tokens/`
- Files should be named: `BhavCopy_BSE_CM_DDMMYYYY.csv` and `BSE_EQD_CONTRACT_DDMMYYYY.csv`
- **Note:** Unlike Go version, C++ version requires pre-downloaded files (no HTTP download to avoid WinHTTP dependency)
- Download files manually from BSE or use Go version's API download

#### 4. High packet drops
**Solution:**
- Increase `socket_buffer` in config.json
- Increase `ring_buffer.size`
- Close other network-intensive applications

### Performance Tips

1. **Run as Administrator** for better network performance
2. **Disable antivirus** temporarily for benchmarks
3. **Use wired connection** instead of WiFi
4. **Close unnecessary applications** to reduce CPU load

---

## Quick Reference

```cmd
# Build
build_manual.bat

# Run (default)
.\bin\bse-hft-cpp.exe

# Run for 60 seconds
.\bin\bse-hft-cpp.exe -duration 60

# EQ only, 5 minutes
.\bin\bse-hft-cpp.exe -duration 300 -eq-only
```

---

**Last Updated:** December 4, 2025
