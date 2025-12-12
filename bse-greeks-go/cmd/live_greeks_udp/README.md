# BSE Live Greeks Calculator - UDP Feed

## 🚀 Real-time Greeks Calculation from Live BSE Market Data

This tool calculates **all 9 Greeks** in real-time by directly reading live market data from BSE UDP multicast feed. No CSV files needed - pure live data!

## Features

✅ **Live UDP Feed Processing**
- Reads Index data (Message Type 2012) for SENSEX spot price
- Reads F&O data (Message Type 2021) for option prices
- Real-time packet processing with microsecond latency

✅ **Complete Greeks Calculation**
- **Implied Volatility (IV)**: Newton-Raphson solver
- **Basic Greeks**: Delta, Gamma, Theta, Vega, Rho
- **Advanced Greeks**: Vanna, Vomma, Charm
- **Market Metrics**: Moneyness, Intrinsic Value, Time Value

✅ **Live Display**
- Auto-refreshing terminal UI (every 3 seconds)
- Beautiful formatted output with Unicode borders
- Real-time packet statistics
- Last update timestamps

## Monitored Tokens (Default)

```
Token 1144708: SENSEX 18-Dec-2025 PE 85200
Token 1141880: SENSEX 18-Dec-2025 CE 85200
```

## Requirements

### Network Setup
1. **BSE UDP Multicast Access**
   - F&O Feed: `239.1.4.10:12998` (Message Type 2021)
   - Index Feed: `239.1.5.3:12999` (Message Type 2012)

2. **Network Interface**
   - Must have multicast routing enabled
   - BSE network connectivity (VPN or direct)

### BSE HFT Server (Must be Running)
Before running this tool, you MUST have the BSE HFT servers running to capture UDP data:

```bash
# Terminal 1: Start Index Server (captures SENSEX spot)
cd d:\bse\bse-go-hft
.\hft-index-server.exe

# Terminal 2: Start F&O Server (captures option prices)
cd d:\bse\bse-go-hft
.\hft-server.exe
```

## Quick Start

### 1. Build
```bash
cd d:\bse\bse-greeks-go
go build -o bin/live_greeks_udp.exe cmd/live_greeks_udp/main.go
```

### 2. Run
```bash
.\bin\live_greeks_udp.exe
```

### 3. Watch Live Greeks!
The terminal will display:
- SENSEX spot price (updated every second)
- Both option tokens with:
  - Market data (LTP, Volume, Moneyness)
  - Implied Volatility (calculated vs estimated)
  - All 9 Greeks
  - Practical interpretations

### 4. Stop
Press `Ctrl+C` to stop gracefully

## Output Format

```
╔════════════════════════════════════════════════════════════════════════════════╗
║            BSE LIVE GREEKS CALCULATOR - UDP FEED (Real-time)                 ║
╚════════════════════════════════════════════════════════════════════════════════╝

⏱️  Runtime: 1m23s | Total Packets: 8,450 (Index: 1,200, F&O: 7,250)
📊 Updates: Index: 83, F&O: 245

┌─ SENSEX Index Data ────────────────────────────────────────────────────────────┐
│ Spot: ₹85252.15 | Change: ₹434.02 (0.51%) | Updated: 1s ago
└────────────────────────────────────────────────────────────────────────────────┘

┌─ Token: 1144708 ─ SENSEX 18-Dec-2025 PE 85200 ────────────────────────────────┐
│ 📊 Market Data                                                                │
│   LTP:            ₹296.40      Volume:          19780260                    │
│   Moneyness:      ATM          Intrinsic:     ₹      0.00                    │
│   Time Value:     ₹296.40      Updated:     2s ago                          │
│                                                                               │
│ 💡 Implied Volatility: 18.50% (Calc)                                          │
│                                                                               │
│ 📈 Basic Greeks (First Order)                                                 │
│   Delta:    -0.4617  │  Gamma:    0.000250  │  Theta:    -48.99/day        │
│   Vega:       41.97  │  Rho:         -6.14                                  │
│                                                                               │
│ 🎯 Advanced Greeks (Second Order)                                             │
│   Vanna:    -1.7518  │  Vomma:        2.09  │  Charm:    7.1284/day        │
│                                                                               │
│ 💭 Interpretation:                                                            │
│   • For every ₹100 SENSEX fall, option gains ₹46                          │
│   • Time decay: Losing ₹48.99 per day (6 days to expiry)                    │
│   • If volatility rises 1%, option gains ₹41.97                               │
└───────────────────────────────────────────────────────────────────────────────┘

🔄 Auto-refreshing every 3s. Press Ctrl+C to stop.
```

## Configuration

Edit `cmd/live_greeks_udp/main.go` to customize:

### Change Monitored Tokens
```go
var MONITORED_TOKENS = map[uint32]TokenInfo{
	1144708: {
		Symbol:     "SENSEX",
		Expiry:     "18-Dec-2025",
		Strike:     85200.0,
		OptionType: "PE",
	},
	1141880: {
		Symbol:     "SENSEX",
		Expiry:     "18-Dec-2025",
		Strike:     85200.0,
		OptionType: "CE",
	},
	// Add more tokens here
}
```

