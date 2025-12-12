# BSE Go HFT - Quick Start Guide

## 🚀 Start Here

### Current Status
✅ **Working:** Message types 2020 (Equity) and 2021 (Derivatives)  
🔴 **To Do:** Implement message types 2011, 2012, 2013, 2015, and others

---

## 📦 What You Have Now

### Working Commands

```powershell
# Run the main HFT server (captures 2020/2021 only)
cd d:\bse\bse-go-hft
go run ./cmd/hft-server/main.go

# Run for specific duration
go run ./cmd/hft-server/main.go -duration 5m

# Run benchmark test
go run ./cmd/benchmark/main.go -duration 1m

# NEW: Capture and analyze ALL message types
go run ./cmd/packet-sniffer/main.go -duration 5m -output ./captures
```

---

## 🎯 Your Next Steps (In Order)

### Step 1: Capture Packets (Do This First!)

```powershell
# Capture all message types for 5 minutes
cd d:\bse\bse-go-hft
go run ./cmd/packet-sniffer/main.go -duration 5m -output ./captures

# While running, you'll see live updates like:
# [MsgType 2011] Captured: 100 packets, Size: 150-200 bytes
# [MsgType 2020] Captured: 5000 packets, Size: 564-1620 bytes
# [MsgType 2021] Captured: 3000 packets, Size: 564-1620 bytes
```

**Output:** Packet samples saved in `./captures/msgtype_XXXX/`

### Step 2: Analyze Captured Data

```powershell
# View what message types were captured
dir captures

# Look at message type 2011 samples
dir captures\msgtype_2011

# View hex dump
notepad captures\msgtype_2011\sample_1.hex

# View analysis
notepad captures\msgtype_2011\sample_1.txt
```

### Step 3: Read BSE Documentation

Open these PDFs and search for "Message Type 2011":
- `d:\bse\bse-go-hft\docs\BSE_DIRECT_NFCAST_Manual.pdf`
- `d:\bse\bse-go-hft\docs\BOLTPLUS Connectivity Manual V1.14.1.pdf`

**What to look for:**
- Field names and offsets
- Data types (uint32, int32, string, etc.)
- Byte order (Big-Endian or Little-Endian)
- Field sizes

### Step 4: Create Message Type 2011 Decoder

Look at the existing decoder as a template:
- **Template:** `internal/decoder/decoder.go`
- **Create:** `internal/decoder/decoder_2011.go`

Copy the pattern from the existing decoder:
```go
func (d *Decoder) DecodeMsgType2011(packet []byte, length int) (*domain.MarketBroadcast, error) {
    // Parse header (same as 2020/2021)
    header := d.parseHeader(packet)
    
    // Parse data fields (different for each message type)
    // TODO: Extract fields based on BSE documentation
    
    return broadcast, nil
}
```

### Step 5: Test Your Decoder

```powershell
# Create test file
# internal/decoder/decoder_2011_test.go

# Run test
go test ./internal/decoder/ -v
```

### Step 6: Integrate into Main Pipeline

Edit `cmd/hft-server/main.go` and add message type routing:
```go
msgType := binary.LittleEndian.Uint16(buffer[8:10])

switch msgType {
case 2011:
    // Decode market broadcast (NEW!)
    broadcast, err := dec.DecodeMsgType2011(buffer[:n], n)
    // ... save to CSV
    
case 2020:
    // Existing equity decoder
    
case 2021:
    // Existing derivative decoder
}
```

### Step 7: Run Live Test

```powershell
# Run for 1 minute
go run ./cmd/hft-server/main.go -duration 1m

# Check output
dir data\processed_csv
type data\processed_csv\20251211_market_broadcast.csv
```

---

## 📁 Important Files

### Code Files to Modify

| File | What to Do | Priority |
|------|-----------|----------|
| `internal/decoder/decoder_2011.go` | **CREATE** - Decoder for message type 2011 | 🔴 HIGH |
| `pkg/domain/market_broadcast.go` | **CREATE** - Data structure for 2011 | 🔴 HIGH |
| `internal/saver/market_broadcast.go` | **CREATE** - CSV saver for 2011 | 🔴 HIGH |
| `cmd/hft-server/main.go` | **MODIFY** - Add routing for 2011 | 🔴 HIGH |

### Documentation Files

| File | Purpose |
|------|---------|
| `docs/PROJECT_SUMMARY_AND_NEXT_STEPS.md` | Complete implementation plan |
| `docs/MESSAGE_TYPES_ANALYSIS.md` | All message types analysis |
| `docs/COMPLETE_HFT_GUIDE.md` | Technical documentation |
| `QUICK_START.md` | This file |

