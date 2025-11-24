# BSE UDP Market Data Reader - AI Agent Instructions

**Phase 3 COMPLETE** - Real-time BSE derivatives market data from UDP multicast with full decoding pipeline.

## Project Architecture

**Core Pipeline** (5 components):
1. **connection.py** - IGMPv2 multicast socket setup (239.1.2.5:26002 production, 226.1.0.1:11401 simulation)
2. **packet_receiver.py** - Continuous UDP loop, validates packets (type 2020/2021), coordinates pipeline
3. **decoder.py** - Parses 564-byte packets: header (36B) + 8 records (66B each), extracts base prices
4. **decompressor.py** - Normalizes NFCAST data (already uncompressed in production), converts paise→Rupees
5. **data_collector.py** + **saver.py** - Token→symbol resolution, JSON/CSV output with millisecond timestamps

**Data Flow**: UDP → packet_receiver → decoder → decompressor → data_collector → saver → `data/processed_json/` + `data/processed_csv/`

## Critical Implementation Details

### 🔴 Mixed Endianness (Empirically Validated)
**This is the #1 source of bugs.** BSE uses inconsistent byte ordering:

| Field | Format | Offset | Endianness |
|-------|--------|--------|------------|
| format_id | uint16 | 4-6 | Big-Endian |
| msg_type | uint16 | 8-10 | **Little-Endian** ⚠️ |
| hour/minute/second | uint16 × 3 | 20-26 | **Little-Endian** ⚠️ |
| token (per record) | uint32 | record[0:4] | **Little-Endian** ⚠️ |
| ltp/volume/prev_close | int32 | record[20:24] | Big-Endian |

**Rule**: Token (uint32 LE) + timestamps (uint16 LE) ONLY. Everything else is Big-Endian. See `src/decoder.py` lines 95-120 for validation.

**Paise→Rupees**: All prices in paise (divide by 100). LTP of 8500000 = ₹85,000.

### 🔴 Token Handling
- `token_details.json` uses **string keys** (`"873870"` not `873870`)
- Always convert: `token_str = str(token)` before lookup
- Empty records: token=0 or 1, skip these. Valid tokens start ~15,000+

### 🔴 Packet Structure (564 bytes)
```
Header (36B): Leading zeros (0x00000000) | Format ID (0x022C BE) | Msg type (2020/2021 LE) | Timestamp (HH:MM:SS LE)
Record 1 (66B): Token (LE) | Prices (BE paise) | Sequence fields
Record 2 (66B): ... up to 8 records
```

### 🔴 Socket Timeout MUST Be Set
`sock.settimeout(1.0)` required - allows Ctrl+C to interrupt receive loop. Without it, process hangs indefinitely blocking on `recvfrom()`.

## Code Patterns (Copy These)

### Statistics Tracking
```python
self.stats = {
    'packets_received': 0,
    'valid_packets': 0,
    'decode_errors': 0,
    'quotes_collected': 0
}

# Log every 10 packets to avoid spam
if self.stats['packets_received'] % 10 == 0:
    logger.info(f"📦 {self.stats['packets_received']} packets, {self.stats['valid_packets']} valid")
```

### Binary Parsing Template
```python
# Pattern from decoder.py
import struct

def parse_header(packet):
    leading_zeros = struct.unpack('>I', packet[0:4])[0]       # BE
    format_id = struct.unpack('>H', packet[4:6])[0]           # BE
    msg_type = struct.unpack('<H', packet[8:10])[0]           # LE ⚠️
    hour = struct.unpack('<H', packet[20:22])[0]              # LE ⚠️
    return {'format_id': format_id, 'msg_type': msg_type, 'hour': hour}

def parse_record(record):
    token = struct.unpack('<I', record[0:4])[0]               # LE ⚠️
    ltp = struct.unpack('>i', record[20:24])[0] / 100.0       # BE, paise→Rupees
    return {'token': token, 'ltp': ltp}
```

### Token Resolution Pattern
```python
# From data_collector.py
token_str = str(token)
if token_str not in self.token_map:
    logger.warning(f"Unknown token {token}")
    return None

contract = self.token_map[token_str]  # {'symbol': 'SENSEX', 'expiry': '27-NOV-2025', ...}
```

### Timestamp Handling
- Parse from packet header (HH:MM:SS as 3×uint16 LE)
- Validate ranges (hour<24, minute<60, second<60)
- Fall back to system time if invalid
- Append milliseconds: `f"{hour:02d}:{minute:02d}:{second:02d}.{milliseconds:03d}"`

## Workflows

### Running Tests
```powershell
# Unit tests (individual modules)
python tests/test_connection.py
python tests/test_decoder.py

# All tests
python -m unittest discover tests -v

# Validation script (check endianness fixes)
python tests/validate_decoder_fix.py
```

