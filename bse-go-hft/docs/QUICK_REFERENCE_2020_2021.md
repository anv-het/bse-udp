# Quick Reference - Message Types 2020 & 2021

## 🎯 **Your Questions - Answered**

### ❓ "Does we get symbol in package?"
**❌ NO** - Symbols are **NOT** in packets. Only Token IDs (uint32).

### ❓ "Does timestamp get or not?"
**✅ YES** - Timestamp is in packet header (Hour:Minute:Second.Millisecond).

### ❓ "Check what other data we get?"
**✅ YES** - We get 38 fields total (see below).

---

## 📊 **Complete Field List (38 Total)**

```
┌─────────────────────────────────────────────────────────────┐
│ ✅ AVAILABLE IN PACKETS (38 fields)                         │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ 📦 HEADER (7 fields):                                       │
│    • Format ID                                              │
│    • Message Type (2020=Equity, 2021=Derivatives)           │
│    • Hour, Minute, Second, Millisecond                      │
│    • Combined Timestamp                                     │
│                                                             │
│ 🔢 TOKEN (1 field):                                         │
│    • Token ID (uint32) - Example: 543904                    │
│                                                             │
│ 💰 PRICES (6 fields):                                       │
│    • Open Price         - ₹2168.85                          │
│    • High Price         - ₹2177.35                          │
│    • Low Price          - ₹2168.85                          │
│    • Previous Close     - ₹2166.70                          │
│    • LTP (Last Trade)   - ₹2170.95                          │
│    • ATP (Avg Trade)    - ₹2174.85                          │
│                                                             │
│ 📊 VOLUME (3 fields):                                       │
│    • Volume             - 730 shares                        │
│    • Turnover           - ₹1587 Lakhs (₹15.87 Cr)          │
│    • Lot Size           - 1                                 │
│                                                             │
│ 📈 ORDER BOOK (20 fields - 5 levels):                       │
│    Level 1: Bid Price, Bid Qty, Ask Price, Ask Qty         │
│    Level 2: Bid Price, Bid Qty, Ask Price, Ask Qty         │
│    Level 3: Bid Price, Bid Qty, Ask Price, Ask Qty         │
│    Level 4: Bid Price, Bid Qty, Ask Price, Ask Qty         │
│    Level 5: Bid Price, Bid Qty, Ask Price, Ask Qty         │
│                                                             │
│    Example:                                                 │
│    Lvl 1: ₹2169.40 × 52  |  ₹2170.95 × 32  (Spread ₹1.55)  │
│    Lvl 2: ₹2169.35 × 5   |  ₹2171.00 × 6   (Spread ₹1.65)  │
│                                                             │
│ 🔐 METADATA (1 field):                                      │
│    • Sequence Number    - 1450001457                        │
│                                                             │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│ ❌ NOT AVAILABLE IN PACKETS (3 fields)                      │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ 🏢 SYMBOL INFORMATION:                                      │
│    • Symbol Name        - "RELIANCE"                        │
│    • Company Name       - "Reliance Industries Limited"     │
│    • ISIN Code          - "INE002A01018"                    │
│                                                             │
│ 📝 HOW TO GET SYMBOLS:                                      │
│    Load BhavCopy CSV files:                                 │
│    • Equity:      data/tokens/BhavCopy_BSE_CM_*.csv         │
│    • Derivatives: data/tokens/BSE_EQD_CONTRACT_*.csv        │
│                                                             │
│    tokenMap := domain.NewTokenMap()                         │
│    tokenMap.LoadFromBhavCopy("BhavCopy_BSE_CM_12122025.csv")│
│    contract, _ := tokenMap.Get(543904)                      │
│    symbol := contract.Symbol  // "RELIANCE"                 │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 📐 **Packet Structure**

### **Total Size:** 36 bytes (header) + 264 bytes × N records
- **1 record:**  300 bytes
- **2 records:** 564 bytes
- **6 records:** 1620 bytes (maximum)

### **Endianness:** 
- **Header FormatID:** Big-Endian
- **Everything Else:** Little-Endian (Message Type, all record fields)

### **Record Layout (264 bytes):**
```
Offset   Size  Type    Field
------   ----  ------  -------------------------
0        4     uint32  Token ID (LE)
4        4     int32   Open Price (LE, paise)
8        4     int32   Previous Close (LE, paise)
12       4     int32   High Price (LE, paise)
16       4     int32   Low Price (LE, paise)
24       4     int32   Volume (LE)
28       4     uint32  Turnover (LE, lakhs)
32       4     uint32  Lot Size (LE)
36       4     int32   LTP (LE, paise)
44       4     uint32  Sequence Number (LE)
84       4     int32   ATP (LE, paise)
104      160   mixed   Order Book (5 × 32 bytes)
```

---

## 🔍 **Real Example (Live Capture)**

**From:** December 12, 2025 10:10:57  
**Message Type:** 2020 (Equity)

```
Token:           543904
Symbol:          ⚠ NOT IN PACKET (need BhavCopy)
Timestamp:       10:10:57.325 ✅ FROM PACKET
Open:            ₹2168.85 ✅
High:            ₹2177.35 ✅
Low:             ₹2168.85 ✅
Prev Close:      ₹2166.70 ✅
LTP:             ₹2170.95 ✅
ATP:             ₹2174.85 ✅
Volume:          730 shares ✅
Turnover:        ₹15.87 Crores ✅
Sequence:        1450001457 ✅

