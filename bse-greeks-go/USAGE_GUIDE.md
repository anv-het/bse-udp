# BSE Live Greeks Monitor - Usage Guide

## 🎯 Quick Start

### Basic Usage
```bash
# Monitor single token (1146822 - SENSEX CE 84900)
go run cmd/live-greeks-monitor/main.go -token 1146822 -ticks 10

# Monitor multiple tokens simultaneously
go run cmd/live-greeks-monitor/main.go -token "1146822,1149680" -ticks 20

# Custom output directory and risk-free rate
go run cmd/live-greeks-monitor/main.go -token 1146822 -ticks 5 -output "results" -rate 0.07
```

## 📋 Command-Line Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-token` | string | **Required** | Comma-separated token IDs (e.g., "1146822,1149680") |
| `-ticks` | int | 10 | Maximum number of ticks to display per token |
| `-rate` | float | 0.065 | Risk-free rate (6.5% = 0.065 for Indian T-Bills) |
| `-output` | string | "data/output" | Output directory for CSV files |

## 🧪 Test Tokens (SENSEX 18-Dec-2025, Strike 84900)

| Token | Type | Strike | Current Premium | Greeks Status |
|-------|------|--------|-----------------|---------------|
| **1146822** | CE (Call) | ₹84,900 | ₹383.85 | ✅ All Greeks valid |
| **1149680** | PE (Put) | ₹84,900 | ₹347.15 | ✅ All Greeks valid |

### Example Test Scenarios

#### 1. Single Token (Call Option)
```bash
go run cmd/live-greeks-monitor/main.go -token 1146822 -ticks 5
```
**Expected Output:**
- CE option with positive Delta (~0.52)
- Negative Theta (time decay)
- Positive Vega (benefits from volatility increase)
- IV converges to ~12.30%

#### 2. Call-Put Pair (Same Strike)
```bash
go run cmd/live-greeks-monitor/main.go -token "1146822,1149680" -ticks 10
```
**Expected Output:**
- CE Delta + PE Delta ≈ 1 (Put-Call parity)
- Both have similar Gamma and Vega (ATM options)
- Opposite Delta signs (CE positive, PE negative)
- PE has negative Rho, CE has positive Rho

#### 3. Extended Monitoring (50 ticks)
```bash
go run cmd/live-greeks-monitor/main.go -token 1146822 -ticks 50
```
**Use Case:** Track IV and Greeks changes over multiple updates

#### 4. High Volume (100 ticks)
```bash
go run cmd/live-greeks-monitor/main.go -token "1146822,1149680" -ticks 100
```
**Use Case:** Stress test performance and accuracy

## 📊 Output Format

### Terminal Display
```
════════════════════════════════════════════════════════════════════════════════
  TICK #1  │  09:36:00.676  │  Token: 1146822  │  SENSEX25D1884900CE
════════════════════════════════════════════════════════════════════════════════

  💰 LTP: ₹384.25  ▼ -40.98%
  ────────────────────────────────────────────────────────────────────────────
  Open: ₹657.55  │  High: ₹657.55  │  Low: ₹373.05  │  Prev: ₹651.05
  ATP:  ₹414.14  │  Volume: 1224480  │  Turnover: ₹10L

  📋 OPTION DETAILS
  ────────────────────────────────────────────────────────────────────────────
  Symbol: SENSEX  │  Type: CE  │  Strike: ₹84900.00  │  Expiry: 18-Dec-2025
  Spot Price: ₹84900.06  │  Moneyness: ATM  │  Days to Expiry: 2
  Intrinsic Value: ₹0.06  │  Time Value: ₹384.19

  🎯 OPTION GREEKS
  ────────────────────────────────────────────────────────────────────────────
  Implied Volatility: 12.30%

  Delta:            0.520538   │   Gamma:            0.000438
  Theta:              -73.17   │   Vega:                29.49
  Rho:                  3.33

  🔬 ADVANCED GREEKS
  ────────────────────────────────────────────────────────────────────────────
  Vanna:  -0.132125   │   Vomma:       0.50   │   Charm:  -7.774571

  ⚡ Calculation Time: 0.00 ms
────────────────────────────────────────────────────────────────────────────────
```