### Reference Files

| File | Use Case |
|------|----------|
| `internal/decoder/decoder.go` | Template for new decoders |
| `internal/saver/csv.go` | Template for CSV savers |
| `cmd/packet-sniffer/main.go` | Analyze packets |

---

## 🔍 Understanding the Current Code

### How Message Type 2020/2021 Works

```
1. UDP Packet Received
   ↓
2. Check Message Type (offset 8-9)
   ↓
3. If 2020 or 2021:
   - Parse header (36 bytes)
   - Parse records (up to 6 × 264 bytes)
   - Extract: Token, LTP, Volume, Order Book
   - Convert paise to rupees (÷ 100)
   - Save to CSV
   ↓
4. Record statistics
```

### Key Concepts

**Endianness:**
```go
// Big-Endian (Network byte order)
value := binary.BigEndian.Uint16(packet[4:6])

// Little-Endian (x86 native)
value := binary.LittleEndian.Uint16(packet[8:10])
```

**Paise to Rupees:**
```go
// All prices in BSE are in paise (1 Rupee = 100 paise)
priceInPaise := int32(binary.LittleEndian.Uint32(packet[offset:]))
priceInRupees := float64(priceInPaise) / 100.0
```

**Skip Empty Records:**
```go
token := binary.LittleEndian.Uint32(packet[offset:])
if token <= 1 {
    // Skip this record (NULL/empty)
    continue
}
```

---

## 🛠️ Troubleshooting

### Issue: "No packets received"
**Solution:** Check network connection and multicast routing

### Issue: "All data looks like garbage"
**Solution:** Wrong endianness - try switching Big/Little-Endian

### Issue: "Prices are way off"
**Solution:** Forgot to divide by 100 (paise → rupees)

### Issue: "Lots of zero/NULL records"
**Solution:** This is normal - skip records where token <= 1

### Issue: "Packet too small errors"
**Solution:** Check if packet length < 36 bytes (minimum header size)

---

## 📊 Expected Output

### After Implementing Message Type 2011

```
data/processed_csv/
├── 20251211_market_broadcast.csv   ← NEW! Message type 2011
├── 20251211_EQ_quotes.csv          ← Existing (message type 2020)
└── 20251211_FO_quotes.csv          ← Existing (message type 2021)
```

### Sample Market Broadcast CSV

```csv
Timestamp,Market_Status,Session_ID,Trading_Phase,Circuit_Status,System_Message
2025-12-11 09:00:00.000,PRE_OPEN,1,1,NORMAL,Pre-open session started
2025-12-11 09:15:00.000,OPEN,1,2,NORMAL,Market opened
2025-12-11 15:30:00.000,CLOSED,1,5,NORMAL,Market closed
```

---

## 🎯 Success Checklist

For Message Type 2011 to be "Done":

- [ ] Packet sniffer captures message type 2011
- [ ] BSE documentation reviewed for field layouts
- [ ] Data structure created (`market_broadcast.go`)
- [ ] Decoder implemented (`decoder_2011.go`)
- [ ] CSV saver created (`market_broadcast.go`)
- [ ] Integration into main.go complete
- [ ] Unit tests written and passing
- [ ] Live test runs for 1 hour without errors
- [ ] CSV output validated manually
- [ ] Performance metrics acceptable (latency < 100µs)
- [ ] Documentation updated

---

## 💡 Pro Tips

1. **Start Small:** Get one field working first, then add more
2. **Use Hex Dumps:** They're your best friend for debugging
3. **Test Incrementally:** Don't wait until everything is done
4. **Copy Patterns:** Use existing decoder.go as template
5. **Add Debug Logs:** Print hex values when stuck
6. **Compare Packets:** Look at multiple samples to find patterns

---

## 🚨 Common Mistakes to Avoid

❌ **Don't:** Try to implement all message types at once  
✅ **Do:** Focus on message type 2011 first

❌ **Don't:** Guess field offsets  
✅ **Do:** Count bytes carefully from hex dumps

❌ **Don't:** Assume same structure as 2020/2021  
✅ **Do:** Read BSE documentation for each message type

❌ **Don't:** Ignore NULL records  
✅ **Do:** Skip records where token == 0

---

## 📞 Need Help?

1. **Check hex dumps** in captures folder
2. **Compare with** existing decoder.go
3. **Read** BSE PDF documentation
4. **Test with** small packets first
5. **Add** debug logging

---

**Ready to start? Run the packet sniffer!**

```powershell
cd d:\bse\bse-go-hft
go run ./cmd/packet-sniffer/main.go -duration 5m -output ./captures
```

**Good luck! 🚀**
