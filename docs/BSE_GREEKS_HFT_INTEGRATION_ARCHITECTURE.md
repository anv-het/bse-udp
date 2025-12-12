# BSE Greeks Calculator - HFT Integration Architecture

## 📋 Executive Summary

**Project Goal:** Real-time Greeks calculation for BSE derivatives (F&O) market data received via UDP multicast feed.

**Current State:**
- ✅ **BSE HFT Go Server**: Receives 600+ packets/sec from BSE UDP feed (239.1.2.5:26002)
- ✅ **Data Rate**: 1,330 records/sec, 78,271 quotes/min, ~4.7M quotes/hour
- ✅ **Python Greeks Calculator**: Batch processing of CSV files (10,716 options in 40 seconds)
- ⚠️ **Gap**: No real-time Greeks calculation during market hours

**Target State:**
- 🎯 Real-time Greeks calculation as data arrives (< 100µs latency per quote)
- 🎯 Live Greeks updates in CSV/database/WebSocket stream
- 🎯 Handle 1,330 calculations/second sustainably

---

## 🏗️ Architecture Overview

### Current Data Flow
```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                     CURRENT ARCHITECTURE (SEPARATED)                            │
└─────────────────────────────────────────────────────────────────────────────────┘

BSE UDP Feed (239.1.2.5:26002)
        │
        │ 600 pkts/sec (NFCAST protocol)
        ↓
┌──────────────────────────────┐
│   BSE-GO-HFT Server (Go)     │
│   - Multicast receiver       │
│   - Binary decoder           │
│   - Token mapping            │
│   - CSV writer               │
└──────────────────────────────┘
        │
        │ CSV files: 20251208_FO_quotes.csv
        ↓
┌──────────────────────────────┐
│  File System (Delayed)       │
│  - 59,992 EQ quotes/min      │
│  - 18,279 FO quotes/min      │
└──────────────────────────────┘
        │
        │ Manual batch processing (OFFLINE)
        ↓
┌──────────────────────────────┐
│ Greeks Calculator (Python)   │
│ - Read CSV                   │
│ - Black-Scholes              │
│ - IV calculation             │
│ - Write Greeks CSV           │
└──────────────────────────────┘
        │
        ↓
    Greeks Output CSV (40 sec latency for 10,716 options)
```

### Proposed Integrated Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                   PROPOSED REAL-TIME ARCHITECTURE                               │
└─────────────────────────────────────────────────────────────────────────────────┘

BSE UDP Feed (239.1.2.5:26002)
        │
        │ 600 pkts/sec, 1,330 records/sec
        ↓
┌────────────────────────────────────────────────────────────────┐
│              BSE-GO-HFT Server (Enhanced)                      │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Stage 1: UDP Receiver (Existing)                       │   │
│  │  - Multicast join                                       │   │
│  │  - Binary packet reception                              │   │
│  │  - Ring buffer (lock-free)                              │   │
│  └─────────────────────────────────────────────────────────┘   │
│                          ↓                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Stage 2: Decoder (Existing)                            │   │
│  │  - NFCAST decompression                                 │   │
│  │  - Token→Symbol mapping                                 │   │
│  │  - Quote extraction                                     │   │
│  └─────────────────────────────────────────────────────────┘   │
│                          ↓                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Stage 3: Greeks Calculation (NEW)                      │   │
│  │  - Filter FO quotes (CE/PE only)                        │   │
│  │  - Black-Scholes pricing                                │   │
│  │  - Delta, Gamma, Theta, Vega, Rho                       │   │
│  │  - Implied Volatility (optional)                        │   │
│  │  - Latency target: < 50µs per option                    │   │
│  └─────────────────────────────────────────────────────────┘   │
│                          ↓                                     │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  Stage 4: Multi-Output (NEW)                            │   │
│  │  ├─ CSV Writer (existing)                               │   │
│  │  ├─ Greeks CSV Writer (new)                             │   │
│  │  ├─ Database Writer (optional)                          │   │
│  │  └─ WebSocket Stream (optional)                         │   │
│  └─────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────┘
        │
        ↓
