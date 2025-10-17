# Step-by-Step Integration Guide
## Fixing Your BSE Decoder - Exact Changes Needed

---

## STEP 1: Fix src/decoder.py

### Change 1A: Fix _parse_header method (around line 140)

**FIND THIS:**
```python
# Number of records (offset 32-33, Little-Endian)
num_records = struct.unpack('<H', packet[32:34])[0]
```

**REPLACE WITH:**
```python
# Number of records (offset 34-35, Little-Endian) - CORRECTED OFFSET!
num_records = struct.unpack('<H', packet[34:36])[0]
```

### Change 1B: Fix _get_num_records method (around line 200)

**FIND THIS:**
```python
def _get_num_records(self, packet_size: int) -> int:
    """Determine number of records based on packet size."""
    if packet_size == 300:
        return 4
    elif packet_size == 556:
        return 6
    else:
        logger.warning(f"Unknown packet size {packet_size}, assuming 4 records")
        return 4
```

**REPLACE WITH:**
```python
def _get_num_records(self, packet_size: int) -> int:
    """Determine number of records based on packet size."""
    # Records are 264 bytes each, header is 36 bytes
    # Formula: (packet_size - 36) / 264
    if packet_size < 36:
        return 0
    num_records = (packet_size - 36) // 264
    logger.debug(f"Packet size {packet_size}: calculated {num_records} records")
    return num_records
```

### Change 1C: Fix decode_packet method record iteration (around line 120)

**FIND THIS:**
```python
# Parse records starting at offset 36
records = []
for i in range(num_records):
    offset = 36 + (i * 64)  # ❌ WRONG: 64 bytes
    if offset + 64 > packet_size:
        logger.debug(f"Record {i} would exceed packet size, stopping")
        break
```

**REPLACE WITH:**
```python
# Parse records starting at offset 36
records = []
for i in range(num_records):
    offset = 36 + (i * 264)  # ✅ CORRECT: 264 bytes per record
    if offset + 264 > packet_size:
        logger.debug(f"Record {i} would exceed packet size, stopping")
        break
```

### Change 1D: COMPLETELY REPLACE _parse_record method (around line 190)

**DELETE THE ENTIRE METHOD and REPLACE WITH:**

```python
def _parse_record(self, record_bytes: bytes, offset: int) -> Optional[Dict]:
    """
    Parse 264-byte market data record (CORRECTED VERSION).
    
    All price fields are LITTLE-ENDIAN (not Big-Endian as previously assumed).
    Fields are NOT compressed - can be read directly.
    
    Record Structure (empirically determined):
    [+00-03] Token (LE uint32)
    [+04-07] Close Rate (LE int32, paise)
    [+08-11] Open Rate (LE int32, paise)
    [+12-15] High Rate (LE int32, paise)
    [+16-19] Low Rate (LE int32, paise)
    [+20-23] Num Trades (LE uint32)
    [+24-27] Volume (LE uint32)
    [+36-39] LTP - Last Traded Price (LE int32, paise)
    """
    try:
        # Token (Little-Endian uint32) at offset 0
        token = struct.unpack('<I', record_bytes[0:4])[0]
        logger.debug(f"Token: {token}")
        
        # Check for empty record markers
        if token == 0 or token == 1:
            logger.debug(f"Empty record detected (token={token})")
            return None
        
        # All price fields are Little-Endian int32 in paise
        close_rate = struct.unpack('<i', record_bytes[4:8])[0]
        open_rate = struct.unpack('<i', record_bytes[8:12])[0]
        high_rate = struct.unpack('<i', record_bytes[12:16])[0]
        low_rate = struct.unpack('<i', record_bytes[16:20])[0]
        ltp = struct.unpack('<i', record_bytes[36:40])[0]
        
        # Other fields
        num_trades = struct.unpack('<I', record_bytes[20:24])[0]
        volume = struct.unpack('<I', record_bytes[24:28])[0]
        
        # LTQ (Last Traded Quantity) - using a placeholder for now
        # Actual offset needs to be determined
        ltq = 0
        
        logger.debug(f"Parsed: Token={token}, LTP={ltp} paise, "
                    f"Open={open_rate}, High={high_rate}, Low={low_rate}, "
                    f"Volume={volume}, Trades={num_trades}")
        
        return {
            'token': token,
            'num_trades': num_trades,
            'volume': volume,
            'close_rate': close_rate,  # paise
            'ltq': ltq,
            'ltp': ltp,  # paise
            'open_rate': open_rate,  # paise
            'high_rate': high_rate,  # paise
            'low_rate': low_rate,  # paise
            'compressed_offset': offset + 40,  # Placeholder
            'empty': False
        }
        
    except Exception as e:
        logger.error(f"Record parse error: {e}", exc_info=True)
        return None
```

