# BSE Equity (CM) Feed Statistics & Benchmark Tool

## Overview
This tool measures and benchmarks the BSE Equity (Cash Market) multicast feed performance for HFT system evaluation.

## Metrics Measured

### 1. Packet Statistics
- **Total Packets**: Total number of UDP packets received
- **Total Bytes**: Total data received
- **Valid Packets**: Packets with valid BSE NFCAST format
- **Packet Size Distribution**: Breakdown of different packet sizes

### 2. Throughput
- **Packets/second**: Real-time and average packet rate
- **Bytes/second**: Data throughput
- **Records/second**: Market data records per second
- **Max/Min Packets/sec**: Peak and minimum rates

### 3. Latency (Processing Time)
- **Decode Latency**: Time to parse packet headers and validate
- **Process Latency**: Total time from receive to processing complete
- **Min/Max/Average**: Statistical breakdown

### 4. System Resources
- **Memory Usage**: Current, average, and peak memory (MB)
- **GC Pauses**: Garbage collection count
- **Goroutines**: Active goroutine count
- **CPU**: GOMAXPROCS setting

## Usage

```bash
# Run until Ctrl+C (default)
./benchmark_eq.exe

# Run for specific duration
./benchmark_eq.exe -duration 1m
./benchmark_eq.exe -duration 5m
./benchmark_eq.exe -duration 1h

# Custom multicast settings
./benchmark_eq.exe -ip 227.0.0.21 -port 26001

# Full options
./benchmark_eq.exe -ip <multicast_ip> -port <port> -duration <time>
```

## Output

### Live Display (per second)
```
[1m30s] Pkts/s:   1234 | Bytes/s:   654321 | Records/s:   4567 | Decode:   2.5µs | Mem:   5.2MB | Goroutines:   4
```

### 5-Minute Summary
- Cumulative totals
- Packet size distribution
- Peak values

### Final Report
- Complete benchmark results
- HFT readiness assessment
- Recommendations

## HFT Readiness Assessment

| Metric | Excellent | Acceptable | Needs Work |
|--------|-----------|------------|------------|
| Latency | <100µs | <1ms | >1ms |
| Memory | <500MB | <1GB | >1GB |
| Throughput | Handles feed | - | Drops packets |

## Building

```bash
cd multicast_reader_go_eq
go mod init multicast_reader_go_eq
go get golang.org/x/net/ipv4
go build -o benchmark_eq.exe .
```

## Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| ip | 227.0.0.21 | BSE Equity multicast IP |
| port | 26001 | BSE Equity UDP port |
| duration | 0 (infinite) | Benchmark duration |
| buffer | 65536 | UDP receive buffer size |

## Network Requirements

- Must be connected to BSE network
- Multicast routing enabled
- Port 26001/UDP open
- Market hours: 9:00 AM - 3:30 PM IST