Order Book:
Level 1: ₹2169.40 × 52  |  ₹2170.95 × 32 ✅
Level 2: ₹2169.35 × 5   |  ₹2171.00 × 6  ✅
Level 3: ₹2169.30 × 6   |  ₹2171.45 × 32 ✅
Level 4: ₹2169.25 × 14  |  ₹2171.50 × 1  ✅
Level 5: ₹2169.10 × 4   |  ₹2171.85 × 6  ✅
```

---

## 🛠️ **Tools to Explore Data**

### **1. Packet Analyzer (See ALL Fields)**
```powershell
# View equity packets with full details
.\packet-analyzer.exe -max 10 -type 2020

# View derivatives packets
.\packet-analyzer.exe -max 10 -type 2021

# Show hex dump for debugging
.\packet-analyzer.exe -max 5 -hex
```

### **2. HFT Server (Capture to CSV)**
```powershell
# Capture equity quotes for 5 minutes
.\hft-server.exe -duration 5m -type 2020

# Output: data/processed_csv/YYYYMMDD_EQ_quotes.csv
```

### **3. Trade Server (Message Type 2017)**
```powershell
# Capture trade executions
.\hft-trade-server.exe -duration 5m

# Output: data/processed_csv/YYYYMMDD_trades.csv
```

---

## 💡 **Why No Symbols in Packets?**

### **Performance Optimization:**

| With Symbols | With Token IDs | Savings |
|--------------|----------------|---------|
| 8-20 bytes/record | 4 bytes/record | **4-16 bytes** |
| String comparison | Integer comparison | **10-100× faster** |
| Variable length | Fixed length | **Easier parsing** |

**Example:**
- 6 records with symbols: 48-120 extra bytes per packet
- At 100 packets/sec: 4.8-12 KB/sec wasted bandwidth
- **Solution:** Load symbols once at startup, use token IDs in real-time

---

## 📊 **Data Completeness**

```
Total Possible Fields:  41
Fields in Packets:      38 ✅
Fields from BhavCopy:   3  📁

Data Completeness:      92.7%
```

### **Missing 3 Fields:**
1. ❌ Symbol Name → Load from BhavCopy
2. ❌ Company Name → Load from BhavCopy
3. ❌ ISIN Code → Load from BhavCopy

---

## 🎯 **Summary**

| Question | Answer | Details |
|----------|--------|---------|
| **Symbol in packet?** | ❌ NO | Load from BhavCopy CSV |
| **Timestamp in packet?** | ✅ YES | Header: HH:MM:SS.mmm |
| **Prices available?** | ✅ YES | 6 fields (Open/High/Low/Close/LTP/ATP) |
| **Volume data?** | ✅ YES | Volume, Turnover, Lot Size |
| **Order book?** | ✅ YES | 5 levels with bid/ask prices & quantities |
| **Total fields?** | **38 fields** | 92.7% complete without external data |

---

## 📚 **Full Documentation**

For complete field specifications with offsets, types, and examples:
→ `docs/MESSAGE_TYPES_2020_2021_COMPLETE_FIELD_LIST.md`

For decoder implementation:
→ `internal/decoder/decoder.go`

For token lookup:
→ `internal/tokens/manager.go`

---

**Last Updated:** December 12, 2025  
**Based on:** Live packet captures and BSE NFCAST protocol analysis
