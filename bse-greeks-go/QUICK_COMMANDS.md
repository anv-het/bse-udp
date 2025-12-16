# Quick Command Reference - BSE Greeks Calculators

## 🚀 UDP-Based (Live Real-Time) ⭐ **NEW!**

### Start UDP Servers (Required First!)

```bash
# Terminal 1: F&O Feed
cd d:\bse\bse-go-hft
.\hft-server.exe

# Terminal 2: Index Feed  
cd d:\bse\bse-go-hft
.\hft-index-server.exe
```

### Monitor Live Greeks

```bash
cd d:\bse\bse-greeks-go

# Single token (SENSEX CE 84900)
.\bin\live-greeks-udp.exe -token 1146822 -ticks 10

# Different token (SENSEX PE 84900)
.\bin\live-greeks-udp.exe -token 1149680 -ticks 20

# Custom risk-free rate
.\bin\live-greeks-udp.exe -token 1146822 -rate 0.07 -ticks 10

# More ticks
.\bin\live-greeks-udp.exe -token 1146822 -ticks 50
```

---

## 📁 CSV-Based (Saved Data)

### Run Greeks Monitor (No UDP needed)

```bash
cd d:\bse\bse-greeks-go

# Single token
go run cmd/live-greeks-monitor/main.go -token 1146822 -ticks 10

# Multiple tokens
go run cmd/live-greeks-monitor/main.go -token "1146822,1149680" -ticks 20

# Custom CSV file
go run cmd/live-greeks-monitor/main.go -token 1146822 -ticks 5 -output "custom_results"
```

---

## 🧪 Test Tokens

| Token | Symbol | Type | Strike | Expiry |
|-------|--------|------|--------|--------|
| 1146822 | SENSEX | CE | 84,900 | 18-Dec-2025 |
| 1149680 | SENSEX | PE | 84,900 | 18-Dec-2025 |

---

## 📊 Quick Comparison

| Feature | UDP Monitor | CSV Monitor |
|---------|-------------|-------------|
| **Command** | `.\bin\live-greeks-udp.exe` | `go run cmd/live-greeks-monitor/main.go` |
| **Requires UDP** | ✅ Yes | ❌ No |
| **Latency** | <5 ms | Minutes old |
| **Use Case** | Live trading | Backtesting |
| **Spot Price** | Real-time | From CSV |

---

## 🔧 Build Commands

```bash
cd d:\bse\bse-greeks-go

# Build UDP monitor
go build -o bin/live-greeks-udp.exe ./cmd/live-greeks-udp

# Build CSV monitor
go build -o bin/live-greeks-monitor.exe ./cmd/live-greeks-monitor
```

---

## 📚 Documentation

- `LIVE_UDP_GREEKS_GUIDE.md` - Complete UDP monitor guide
- `USAGE_GUIDE.md` - CSV monitor guide  
- `LIVE_UDP_COMPLETE.md` - Project completion summary
- `LIVE_GREEKS_ARCHITECTURE.md` - System architecture

---

**Last Updated:** December 15, 2025