┌──────────────────────────────────────────────────────────────┐
│                    OUTPUT DESTINATIONS                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐       │
│  │ CSV Files    │  │ PostgreSQL/  │  │ WebSocket    │       │
│  │ - Raw quotes │  │ TimescaleDB  │  │ - Live feed  │       │
│  │ - Greeks     │  │ - Greeks DB  │  │ - Traders    │       │
│  └──────────────┘  └──────────────┘  └──────────────┘       │
└──────────────────────────────────────────────────────────────┘
```

---

## 🔬 Greeks Calculation Strategy

### Option 1: Go Implementation (RECOMMENDED for HFT)

**Pros:**
- Native integration with existing Go pipeline
- Ultra-low latency (< 10µs per option)
- No serialization overhead
- Single binary deployment

**Implementation:**
```go
// pkg/greeks/calculator.go
package greeks

import (
    "math"
)

type Greeks struct {
    Delta float64  // 0 to 1 for calls, -1 to 0 for puts
    Gamma float64  // Rate of delta change
    Theta float64  // Time decay (per day)
    Vega  float64  // Volatility sensitivity (per 1%)
    Rho   float64  // Interest rate sensitivity
    IV    float64  // Implied Volatility (optional)
}

type OptionQuote struct {
    Symbol      string
    Expiry      time.Time
    OptionType  string  // "CE" or "PE"
    StrikePrice float64
    LTP         float64
    Volume      int
    // ... other fields from UDP feed
}

// BlackScholes calculates option Greeks
func (c *Calculator) CalculateGreeks(
    opt OptionQuote,
    spotPrice float64,
    riskFreeRate float64,
    volatility float64,
) Greeks {
    // Time to expiry in years
    t := opt.Expiry.Sub(time.Now()).Hours() / (24 * 365)
    
    // d1 and d2 for Black-Scholes
    d1 := c.calculateD1(spotPrice, opt.StrikePrice, riskFreeRate, volatility, t)
    d2 := d1 - volatility*math.Sqrt(t)
    
    // Calculate Greeks
    greeks := Greeks{}
    
    if opt.OptionType == "CE" {
        greeks.Delta = c.normalCDF(d1)
    } else {
        greeks.Delta = c.normalCDF(d1) - 1
    }
    
    greeks.Gamma = c.normalPDF(d1) / (spotPrice * volatility * math.Sqrt(t))
    greeks.Vega = spotPrice * c.normalPDF(d1) * math.Sqrt(t) / 100
    
    if opt.OptionType == "CE" {
        greeks.Theta = c.calculateCallTheta(spotPrice, opt.StrikePrice, 
            riskFreeRate, volatility, t, d1, d2)
    } else {
        greeks.Theta = c.calculatePutTheta(spotPrice, opt.StrikePrice, 
            riskFreeRate, volatility, t, d1, d2)
    }
    
    // ... rest of Greeks calculation
    
    return greeks
}
```

**Performance Estimate:**
- Black-Scholes calculation: ~5µs per option
- Greeks calculation (all 5): ~8µs per option
- Total latency: < 15µs per option
- Throughput: **66,000 options/second** (far exceeds 1,330/sec requirement)

---

### Option 2: Hybrid Go + Python (Flexible but Slower)

**Architecture:**
```
Go HFT Server
    │
    ↓
Unix Domain Socket / Named Pipe
    │
    ↓
Python Greeks Calculator (Daemon)
    │
    ↓