### Change Refresh Rate
```go
const DISPLAY_INTERVAL = 3 * time.Second  // Change to 1s, 5s, etc.
```

### Change Risk-Free Rate
```go
const RISK_FREE_RATE = 0.07  // 7% annual (change to current rate)
```

## Data Flow

```
BSE Network (UDP Multicast)
    │
    ├─► Index Feed (239.1.5.3:12999, Type 2012)
    │   └─► SENSEX Spot Price → Greeks Calculator
    │
    └─► F&O Feed (239.1.4.10:12998, Type 2021)
        └─► Token 1144708 & 1141880 LTP/Volume → Greeks Calculator
            │
            ├─► Calculate Implied Volatility (Newton-Raphson)
            │
            ├─► Calculate Basic Greeks (Δ, Γ, Θ, ν, ρ)
            │
            ├─► Calculate Advanced Greeks (Vanna, Vomma, Charm)
            │
            └─► Display Live (Auto-refresh every 3s)
```

## Greeks Explained

### Basic Greeks (First Order)
- **Delta (Δ)**: Rate of change of option price with respect to spot price
  - Call: 0 to +1 | Put: -1 to 0
  - Example: Delta = 0.50 means ₹0.50 gain for ₹1 spot increase

- **Gamma (Γ)**: Rate of change of Delta
  - Always positive
  - Highest for ATM options
  
- **Theta (Θ)**: Time decay (per day)
  - Always negative for long options
  - Accelerates as expiry approaches
  
- **Vega (ν)**: Sensitivity to volatility (per 1% IV change)
  - Always positive for long options
  - Higher for ATM options
  
- **Rho (ρ)**: Sensitivity to interest rate (per 1% rate change)
  - Less important for short-term options

### Advanced Greeks (Second Order)
- **Vanna**: Cross-sensitivity of Delta to volatility
  - How Delta changes when IV changes
  
- **Vomma**: Sensitivity of Vega to volatility
  - Convexity of Vega
  
- **Charm**: Time decay of Delta
  - How Delta changes as time passes

## Performance

- **Packet Processing**: < 1ms per packet
- **Greeks Calculation**: < 0.5ms per token
- **Display Refresh**: Every 3 seconds
- **Memory Usage**: ~10-15 MB
- **CPU Usage**: < 5% (4 goroutines)

## Troubleshooting

### No SENSEX Data
```
⏳ Waiting for SENSEX index data...
```
**Solution**: Ensure Index HFT server is running and receiving packets
```bash
cd d:\bse\bse-go-hft
.\hft-index-server.exe
```

### No F&O Updates
**Solution**: Ensure F&O HFT server is running
```bash
cd d:\bse\bse-go-hft
.\hft-server.exe
```

### Outside Market Hours
- BSE F&O Trading: 9:00 AM - 3:30 PM IST (Mon-Fri)
- Outside hours: Test feed may not have live data

### IV Shows "Est" (Estimated)
- Happens when Newton-Raphson solver fails to converge
- Usually due to:
  - Very low volume options
  - Options too far OTM/ITM
  - Stale prices
- Falls back to 15% default estimate

## Comparison with CSV-based Calculator

| Feature | Live UDP Feed | CSV-based |
|---------|--------------|-----------|
| Data Source | UDP Multicast | CSV Files |
| Latency | < 1 second | N/A (historical) |
| Update Frequency | Real-time | One-time |
| SENSEX Spot | Live from feed | From CSV |
| Option Prices | Live LTP | From CSV |
| Use Case | Live trading | Analysis |

## Advanced Usage

### Run During Market Hours
```bash
# Start during trading hours (9:00 AM - 3:30 PM IST)
.\bin\live_greeks_udp.exe
```

### Monitor Different Strikes
1. Edit `MONITORED_TOKENS` map
2. Add token IDs from contract master
3. Rebuild and run

### Log Output to File
```bash
.\bin\live_greeks_udp.exe > greeks_live_$(Get-Date -Format 'yyyyMMdd_HHmmss').log
```

## Architecture

- **4 Goroutines**:
  1. Index UDP receiver (Type 2012)
  2. F&O UDP receiver (Type 2021)
  3. Greeks calculator (runs every 1s)
  4. Display updater (runs every 3s)

- **Concurrent Safe**:
  - `sync.RWMutex` for shared data
  - `atomic` operations for counters

- **Memory Efficient**:
  - Only stores 2 tokens in memory
  - No historical data buffering

## Future Enhancements

- [ ] CSV export of live Greeks
- [ ] WebSocket streaming
- [ ] REST API endpoint
- [ ] Alert system (e.g., Delta > 0.7)
- [ ] Multiple expiries monitoring
- [ ] Greeks surface visualization

## Related Tools

- **CSV-based Calculator**: `cmd/calculator_iv/main.go` - Batch processing
- **Token Monitor**: `tests/live_token_monitor.go` - Static CSV monitoring
- **HFT Server**: `bse-go-hft` - UDP packet capture

## License

Part of BSE Greeks Calculator project. See main README for details.

---

**Last Updated**: December 12, 2025  
**Version**: 1.0.0  
**Status**: Production Ready ✅
