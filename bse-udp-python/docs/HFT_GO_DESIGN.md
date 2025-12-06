# BSE HFT System - Design & Optimization Document
## High-Frequency Trading Platform in Go

**Version:** 1.0  
**Date:** November 27, 2025  
**Project:** bse-go-hft (Fresh HFT Implementation)

---

## Table of Contents
1. [Executive Summary](#1-executive-summary)
2. [Python vs Go Comparison](#2-python-vs-go-comparison)
3. [Current Implementation Analysis](#3-current-implementation-analysis)
4. [HFT Architecture Design](#4-hft-architecture-design)
5. [Go-Specific Optimizations](#5-go-specific-optimizations)
6. [Project Structure](#6-project-structure)
7. [Implementation Plan](#7-implementation-plan)
8. [Benchmarking Strategy](#8-benchmarking-strategy)

---

## 1. Executive Summary

### Goal
Create a **production-grade HFT (High-Frequency Trading) system** in Go that achieves:
- **Sub-microsecond packet processing** (< 1µs per packet)
- **Zero-allocation hot path** (no GC during trading)
- **Lock-free data structures** for concurrent access
- **NUMA-aware memory allocation** for multi-socket systems
- **Kernel bypass** options (AF_XDP, DPDK-ready)

### Why Fresh Project (bse-go-hft)?
The current `bse-go` is a **functional port** of Python. For true HFT, we need:
- Complete architectural redesign
- Different data structures (arrays vs maps, fixed-size buffers)
- Different concurrency model (lock-free vs channels)
- Different memory management (object pools vs dynamic allocation)

---

## 2. Python vs Go Comparison

### 2.1 Performance Characteristics

| Aspect | Python | Go (Current) | Go (HFT Target) |
|--------|--------|--------------|-----------------|
| **Packet Decode** | ~50-100µs | ~1-5µs | **< 500ns** |
| **Memory per Quote** | ~2-5KB (dict) | ~500B (struct) | **< 200B** |
| **GC Pause** | N/A (GIL) | ~1-10ms | **< 100µs** |
| **Concurrency Model** | Threading (GIL) | Goroutines + Channels | **Lock-free + Atomics** |
| **Hot Path Allocations** | Many (dict/list) | Some (slices) | **Zero** |
| **CPU Utilization** | Single core (GIL) | Multi-core | **CPU Affinity + Pinning** |

### 2.2 Code Comparison

#### Python Decoder (Current)
```python
# Heavy dict allocations on hot path
def _parse_record(self, record_bytes, offset):
    token = struct.unpack('<I', record_bytes[0:4])[0]
    return {  # Dict allocation every record!
        'token': token,
        'ltp': struct.unpack('<i', record_bytes[36:40])[0] / 100.0,
        'volume': struct.unpack('<i', record_bytes[24:28])[0],
        # ... 20+ fields creating new dict objects
    }
```

#### Go Current (bse-go)
```go
// Better but still allocates
func (d *PacketDecoder) parseRecord(data []byte) (*MarketRecord, error) {
    return &MarketRecord{  // Heap allocation every record
        Token:  binary.LittleEndian.Uint32(data[0:4]),
        LTP:    float64(int32(binary.LittleEndian.Uint32(data[36:40]))) / 100.0,
        // ...
    }, nil
}
```

#### Go HFT (Target)
```go
// Zero-allocation with object pool
func (d *Decoder) DecodeInto(packet []byte, record *Record) bool {
    record.Token = binary.LittleEndian.Uint32(packet[0:4])
    record.LTPRaw = binary.LittleEndian.Uint32(packet[36:40]) // Keep as int, convert later
    // No allocation - reuses pre-allocated Record
    return true
}
```

---

## 3. Current Implementation Analysis

### 3.1 Python Bottlenecks

| Component | Issue | Impact |
|-----------|-------|--------|
| **GIL** | Single-threaded execution | Only 1 core used |
| **Dict Creation** | New dict per record | ~10-50µs per record |
| **Float Conversion** | `/ 100.0` on hot path | CPU cycles wasted |
| **Logging** | String formatting | Memory allocation |
| **CSV Writer** | Sync I/O | Blocks main loop |

### 3.2 Go (bse-go) Bottlenecks

| Component | Issue | Impact |
|-----------|-------|--------|
| **Channels** | Buffered but still synced | Context switches |
| **Struct Pointers** | `*MarketRecord` heap alloc | GC pressure |
| **Float64** | Unnecessary precision | Memory/cache waste |
| **Map Lookups** | Token map string keys | Hash computation |
| **JSON Encoding** | Reflection-based | Slow serialization |

### 3.3 Latency Breakdown (Estimated)

```
Current Go Pipeline (per packet):
┌─────────────────────────────────────────────────────────┐
│ UDP Recv      │ ~1-5µs    │ System call overhead        │
│ Header Parse  │ ~100ns    │ Binary decode               │
│ Record Parse  │ ~500ns    │ Per record × 3 avg          │
│ Channel Send  │ ~200ns    │ Buffered channel            │
│ Decompress    │ ~200ns    │ Minimal work                │
│ Token Lookup  │ ~500ns    │ Map with string key         │
│ Quote Create  │ ~300ns    │ Struct allocation           │
│ CSV Write     │ ~10-50µs  │ File I/O (batched)          │
├─────────────────────────────────────────────────────────┤
│ TOTAL         │ ~15-60µs  │ End-to-end latency          │
└─────────────────────────────────────────────────────────┘
```

---

## 4. HFT Architecture Design

### 4.1 Core Principles

1. **Zero-Allocation Hot Path** - Pre-allocate everything
2. **Lock-Free Structures** - No mutexes on critical path
3. **Data Locality** - Keep hot data in L1/L2 cache
4. **Batch Processing** - Amortize system call overhead
5. **Separation of Concerns** - Fast path vs slow path

### 4.2 Architecture Diagram

```
                    ┌─────────────────────────────────────────────────────────┐
                    │                   BSE HFT SYSTEM                        │
                    └─────────────────────────────────────────────────────────┘
                                              │
                    ┌─────────────────────────┴─────────────────────────┐
                    │                                                    │
           ┌────────▼────────┐                              ┌────────────▼────────────┐
           │   FAST PATH     │                              │      SLOW PATH          │
           │  (< 1µs target) │                              │  (Background workers)   │
           └────────┬────────┘                              └────────────┬────────────┘
                    │                                                    │
    ┌───────────────┼───────────────┐                    ┌───────────────┼───────────────┐
    │               │               │                    │               │               │
┌───▼───┐     ┌─────▼─────┐   ┌─────▼─────┐        ┌─────▼─────┐   ┌─────▼─────┐   ┌─────▼─────┐
│UDP RX │────▶│  Decoder  │──▶│  Ring     │───────▶│ Persister │   │  Logger   │   │ Metrics   │
│(recvmmsg)   │(Zero-Alloc)   │  Buffer   │        │ (Async)   │   │ (Async)   │   │ (Atomic)  │
└───────┘     └───────────┘   │(Lock-Free)│        └───────────┘   └───────────┘   └───────────┘
                              └───────────┘
                                    │
                              ┌─────▼─────┐
                              │ Consumers │
                              │(Strategy) │
                              └───────────┘
```

### 4.3 Component Design

#### 4.3.1 UDP Receiver (Kernel Bypass Ready)
```go
// HFT: Use recvmmsg for batch receive (Linux)
type UDPReceiver struct {
    fd          int              // Raw file descriptor
    buffers     [][]byte         // Pre-allocated receive buffers
    msgs        []unix.Mmsghdr   // Message headers for recvmmsg
    batchSize   int              // Packets per syscall
}

func (r *UDPReceiver) ReceiveBatch() int {
    // Single syscall receives multiple packets
    n, _ := unix.Recvmmsg(r.fd, r.msgs, 0, nil)
    return n
}
```

#### 4.3.2 Zero-Allocation Decoder
```go
// HFT: Fixed-size record, no pointers
type Record struct {
    Token         uint32
    OpenRaw       int32   // Keep as paise (no float conversion)
    HighRaw       int32
    LowRaw        int32
    LTPRaw        int32
    VolumeRaw     int32
    Timestamp     int64   // Unix nano
    // Order book as fixed array (not slice)
    BidPrices     [5]int32
    BidQtys       [5]int32
    AskPrices     [5]int32
    AskQtys       [5]int32
}

// Size: 88 bytes (fits in cache line)
```

#### 4.3.3 Lock-Free Ring Buffer
```go
// HFT: SPSC (Single Producer Single Consumer) ring buffer
type RingBuffer struct {
    buffer    []Record          // Pre-allocated records
    mask      uint64            // Power of 2 - 1
    head      atomic.Uint64     // Write position (producer)
    tail      atomic.Uint64     // Read position (consumer)
    _padding  [56]byte          // Prevent false sharing
}

func (r *RingBuffer) TryPush(rec *Record) bool {
    head := r.head.Load()
    next := (head + 1) & r.mask
    if next == r.tail.Load() {
        return false // Full
    }
    r.buffer[head] = *rec // Copy, not pointer
    r.head.Store(next)
    return true
}
```

#### 4.3.4 Token Lookup (Array-Based)
```go
// HFT: O(1) array lookup instead of map
type TokenTable struct {
    symbols [2_000_000]SymbolInfo // Max token ID coverage
    valid   [2_000_000]bool       // Validity bitmap
}

func (t *TokenTable) Lookup(token uint32) *SymbolInfo {
    if token < 2_000_000 && t.valid[token] {
        return &t.symbols[token]
    }
    return nil
}
```

---

## 5. Go-Specific Optimizations

### 5.1 Compiler Optimizations

| Technique | Description | Benefit |
|-----------|-------------|---------|
| **`//go:noinline`** | Prevent inlining for profiling | Debug builds |
| **`//go:nosplit`** | No stack split check | Reduce overhead |
| **`//go:linkname`** | Access runtime internals | Advanced tuning |
| **Bounds Check Elimination** | Use `_ = data[263]` | Remove bounds checks |

```go
// Example: BCE (Bounds Check Elimination)
func decodeRecord(data []byte) {
    _ = data[263] // Compiler eliminates bounds checks below
    token := binary.LittleEndian.Uint32(data[0:4])
    ltp := binary.LittleEndian.Uint32(data[36:40])
    // ...
}
```

### 5.2 Memory Optimizations

| Technique | Description | Benefit |
|-----------|-------------|---------|
| **Object Pools** | `sync.Pool` for temp objects | Reduce allocations |
| **Arena Allocation** | Pre-allocate large blocks | Reduce GC |
| **Stack Allocation** | Small structs on stack | Zero GC |
| **Memory Alignment** | 64-byte cache line alignment | Reduce cache misses |

```go
// Object Pool Example
var recordPool = sync.Pool{
    New: func() interface{} {
        return &Record{}
    },
}

func processPacket(data []byte) {
    rec := recordPool.Get().(*Record)
    defer recordPool.Put(rec)
    // Use rec...
}
```

### 5.3 Concurrency Optimizations

| Technique | Description | When to Use |
|-----------|-------------|-------------|
| **Atomics** | `atomic.Uint64` | Counters, flags |
| **Lock-Free Queues** | SPSC/MPSC ring buffers | Producer-consumer |
| **CPU Affinity** | Pin goroutines to cores | Reduce context switches |
| **GOMAXPROCS** | Control scheduler | Isolate cores |

```go
// CPU Affinity (Linux)
import "golang.org/x/sys/unix"

func pinToCore(coreID int) {
    var mask unix.CPUSet
    mask.Set(coreID)
    unix.SchedSetaffinity(0, &mask)
}
```

### 5.4 I/O Optimizations

| Technique | Description | Benefit |
|-----------|-------------|---------|
| **recvmmsg** | Batch UDP receive | Reduce syscalls |
| **mmap** | Memory-mapped files | Zero-copy I/O |
| **io_uring** | Async I/O (Linux 5.1+) | Non-blocking |
| **Buffered Writers** | Large write buffers | Reduce writes |

---

## 6. Project Structure

```
bse-go-hft/
├── cmd/
│   ├── hft-reader/          # Main HFT application
│   │   └── main.go
│   ├── benchmark/           # Performance benchmarks
│   │   └── main.go
│   └── tools/
│       ├── packet-replay/   # Replay captured packets
│       └── latency-test/    # Measure end-to-end latency
│
├── internal/                # Private packages (not importable)
│   ├── decoder/
│   │   ├── decoder.go       # Zero-alloc decoder
│   │   ├── decoder_test.go
│   │   └── decoder_bench_test.go
│   │
│   ├── ringbuffer/
│   │   ├── spsc.go          # Single-producer single-consumer
│   │   ├── mpsc.go          # Multi-producer single-consumer
│   │   └── ringbuffer_test.go
│   │
│   ├── receiver/
│   │   ├── udp.go           # Standard UDP receiver
│   │   ├── udp_linux.go     # recvmmsg optimization
│   │   └── receiver_test.go
│   │
│   ├── tokentable/
│   │   ├── table.go         # Array-based O(1) lookup
│   │   └── loader.go        # CSV loader
│   │
│   ├── types/
│   │   ├── record.go        # Fixed-size record struct
│   │   ├── quote.go         # Output quote struct
│   │   └── orderbook.go     # Order book types
│   │
│   ├── pipeline/
│   │   ├── pipeline.go      # Main processing pipeline
│   │   └── workers.go       # Worker goroutines
│   │
│   └── metrics/
│       ├── latency.go       # Latency histograms
│       └── counters.go      # Atomic counters
│
├── pkg/                     # Public packages (reusable)
│   ├── config/
│   │   └── config.go
│   └── saver/
│       ├── csv.go           # Async CSV writer
│       └── json.go          # Async JSON writer
│
├── config/
│   └── hft_config.json
│
├── data/
│   ├── tokens/              # Token CSV files
│   ├── output/              # Output files
│   └── replay/              # Captured packets for testing
│
├── scripts/
│   ├── build.sh             # Build with optimizations
│   └── benchmark.sh         # Run benchmarks
│
├── go.mod
├── go.sum
└── README.md
```

---

## 7. Implementation Plan

### Phase 1: Core Infrastructure (Week 1)
- [ ] Project setup with Go modules
- [ ] Fixed-size `Record` struct (88 bytes)
- [ ] Zero-allocation decoder
- [ ] Basic ring buffer (SPSC)
- [ ] Benchmark framework

### Phase 2: Optimized Receiver (Week 2)
- [ ] Standard UDP receiver
- [ ] `recvmmsg` batch receiver (Linux)
- [ ] Buffer pooling
- [ ] CPU affinity support

### Phase 3: Processing Pipeline (Week 3)
- [ ] Token table (array-based)
- [ ] Pipeline orchestration
- [ ] Async CSV/JSON writers
- [ ] Metrics collection

### Phase 4: Advanced Optimizations (Week 4)
- [ ] Lock-free MPSC queue
- [ ] Memory alignment tuning
- [ ] GC tuning (`GOGC`, `GOMEMLIMIT`)
- [ ] Bounds check elimination

### Phase 5: Production Hardening (Week 5)
- [ ] Error handling
- [ ] Graceful shutdown
- [ ] Configuration management
- [ ] Logging (structured, async)
- [ ] Monitoring/Alerting hooks

---

## 8. Benchmarking Strategy

### 8.1 Micro-Benchmarks

```go
// decoder_bench_test.go
func BenchmarkDecodeRecord(b *testing.B) {
    data := loadTestPacket()
    var rec Record
    
    b.ResetTimer()
    b.ReportAllocs()
    
    for i := 0; i < b.N; i++ {
        decodeRecordInto(data, &rec)
    }
}
```

### 8.2 Latency Measurement

```go
// Use high-resolution timer
start := time.Now().UnixNano()
processPacket(packet)
latency := time.Now().UnixNano() - start

// Histogram buckets: p50, p90, p99, p99.9, max
histogram.Record(latency)
```

### 8.3 Target Metrics

| Metric | Target | Current Go | Python |
|--------|--------|------------|--------|
| **Packet Decode** | < 500ns | ~2µs | ~50µs |
| **End-to-End Latency** | < 5µs | ~20µs | ~100µs |
| **Throughput** | 1M msg/sec | 100K msg/sec | 10K msg/sec |
| **Memory/Quote** | < 200B | ~500B | ~2KB |
| **GC Pause** | < 100µs | ~1ms | N/A |
| **Allocations (hot path)** | 0 | 5-10 | 50+ |

### 8.4 Comparison Script

```bash
#!/bin/bash
# benchmark.sh

echo "=== Python Benchmark ==="
time python src/benchmark.py --packets 100000

echo "=== Go Current Benchmark ==="
cd bse-go && go test -bench=. -benchmem ./...

echo "=== Go HFT Benchmark ==="
cd bse-go-hft && go test -bench=. -benchmem ./...
```

---

## 9. Risk Assessment

| Risk | Mitigation |
|------|------------|
| **Complexity** | Start simple, optimize incrementally |
| **Portability** | Abstract OS-specific code behind interfaces |
| **Debugging** | Add extensive metrics, keep debug mode |
| **Over-optimization** | Profile first, optimize bottlenecks only |

---

## 10. References

- [Go Performance Book](https://github.com/dgryski/go-perfbook)
- [Lock-Free Programming](https://www.1024cores.net/home/lock-free-algorithms)
- [LMAX Disruptor Pattern](https://lmax-exchange.github.io/disruptor/)
- [Linux Network Performance](https://blog.cloudflare.com/how-to-receive-a-million-packets/)
- [Go Runtime Internals](https://golang.org/doc/diagnostics)

---

## Next Steps

1. **Create `bse-go-hft/` directory structure**
2. **Implement core types (`Record`, `Quote`)**
3. **Build zero-allocation decoder**
4. **Create benchmark suite**
5. **Iterate based on profiling results**

---

*Document prepared for BSE HFT Go Implementation Project*