---

## STEP 2: Simplify src/decompressor.py

### Change 2: Make decompress_record a pass-through (around line 50)

**FIND THIS (entire decompress_record method):**
```python
def decompress_record(self, packet: bytes, record: Dict) -> Optional[Dict]:
    """
    Decompress a single market data record.
    ...
    [lots of decompression code]
    """
```

**REPLACE THE ENTIRE METHOD WITH:**

```python
def decompress_record(self, packet: bytes, record: Dict) -> Optional[Dict]:
    """
    Decompress a single market data record.
    
    NOTE: Current BSE packets appear to be UNCOMPRESSED.
    This method now just normalizes the data (paise → Rupees).
    If compressed packets are encountered in the future, add
    decompression logic here.
    """
    if record.get('empty'):
        logger.warning(f"Skipping empty record (token={record.get('token')})")
        return None
    
    try:
        # Simply normalize prices from paise to Rupees
        # No differential decompression needed
        
        decompressed = {
            'token': record['token'],
            'open': record.get('open_rate', record['ltp']) / 100.0,  # paise → Rs
            'high': record.get('high_rate', record['ltp']) / 100.0,
            'low': record.get('low_rate', record['ltp']) / 100.0,
            'close': record['close_rate'] / 100.0,
            'ltp': record['ltp'] / 100.0,
            'volume': record['volume'],
            'num_trades': record['num_trades'],
            'prev_close': record['close_rate'] / 100.0,
            'bid_levels': [],  # TODO: Parse Best 5 bid levels if needed
            'ask_levels': []   # TODO: Parse Best 5 ask levels if needed
        }
        
        self.stats['records_decompressed'] += 1
        logger.debug(f"Normalized record: token={record['token']}, "
                    f"ltp={decompressed['ltp']:.2f} Rs")
        
        return decompressed
        
    except Exception as e:
        self.stats['decompress_errors'] += 1
        logger.error(f"Decompression error for token {record.get('token')}: {e}", 
                    exc_info=True)
        return None
```

---

## STEP 3: Fix src/data_collector.py

### Change 3: Fix timestamp assignment (around line 80-100)

**FIND THIS:**
```python
# Get current date for timestamp
current_date = datetime.now().date()
```

**REPLACE WITH:**
```python
# Use timestamp from packet header (already in decoded_data['header'])
packet_timestamp = decoded_data['header'].get('timestamp', datetime.now())
```

**THEN FIND THIS (in the loop):**
```python
quote = {
    'token': record['token'],
    'symbol': symbol,
    'timestamp': datetime.combine(current_date, datetime.min.time()),
    ...
}
```

**REPLACE WITH:**
```python
quote = {
    'token': record['token'],
    'symbol': symbol,
    'timestamp': packet_timestamp,  # Use actual packet timestamp!
    ...
}
```

---

## STEP 4: Test Your Changes

### Quick Test Script:

Create `tests/test_fixed_decoder.py`:

