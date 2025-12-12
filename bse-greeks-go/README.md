# BSE Greeks Calculator (Go)

Comprehensive Greeks calculator for BSE F&O derivatives with **real-time UDP feed support**. Calculates all 9 Greeks including Implied Volatility using Black-Scholes-Merton model.

## 🎯 Key Features

- ✅ **Live Greeks Calculator**: Real-time calculation from BSE UDP multicast feed
- ✅ **All 9 Greeks**: Delta, Gamma, Theta, Vega, Rho, Vanna, Vomma, Charm
- ✅ **Implied Volatility**: Newton-Raphson solver for market IV
- ✅ **High Performance**: Processes 50,000+ options/second
- ✅ **CSV Processing**: Batch processing with enhanced output
- ✅ **Zero Dependencies**: Uses only Go standard library
- ✅ **Production Ready**: Thread-safe, low latency (<1ms)

## 🚀 Quick Start Options

### Option 1: Live Real-Time Greeks (NEW! ⭐)

**Monitor live Greeks from BSE UDP feed - no CSV files needed!**

```powershell
# Step 1: Start BSE HFT Servers (required)
# Terminal 1: Index Server
cd d:\bse\bse-go-hft
.\hft-index-server.exe

# Terminal 2: F&O Server
cd d:\bse\bse-go-hft
.\hft-server.exe

# Step 2: Run Live Greeks Calculator
cd d:\bse\bse-greeks-go
.\bin\live_greeks_udp.exe
```

**See**: [Live Greeks Complete Guide](LIVE_GREEKS_COMPLETE.md)

### Option 2: Batch CSV Processing

**Process historical data from CSV files:**

```powershell
cd d:\bse\bse-greeks-go

# Process with automatic IV calculation
go run cmd/calculator_iv/main.go `
  -fo-file "d:\bse\bse-go-hft\data\processed_csv\20251212_FO_quotes.csv" `
  -index-file "d:\bse\bse-go-hft\data\processed_csv\20251212_index_regular.csv" `
  -output "data/output/greeks_iv_20251212.csv"
```

### Option 3: Live Token Monitor

**Monitor specific tokens with auto-refresh:**

```powershell
cd d:\bse\bse-greeks-go

.\bin\live_token_monitor.exe `
  -fo-file "d:\bse\bse-go-hft\data\processed_csv\20251212_FO_quotes.csv" `
  -index-file "d:\bse\bse-go-hft\data\processed_csv\20251212_index_regular.csv" `
  -tokens "1144708,1141880" `
  -refresh 5
```

## 📊 What You Get

### Live Greeks Display
```
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
```

## Project Structure

```
bse-greeks-go/
├── pkg/greeks/          # Core Greeks calculator
│   ├── normal.go        # Normal distribution (CDF/PDF)
│   ├── calculator.go    # Black-Scholes & Greeks
│   └── calculator_test.go
├── pkg/processor/       # CSV processor
│   └── csv_processor.go
├── cmd/calculator/      # Main application
│   └── main.go
├── data/
│   ├── input/          # BSE FO quotes CSV
│   └── output/         # Enhanced CSV with Greeks
└── go.mod
```

## Greeks Calculated

| Greek | Description | Range |
|-------|-------------|-------|
| **Delta** | Directional risk | 0 to 1 (calls), -1 to 0 (puts) |
| **Gamma** | Delta sensitivity | Always positive |
| **Theta** | Time decay per day | Usually negative |
| **Vega** | Volatility sensitivity | Always positive |
| **Rho** | Interest rate sensitivity | Varies by option type |

## Input CSV Format

BSE FO quotes CSV must have these columns:

```csv
timestamp,token,symbol,expiry,option_type,strike_price,ltp,volume,bid_prices,ask_prices
2025-12-08 15:29:59,842364,SENSEX,2025-12-12,CE,85500.00,45.50,1000,44.0,46.0
```

## Output CSV Format

Enhanced CSV includes all input columns plus:

```csv
...,moneyness,delta,gamma,theta,vega,rho,intrinsic_value,time_value
...,OTM,0.3245,0.000123,-12.45,18.32,5.67,0.00,45.50
```

## Configuration Parameters

```bash
-input    # Input CSV file path (required)
-output   # Output CSV file path (auto-generated if omitted)
-rate     # Risk-free rate, default 0.07 (7%)
-vol      # Volatility, default 0.15 (15%)
-sensex   # SENSEX spot price, default 84733.0
-bankex   # BANKEX spot price, default 67250.0
```

## Performance

Expected performance on typical hardware:

- **Throughput**: 50,000+ options/second
- **Latency**: ~20 µs per option
- **Memory**: Minimal (no external dependencies)

```
BenchmarkGreeksCalculation-8    100000    15234 ns/op
```

## Example Output

```
=== BSE Greeks Calculator ===
Input:  data/input/20251208_FO_quotes.csv
Output: data/output/greeks_20251208.csv

Parameters:
  Risk-Free Rate: 7.00%
  Volatility:     15.00%
  SENSEX Spot:    84733.00
  BANKEX Spot:    67250.00

Processing options...

=== Processing Summary ===
Total Options: 1245

By Symbol:
  SENSEX: 892 options
  BANKEX: 353 options

By Moneyness:
  ITM: 412 options
  ATM: 156 options
  OTM: 677 options

Sample Greeks (first 5 options):

SENSEX CE 85500 OTM (Expiry: 2025-12-12):
  Delta:  0.3245  Gamma: 0.000123
  Theta: -12.45  Vega:   18.32  Rho:   5.67

=== Performance ===
Total Time:    24.567ms
Options/sec:   50678
µs per option: 19.73

✅ Processing complete!
```

## Testing with Real Data

```powershell
# Test with all available dates
$dates = @("20251203", "20251204", "20251205", "20251206", "20251208")

foreach ($date in $dates) {
    Write-Host "Processing $date..."
    
    Copy-Item "d:\bse\bse-go-hft\data\processed_csv\${date}_FO_quotes.csv" `
              "d:\bse\bse-greeks-go\data\input\"
    
    go run cmd/calculator/main.go `
      -input "data/input/${date}_FO_quotes.csv" `
      -output "data/output/greeks_${date}.csv"
}
```

## Validation

Compare results with Python Greeks calculator:

```powershell
# Python calculator
cd d:\bse\geek_cal_python
python src\main.py

# Go calculator
cd d:\bse\bse-greeks-go
go run cmd/calculator/main.go -input data/input/20251208_FO_quotes.csv

# Compare Delta values for same options
```

## Next Steps

1. **Validate**: Compare Go vs Python Greeks for same options
2. **Optimize**: Profile and optimize if needed (target <150µs P99)
3. **Integrate**: Add to bse-go-hft server as real-time module
4. **Enhance**: Add implied volatility calculation
5. **Monitor**: Add metrics and logging

## References

- [Black-Scholes Model](https://en.wikipedia.org/wiki/Black%E2%80%93Scholes_model)
- [Greeks (finance)](https://en.wikipedia.org/wiki/Greeks_(finance))
- [BSE Market Data Documentation](https://www.bseindia.com/)

---

**Status**: ✅ Ready for testing with BSE FO quotes data  
**Performance Target**: < 20 µs per option (tested)  
**Dependencies**: Go 1.21+ only (no external packages)
