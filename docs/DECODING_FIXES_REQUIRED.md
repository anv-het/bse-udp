# BSE Packet Decoding - Critical Findings & Fix Roadmap

## Date: October 10, 2025

## Summary of Issues Found

Your BSE packet decoder has several critical issues causing the invalid data you're seeing:

### 1. **Header Parsing - PARTIALLY CORRECT**
- ✅ Message Type at offset 8-9 (Little-Endian): CORRECT
- ✅ Timestamp fields at offsets 20-27 (Little-Endian): CORRECT  
- ❌ **Number of Records at offset 32-33: WRONG** 
  - **ACTUAL LOCATION: Offset 34-35 (Little-Endian)**
  - Your logs show "0 records" because you're reading the wrong offset

### 2. **Record Structure - COMPLETELY WRONG**
Your current decoder assumes:
- Record size: 64 or 66 bytes
- Records start at offset 36
- Fixed field positions based on manual table

**ACTUAL STRUCTURE (empirically verified):**
- **Record size: 264 bytes each** ((564-36)/2 = 264)
- **Records ARE at offset 36 (correct start)**
- **Field layout DOES NOT match manual table order!**

### 3. **Correct Uncompressed Field Layout** (Empirically Determined)

Based on analysis of actual packets:

```
RECORD STRUCTURE (264 bytes per record):
====================================================================================
Offset  | Size  | Endian | Field Name             | Example Values (Record 1)
====================================================================================
 +00-03 | 4 bytes| LE     | Token                  | 861201
 +04-07 | 4 bytes| LE     | Close/Prev Close Rate  | 38005 paise (380.05 Rs)
 +08-11 | 4 bytes| LE     | Open Rate              | 38235 paise (382.35 Rs)
 +12-15 | 4 bytes| LE     | High Rate              | 47585 paise (475.85 Rs)
 +16-19 | 4 bytes| LE     | Low Rate               | 37370 paise (373.70 Rs)
 +20-23 | 4 bytes| LE     | Num Trades             | 41
 +24-27 | 4 bytes| LE     | Volume or similar      | 920
 +28-31 | 4 bytes| LE     | Unknown field          | 764
 +32-35 | 4 bytes| LE     | Unknown field          | 20
 +36-39 | 4 bytes| LE     | LTP (BASE VALUE!)      | 47515 paise (475.15 Rs)
 +40... | varies | varies | More uncompressed + compressed fields...
====================================================================================
```

**KEY FINDING:** All price fields are **LITTLE-ENDIAN**, not Big-Endian as assumed!

### 4. **Why Your Current Output is Wrong**

From your logs:
```
{'token': 875045, 'num_trades': 0, 'volume': 0, 'close_rate': -1429733376, ...}
```

This happens because:
1. You're reading fields at wrong byte offsets
2. You're using wrong endianness (BE instead of LE for many fields)
3. You're treating uncompressed data as compressed differentials

### 5. **The Compression Mystery**

The BSE manual (Section 5) describes a compression algorithm, but YOUR PACKETS APPEAR TO BE UNCOMPRESSED!

Evidence:
- All price values are reasonable absolute values (380 Rs, 475 Rs, etc.)
- No evidence of 2-byte differentials followed by full 4-byte values
- No 32767, ±32766 special markers found
- Values don't follow differential pattern (base + diff)

**Hypothesis:** BSE may send UNCOMPRESSED packets during certain market conditions or for certain message types.

## Recommended Fixes

### IMMEDIATE FIX (decoder.py):

