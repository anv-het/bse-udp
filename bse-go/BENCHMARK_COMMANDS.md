# 📊 BSE Feed Benchmark Tool - Command Guide

## Quick Start Commands

### 🔷 F&O (Derivatives) Feed Benchmark
```powershell
# Navigate to F&O folder
cd d:\bse\bse-go\multicast_reader_go_fo

# Run for specific durations:
go run main.go -duration 30s     # 30 seconds
go run main.go -duration 1m      # 1 minute
go run main.go -duration 5m      # 5 minutes
go run main.go -duration 10m     # 10 minutes
go run main.go -duration 1h      # 1 hour

# Run until Ctrl+C (infinite)
go run main.go

# Custom IP/Port
go run main.go -ip 239.1.2.5 -port 26001 -duration 5m
go run main.go -ip 239.1.2.5 -port 26002 -duration 5m

# Using pre-built executable
.\benchmark_fo.exe -duration 5m
```

### 🔷 Equity (CM) Feed Benchmark
```powershell
# Navigate to EQ folder
cd d:\bse\bse-go\multicast_reader_go_eq

# Run for specific durations:
go run main.go -duration 30s     # 30 seconds
go run main.go -duration 1m      # 1 minute
go run main.go -duration 5m      # 5 minutes

# Run until Ctrl+C
go run main.go

# Custom IP/Port
go run main.go -ip 227.0.0.21 -port 26001 -duration 5m

# Using pre-built executable
.\benchmark_eq.exe -duration 5m
```

---

## ⏱️ Duration Examples

| Duration Flag | Time | Use Case |
|---------------|------|----------|
| `-duration 5s` | 5 seconds | Quick connectivity test |
| `-duration 30s` | 30 seconds | Basic stats check |
| `-duration 1m` | 1 minute | Short benchmark |
| `-duration 5m` | 5 minutes | Standard benchmark (recommended) |
| `-duration 10m` | 10 minutes | Extended benchmark |
| `-duration 30m` | 30 minutes | Long-term stability test |
| `-duration 1h` | 1 hour | Production evaluation |
| (no flag) | Until Ctrl+C | Manual monitoring |

---

## 📈 What the Tools Measure

### Per-Second Live Display
```
[0h5m32s] Pkts/s:  1234 | Records/s:  5678 | Decode:  12.5µs | Mem:  45.2MB | Tokens: 12345
```
- **Pkts/s**: UDP packets received per second
- **Records/s**: Market data records decoded per second
- **Decode**: Average decode latency in microseconds
- **Mem**: Current memory allocation
- **Tokens**: Unique instrument tokens seen (F&O only)

### 5-Minute Summary (Auto)
Every 5 minutes, displays:
- Total packets/bytes/records
- Valid/Invalid packet ratio
- Peak throughput
- Memory usage
- Packet size distribution

### Final Report (On Exit)
- **Packet Statistics**: Total counts, bytes, records
- **Packet Loss Detection**: Sequence gaps, missed updates, loss rate
- **Throughput**: Avg/Max/Min packets per second
- **Latency**: Decode and process timing (µs)
- **Memory**: Peak and average usage
- **HFT Readiness Assessment**: Auto-evaluation

---

## 🔍 Packet Loss Detection (F&O Only)

The F&O benchmark tracks **sequence numbers** for each token to detect packet loss:

### What It Tracks
- **Sequence Number**: Each market data update has a per-token sequence number
- **Gap Detection**: If sequence jumps (e.g., 1000 → 1003), a gap of 2 is recorded
- **Loss Rate**: `(Missed Updates / Total Records) × 100%`

### Interpreting Results
```
┌─────────────────────────────────────────────────────────────────────────────┐
│ PACKET LOSS DETECTION                                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│ Tracked Tokens:        12345                                                │
│ Sequence Gaps Found:   0                                                    │
│ Missed Updates:        0                                                    │
│ Packet Loss Rate:      ✅ 0% (NO LOSS DETECTED)                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

| Loss Rate | Status | Action |
|-----------|--------|--------|
| 0% | ✅ EXCELLENT | No issues |
| < 0.01% | ⚠️ MINIMAL | Monitor, likely acceptable |
| < 0.1% | ⚠️ LOW | Investigate network/buffer |
| > 0.1% | ❌ HIGH | Check network, increase buffers |

### If Gaps Are Found
```
┌─────────────────────────────────────────────────────────────────────────────┐
│ SEQUENCE GAP EVENTS (First 10)                                              │
├─────────────────────────────────────────────────────────────────────────────┤
│ Token 861384    : Expected seq 1000      , Got 1003       (Gap: 2)          │
│ Token 861385    : Expected seq 5000      , Got 5005       (Gap: 4)          │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 🏁 HFT Readiness Criteria

### Latency Targets
| Metric | Excellent | Acceptable | Needs Work |
|--------|-----------|------------|------------|
| Avg Decode Latency | < 10 µs | < 100 µs | > 100 µs |
| Avg Process Latency | < 100 µs | < 1 ms | > 1 ms |

### Throughput
- **Good**: > 100 packets/sec sustained
- **Check network if**: Very low or zero packets

### Memory
- **Efficient**: < 500 MB peak
- **High**: > 500 MB (consider optimization)

---

## 🛠️ Troubleshooting

### No Packets Received
```powershell
# Check if correct multicast group
go run main.go -ip 239.1.2.5 -port 26001   # F&O Port 1
go run main.go -ip 239.1.2.5 -port 26002   # F&O Port 2

# Check firewall
netsh advfirewall firewall show rule name=all | findstr UDP
```

### High Packet Loss
1. **Increase buffer size**: Edit `BufferSize` constant in main.go
2. **Check network congestion**: Run during less busy times
3. **Verify multicast routing**: Ensure switches support IGMP

### Build Errors
```powershell
# Re-download dependencies
go mod tidy
go get golang.org/x/net/ipv4
```

---

## 📁 File Locations

| Tool | Location |
|------|----------|
| F&O Benchmark Source | `d:\bse\bse-go\multicast_reader_go_fo\main.go` |
| F&O Benchmark Executable | `d:\bse\bse-go\multicast_reader_go_fo\benchmark_fo.exe` |
| EQ Benchmark Source | `d:\bse\bse-go\multicast_reader_go_eq\main.go` |
| EQ Benchmark Executable | `d:\bse\bse-go\multicast_reader_go_eq\benchmark_eq.exe` |

---

## 📋 Quick Test Sequence

```powershell
# 1. Quick 30-second F&O test
cd d:\bse\bse-go\multicast_reader_go_fo
go run main.go -duration 30s

# 2. If working, run 5-minute benchmark
go run main.go -duration 5m

# 3. For production evaluation, run 1 hour
go run main.go -duration 1h
```

---

**Generated**: Auto-created by BSE HFT Benchmark Tool