### CSV Output
**Filename Format:** `YYYYMMDD_HHMMSS_TOKEN1_TOKEN2_greeks_live.csv`

**Columns (30 total):**
```csv
timestamp,token,symbol,expiry_date,option_type,strike_price,
ltp,prev_close,open,high,low,volume,value,no_of_trades,atp,
spot_price,days_to_expiry,implied_volatility,
delta,gamma,theta,vega,rho,
vanna,vomma,charm,
intrinsic_value,time_value,moneyness,calc_time_ms
```

## 📈 Understanding the Greeks

### Basic Greeks

| Greek | Range | Interpretation | Example (CE 84900) |
|-------|-------|----------------|---------------------|
| **Delta** | -1 to +1 | Price sensitivity to ₹1 spot move | 0.520538 (52 paise per ₹1) |
| **Gamma** | 0 to 0.1 | Rate of Delta change | 0.000438 (Delta changes by this) |
| **Theta** | Negative | Daily time decay | -73.17 (loses ₹73.17 per day) |
| **Vega** | 0 to 100 | Sensitivity to 1% IV change | 29.49 (gains ₹29.49 if IV +1%) |
| **Rho** | -10 to +10 | Sensitivity to 1% rate change | 3.33 (gains ₹3.33 if rate +1%) |

### Advanced Greeks

| Greek | Purpose | Example (CE 84900) |
|-------|---------|---------------------|
| **Vanna** | Delta sensitivity to volatility | -0.132125 |
| **Vomma** | Vega sensitivity to volatility | 0.50 |
| **Charm** | Delta decay over time | -7.774571 |

### Moneyness Classification

| Status | Condition | Color | Example |
|--------|-----------|-------|---------|
| **ITM** (In The Money) | CE: Spot > Strike, PE: Strike > Spot | 🟢 Green | CE at ₹83,000 strike |
| **ATM** (At The Money) | Spot ≈ Strike (within 100 points) | 🟡 Yellow | Both at ₹84,900 |
| **OTM** (Out of The Money) | CE: Strike > Spot, PE: Spot > Strike | 🔴 Red | CE at ₹86,000 strike |

## ⚙️ Performance Metrics

| Metric | Value | Notes |
|--------|-------|-------|
| **Calculation Speed** | <0.01 ms | Per option Greeks calculation |
| **Throughput** | 73,410 ops/sec | Maximum options processed/second |
| **IV Convergence** | 97.8% | Newton-Raphson success rate |
| **Memory Usage** | ~50 MB | For 72 index spot prices + processor |

## 🔧 Troubleshooting

### Issue: "Spot price not found"
**Solution:** Ensure `data/processed_csv/20251215_index_regular.csv` exists with SENSEX/BANKEX data.
```bash
# Check if index file exists
ls data/processed_csv/*_index_regular.csv
```

### Issue: "Failed to open FO CSV"
**Solution:** Run HFT server first to generate FO data:
```bash
cd ..\bse-go-hft
hft-server.exe  # Generates data/processed_csv/YYYYMMDD_fo_regular.csv
```

### Issue: "IV did not converge"
**Causes:**
- Option deeply ITM/OTM (Delta near 0 or 1)
- Extremely short time to expiry (<1 hour)
- LTP = 0 or invalid market price

**Solution:** Check that LTP > 0 and Volume > 0 in CSV data.

### Issue: Terminal colors not working
**Solution:** Windows 10+ required. Enable ANSI support:
```powershell
Set-ItemProperty HKCU:\Console VirtualTerminalLevel -Type DWORD 1
```

## 🚀 Next Steps

