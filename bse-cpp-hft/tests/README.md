# BSE HFT C++ - Test Suite

Test and benchmark tools for the BSE HFT C++ system.

## Build Tests

From the `bse-cpp-hft` directory:

```batch
# Benchmark Tool
cl.exe /EHsc /std:c++17 /O2 /I include tests\benchmark.cpp /Fe:bin\benchmark.exe ws2_32.lib

# Live SENSEX Monitor
cl.exe /EHsc /std:c++17 /O2 /I include tests\test_live_sensex.cpp /Fe:bin\test_live_sensex.exe ws2_32.lib

# Live Token Monitor
cl.exe /EHsc /std:c++17 /O2 /I include tests\test_live_token.cpp /Fe:bin\test_live_token.exe ws2_32.lib
```

## Test Tools

### 1. Benchmark Tool (`benchmark.exe`)

Comprehensive performance benchmark for the HFT system.

**Usage:**
```batch
# Benchmark EQ feed (Port 26001)
.\bin\benchmark.exe -port 26001 -duration 30

# Benchmark FO feed (Port 26002)
.\bin\benchmark.exe -port 26002 -duration 60

# Custom settings
.\bin\benchmark.exe -ip 239.1.2.5 -port 26002 -duration 120
```

**Metrics:**
- Throughput: Packets/sec, Records/sec, MB/s
- Latency: Min, Mean, Max, P50, P90, P99, P99.9 (microseconds)
- Reliability: Drops, Drop Rate %

### 2. Live SENSEX Monitor (`test_live_sensex.exe`)

Monitors SENSEX futures and options in real-time.

**Usage:**
```batch
# Default: 30 seconds on FO port
.\bin\test_live_sensex.exe

# Custom duration
.\bin\test_live_sensex.exe -duration 60

# Custom port (EQ)
.\bin\test_live_sensex.exe -port 26001 -duration 30
```

**Features:**
- Tracks predefined SENSEX contracts
- Shows live price updates
- Calculates price changes and statistics

### 3. Live Token Monitor (`test_live_token.exe`)

Monitors any specific token with tick-by-tick updates.

**Usage:**
```batch
# Monitor SENSEX Future (Token 1102290 on FO port)
.\bin\test_live_token.exe -token 1102290 -port 26002 -ticks 50

# Monitor Reliance (Token 500325 on EQ port)
.\bin\test_live_token.exe -token 500325 -port 26001 -ticks 100

# Monitor with more ticks
.\bin\test_live_token.exe -token 873830 -port 26002 -ticks 200
```

**Features:**
- Real-time tick display
- Order book visualization
- Price change tracking (color-coded)
- Summary statistics

## BSE Feed Ports

| Port  | Feed Type     | Description              |
|-------|---------------|--------------------------|
| 26001 | EQ (Cash)     | Equity Cash Market       |
| 26002 | FO (F&O)      | Futures & Options        |

## Output Example

### Benchmark Results
```
+==============================================================================+
|                         BENCHMARK RESULTS                                   |
+==============================================================================+
| Duration: 30.0s  Data: 18.5 MB
+------------------------------------------------------------------------------+
| THROUGHPUT                                                                   |
|   Packets:          12,450  Rate:       415/s                               |
|   Records:          31,200  Rate:     1.04K/s                               |
|   Data:           18.5 MB   Rate:    632 KB/s                               |
+------------------------------------------------------------------------------+
| LATENCY (microseconds)                                                       |
|   Min:      0.5  Mean:      4.2  Max:     125.3                             |
|   P50:      3.2  P90:       8.5  P99:      22.1  P99.9:   65.2              |
+------------------------------------------------------------------------------+
| RELIABILITY                                                                  |
|   Drops:          0  Drop Rate: 0.0000%                                     |
|   Rating: PERFECT - Zero packet drops!                                      |
+==============================================================================+
```

## Tips

1. **Run during market hours** for live data (9:15 AM - 3:30 PM IST)
2. **Use larger duration** for more accurate statistics
3. **Check token validity** - tokens change with contract expiry
4. **Monitor both feeds** - EQ and FO have different characteristics
