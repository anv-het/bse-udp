# SENSEX Live Data Test Summary
**Date:** October 17, 2025, 14:35  
**Test Duration:** Analysis of today's captured data

## Test Configuration

### Target Contracts (Expected @ 14:22:23)
1. **SENSEX 30-OCT-2025 FUT** (Token: 861384)
   - Expected LTP: ₹83,847

2. **SENSEX 83900 CE 23-OCT** (Token: 878196)
   - Expected LTP: ₹486

3. **SENSEX 83800 PE 23-OCT** (Token: 878015)
   - Expected LTP: ₹340

4. **SENSEX 83700 PE 23-OCT** (Token: 877845)
   - Expected LTP: ₹304

5. **SENSEX 84000 CE 23-OCT** (Token: 877761)
   - Expected LTP: ₹430

## Results

### ✅ **SUCCESS METRICS**
- **All 5 target tokens captured** ✓
- **CSV output generated** ✓
- **Multiple updates per token** (4-25 updates each) ✓
- **Timestamped output files** ✓
- **Pipeline fully operational** ✓

### ⚠️ **REMAINING ISSUES**

#### Value Accuracy
All LTP values are **significantly higher than expected**:

| Token | Contract | Expected | Captured | Difference |
|-------|----------|----------|----------|------------|
| 861384 | SENSEX FUT 30-OCT | ₹83,847 | ₹157,324 | 87.6% higher |
| 878196 | SENSEX 83900 CE | ₹486 | ₹142,054 | 29,129% higher |
| 878015 | SENSEX 83800 PE | ₹340 | ₹121,024 | 35,495% higher |
| 877845 | SENSEX 83700 PE | ₹304 | ₹105,203 | 34,506% higher |
| 877761 | SENSEX 84000 CE | ₹430 | ₹50,007 | 11,529% higher |

**Root Cause:** Field offset parsing in decoder is still incorrect.

**Progress Made:**
- Initial error was 882,019x (token 878192 showing ₹9.70 vs expected ₹85,563)
- After corrections, got to 1.8% error on some test packets
- Current issue: Field offsets may vary by contract type or packet structure

#### Volume Values
Volumes are showing extremely large numbers (e.g., 36 quadrillion), indicating:
- Volume field offset is incorrect
- Or endianness issue
- Or volume is stored in different units than expected

## Technical Analysis

### What's Working
1. **Multicast Connection:** Successfully receiving packets from BSE feed (239.1.2.5:26002)
2. **Token Identification:** Correctly identifying target tokens
3. **Packet Processing:** Decoder processing variable-length records
4. **Data Pipeline:** Full flow working: Connection → Receiver → Decoder → Decompressor → Collector → Saver
5. **CSV Generation:** Proper format with OHLC, Volume, Timestamp

### What Needs Fix
1. **LTP Offset:** Currently reading from offset +63 (Big-Endian), but values suggest different structure for options vs futures
2. **Volume Offset:** Currently offset +8-15 (Little-Endian), giving unrealistic values
3. **Close Rate:** Showing same as LTP, should be different

### Suspected Issues
- **Different record structures for Futures vs Options?**
  - Future (861384) is off by 87% → closer to reality
  - Options (878xxx) are off by 11,000-35,000% → completely wrong structure?

- **Packet header interpretation:** Message type 2020 vs 2021 may have different layouts
  
- **Compression state:** May be reading compressed differentials as absolute values

## Next Steps

### Immediate Actions Needed
1. **Capture raw packet for one of the option tokens** (e.g., 878196)
2. **Hex dump analysis** to find actual LTP value (₹486 = 48600 paise)
3. **Compare Future vs Option packet structures** side-by-side
4. **Check if decompression is being applied** (currently using base values only)

### Code Changes Required
1. **decoder.py:** Adjust field offsets based on empirical packet analysis
2. **Test with live data** in real-time to validate corrections
3. **Add packet structure detection** (Future vs Option vs Index)

## Files Generated
- `data/processed_csv/20251017_quotes.csv` - Daily CSV with all market data
- `data/processed_json/20251017_quotes.json` - Daily JSON with all quotes
- `data/test_results/sensex_live_YYYYMMDD_HHMMSS.csv` - Timestamped test results

## Conclusion
The **BSE NFCAST data pipeline is fully operational** and successfully capturing live market data for the target SENSEX contracts. However, **field parsing accuracy needs refinement** through empirical analysis of actual packet structures. The framework is solid; only the byte-level interpretation needs adjustment.

**Status:** 🟡 **PARTIAL SUCCESS** - Data capture working, value parsing needs correction

---
*Generated:* 2025-10-17 14:36  
*Test Script:* `tests/check_sensex_today.py`  
*Tokens Tested:* 5/5 captured, 0/5 accurate values
