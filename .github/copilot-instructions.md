# BSE UDP Market Data Reader - AI Agent Instructions

## Project Overview
**PHASE 3 COMPLETE** - Production-ready Python UDP multicast reader for Bombay Stock Exchange (BSE) market data via proprietary **Direct NFCAST protocol** (low bandwidth ~2-3 MBPS). Decodes real-time BSE derivatives quotes (SENSEX/BANKEX options & futures) with full order book depth (best 5 bid/ask levels).

**Status:** Operational pipeline from UDP packets → decoded/decompressed data → normalized quotes → JSON/CSV output  
**Core Purpose:** Real-time market data aggregation for trading applications without BSE terminal dependency

## Critical Discovery: BSE's Proprietary Protocol

### Mixed Endianness (Most Critical Issue)
BSE uses **inconsistent byte ordering** across packet fields - this MUST be handled correctly:

**Header Fields (36 bytes):**
```python
leading_zeros = struct.unpack('>I', packet[0:4])[0]      # Big-Endian (0x00000000)
format_id     = struct.unpack('>H', packet[4:6])[0]      # Big-Endian (0x0234)
message_type  = struct.unpack('<H', packet[8:10])[0]     # Little-Endian ⚠️ (2020/2021)
hour          = struct.unpack('<H', packet[20:22])[0]    # Little-Endian ⚠️
minute        = struct.unpack('<H', packet[22:24])[0]    # Little-Endian ⚠️
second        = struct.unpack('<H', packet[24:26])[0]    # Little-Endian ⚠️
```

**Market Data Records (66 bytes each, starting offset 36):**
```python
token      = struct.unpack('<I', record[0:4])[0]      # Little-Endian ⚠️
prev_close = struct.unpack('>i', record[8:12])[0]     # Big-Endian (paise)
ltp        = struct.unpack('>i', record[20:24])[0]    # Big-Endian (paise)
volume     = struct.unpack('>i', record[24:28])[0]    # Big-Endian
# ALL compressed fields: Big-Endian 2-byte differentials
```

**Rule:** Token & timestamps are Little-Endian, everything else is Big-Endian. Mixing these causes invalid negative prices/volumes.

### Packet Format: 564 Bytes Total
```
┌─────────────────────────────────────────────────────────┐
│ HEADER (36 bytes)                                       │
│ • Leading zeros, format ID, msg type, timestamp         │
├─────────────────────────────────────────────────────────┤
│ RECORD 1 (66 bytes) - Token, prices, compressed data   │
├─────────────────────────────────────────────────────────┤
│ RECORD 2 (66 bytes)                                     │
├─────────────────────────────────────────────────────────┤
│ ... up to 8 records total (528 bytes)                   │
└─────────────────────────────────────────────────────────┘
```

**Empty records:** Token=0 or 1 indicates no data (skip processing). Real tokens start from ~15,000+.

## Architecture & Data Flow (Phases 1-3 Complete)

```
BSE Network (227.0.0.21:12996 prod, 226.1.0.1:11401 test)
    │ UDP Multicast (IGMPv2)
    ↓
connection.py - UDP socket setup, multicast join
    │ Raw packets (564 bytes)
    ↓
packet_receiver.py - Continuous receive loop, size validation
    │ Valid packets (type 2020/2021)
    ↓
decoder.py - Header parsing, base value extraction
    │ Decoded records {token, ltp, volume, prev_close, ...}
    ↓
decompressor.py - NFCAST differential decompression, Best 5 extraction
    │ Decompressed market data {open, high, low, bid_levels, ask_levels}
    ↓
data_collector.py - Token→symbol mapping, validation (LTP>0, Vol>0)
    │ Normalized quotes {symbol, expiry, strike, option_type, ...}
    ↓
saver.py - JSON append (20251006_quotes.json), CSV append (20251006_quotes.csv)
    │
    ↓
data/processed_json/  &  data/processed_csv/
```