Output (CSV/DB/WebSocket)
```

**Pros:**
- Reuse existing Python Greeks code
- Easier to modify Greeks logic
- Python ecosystem (pandas, numpy, scipy)

**Cons:**
- IPC latency (200-500µs per message)
- Serialization overhead (JSON/msgpack)
- Additional process management
- Total latency: ~1-2ms per option

**Not recommended for HFT** but acceptable if sub-millisecond latency is not critical.

---

## 📊 Greeks Calculation Logic

### Required Inputs for Each Option

| Input | Source | Example |
|-------|--------|---------|
| **Strike Price** | UDP feed | 85,500 |
| **Option Type** | UDP feed | "CE" or "PE" |
| **Expiry Date** | UDP feed / Token map | "2025-12-11" |
| **Current LTP** | UDP feed | 644.10 |
| **Spot Price** | External feed or manual | 84,733 (SENSEX) |
| **Risk-Free Rate** | Configuration | 0.07 (7% annual) |
| **Volatility** | Historical or IV | 0.15 (15% annual) |

### Calculation Flow for Each Quote

```
┌────────────────────────────────────────────────────────────────┐
│  Step 1: Filter Options                                        │
│  - Check: option_type == "CE" OR option_type == "PE"           │
│  - Check: expiry > current_date                                │
│  - Check: ltp > 0 AND strike_price > 0                         │
└────────────────────────────────────────────────────────────────┘
                         ↓
┌────────────────────────────────────────────────────────────────┐
│  Step 2: Calculate Time to Expiry                             │
│  - T = (expiry_date - current_date) / 365                      │
│  - Example: 3 days = 0.0082 years                              │
└────────────────────────────────────────────────────────────────┘
                         ↓
┌────────────────────────────────────────────────────────────────┐
│  Step 3: Get Spot Price                                        │
│  - SENSEX: From live index feed or BSE API                     │
│  - BANKEX: From live index feed                                │
│  - Cache spot price (update every 100ms)                       │
└────────────────────────────────────────────────────────────────┘
                         ↓
┌────────────────────────────────────────────────────────────────┐
│  Step 4: Calculate d1 and d2 (Black-Scholes)                  │
│                                                                │
│  d1 = [ln(S/K) + (r + σ²/2) × T] / (σ × √T)                   │
│  d2 = d1 - σ × √T                                              │
│                                                                │
│  Where:                                                        │
│  - S = Spot price (84,733)                                     │
│  - K = Strike price (85,500)                                   │
│  - r = Risk-free rate (0.07)                                   │
│  - σ = Volatility (0.15)                                       │
│  - T = Time to expiry (0.0082)                                 │
└────────────────────────────────────────────────────────────────┘
                         ↓