### Development Setup
```powershell
# Windows CMD
call .venv\Scripts\activate.bat
pip install -r requirements.txt  # Standard library only

# Configuration
code config.json  # Edit multicast IPs, buffer size

# Run application (market hours: 9:00-15:30 IST)
python src/main.py
# Press Ctrl+C to shutdown gracefully
```

### Debugging Packets
```python
# Analyze raw packet (from test utilities)
import struct
packet_size = len(packet)
format_id = struct.unpack('>H', packet[4:6])[0]
msg_type = struct.unpack('<H', packet[8:10])[0]
print(f"Packet: {packet_size}B, format=0x{format_id:04X}, type={msg_type}")
```

## Common Pitfalls (⚠️ Prevent These)

| Pitfall | Consequence | Fix |
|---------|-------------|-----|
| Forget Little-Endian on token/timestamp | Parse negative/invalid values | Use `struct.unpack('<...')` for token & time fields only |
| Token lookup fails (int instead of str) | KeyError every time | `token_str = str(token)` before `token_map[token_str]` |
| No socket timeout | Ctrl+C doesn't work, process hangs | `sock.settimeout(1.0)` in connection.py |
| Log every packet | Log file explodes | Only log on multiples (`if count % 10 == 0`) |
| Empty records (token=0,1) → lookup fails | Unknown token warnings | Skip if `token < 15000` |
| Forget paise conversion | Prices off by 100× | All prices: `divide by 100` before output |
| Timestamp validation skipped | Crash on invalid HH:MM:SS | Always validate `hour < 24, minute < 60, second < 60` |

## Key Files Quick Reference

| File | Responsibility | Lines | Pattern |
|------|-----------------|-------|---------|
| `src/main.py` | Orchestration, startup, graceful shutdown | 340 | Signal handler for Ctrl+C, load config + token_map |
| `src/connection.py` | UDP socket setup, multicast join | 180 | `sock.setsockopt(socket.IPPROTO_IP, socket.IP_ADD_MEMBERSHIP, ...)` |
| `src/decoder.py` | Binary parsing with mixed endianness | 450 | Separate BE/LE parsing functions (lines 95-120 pattern) |
| `src/decompressor.py` | Normalize prices (paise→Rupees) | 274 | Pass-through normalizer for production feed |
| `src/data_collector.py` | Token→symbol mapping, validation | 430 | String keys in token_map, stat tracking |
| `src/saver.py` | JSON/CSV output | 344 | Formula-wrapped timestamps to prevent Excel auto-format |
| `config.json` | Multicast IP, buffer size, timeouts | 20 | Production: 239.1.2.5:26002, Simulation: 226.1.0.1:11401 |
| `data/tokens/token_details.json` | ~29k contract master | 29k+ | {"873870": {"symbol": "SENSEX", "expiry": "27-NOV-2025", ...}} |

## Testing Strategy

**Unit Tests**: Located in `tests/`. Run individual modules:
- `test_connection.py` - Socket creation, multicast join
- `test_decoder.py` - Header/record parsing with real packet data
- `test_decompressor.py` - Price conversion (paise→Rupees)
- `test_packet_receiver.py` - Full pipeline integration

**Integration Tests**: Must run during market hours (9:00-15:30 IST, Mon-Fri) for live feed validation.

**Test Data**: Use captured packets in `data/raw_packets/` for offline decoder testing.

## External Dependencies & Network

**Required**:
- IGMPv2 multicast support (check: `netsh interface ipv4 show joins` on Windows)
- VPN/direct network access to BSE (IP ranges: 137.x.x.x, 227.x.x.x)
- BSE market hours: 9:00 AM - 3:30 PM IST, Monday-Friday

**Configuration**:
- Production: 239.1.2.5:26002 (live market data)
- Simulation: 226.1.0.1:11401 (test feed, may have no data outside hours)
- Buffer: 2048 bytes (from config.json)

## Documentation References

| Doc | Purpose | Start Here |
|-----|---------|-----------|
| `docs/PROJECT_DOCUMENTATION.md` | Complete system reference | ⭐ Start here for full overview |
| `docs/ARCHITECTURE_GUIDE.md` | Component diagrams & data flow | For understanding system design |
| `docs/COMPLETE_PACKET_STRUCTURE_ANALYSIS.md` | 564-byte packet format details | Before touching decoder.py |
| README.md | Setup, running, troubleshooting | For deployment questions |

## Implementation Status

**✅ Complete (Production Ready)**:
- Connection, packet reception, decoding, decompression, normalization, output
- Excel-compatible timestamps (formula-wrapped to prevent auto-format)
- Full 5-level order book extraction
- ~29k token resolution (SENSEX/BANKEX derivatives)

**❌ Not Implemented**:
- BOLTPLUS API (contract master is static JSON)
- Error recovery for multicast disconnects
- Packet sequence tracking (UDP can drop/reorder)
- Multi-threaded processing
