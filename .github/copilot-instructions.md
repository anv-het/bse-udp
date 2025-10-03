# BSE UDP Market Data Reader - AI Agent Instructions

## Project Overview
Python-based real-time market data feed parser for Bombay Stock Exchange (BSE) via UDP multicast using the **Direct NFCAST protocol** (low bandwidth interface). Receives, decodes, and normalizes market quotes from BSE derivatives (SENSEX/BANKEX options & futures).

**Repository:** xts-pythonclient-api-sdk (symphonyfintech)  
**Core Purpose:** Parse proprietary BSE packet format (NOT standard NFCAST) → Extract touchline data → Output JSON/CSV

## Critical Protocol Deviations

### BSE's "Modified NFCAST" Format
BSE packets **do NOT follow** the official NFCAST spec. Key differences discovered through analysis:

**Header Structure (36 bytes):**
```
Offset 0:  0x00000000 (leading zeros, NOT message type 2020/2021)
Offset 4:  Format ID (0x0124=300B, 0x022C=556B) - Big-Endian
Offset 8:  Type Field 0x07E4 (2020) - Little-Endian ⚠️
Offset 20: Time HH:MM:SS (3x uint16 Big-Endian)
```

**Market Data Records (64-byte intervals starting at offset 36):**
```python
# Mixed endianness - critical for parsing!
token      = struct.unpack('<I', bytes[0:4])[0]   # Little-Endian
open_price = struct.unpack('>i', bytes[4:8])[0]   # Big-Endian (paise)
prev_close = struct.unpack('>i', bytes[8:12])[0]  # Field name is misleading!
# ... all prices Big-Endian, divide by 100 for Rupees
```

**Multi-instrument packets:** Up to 6 tokens per packet at fixed offsets (36, 100, 164, 228...). Empty slots have `token=0` (skip them).

## Architecture & Data Flow

```
Token Database (token_details.json)
    ↓
UDP Multicast (227.0.0.21:12996 prod, 226.1.0.1:11401 test)
    ↓
Packet Parser (handle 300B/556B formats)
    ↓
Quote Normalization (token → symbol lookup)
    ↓
Output Handlers (JSON/CSV streams)
```

**No BOLTPLUS authentication yet** - direct multicast join only. Authentication flow is planned but not implemented.

## Development Workflows

### Environment Setup
```cmd
REM Windows CMD - always use .venv
call .venv\Scripts\activate.bat
pip install -r requirements.txt
```

**Critical Files:**
- `.venv/` - MUST use virtual environment (project standard)
- `data/tokens/token_details.json` - ~29k derivatives contracts (SENSEX/BANKEX options)
- `docs/BSE_Complete_Technical_Knowledge_Base.md` - 850-line protocol reference (READ FIRST)

### Testing Commands
```cmd
REM Run specific test suites (batch files exist but are empty - to be implemented)
run_token_manager_test.bat
run_parser_test.bat
run_csv_test.bat
```

**Market Hours:** BSE opens 9:00 AM - 3:30 PM IST. Test data outside hours may be stale.

### Key Documentation
- `docs/BSE_Complete_Technical_Knowledge_Base.md` - **PRIMARY REFERENCE** (packet formats, decompression algorithm, API flows)
- `docs/BSE_Final_Analysis_Report.md` - Real packet validation findings
- `docs/BSE_NFCAST_Analysis.md` - Protocol implementation notes
- `.prompt.md` - Original Phase 1 project spec (historical context)

## Code Patterns & Conventions

### Packet Parsing Pattern
```python
# Standard approach for BSE packets
def parse_300b_format(packet: bytes) -> List[MarketQuote]:
    """Parse proprietary 300-byte format"""
    # 1. Validate header
    format_id = struct.unpack('>H', packet[4:6])[0]  # BE
    if format_id != 0x0124:
        return []
    
    # 2. Extract timestamp
    hour = struct.unpack('>H', packet[20:22])[0]
    minute = struct.unpack('>H', packet[22:24])[0]
    second = struct.unpack('>H', packet[24:26])[0]
    
    # 3. Parse instruments at fixed offsets
    quotes = []
    for offset in [36, 100, 164, 228]:
        token = struct.unpack('<I', packet[offset:offset+4])[0]  # LE!
        if token == 0:
            continue  # Empty slot
        
        # Parse prices (all Big-Endian, in paise)
        open_price = struct.unpack('>i', packet[offset+4:offset+8])[0] / 100.0
        # ... (see BSE_Complete_Technical_Knowledge_Base.md line 384+)
```