```python
"""Test the fixed decoder."""
import sys
sys.path.insert(0, 'd:/bse/src')

from decoder import PacketDecoder
from decompressor import NFCASTDecompressor
from pathlib import Path

# Load test packet
packet_file = Path("d:/bse/data/raw_packets/20251006_115104_524083_type2020_packet.bin")
with open(packet_file, 'rb') as f:
    packet = f.read()

print(f"Testing with: {packet_file.name} ({len(packet)} bytes)\n")

# Decode
decoder = PacketDecoder()
decoded = decoder.decode_packet(packet)

if decoded:
    print(f"✅ Header: {decoded['header']}")
    print(f"✅ Records: {len(decoded['records'])}\n")
    
    # Decompress
    decompressor = NFCASTDecompressor()
    for i, record in enumerate(decoded['records']):
        decompressed = decompressor.decompress_record(packet, record)
        if decompressed:
            print(f"Record {i+1}:")
            print(f"  Token: {decompressed['token']}")
            print(f"  LTP: {decompressed['ltp']:.2f} Rs")
            print(f"  Open: {decompressed['open']:.2f} Rs")
            print(f"  High: {decompressed['high']:.2f} Rs")
            print(f"  Low: {decompressed['low']:.2f} Rs")
            print(f"  Volume: {decompressed['volume']}")
            print(f"  Num Trades: {decompressed['num_trades']}")
            print()
else:
    print("❌ Decoding failed!")
```

Run it:
```bash
python tests/test_fixed_decoder.py
```

**Expected output:**
```
✅ Header: {...}
✅ Records: 2

Record 1:
  Token: 861201
  LTP: 475.15 Rs
  Open: 382.35 Rs
  High: 475.85 Rs
  Low: 373.70 Rs
  Volume: 920
  Num Trades: 41

Record 2:
  Token: 861289
  LTP: 90.05 Rs
  ...
```

---

## STEP 5: Add Token Master File (Optional but Recommended)

Create `data/tokens/token_master.json`:

```json
{
  "861201": {
    "symbol": "SENSEX25OCT24500CE",
    "name": "SENSEX OCT 2024 24500 CE",
    "segment": "Equity Derivatives",
    "expiry": "2024-10-31"
  },
  "861289": {
    "symbol": "SENSEX25OCT24600CE",
    "name": "SENSEX OCT 2024 24600 CE",
    "segment": "Equity Derivatives",
    "expiry": "2024-10-31"
  }
}
```

Then in `data_collector.py`, load it:

```python
import json
from pathlib import Path

class DataCollector:
    def __init__(self):
        # Load token master
        token_file = Path("data/tokens/token_master.json")
        if token_file.exists():
            with open(token_file) as f:
                self.token_map = json.load(f)
        else:
            self.token_map = {}
            logger.warning("Token master file not found")
    
    def _get_symbol(self, token: int) -> str:
        """Get symbol for token."""
        token_str = str(token)
        if token_str in self.token_map:
            return self.token_map[token_str]['symbol']
        return f"UNKNOWN_{token}"
```

---

## Summary of Changes

| File | Lines Changed | What | Difficulty |
|------|---------------|------|------------|
| `src/decoder.py` | ~80 lines | Fix offsets, endianness, record size | ⭐⭐ Medium |
| `src/decompressor.py` | ~50 lines | Remove decompression logic | ⭐ Easy |
| `src/data_collector.py` | ~10 lines | Fix timestamp | ⭐ Easy |
| `data/tokens/*.json` | NEW | Add token mapping | ⭐ Easy |

**Total time:** 30-45 minutes

---

## Verification Checklist

After making changes:

- [ ] Run `tests/test_fixed_decoder.py` - should show reasonable prices
- [ ] Run your main collector - check CSV output
- [ ] Verify timestamps are correct (not all "00:00:00")
- [ ] Check LTP values are realistic (10-5000 Rs range for options)
- [ ] Verify volumes are reasonable (not billions)
- [ ] Add token master file if symbols show "UNKNOWN"

---

## Need Help?

If you get stuck:

1. Compare your code with `tests/simple_decoder_working.py` - that's the working reference
2. Check that ALL price reads use `'<i'` (Little-Endian), not `'>i'`
3. Verify record size is 264 bytes everywhere, not 64
4. Make sure num_records reads from offset 34-35, not 32-33

Good luck! The decoder is working in the test file, so integration should be straightforward. 🚀
