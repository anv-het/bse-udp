# BSE F&O (Derivatives) Feed Statistics & Benchmark Tool

## Overview
This tool measures and benchmarks the BSE F&O (Futures & Options) multicast feed performance for HFT system evaluation.

## Metrics Measured

### 1. Packet Statistics
- **Total Packets**: Total number of UDP packets received
- **Total Bytes**: Total data received
- **Valid Packets**: Packets with valid BSE NFCAST format (msg type 2020/2021)
- **Packet Size Distribution**: Breakdown of different packet sizes
- **Message Type Distribution**: Count by BSE message types
- **Unique Tokens**: Number of unique F&O contracts seen

### 2. Throughput
- **Packets/second**: Real-time and average packet rate
- **Bytes/second**: Data throughput
- **Records/second**: Market data records per second
- **Max/Min Packets/sec**: Peak and minimum rates

### 3. Latency (Processing Time)
- **Decode Latency**: Time to parse packet headers, validate, and extract tokens
- **Process Latency**: Total time from receive to processing complete
- **Min/Max/Average**: Statistical breakdown (in microseconds)

### 4. System Resources
- **Memory Usage**: Current, average, and peak memory (MB)
- **GC Pauses**: Garbage collection count
- **Goroutines**: Active goroutine count
- **CPU**: GOMAXPROCS setting

### 5. F&O Specific
- **Unique Token Count**: Number of distinct F&O contracts received
- **Token Coverage Assessment**: Compared against expected ~40k+ contracts

## Usage

```bash
# Run until Ctrl+C (default)
./benchmark_fo.exe

# Run for specific duration
./benchmark_fo.exe -duration 1m
./benchmark_fo.exe -duration 5m
./benchmark_fo.exe -duration 1h

# Custom multicast settings
./benchmark_fo.exe -ip 239.1.2.5 -port 26002

# Full options
./benchmark_fo.exe -ip <multicast_ip> -port <port> -duration <time>
```

## Output

### Live Display (per second)
```
[1m30s] Pkts/s:   2345 | Records/s:   8901 | Decode:   3.2µs | Mem:   8.5MB | Tokens: 15432
```

### 5-Minute Summary
- Cumulative totals
- Message type distribution
- Packet size distribution
- Unique token count

### Final Report
- Complete benchmark results
- HFT readiness assessment
- F&O-specific metrics
- Recommendations

## HFT Readiness Assessment

| Metric | Excellent | Acceptable | Needs Work |
|--------|-----------|------------|------------|
| Latency | <100µs | <1ms | >1ms |
| Memory | <500MB | <1GB | >1GB |
| Throughput | Handles feed | - | Drops packets |
| Token Coverage | >10k contracts | >1k contracts | <1k contracts |

## Building

```bash
cd multicast_reader_go_fo
go mod init multicast_reader_go_fo
go get golang.org/x/net/ipv4
go build -o benchmark_fo.exe .
```

## Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| ip | 239.1.2.5 | BSE F&O multicast IP |
| port | 26002 | BSE F&O UDP port |
| duration | 0 (infinite) | Benchmark duration |
| buffer | 65536 | UDP receive buffer size |

## Network Requirements

- Must be connected to BSE network
- Multicast routing enabled
- Port 26002/UDP open
- Market hours: 9:00 AM - 3:30 PM IST (F&O)

## F&O Contract Information

BSE F&O typically includes:
- **SENSEX Options**: ~20,000+ contracts (various strikes/expiries)
- **BANKEX Options**: ~15,000+ contracts
- **SENSEX Futures**: Weekly/Monthly expiries
- **BANKEX Futures**: Weekly/Monthly expiries
- **Stock F&O**: Various underlying stocks

Expected unique tokens during market hours: **30,000 - 50,000**

## Comparison with Python Reader

Run this benchmark alongside the Python reader to compare:

| Metric | Go | Python | Notes |
|--------|-----|--------|-------|
| Decode Latency | ~2-5µs | ~50-100µs | Go ~20x faster |
| Memory | ~5-20MB | ~50-100MB | Go ~5x lower |
| GC Pauses | Minimal | N/A | Python uses different GC |
| Throughput | Higher | Lower | Go handles bursts better |
