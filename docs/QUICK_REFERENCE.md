# BSE Decoder - Quick Reference Card

## 🔧 The Fix in 30 Seconds

**Problem:** Wrong byte offsets + wrong endianness = garbage data  
**Solution:** Use correct offsets + Little-Endian for prices = realistic data

---

## 📊 Correct Field Offsets (264-byte records)

```python
# In each record (starting at offset 36 for record 1, 300 for record 2):
token       = struct.unpack('<I', bytes[offset+0:offset+4])[0]    # LE uint32
close_paise = struct.unpack('<i', bytes[offset+4:offset+8])[0]    # LE int32 ⭐
open_paise  = struct.unpack('<i', bytes[offset+8:offset+12])[0]   # LE int32 ⭐
high_paise  = struct.unpack('<i', bytes[offset+12:offset+16])[0]  # LE int32 ⭐
low_paise   = struct.unpack('<i', bytes[offset+16:offset+20])[0]  # LE int32 ⭐
num_trades  = struct.unpack('<I', bytes[offset+20:offset+24])[0]  # LE uint32
volume      = struct.unpack('<I', bytes[offset+24:offset+28])[0]  # LE uint32
ltp_paise   = struct.unpack('<i', bytes[offset+36:offset+40])[0]  # LE int32 ⭐

# Convert paise to Rupees
price_rs = price_paise / 100.0
```

⭐ = **LITTLE-ENDIAN** (`<i` not `>i`)

---

## 📍 Header Fields (36 bytes)

```python
msg_type    = struct.unpack('<H', packet[8:10])[0]    # offset 8-9
hour        = struct.unpack('<H', packet[20:22])[0]   # offset 20-21
minute      = struct.unpack('<H', packet[22:24])[0]   # offset 22-23
second      = struct.unpack('<H', packet[24:26])[0]   # offset 24-25
millisecond = struct.unpack('<H', packet[26:28])[0]   # offset 26-27
num_records = struct.unpack('<H', packet[34:36])[0]   # offset 34-35 ⚠️ NOT 32-33!
```

---

## 🎯 Critical Corrections

| What | Wrong | Correct |
|------|-------|---------|
| **Price Endian** | `'>i'` Big-Endian | `'<i'` Little-Endian ⭐ |
| **Record Size** | 64 or 66 bytes | 264 bytes |
| **Num Records Offset** | 32-33 | 34-35 |
| **Decompression** | Applied | NOT NEEDED (uncompressed) |
| **Record Offset** | `36 + (i * 64)` | `36 + (i * 264)` |

---

## ✅ Working Example

```python
# Correct decoder snippet
def decode_record(packet: bytes, offset: int) -> dict:
    token = struct.unpack('<I', packet[offset:offset+4])[0]
    
    if token == 0 or token == 1:  # Empty record
        return {'empty': True}
    
    # All prices in paise, Little-Endian
    close = struct.unpack('<i', packet[offset+4:offset+8])[0] / 100.0
    open_ = struct.unpack('<i', packet[offset+8:offset+12])[0] / 100.0
    high = struct.unpack('<i', packet[offset+12:offset+16])[0] / 100.0
    low = struct.unpack('<i', packet[offset+16:offset+20])[0] / 100.0
    ltp = struct.unpack('<i', packet[offset+36:offset+40])[0] / 100.0
    
    return {
        'token': token,
        'open': open_,
        'high': high,
        'low': low,
        'close': close,
        'ltp': ltp,
        'volume': struct.unpack('<I', packet[offset+24:offset+28])[0],
        'num_trades': struct.unpack('<I', packet[offset+20:offset+24])[0],
    }
```

**Output:** LTP=475.15 Rs ✅ (not 10066329.6 Rs ❌)

---

## 📂 Files to Reference

1. **Working Code:** `tests/simple_decoder_working.py`
2. **Integration Steps:** `docs/INTEGRATION_GUIDE.md`
3. **Full Analysis:** `docs/SOLUTION_IMPLEMENTED.md`

---

## ⏱️ Integration Time

- **Step 1:** Fix decoder.py → 20 min
- **Step 2:** Simplify decompressor.py → 10 min  
- **Step 3:** Fix timestamps → 5 min
- **Step 4:** Test → 5 min

**Total: ~40 minutes** ⏰

---

## 🧪 Quick Test

```bash
# Test the working decoder
python tests/simple_decoder_working.py

# Expected: Token 861201, LTP=475.15 Rs ✅
# NOT:      Token 56800, LTP=10066329.6 Rs ❌
```

---

## 🚨 Common Mistakes to Avoid

1. ❌ Using `'>i'` (Big-Endian) for prices
2. ❌ Reading num_records from offset 32-33
3. ❌ Using 64-byte record size
4. ❌ Trying to decompress uncompressed data
5. ❌ Using `datetime.now()` instead of packet timestamp

---

## ✅ Verification Checklist

After fixing:

- [ ] Prices are 10-5000 Rs (not millions)
- [ ] Volumes are < 10M (not billions)
- [ ] Timestamps show actual packet time (not 00:00:00)
- [ ] All records decode successfully
- [ ] No "struct.unpack" errors in logs

---

## 🎯 One-Line Summary

**Change prices from Big-Endian to Little-Endian, use offsets +4/+8/+12/+16/+36, and 264-byte records.**

That's it! 🚀
