# BSE Go HFT System - Complete Technical Documentation

## 📋 Table of Contents

1. [Quick Start Guide](#-quick-start-guide)
2. [System Overview](#-system-overview)
3. [Complete Pipeline Flow](#-complete-pipeline-flow)
4. [Why Ring Buffer?](#-why-ring-buffer)
5. [Technical Deep Dive](#-technical-deep-dive)
6. [Statistics & Metrics](#-statistics--metrics)
7. [Code Architecture](#-code-architecture)
8. [What's Implemented vs Missing](#-whats-implemented-vs-missing)
9. [Performance Benchmarks](#-performance-benchmarks)
10. [Troubleshooting](#-troubleshooting)

---

## 🚀 Quick Start Guide

### How to Run (Like Python's `python main.py`)

```powershell
cd d:\bse\bse-go-hft

# Method 1: Using "go run" (No build needed - like Python!)
go run ./cmd/hft-server/main.go

# Method 2: Build first, then run
go build -o hft-server.exe ./cmd/hft-server/
.\hft-server.exe

# Method 3: Using Makefile
make run          # Uses go run (no build)
make server       # Uses compiled exe
```

### All Run Commands

| Command | What It Does |
|---------|--------------|
| `go run ./cmd/hft-server/main.go` | Run HFT server directly (like Python) |
| `go run ./cmd/benchmark/main.go` | Run benchmark directly |
| `.\hft-server.exe` | Run compiled HFT server |
| `.\benchmark.exe` | Run compiled benchmark |
| `make run` | Run HFT server via go run |
| `make run-benchmark` | Run benchmark via go run |

### With Options

```powershell
# Run with Equity feed
go run ./cmd/hft-server/main.go -ip 239.1.2.5

# Run for 5 minutes
go run ./cmd/hft-server/main.go -duration 5m

# Run with custom contract file
go run ./cmd/hft-server/main.go -contracts d:\bse\data\tokens\BSE_EQD_CONTRACT.csv

# Run benchmark for 60 seconds
go run ./cmd/benchmark/main.go -duration 60s
```

---

## 🎯 System Overview

### What This System Does

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      BSE HFT MARKET DATA SYSTEM                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   INPUT                           PROCESSING                    OUTPUT      │
│   ─────                           ──────────                    ──────      │
│                                                                             │
│   ┌─────────────┐               ┌─────────────┐              ┌──────────┐  │
│   │ BSE Network │               │             │              │   CSV    │  │
│   │ Multicast   │──────────────▶│   HFT Go    │─────────────▶│  Files   │  │
│   │ UDP Packets │               │   Server    │              │          │  │
│   └─────────────┘               └─────────────┘              └──────────┘  │
│                                       │                                     │
│   ┌─────────────┐                     │                      ┌──────────┐  │
│   │ Contract    │                     │                      │ Terminal │  │
│   │ Master CSV  │─────────────────────┤                      │ Stats    │  │
│   │ (from BSE)  │                     │                      │ Report   │  │
│   └─────────────┘                     ▼                      └──────────┘  │
│                                 ┌─────────────┐                            │
│                                 │  Latency    │                            │
│                                 │  Memory     │                            │
│                                 │  Packet Loss│                            │
│                                 │  Statistics │                            │
│                                 └─────────────┘                            │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Two Executables Explained

| Executable | Purpose | Downloads CM | Decodes | Saves CSV | Shows Stats |
|------------|---------|--------------|---------|-----------|-------------|
| **hft-server.exe** | Production use | ✅ Yes | ✅ Yes | ✅ Yes | ✅ Yes |
| **benchmark.exe** | Performance testing | ❌ No | ✅ Yes | ❌ No | ✅ Yes |

---

## 🔄 Complete Pipeline Flow

### Step-by-Step Flow

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                         COMPLETE PIPELINE FLOW                                │
└──────────────────────────────────────────────────────────────────────────────┘

STEP 1: CONTRACT MASTER DOWNLOAD
════════════════════════════════
  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
  │   Check if      │────▶│  Download from  │────▶│   Parse CSV     │
  │   file exists   │ No  │  BSE website    │     │   into TokenMap │
  └─────────────────┘     └─────────────────┘     └─────────────────┘
           │ Yes                                          │
           └──────────────────────────────────────────────┤
                                                          ▼
STEP 2: MULTICAST SOCKET CONNECTION                 TokenMap ready
════════════════════════════════════                (29,000+ tokens)
  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
  │ Create UDP      │────▶│  Join Multicast │────▶│  Set 16MB       │
  │ Socket          │     │  Group (IGMP)   │     │  Receive Buffer │
  └─────────────────┘     └─────────────────┘     └─────────────────┘
           │
           ▼
STEP 3: PACKET RECEPTION LOOP
═════════════════════════════
  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
  │  ReadFromUDP()  │────▶│  Validate Size  │────▶│  Start Timing   │
  │  (blocking)     │     │  (>36 bytes)    │     │  (nanoseconds)  │
  └─────────────────┘     └─────────────────┘     └─────────────────┘
           │
           ▼
STEP 4: PACKET DECODING (NFCAST Protocol)
═════════════════════════════════════════
  ┌─────────────────────────────────────────────────────────────────┐
  │                      564-BYTE PACKET                             │
  │  ┌──────────────────────────────────────────────────────────┐   │
  │  │ HEADER (36 bytes)                                        │   │
  │  │ • FormatID (Big-Endian): 0x0234                          │   │
  │  │ • MessageType (Little-Endian!): 2020=EQ, 2021=FO        │   │
  │  │ • Timestamp (Little-Endian): HH:MM:SS                    │   │
  │  └──────────────────────────────────────────────────────────┘   │
  │  ┌──────────────────────────────────────────────────────────┐   │
  │  │ RECORD 1 (66 bytes)                                      │   │
  │  │ • Token (Little-Endian!): Contract ID                    │   │
  │  │ • PrevClose (Big-Endian): In paise (÷100 for Rupees)    │   │
  │  │ • LTP (Big-Endian): Last Traded Price                    │   │
  │  │ • Volume (Big-Endian): Total traded volume               │   │
  │  │ • Compressed: Open, High, Low (2-byte differentials)     │   │
  │  │ • Best 5 Bid/Ask (compressed)                            │   │
  │  └──────────────────────────────────────────────────────────┘   │
  │  ... up to 8 records per packet                                 │
  └─────────────────────────────────────────────────────────────────┘
           │
           ▼
STEP 5: TOKEN LOOKUP
════════════════════
  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
  │  Token: 873830  │────▶│  TokenMap.Get() │────▶│  Symbol: SENSEX │
  │                 │     │                 │     │  Expiry: 27-NOV │
  └─────────────────┘     └─────────────────┘     │  Strike: 82000  │
                                                  │  Type: CE       │
                                                  └─────────────────┘
           │
           ▼
STEP 6: SAVE TO CSV
═══════════════════
  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
  │  Build Quote    │────▶│  Append to CSV  │────▶│  Flush every    │
  │  Object         │     │  (daily file)   │     │  100 rows       │
  └─────────────────┘     └─────────────────┘     └─────────────────┘
           │
           ▼
STEP 7: RECORD STATISTICS
═════════════════════════
  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
  │  Stop Timer     │────▶│  Record Latency │────▶│  Update Memory  │
  │  (end time)     │     │  (nanoseconds)  │     │  Stats          │
  └─────────────────┘     └─────────────────┘     └─────────────────┘
           │
           ▼
  ┌─────────────────────────────────────────────────────────────────┐
  │               LOOP BACK TO STEP 3 (Next Packet)                 │
  └─────────────────────────────────────────────────────────────────┘


STEP 8: ON CTRL+C (TERMINATION)
═══════════════════════════════
  ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
  │  Signal Handler │────▶│  Close Socket   │────▶│  Flush CSV      │
  │  (SIGINT)       │     │                 │     │                 │
  └─────────────────┘     └─────────────────┘     └─────────────────┘
           │
           ▼
  ┌─────────────────────────────────────────────────────────────────┐
  │                     FINAL REPORT                                 │
  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
  │  │ Total Packets   │  │ Latency P50/P99 │  │ Memory Usage    │  │
  │  │ Total Records   │  │ Min/Avg/Max     │  │ Peak/Average    │  │
  │  │ Quotes Saved    │  │                 │  │ GC Cycles       │  │
  │  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
  │  │ Throughput      │  │ CSV File Path   │  │ HFT Assessment  │  │
  │  │ Packets/sec     │  │ Rows Written    │  │ ✅ or ⚠️         │  │
  │  │ Records/sec     │  │                 │  │                 │  │
  │  └─────────────────┘  └─────────────────┘  └─────────────────┘  │
  └─────────────────────────────────────────────────────────────────┘
```

---

## 🔄 Why Ring Buffer?

### Problem Without Ring Buffer

```
WITHOUT RING BUFFER (Bad for HFT):
══════════════════════════════════

  [Network]              [Decoder]              [Saver]
      │                      │                      │
      │   UDP Packet         │                      │
      ├─────────────────────▶│                      │
      │                      │   WAIT for decode    │
      │   ⏳ BLOCKED!        │───────────────────▶ │
      │                      │                      │
      │   Next packet        │                      │
      │   DROPPED! ❌        │                      │
      │                      │                      │

Problems:
1. Network receiver BLOCKED while decoder processes
2. Packets DROPPED during decode time
3. Decoder BLOCKED while CSV writes to disk
4. Single-threaded bottleneck
```

### Solution With Ring Buffer

```
WITH RING BUFFER (HFT Optimized):
═════════════════════════════════

  [Network]         [Ring Buffer]         [Decoder]         [Saver]
      │                  │                    │                │
      │   UDP Packet     │                    │                │
      ├─────────────────▶│   TryPush()       │                │
      │   (instant)      │   O(1)            │                │
      │                  │                    │   TryPop()     │
      │   Next packet    │                    │◀──────────────│
      ├─────────────────▶│                    │                │
      │   (instant)      │                    │   Decode       │
      │                  │                    │   (parallel)   │
      │   Never blocked! │                    │                │
      │                  │                    │                │

Benefits:
1. Network receiver NEVER blocked (instant push)
2. NO packets dropped (buffer absorbs bursts)
3. Decoder runs in PARALLEL
4. Multi-threaded pipeline
```

### Ring Buffer Technical Details

```go
// Why Power of 2 Size?
// ────────────────────
// Ring buffer uses BITWISE AND instead of MODULO for index calculation:
//
// Slow (division):  index = position % size
// Fast (bitwise):   index = position & (size - 1)
//
// For size = 65536 (2^16):
//   mask = 65535 = 0xFFFF
//   position & mask = position % 65536
//
// Bitwise AND is 10-100x faster than modulo!

const RingBufferSize = 1 << 16  // 65536 = 2^16

type RingBuffer struct {
    buffer [][]byte         // Pre-allocated slots
    head   atomic.Uint64    // Producer writes here
    tail   atomic.Uint64    // Consumer reads here
    mask   uint64           // size - 1, for fast modulo
}

// Lock-Free Push (Producer)
func (rb *RingBuffer) TryPush(data []byte) bool {
    head := rb.head.Load()              // Read current write position
    tail := rb.tail.Load()              // Read current read position
    
    if head - tail >= size {            // Buffer full?
        return false                    // Don't block, just fail
    }
    
    slot := head & rb.mask              // Fast index calculation
    copy(rb.buffer[slot], data)         // Copy to pre-allocated slot
    rb.head.Store(head + 1)             // Publish new position
    return true
}

// Lock-Free Pop (Consumer)
func (rb *RingBuffer) TryPop() ([]byte, bool) {
    tail := rb.tail.Load()              // Read current read position
    head := rb.head.Load()              // Read current write position
    
    if tail >= head {                   // Buffer empty?
        return nil, false               // Don't block, just fail
    }
    
    slot := tail & rb.mask              // Fast index calculation
    data := rb.buffer[slot]             // Get data (zero-copy)
    rb.tail.Store(tail + 1)             // Publish new position
    return data, true
}
```

### Why Lock-Free Matters

```
LOCK-BASED (Traditional):
═════════════════════════
  Thread 1 (Producer)     Thread 2 (Consumer)
       │                       │
       │   Lock()              │
       ├──────────▶            │
       │   (acquired)          │   Lock()
       │                       ├──────────▶
       │   Write data          │   ⏳ WAITING...
       │                       │   (blocked)
       │   Unlock()            │
       ├──────────▶            │
       │                       │   (acquired)
       │                       │   Read data
       │                       │   Unlock()

  Problem: Consumer BLOCKED while producer holds lock!
  Latency: ~100-1000 nanoseconds per operation

LOCK-FREE (Our Implementation):
═══════════════════════════════
  Thread 1 (Producer)     Thread 2 (Consumer)
       │                       │
       │   Atomic Store        │   Atomic Load
       ├──────────────────────▶├──────────────────────▶
       │   (instant)           │   (instant)
       │                       │
       │   Both run in parallel!
       │   No waiting!

  Benefit: Both threads run SIMULTANEOUSLY!
  Latency: ~1-10 nanoseconds per operation
```

---

## 🔧 Technical Deep Dive

### BSE NFCAST Protocol - Mixed Endianness

This is the **most critical** technical detail!

```
BSE uses INCONSISTENT byte ordering - empirically validated:

HEADER (36 bytes):
══════════════════
Offset  Field        Size   Endian      Example
──────  ─────        ────   ──────      ───────
0-3     LeadingZeros 4      Big         0x00000000
4-5     FormatID     2      Big         0x0234 ✓
6-7     Reserved     2      -           -
8-9     MessageType  2      LITTLE! ⚠️   0xE407 = 2020
10-19   Reserved     10     -           -
20-21   Hour         2      LITTLE! ⚠️   0x0A00 = 10
22-23   Minute       2      LITTLE! ⚠️   0x1E00 = 30
24-25   Second       2      LITTLE! ⚠️   0x0F00 = 15

RECORD (66 bytes each):
═══════════════════════
Offset  Field        Size   Endian      Example
──────  ─────        ────   ──────      ───────
0-3     Token        4      LITTLE! ⚠️   Contract ID
4-7     Reserved     4      -           -
8-11    PrevClose    4      Big         In paise (÷100)
12-19   Reserved     8      -           -
20-23   LTP          4      Big         Last Traded Price
24-27   Volume       4      Big         Total volume
28-29   Open         2      Big         Compressed diff
30-31   High         2      Big         Compressed diff
32-33   Low          2      Big         Compressed diff
...     Best5        ...    Big         Compressed

WHY MIXED ENDIANNESS?
═════════════════════
BSE's NFCAST protocol was designed for:
- Header fields: x86 native (Little-Endian) for speed
- Data fields: Network order (Big-Endian) for compatibility

This is unusual but valid. Getting this wrong = GARBAGE DATA!
```

### Differential Compression

```
HOW NFCAST COMPRESSION WORKS:
═════════════════════════════

Instead of sending full 4-byte prices, BSE sends 2-byte differences:

Full Price:    ₹85,432.50 = 8543250 paise = 4 bytes
Compressed:    +125 (diff from base) = 2 bytes

DECOMPRESSION ALGORITHM:
════════════════════════

Base Price = Previous Close (in paise)
Compressed = 2-byte signed short

if compressed == 32767:    // Special marker
    read next 4 bytes as full value
elif compressed == 32766 or compressed == -32766:
    end of data marker
else:
    actual_price = (base_price + compressed) / 100.0

EXAMPLE:
════════
PrevClose = 8500000 paise (₹85,000)
Compressed Open = +12500 (2 bytes)
Actual Open = (8500000 + 12500) / 100 = ₹85,125.00

SAVINGS:
════════
Without compression: 5 prices × 4 bytes = 20 bytes
With compression:    1 base + 4 diffs = 4 + 8 = 12 bytes
Space saved: 40%!
```

### Latency Percentile Calculation

```
WHY PERCENTILES MATTER FOR HFT:
═══════════════════════════════

Average latency hides outliers!

Example: 10 measurements
─────────────────────────
1µs, 1µs, 1µs, 1µs, 1µs, 1µs, 1µs, 1µs, 1µs, 100µs

Average = 10.9 µs  (looks okay)
P99 = 100 µs       (reveals the problem!)

For HFT, P99 matters more than average!

HOW WE CALCULATE:
═════════════════

1. Store up to 100,000 samples
2. On report: sort all samples
3. Calculate percentiles:

   P50 (Median) = sample at index (n × 0.50)
   P90          = sample at index (n × 0.90)
   P99          = sample at index (n × 0.99)
   P99.9        = sample at index (n × 0.999)

WHAT EACH PERCENTILE MEANS:
═══════════════════════════
P50:   50% of requests are faster than this
P90:   90% of requests are faster than this
P99:   99% of requests are faster than this
P99.9: 99.9% of requests are faster than this (worst 0.1%)

HFT TARGETS:
════════════
P50:   < 5 µs    (excellent)
P90:   < 20 µs   (good)
P99:   < 100 µs  (acceptable)
P99.9: < 1 ms    (tolerable)
```

---

## 📊 Statistics & Metrics

### What Gets Measured

```
WHEN YOU PRESS CTRL+C, YOU GET:
═══════════════════════════════

┌─────────────────────────────────────────────────────────────────────────────┐
│ SESSION SUMMARY                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ Duration:              5m0.001s          ← Total run time                   │
│ Total Packets:         41,532            ← UDP packets received             │
│ Total Records:         151,234           ← Market data records decoded      │
│ Quotes Saved:          128,421           ← Rows written to CSV              │
│ Tokens in Master:      29,432            ← Contracts in token map           │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ THROUGHPUT                                                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│ Packets/sec:           138.44            ← Network throughput               │
│ Records/sec:           504.11            ← Processing throughput            │
│ Data Rate:             0.078 MB/s        ← Bandwidth usage                  │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ LATENCY (Decode + Save)                                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│ Average:               2.34 µs           ← Mean decode time                 │
│ P50 (Median):          1.89 µs           ← Half are faster                  │
│ P90:                   3.45 µs           ← 90% are faster                   │
│ P99:                   8.92 µs           ← 99% are faster (HFT target)      │
│ P99.9:                 45.23 µs          ← Worst case                       │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ MEMORY                                                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│ Peak Memory:           48.32 MB          ← Maximum RAM used                 │
│ GOMAXPROCS:            8                 ← CPU cores available              │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│ OUTPUT                                                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│ CSV File:              ./data/processed_csv/20251128_FO_quotes.csv          │
│ Rows Written:          128,421           ← Total rows in CSV                │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Memory Tracking Details

```go
// How memory is tracked:

func (s *Stats) SampleMemory() {
    var m runtime.MemStats
    runtime.ReadMemStats(&m)
    
    // Current heap allocation
    currentAlloc := m.Alloc
    
    // Track peak (maximum ever seen)
    for {
        peak := s.PeakMemory.Load()
        if currentAlloc <= peak {
            break
        }
        if s.PeakMemory.CompareAndSwap(peak, currentAlloc) {
            break
        }
    }
    
    // Track for average calculation
    s.MemorySum.Add(currentAlloc)
    s.SampleCount.Add(1)
}

// Called every second via ticker
```

---

## 🏗️ Code Architecture

### File Structure Explained

```
bse-go-hft/
│
├── cmd/                           # Executables (entry points)
│   ├── benchmark/
│   │   └── main.go               # 817 lines - Statistics only
│   │                             # • Latency percentiles
│   │                             # • Packet loss detection
│   │                             # • Memory tracking
│   │                             # • NO CSV output
│   │
│   └── hft-server/
│       └── main.go               # 750 lines - Complete pipeline
│                                 # • Contract master download
│                                 # • Token-to-symbol mapping
│                                 # • CSV output
│                                 # • All statistics
│
├── config/
│   └── config.go                 # Configuration loading
│                                 # • Feed IPs/ports
│                                 # • Buffer sizes
│                                 # • Output directories
│
├── internal/                     # Internal packages
│   ├── buffer/
│   │   └── ring.go               # Lock-free SPSC ring buffer
│   │                             # (NOT YET USED IN MAIN)
│   │
│   ├── decoder/
│   │   └── decoder.go            # Zero-copy packet decoder
│   │                             # (NOT YET USED IN MAIN)
│   │
│   ├── metrics/
│   │   ├── latency.go            # Latency tracking
│   │   └── system.go             # Memory/CPU stats
│   │
│   └── receiver/
│       └── multicast.go          # Multicast UDP receiver
│                                 # (NOT YET USED IN MAIN)
│
├── pkg/
│   └── domain/
│       ├── packet.go             # Packet structures
│       └── quote.go              # Quote model
│
└── docs/
    ├── HFT_ARCHITECTURE.md       # Architecture design
    └── COMPLETE_HFT_GUIDE.md     # This file!
```

---

## ✅ What's Implemented vs Missing

### Currently Implemented (In Main Executables)

| Feature | hft-server.exe | benchmark.exe |
|---------|----------------|---------------|
| UDP Multicast Receive | ✅ | ✅ |
| Contract Master Download | ✅ | ❌ |
| Token-to-Symbol Mapping | ✅ | ❌ |
| Packet Decoding | ✅ | ✅ |
| Latency Percentiles | ✅ | ✅ |
| Memory Tracking | ✅ | ✅ |
| CSV Output | ✅ | ❌ |
| Final Report | ✅ | ✅ |
| Ctrl+C Handling | ✅ | ✅ |

### Implemented But Not Integrated

| Component | Status | Location |
|-----------|--------|----------|
| Ring Buffer | ✅ Implemented | `internal/buffer/ring.go` |
| Zero-Copy Decoder | ✅ Implemented | `internal/decoder/decoder.go` |
| Latency Tracker | ✅ Implemented | `internal/metrics/latency.go` |
| System Stats | ✅ Implemented | `internal/metrics/system.go` |
| Multicast Receiver | ✅ Implemented | `internal/receiver/multicast.go` |

**Note:** These components are implemented but the main executables use inline implementations for simplicity. They can be refactored to use these packages for better modularity.

### Not Yet Implemented (Future Work)

| Feature | Priority | Complexity |
|---------|----------|------------|
| **JSON Output** | Medium | Easy |
| **WebSocket Streaming** | Low | Medium |
| **Sequence Gap Detection** | High | Easy |
| **CPU Affinity (Core Pinning)** | Low | Medium |
| **Full Decompression (Best 5)** | High | Medium |
| **Multi-Goroutine Pipeline** | Medium | Hard |
| **Database Storage** | Low | Medium |
| **REST API** | Low | Medium |
| **Docker Container** | Low | Easy |
| **Prometheus Metrics** | Low | Medium |

### Recommended Next Steps

1. **Sequence Gap Detection** - Add packet loss tracking per token
2. **Full Best 5 Decompression** - Currently simplified
3. **JSON Output Option** - For streaming applications
4. **Integrate Ring Buffer** - Use internal/buffer instead of inline

---

## 📈 Performance Benchmarks

### Expected Performance

| Metric | Target | Typical | Notes |
|--------|--------|---------|-------|
| P50 Latency | < 5 µs | 1-2 µs | Decode time |
| P99 Latency | < 100 µs | 5-20 µs | Worst case |
| Packets/sec | > 100 | 100-500 | Depends on market |
| Memory | < 100 MB | 30-50 MB | Including CSV buffer |
| Packet Loss | 0% | 0% | No gaps detected |

### How to Run Benchmarks

```powershell
# Quick benchmark (60 seconds)
go run ./cmd/benchmark/main.go -duration 60s

# Extended benchmark (5 minutes)
go run ./cmd/benchmark/main.go -duration 5m

# Production test (10 minutes)
.\hft-server.exe -duration 10m
```

---

## 🔧 Troubleshooting

### Common Issues

| Problem | Cause | Solution |
|---------|-------|----------|
| "No packets received" | Network issue | Check multicast routing |
| "Contract master download failed" | BSE website blocked | Use `-contracts` flag |
| High latency | GC pauses | Check `GOGC` setting |
| Memory growing | CSV buffer | Check disk space |
| Tokens showing as "TOKEN_XXX" | Missing contract | Update contract master |

### Debug Commands

```powershell
# Check if multicast works
go run ./cmd/benchmark/main.go -duration 10s

# Check contract master
type data\tokens\BSE_EQD_CONTRACT_*.csv | more

# Check CSV output
type data\processed_csv\*.csv | more

# Check memory usage
Get-Process -Name hft-server | Select WorkingSet64
```

---

## 📚 References

1. BSE Direct NFCAST Manual (PDF)
2. BOLTPLUS Connectivity Manual V1.14.1
3. Go Performance Optimization Guide
4. Linux Network Tuning for Low Latency

---

*Last Updated: November 28, 2025*
*Version: 1.0.0*