┌────────────────────────────────────────────────────────────────┐
│  Step 5: Calculate Greeks                                     │
│                                                                │
│  Delta (Call) = N(d1)                                          │
│  Delta (Put)  = N(d1) - 1                                      │
│                                                                │
│  Gamma = N'(d1) / (S × σ × √T)                                 │
│                                                                │
│  Theta (Call) = -[S×N'(d1)×σ/(2√T)] - r×K×e^(-rT)×N(d2)       │
│  Theta (Put)  = -[S×N'(d1)×σ/(2√T)] + r×K×e^(-rT)×N(-d2)      │
│                                                                │
│  Vega = S × N'(d1) × √T / 100                                  │
│                                                                │
│  Rho (Call) = K × T × e^(-rT) × N(d2) / 100                    │
│  Rho (Put)  = -K × T × e^(-rT) × N(-d2) / 100                  │
│                                                                │
│  Where N(x) = cumulative normal distribution                   │
│        N'(x) = normal probability density function             │
└────────────────────────────────────────────────────────────────┘
                         ↓
┌────────────────────────────────────────────────────────────────┐
│  Step 6: Optional - Calculate Implied Volatility              │
│  - Use Newton-Raphson iteration                                │
│  - Target: Market LTP = Black-Scholes(σ_implied)               │
│  - Max iterations: 20                                           │
│  - Tolerance: 0.0001                                            │
│  - **Warning:** Computationally expensive (~100µs per option)  │
└────────────────────────────────────────────────────────────────┘
                         ↓
┌────────────────────────────────────────────────────────────────┐
│  Step 7: Attach Greeks to Quote                               │
│  - Original quote fields (token, symbol, LTP, volume...)       │
│  - Add: delta, gamma, theta, vega, rho                         │
│  - Add: moneyness (ITM/ATM/OTM)                                │
│  - Add: intrinsic_value, time_value                            │
└────────────────────────────────────────────────────────────────┘
```

---

## 🎯 Implementation Phases

### Phase 1: Standalone Go Greeks Module (Week 1-2)

**Goals:**
- Port Black-Scholes calculation to Go
- Implement all 5 Greeks (Delta, Gamma, Theta, Vega, Rho)
- Unit tests with validation
- Benchmark performance

**Deliverables:**
```
bse-go-hft/
  pkg/
    greeks/
      calculator.go        # Core Greeks calculator
      black_scholes.go     # Black-Scholes pricing
      normal_dist.go       # CDF/PDF functions
      calculator_test.go   # Unit tests
      benchmark_test.go    # Performance tests
```

**Acceptance Criteria:**
- ✅ All Greeks match Python implementation (< 0.01% difference)
- ✅ Calculation time < 20µs per option
- ✅ Handle edge cases (expired, deep ITM/OTM)

---

### Phase 2: Integrate with HFT Pipeline (Week 3-4)

**Goals:**
- Add Greeks calculator to decoder chain
- Filter FO quotes (CE/PE only)
- Get spot price (SENSEX/BANKEX)
- Calculate Greeks in real-time

**Code Changes:**
```go
// cmd/hft-server/main.go

// Add Greeks calculator
greeksCalc := greeks.NewCalculator(config.RiskFreeRate)

// Add spot price tracker
spotTracker := spotprice.NewTracker()
spotTracker.UpdateSENSEX(84733.0)  // From live feed or manual

// Modify decoder to calculate Greeks for FO quotes
for packet := range packetChannel {
    quotes := decoder.Decode(packet)
    
    for _, quote := range quotes {
        // Save raw quote
        csvWriter.Write(quote)
        
        // If it's an option, calculate Greeks
        if quote.Segment == "FO" && 
           (quote.OptionType == "CE" || quote.OptionType == "PE") {
            
            spotPrice := spotTracker.GetSpot(quote.Symbol)
            
            greeks := greeksCalc.Calculate(
                quote,
                spotPrice,
                config.Volatility,  // Historical or IV
            )
            
            // Attach Greeks to quote
            quoteWithGreeks := QuoteWithGreeks{
                Quote: quote,
                Greeks: greeks,
            }
            
            // Save to Greeks CSV
            greeksWriter.Write(quoteWithGreeks)
        }
    }
}
```

**Deliverables:**
- Enhanced HFT server with Greeks calculation
- Dual CSV output: `20251208_FO_quotes.csv` + `20251208_FO_greeks.csv`
- Performance maintained: P99 latency < 100µs

---

### Phase 3: Spot Price Feed Integration (Week 5)

**Goals:**
- Real-time spot price updates
- Support SENSEX and BANKEX indices
- Handle market hours vs pre-market

**Options for Spot Price:**

**Option A: BSE Equity Feed (239.1.2.5:26001)**
- Real-time SENSEX/BANKEX updates
- Already receiving EQ feed in dual-feed mode
- Need to parse index tokens

**Option B: External API**
- NSE/BSE public APIs
- 5-second delay acceptable (Greeks don't need tick-by-tick spot)
- HTTP polling every 1-5 seconds

**Option C: Manual Input**
- Configuration file: `spot_prices.json`
- Update via admin interface
- Fallback if feeds unavailable

**Recommended: Option A + Option C (hybrid)**

---

### Phase 4: Advanced Features (Week 6+)

**4.1 Implied Volatility Calculation**
- Calculate IV from market LTP
- Cache IV by strike/expiry
- Use for next Greeks calculation

**4.2 Database Integration**
```
PostgreSQL/TimescaleDB Schema:

Table: fo_quotes_live
- timestamp (timestamptz)
- token (int)
- symbol (text)
- strike_price (decimal)
- option_type (text)
- ltp (decimal)
- volume (bigint)
- delta (decimal)
- gamma (decimal)
- theta (decimal)
- vega (decimal)
- rho (decimal)
- iv (decimal)

Indexes:
- ON (timestamp DESC, symbol, strike_price)
- ON (symbol, expiry, strike_price)
```

**4.3 WebSocket Streaming**
```json
{
  "type": "greeks_update",
  "timestamp": "2025-12-08T09:47:04.958Z",
  "symbol": "SENSEX25D1185500CE",
  "quote": {
    "ltp": 644.10,
    "volume": 1941900,
    "bid": [447.90, 447.75, 447.70],
    "ask": [448.80, 448.85, 448.90]
  },
  "greeks": {
    "delta": 0.52,
    "gamma": 0.0003,
    "theta": -125.50,
    "vega": 18.75,
    "rho": 2.15,
    "iv": 0.145
  }
}
```

---

## 📈 Performance Targets

### Current Baseline (Go HFT Server)
| Metric | Value |
|--------|-------|
| Packets/sec | 603 |
| Records/sec | 1,330 |
| Quotes/sec | 1,304 |
| P99 Latency | < 87µs |
| Packet Drops | 0% |

### With Greeks Calculation (Target)
| Metric | Target | Rationale |
|--------|--------|-----------|
| FO Quotes/sec | ~300 | 18,279 quotes/60s = 305/sec |
| Greeks Calc/sec | 300 | One per FO quote |
| Greeks Latency | < 50µs | Add 50µs to existing 87µs |
| **Total P99 Latency** | **< 150µs** | Still HFT-grade |
| Packet Drops | 0% | Maintain zero drops |
| Memory Overhead | +50MB | Greeks calculator state |

**Conclusion:** Greeks calculation adds minimal latency (<50µs), well within HFT requirements.

---

## 🛠️ Development Roadmap

### Week 1-2: Greeks Module Development
- [ ] Port Black-Scholes to Go
- [ ] Implement Delta calculation
- [ ] Implement Gamma calculation
- [ ] Implement Theta calculation
- [ ] Implement Vega calculation
- [ ] Implement Rho calculation
- [ ] Unit tests (match Python results)
- [ ] Benchmark tests (< 20µs per option)

### Week 3-4: HFT Pipeline Integration
- [ ] Add Greeks calculator to decoder
- [ ] Filter FO quotes (CE/PE)
- [ ] Spot price tracker (manual config)
- [ ] Greeks CSV writer
- [ ] End-to-end testing
- [ ] Performance validation (P99 < 150µs)

### Week 5: Spot Price Integration
- [ ] Parse BSE Equity feed for indices
- [ ] Cache spot prices (SENSEX/BANKEX)
- [ ] Fallback to manual config
- [ ] API integration (optional)

### Week 6+: Advanced Features
- [ ] Implied Volatility calculation
- [ ] PostgreSQL/TimescaleDB writer
- [ ] WebSocket server
- [ ] Historical Greeks analysis
- [ ] Greeks charting/visualization

---

## 🔧 Configuration

### Enhanced `config.json`
```json
{
  "feeds": {
    "eq": {
      "ip": "239.1.2.5",
      "port": 26001
    },
    "fo": {
      "ip": "239.1.2.5",
      "port": 26002
    }
  },
  "greeks": {
    "enabled": true,
    "risk_free_rate": 0.07,
    "default_volatility": 0.15,
    "calculate_iv": false,
    "spot_prices": {
      "SENSEX": 84733.0,
      "BANKEX": 67250.0
    },
    "spot_source": "manual",
    "update_interval_ms": 100
  },
  "output": {
    "csv_enabled": true,
    "greeks_csv_enabled": true,
    "database_enabled": false,
    "websocket_enabled": false
  }
}
```

---

## 📝 Sample Output Format

### Enhanced CSV with Greeks
```csv
timestamp,token,symbol,symbol_name,expiry,option_type,strike_price,ltp,volume,delta,gamma,theta,vega,rho,iv,moneyness,intrinsic_value,time_value
2025-12-08 09:47:04.960,1134458,SENSEX,SENSEX25D1185500CE,11-DEC-2025,CE,85500.00,644.10,1941900,0.52,0.0003,-125.50,18.75,2.15,0.145,ATM,0.00,644.10
```

**New Columns:**
- `delta`: Directional risk (0 to 1 for calls, -1 to 0 for puts)
- `gamma`: Delta sensitivity
- `theta`: Time decay per day (negative for long positions)
- `vega`: Volatility sensitivity per 1% move
- `rho`: Interest rate sensitivity
- `iv`: Implied Volatility (if enabled)
- `moneyness`: ITM/ATM/OTM classification
- `intrinsic_value`: Max(S-K, 0) for calls, Max(K-S, 0) for puts
- `time_value`: LTP - intrinsic_value

---

## 🚀 Quick Start (After Implementation)

### Running with Greeks Calculation

```powershell
cd d:\bse\bse-go-hft

# Edit config.json to enable Greeks
# Set spot_prices.SENSEX to current value

# Run HFT server with Greeks
go run ./cmd/hft-server/main.go

# Or build and run
go build -o hft-server.exe ./cmd/hft-server/
.\hft-server.exe -duration 5m
```

### Expected Output
```
================================================================================
         BSE GO HFT SERVER - DUAL FEED WITH GREEKS CALCULATION
================================================================================
Start Time:      2025-12-08 10:00:00
Mode:            BOTH (EQ + FO + GREEKS)
FO Multicast:    239.1.2.5:26002
Greeks:          ENABLED
Spot SENSEX:     84,733.0
Risk-Free Rate:  7.0%
Volatility:      15.0%
================================================================================

✅ All feeds connected! Calculating Greeks...

[10s] Pkts: 6031 | Records: 13543 | EQ: 10144 | FO: 3023 | Greeks: 3023
      Greeks P99: 45µs | Total P99: 132µs
```

---

## 🎓 Greeks Interpretation Guide

### For Traders

| Greek | What It Tells You | Example |
|-------|------------------|---------|
| **Delta** | How much option price changes for ₹1 move in SENSEX | Delta=0.5 means ₹0.50 gain if SENSEX ↑ ₹1 |
| **Gamma** | How fast Delta changes | High Gamma near ATM, watch Delta flip |
| **Theta** | Daily time decay (₹) | Theta=-50 means lose ₹50/day if price flat |
| **Vega** | Gain/loss per 1% volatility change | Vega=20 means ₹20 gain if IV goes 14%→15% |
| **Rho** | Gain/loss per 1% rate change | Rho=2 means ₹2 gain if RBI raises by 1% |

### Key Insights

**High Delta (0.8-1.0):** Deep ITM, behaves like stock  
**Mid Delta (0.4-0.6):** ATM, most sensitive to price moves  
**Low Delta (0.0-0.2):** Deep OTM, lottery tickets

**High Gamma:** Near ATM + Near expiry, Delta changes rapidly  
**High Theta:** Selling options benefits from time decay  
**High Vega:** Long options benefit from volatility spike

---

## 📚 Next Steps

1. **Review this architecture** with development team
2. **Decide on implementation approach** (Go native vs Hybrid)
3. **Set up development environment** for Greeks module
4. **Start Phase 1** (Standalone Greeks calculator)
5. **Iterate and test** with live market data

---

## 📞 Questions to Resolve

1. **Spot Price Source:** Live feed vs API vs Manual?
2. **Implied Volatility:** Calculate live (expensive) or use historical?
3. **Output Destination:** CSV only or also database/WebSocket?
4. **Volatility Model:** Fixed 15% or dynamic per underlying?
5. **Performance Priority:** Sub-100µs (Go) or 1-2ms (Python) acceptable?

---

**Status:** Architecture Design Complete ✅  
**Next:** Team review and implementation decision  
**Timeline:** 6 weeks to production-ready Greeks calculation