### 1. Live UDP Integration
**Current:** Reads from saved CSV files  
**Next:** Connect to live UDP streams from `hft-server.exe` and `hft-index-server.exe`

```bash
# Run both servers (in separate terminals)
cd ..\bse-go-hft
start hft-server.exe            # FO data on port 12996
start hft-index-server.exe      # Index data on port 11401

# Then run Greeks monitor
cd ..\bse-greeks-go
go run cmd/live-greeks-server/main.go  # Future: Full integration
```

### 2. WebSocket Streaming
**Planned:** Stream Greeks to trading applications via WebSocket
```
Greeks Calculator → WebSocket Server → Trading Client
```

### 3. Multi-Token Dashboard
**Planned:** Monitor all SENSEX options (50+ tokens) simultaneously with summary view

## 📝 Examples with Expected Results

### Test 1: Single Call Option (5 ticks)
```bash
go run cmd/live-greeks-monitor/main.go -token 1146822 -ticks 5
```
**Expected Greeks:**
- IV: 12.20% - 12.40%
- Delta: 0.51 - 0.53 (ATM call)
- Gamma: 0.00043 - 0.00045
- Theta: -70 to -75 (2 days to expiry)
- Vega: 28 - 31

### Test 2: Call-Put Pair (10 ticks)
```bash
go run cmd/live-greeks-monitor/main.go -token "1146822,1149680" -ticks 10
```
**Expected Relationships:**
- CE Delta (0.52) + PE Delta (-0.48) ≈ 1.00
- CE Gamma ≈ PE Gamma (both ATM)
- CE Vega ≈ PE Vega (volatility affects both equally)
- CE Theta < PE Theta (call decays faster for dividends)
- CE Rho > 0, PE Rho < 0 (opposite rate sensitivity)

### Test 3: Extended Monitoring (20 ticks)
```bash
go run cmd/live-greeks-monitor/main.go -token 1146822 -ticks 20
```
**Observe:**
- IV stability (should remain 12% ± 0.5%)
- Delta changes with LTP movements
- Gamma peaks when ATM
- Theta accelerates as expiry approaches

## 📚 Related Documentation

- **LIVE_GREEKS_ARCHITECTURE.md** - System design and flow diagrams
- **GREEKS_COMPARISON_REPORT.md** - Accuracy validation against reference
- **BSE_PROJECT_COMPLETE_DOCUMENTATION.md** - Full project overview
- **QUICK_START.md** - Production deployment guide

## ⚡ Performance Tips

1. **Reduce Output:** Use fewer ticks for faster testing
   ```bash
   -ticks 5  # Instead of 100
   ```

2. **Single Token:** Monitor one token for maximum speed
   ```bash
   -token 1146822  # Instead of multiple tokens
   ```

3. **Disable CSV:** Comment out CSV writing in `main.go` for pure calculation speed

4. **Batch Testing:** Use loops for automated testing
   ```bash
   for i in {5,10,20,50,100}; do
     go run cmd/live-greeks-monitor/main.go -token 1146822 -ticks $i
   done
   ```

## 🎓 Educational Use Cases

### 1. Learn Greeks Behavior
Monitor how Greeks change with:
- **Price Movements:** Watch Delta when LTP moves
- **Time Decay:** Observe Theta over hours
- **Volatility Spikes:** See Vega impact during market events

### 2. Validate Put-Call Parity
```bash
# Monitor CE + PE at same strike
go run cmd/live-greeks-monitor/main.go -token "1146822,1149680" -ticks 20
# Verify: CE Premium - PE Premium ≈ Spot - Strike (adjusted for dividends/rates)
```

### 3. Compare ATM vs OTM
```bash
# ATM: 84900 strike (Token 1146822)
# OTM: 85000 strike (Token TBD)
# Compare Delta, Gamma, Vega differences
```

---

**Last Updated:** December 15, 2025  
**Version:** 1.0.0  
**Tested With:** Go 1.21+, Windows 10/11, BSE Live Data (Dec 2025)