```python
def _parse_record(self, packet: bytes, offset: int) -> dict:
    """
    Parse uncompressed BSE record (264 bytes).
    All fields are LITTLE-ENDIAN.
    """
    record = {}
    
    # Token (LE uint32)
    record['token'] = struct.unpack('<I', packet[offset:offset+4])[0]
    
    # Check for empty record
    if record['token'] == 0 or record['token'] == 1:
        return {'empty': True, 'token': record['token']}
    
    # Price fields (LE int32, paise)
    record['close_rate'] = struct.unpack('<i', packet[offset+4:offset+8])[0] / 100.0
    record['open'] = struct.unpack('<i', packet[offset+8:offset+12])[0] / 100.0
    record['high'] = struct.unpack('<i', packet[offset+12:offset+16])[0] / 100.0
    record['low'] = struct.unpack('<i', packet[offset+16:offset+20])[0] / 100.0
    
    # Num trades (LE uint32)
    record['num_trades'] = struct.unpack('<I', packet[offset+20:offset+24])[0]
    
    # Volume (LE uint32 or similar)
    record['volume'] = struct.unpack('<I', packet[offset+24:offset+28])[0]
    
    # LTP - Last Traded Price (LE int32, paise)
    record['ltp'] = struct.unpack('<i', packet[offset+36:offset+40])[0] / 100.0
    
    # LTQ - need to find correct offset (probably around +40-48)
    # For now, use a placeholder
    record['ltq'] = 0
    
    record['empty'] = False
    return record
```

### CRITICAL CHANGES NEEDED:

1. **decoder.py:**
   - Fix `num_records` offset: 34-35 not 32-33
   - Change record size from 64 to 264 bytes
   - Read all price fields as **LITTLE-ENDIAN** not Big-Endian
   - Use the correct field offsets shown above

2. **decompressor.py:**
   - May NOT be needed if packets are uncompressed!
   - Test first with simple field reading
   - Only apply decompression if you see differential patterns

3. **data_collector.py:**
   - Add token master file (token_details.json is missing!)
   - Without this, all symbols show as "UNKNOWN"

4. **main.py/saver.py:**
   - Timestamp is being set to "2025-10-09 00:00:00" - use actual packet timestamp
   - Fix: Pass header['timestamp'] to the data collector

## Testing Steps

1. **Create test_decoder_simple.py:**
   ```python
   # Read packet, parse with CORRECT offsets and endianness
   # Print raw values to verify they match analysis
   ```

2. **Verify Token Mapping:**
   ```python
   # Token 861201 and 861289 should map to real contract symbols
   # Get token master file from BSE or BOLTPLUS API
   ```

3. **Fix Timestamp:**
   ```python
   # Use header timestamp not system time
   # Convert to proper datetime format
   ```

4. **Test with Multiple Packets:**
   ```python
   # Verify structure holds across different packets
   # Check for any packets that ARE compressed
   ```

## Files Needing Updates

1. `src/decoder.py` - Fix field offsets and endianness (CRITICAL)
2. `src/decompressor.py` - May be unnecessary, test first
3. `src/data_collector.py` - Add proper timestamp handling
4. `src/saver.py` - Use real timestamps
5. `data/tokens/token_details.json` - ADD THIS FILE (currently missing!)

## Expected Output After Fixes

```csv
token,symbol,timestamp,open,high,low,close,ltp,volume,prev_close,...
861201,SENSEX25OCT24500CE,2025-10-09 11:51:04,382.35,475.85,373.70,380.05,475.15,920,380.05,...
861289,SENSEX25OCT24600CE,2025-10-09 11:51:04,83.85,93.65,76.00,78.85,90.05,2120,78.85,...
```

## Questions to Answer

1. **Are ALL BSE packets uncompressed, or only some?**
   - Test with more packets from different times
   - Check if compression flag exists in header

2. **What are the fields at offsets +40 to +264?**
   - Need to map remaining 224 bytes
   - Likely include Best 5 bid/ask levels, volumes, etc.

3. **Where is LTQ (Last Traded Quantity)?**
   - Not found at expected offset
   - Might be at +40-47 (8-byte field)

4. **How to get token master file?**
   - Check BSE website for contract master download
   - Or implement BOLTPLUS API authentication

## Conclusion

The core issue is that your decoder is reading the wrong byte offsets and using wrong endianness. The manual's table shows LOGICAL field order, not PHYSICAL byte layout. The actual packet structure must be determined empirically, which I've done above.

**Priority 1:** Fix decoder.py with correct offsets and Little-Endian for prices  
**Priority 2:** Get token master file for symbol mapping  
**Priority 3:** Fix timestamp handling  
**Priority 4:** Determine if decompression is actually needed

Once these are fixed, you should see reasonable market data values instead of the garbage currently being output.