**No BOLTPLUS API authentication yet** - direct multicast join only. Contract master is static JSON file.

## Development Workflows

### Environment Setup
```cmd
REM Windows CMD - always activate virtual environment
call .venv\Scripts\activate.bat
pip install -r requirements.txt  # Only standard library currently
```

**Critical Files:**
- `config.json` - Multicast IPs (production vs simulation), buffer sizes
- `data/tokens/token_details.json` - ~29k derivatives contract master (SENSEX/BANKEX)
- `bse_reader.log` - Main application log (created on first run)
- `data/raw_packets/*.bin` - Saved packets for debugging (if enabled in config)

### Running the Application
```cmd
REM Start the market data reader
python src/main.py

REM Press Ctrl+C to stop gracefully (1-second timeout allows this)
```

**Socket Timeout Critical:** Connection has 1-second socket timeout to allow Ctrl+C interrupts. Without this, the process hangs indefinitely.

### Testing During Market Hours
**BSE Derivatives Trading:** 9:00 AM - 3:30 PM IST, Monday-Friday  
**Outside hours:** Simulation feed (226.1.0.1:11401) may not have live data  
**Tip:** Check logs for "⏱️ Still waiting for packets..." every 30 seconds if no data received

### Key Documentation (Start Here)
1. **`docs/BSE_Complete_Technical_Knowledge_Base.md`** - PRIMARY REFERENCE (850 lines, packet formats, compression algorithm)
2. **`docs/ARCHITECTURE_GUIDE.md`** - Visual diagrams, component responsibilities
3. **`docs/PHASE3_COMPLETE.md`** - Full Phase 3 implementation details
4. **`docs/FIXES_APPLIED.md`** - Critical endianness fixes (Oct 2025)
5. **`docs/BSE_Final_Analysis_Report.md`** - Real packet validation findings

## Code Patterns & Conventions

### Binary Parsing with Mixed Endianness
```python
# ALWAYS verify endianness from docs/FIXES_APPLIED.md
# BSE uses inconsistent byte ordering - empirically validated

# Header parsing example (decoder.py)
format_id = struct.unpack('>H', packet[4:6])[0]     # Big-Endian
msg_type = struct.unpack('<H', packet[8:10])[0]     # Little-Endian ⚠️
hour = struct.unpack('<H', packet[20:22])[0]        # Little-Endian ⚠️

# Record parsing example
token = struct.unpack('<I', record[0:4])[0]         # Little-Endian ⚠️
ltp = struct.unpack('>i', record[20:24])[0] / 100.0 # Big-Endian, paise→Rupees
```

### Differential Decompression Pattern
```python
# From decompressor.py - NFCAST compression algorithm
def _decompress_field(packet, offset, base_value):
    """Decompress 2-byte differential + base value"""
    diff = struct.unpack('>h', packet[offset:offset+2])[0]  # Signed short, BE
    
    if diff == 32767:  # Special: read full 4-byte value
        return struct.unpack('>i', packet[offset+2:offset+6])[0] / 100.0
    elif diff == 32766 or diff == -32766:  # End markers
        return None
    else:
        return (base_value * 100 + diff) / 100.0  # Differential + base
```

### Token-to-Symbol Lookup
```python
# From data_collector.py - always validate token exists
token_str = str(token)  # Keys in JSON are strings!
if token_str not in self.token_map:
    logger.warning(f"Unknown token {token} - not in contract master")
    return None

info = self.token_map[token_str]
# Returns: {'symbol': 'SENSEX', 'expiry': '2025-09-18', 'option_type': 'CE', ...}
```

### Statistics Tracking Pattern
```python
# All modules (decoder, decompressor, collector) track stats
self.stats = {
    'packets_decoded': 0,
    'decode_errors': 0,
    'invalid_headers': 0
}

# Log every 10 packets in main loop
if stats['packets_received'] % 10 == 0:
    logger.info(f"📦 Packets: {stats['packets_received']}, Valid: {stats['valid_packets']}")
```