### Token Database Lookup
```python
# Always validate tokens exist
token_map = json.load(open('data/tokens/token_details.json'))
if token not in token_map:
    logger.warning(f"Unknown token {token} - not in contract master")
    stats['unknown_tokens'] += 1
    return None

symbol_info = token_map[str(token)]  # Keys are strings!
# {'symbol': 'SENSEX', 'expiry': '2025-09-18', 'option_type': 'CE', 'strike_price': '86900'}
```

### Configuration Pattern
No `config.json` exists yet. When implementing:
```json
{
  "multicast": {
    "ip": "226.1.0.1",
    "port": 11401,
    "segment": "Equity",
    "env": "simulation"
  },
  "buffer_size": 2000,
  "logging_level": "INFO"
}
```
**Default to simulation IPs** (226.x.x.x) for safety - production IPs can cause accidental connections.

## Common Pitfalls

1. **Mixed Endianness:** Token is LE, everything else is BE. Don't assume uniform byte order.

2. **Field Name Mismatch:** Official docs call position 8 "close price" but it's actually "previous close". Position 4 is "open". See `BSE_Final_Analysis_Report.md` for corrected mappings.

3. **Compression Not Used:** Despite manual describing compression algorithm (pages 48-55), observed packets are **uncompressed**. Don't apply decompression logic unless explicitly needed for 2020/2021 message types.

4. **UDP Packet Loss:** No recovery mechanism. Each packet is self-contained. Log stats but don't attempt retransmission.

5. **Empty Virtual Env:** Project structure exists but no Python source files yet. When creating:
   - Use `bse/` package directory (not `src/`)
   - Follow structure from `BSE_Complete_Technical_Knowledge_Base.md` line 685
   - Include `__init__.py` in all package directories

## External Dependencies

- **IGMPv2 Multicast:** OS must support multicast routing. Windows: `netsh interface ipv4 show joins`
- **BSE Network:** Requires VPN/direct connectivity to BSE network (137.x.x.x, 227.x.x.x ranges)
- **Contract Master:** `token_details.json` must be current (updated via BOLTPLUS API - not implemented)

## Testing Strategy

**Current State:** No test files exist (tests/ folder empty)

**When Implementing Tests:**
```python
# Use captured packet samples
SAMPLE_300B = bytes.fromhex('00000000 0124 0000 07e4...')  # From real capture
def test_parse_300b_packet():
    parser = BSEPacketParser(load_token_map())
    quotes = parser.parse_packet(SAMPLE_300B)
    assert len(quotes) == 1
    assert quotes[0].token == 842364  # BSX SENSEX Future
    assert quotes[0].symbol == 'SENSEX'
```

**Integration Tests:** Must run during market hours (9:00-15:30 IST) for live data validation.

## What's NOT Implemented Yet

- [ ] Python source code (only docs + empty structure)
- [ ] BOLTPLUS API authentication (`bse/auth.py`)
- [ ] UDP multicast socket setup (`bse/multicast.py`)
- [ ] Packet parser implementation (`bse/parser.py`)
- [ ] NFCAST decompression (if needed - `bse/decompressor.py`)
- [ ] JSON/CSV output handlers (`bse/saver.py`)
- [ ] Main orchestration script (`main.py`)
- [ ] Configuration loading (`config.json`)
- [ ] Test suite (all test files)
- [ ] Batch file implementations (`.bat` files are empty)

**Start Here:** Implement socket connection in `bse/multicast.py` following patterns from `BSE_Complete_Technical_Knowledge_Base.md` line 241 (IGMPv2 setup).

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

*For detailed packet structures, decompression algorithm (if needed), and complete parser implementation, refer to `docs/BSE_Complete_Technical_Knowledge_Base.md`.*