## Common Pitfalls

1. **Mixed Endianness:** Token & timestamps are LE, everything else is BE. Don't assume uniform byte order. See `docs/FIXES_APPLIED.md` for validation history.

2. **Socket Timeout Required:** UDP socket MUST have 1-second timeout (`sock.settimeout(1.0)`) to allow Ctrl+C interrupts. Without this, the receive loop blocks indefinitely.

3. **Timestamp Validation:** Parse timestamp fields but validate ranges (hour<24, minute<60, second<60) before using. Fall back to system time if invalid.

4. **Empty Records:** Token values 0 or 1 indicate empty slots - skip these during parsing. Real token IDs start from ~15,000+.

5. **Paise to Rupees:** All price fields are in paise (divide by 100 for Rupees). LTP of 8500000 paise = 85,000 Rupees.

6. **Token Keys are Strings:** The `token_details.json` file uses string keys (`"842364"`), not integers. Always convert: `str(token)`.

7. **Cascading Best 5 Bases:** Market depth decompression uses previous level as base. Level 1 base = LTP/LTQ, Level 2 base = Level 1 value, etc.

8. **Log Flood Prevention:** Socket timeout fires every second - only log status every 30 timeouts (30 seconds) to avoid log spam.

## External Dependencies

- **IGMPv2 Multicast:** OS must support multicast routing. Windows: `netsh interface ipv4 show joins`
- **BSE Network:** Requires VPN/direct connectivity to BSE network (137.x.x.x, 227.x.x.x ranges)
- **Contract Master:** `token_details.json` must be current (updated via BOLTPLUS API - not implemented)

## Testing Strategy

**Current State:** Limited test coverage (connection and packet receiver tests exist)

**Running Tests:**
```cmd
REM Individual module tests
python tests/test_connection.py
python tests/test_packet_receiver.py
python tests/test_decoder.py
python tests/test_decompressor.py
```

**Integration Tests:** Must run during market hours (9:00-15:30 IST) for live data validation.

**Test Data:** Captured packets stored in `data/raw_packets/` can be used for offline testing.

## Current Implementation Status

### ✅ Phase 1-3 Complete (Production Ready)
- ✅ UDP multicast connection (`connection.py`)
- ✅ Packet reception & filtering (`packet_receiver.py`)
- ✅ Binary packet decoding (`decoder.py`)
- ✅ NFCAST decompression (`decompressor.py`)
- ✅ Token→symbol mapping (`data_collector.py`)
- ✅ JSON/CSV output (`saver.py`)
- ✅ Main orchestration (`main.py`)
- ✅ Configuration (`config.json`)

### ⚠️ Known Issues
- Invalid LTP/volume values in decompressor (negative numbers indicate possible remaining endianness issues in compressed fields)
- No error recovery for multicast disconnections
- No packet sequence tracking (UDP can drop/reorder)
- Contract master is static (no auto-update from BOLTPLUS API)

### 🚧 Future Work (Not Implemented)
- [ ] BOLTPLUS API authentication
- [ ] Contract master synchronization
- [ ] WebSocket streaming interface
- [ ] Packet gap detection/recovery
- [ ] Docker containerization
- [ ] Production monitoring/alerting

## Quick Reference

**Test Token:** 842364 (BSX SENSEX Future - use for validation)  
**Packet Sizes:** 300B or 556B (determine format)  
**Record Size:** 64 bytes per instrument  
**Price Divisor:** 100 (paise → Rupees)  
**Buffer Size:** 2000 bytes (recommended by BSE)  
**Netting Interval:** 0.80 seconds (BSE aggregates updates)

**Production Multicast:** 227.0.0.21:12996 (Equity NFCAST)  
**Simulation Multicast:** 226.1.0.1:11401 (Equity test feed)

---

*For detailed packet structures, decompression algorithm, and complete implementation patterns, refer to `docs/BSE_Complete_Technical_Knowledge_Base.md`.*
