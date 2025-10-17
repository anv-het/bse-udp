# CRITICAL FINDINGS - BSE NFCAST Packet Structure

## Date: January 17, 2025

## ISSUE IDENTIFIED
The current decoder is parsing the packet structure incorrectly. Based on analysis of:
1. BSE_DIRECT_NFCAST_Manual.txt (pages 22-27, 48-51)
2. Old working code (bse_multi_token_processor.py)
3. CSV output from 20251017_quotes.csv

## CORRECT PACKET STRUCTURE (Message Types 2020/2021)

### Header (36 bytes):
```
Offset  Field                Type            Bytes   Endian
0-3     Message Type         Long            4       Little
4-7     Reserved 1           Long            4       
8-11    Reserved 2           Long            4       
12-13   Reserved 3           Unsigned Short  2       
14-15   Hour                 Short           2       Little
16-17   Minute               Short           2       Little
18-19   Second               Short           2       Little
20-21   Millisecond          Short           2       Little
22-23   Reserved 4           Short           2       
24-25   Reserved 5           Short           2       
26-27   No of Records        Short           2       Little
28-35   Padding              -               8       
```

### Per Record Structure (VARIABLE LENGTH due to compression):

#### UNCOMPRESSED SECTION (starts at byte 36 for first record):
```
Offset  Field                    Type            Bytes   Endian      Notes
+0      Instrument Code          Long/Long Long  4/8     Little      Token ID
+4/+8   No of Trades             Unsigned Long   4       Big
+8/+12  Traded Volume            Long Long       8       Big
+16/+20 Traded Value             Long Long       8       Big
+24/+28 Trade Value Flag         Char            1       
+25/+29 Reserved 6               Char            1       
+26/+30 Reserved 7               Char            1       
+27/+31 Reserved 8               Char            1       
+28/+32 Market Type              Short           2       Big
+30/+34 Session Number           Short           2       Big
+32/+36 LTP Hour                 Char            1       
+33/+37 LTP Minute               Char            1       
+34/+38 LTP Second               Char            1       
+35/+39 LTP Millisecond          Char[3]         3       
+38/+42 Reserved 9               Char[2]         2       
+40/+44 Reserved 10              Short           2       
+42/+46 Reserved 11              Long Long       8       
+50/+54 No of Price Points       Short           2       Big         Usually 5
+52/+56 Timestamp                Long Long       8       Big         Julian
+60/+64 Close Rate               Long            4       Big         Paise
+64/+68 LTQ (Base for qty)       Long Long       8       Big         **BASE VALUE**
+72/+76 LTP (Base for price)     Long            4       Big         **BASE VALUE** in paise
```

#### COMPRESSED SECTION (starts after LTP field):
**ALL FOLLOWING FIELDS ARE 2-BYTE DIFFERENTIALS** (unless special marker 32767)

```
Field                       Base Value          Special Markers
Open Rate                   LTP                 32767 = read next 4 bytes
Previous Close              LTP                 32767 = read next 4 bytes
High Rate                   LTP                 32767 = read next 4 bytes
Low Rate                    LTP                 32767 = read next 4 bytes
Block Deal Ref Price        LTP                 32767 = read next 4 bytes
Indicative Eq Price         LTP                 32767 = read next 4 bytes
Indicative Eq Qty           LTQ                 32767 = read next 4 bytes
Total Bid Quantity          LTQ                 32767 = read next 4 bytes
Total Offer Quantity        LTQ                 32767 = read next 4 bytes
Lower Circuit               LTP                 32767 = read next 4 bytes
Upper Circuit               LTP                 32767 = read next 4 bytes
Weighted Avg Price          LTP                 32767 = read next 4 bytes

**Best 5 Bid Levels** (cascading bases):
Level 1: Best Bid Rate      LTP                 32766 = end of bids
         Total Bid Qty       LTQ
         No. of Bid          LTQ
         Implied Buy Qty     LTQ
         
Level 2: Best Bid Rate      Previous Best Bid Rate
         Total Bid Qty       Previous Total Bid Qty
         No. of Bid          Previous No. of Bid
         Implied Buy Qty     Previous Implied Buy Qty
         
... (up to Level 5)

**Best 5 Ask/Offer Levels** (cascading bases):
Level 1: Best Offer Rate    LTP                 -32766 = end of offers
         Total Offer Qty     LTQ
         No. of Ask          LTQ
         Implied Sell Qty    LTQ
         
Level 2: Best Offer Rate    Previous Best Offer Rate
         Total Offer Qty     Previous Total Offer Qty
         No. of Ask          Previous No. of Ask
         Implied Sell Qty    Previous Implied Sell Qty
         
... (up to Level 5)
```

## KEY CORRECTIONS NEEDED

### 1. Decoder (decoder.py):
- **WRONG**: Currently reads token at offset 0, LTP at 16-20
- **CORRECT**: Should read full uncompressed section
- **WRONG**: Assumes 64-byte records
- **CORRECT**: Records are VARIABLE LENGTH due to compression
- Need to parse full uncompressed section including:
  - Token (4 bytes Little-Endian)
  - Trades, Volume, Value (all Big-Endian)
  - Timestamps (LTP time)
  - Close Rate, LTQ, LTP (base values, Big-Endian)

### 2. Decompressor (decompressor.py):
- **CORRECT**: Differential logic is right
- **NEEDS FIX**: compressed_offset calculation is wrong
- **NEEDS FIX**: Must handle ALL compressed fields, not just Open/High/Low
- **CORRECT**: Best 5 cascading logic is implemented

### 3. Record Size:
- **OLD CODE SHOWS**: 264 bytes per record offsets in PACKET_FORMAT_MAP
- **REALITY**: Variable length due to compression
- **SOLUTION**: Parse byte-by-byte, track offset after each field

## CRITICAL DATA FROM CSV (Token 878192, SENSEX option):
```csv
Token: 878192
Symbol: SENSEX25OCT237760000PE_PE__23OCT2025
Timestamp: 2025-10-17 00:00:00
Open: 8556124.24 (855612424 paise)
High: 8556380.16 (855638016 paise) 
Low: 8556482.56 (855648256 paise)
Close: 8556380.16 (855638016 paise)
LTP: 8556380.16 (855638016 paise)
Volume: 403374080
Prev Close: 838860.8 (83886080 paise)
```

## COMPARISON WITH CURRENT OUTPUT:
```
Current: LTP=9.70 (970 paise)
Expected: LTP=8556380.16 (855638016 paise)
RATIO: 882019.59x difference!
```

This confirms **field offsets are completely wrong**.

## ACTION ITEMS:

1. **REWRITE decoder.py**:
   - Parse header correctly (timestamp fields at 14-21)
   - Read full uncompressed section per record
   - Extract Close Rate, LTQ, LTP as base values
   - Calculate correct compressed_offset after LTP field

2. **UPDATE decompressor.py**:
   - Start decompression from correct offset
   - Decompress ALL fields (not just Open/High/Low)
   - Use correct base values

3. **TEST with actual packet**:
   - Token 878192 should show LTP around 855 lakh paise (8.5 million rupees)
   - Verify timestamp parsing
   - Verify Best 5 levels

## REFERENCE FILES:
- BSE_DIRECT_NFCAST_Manual.txt (section 4.8, pages 22-27)
- BSE_DIRECT_NFCAST_Manual.txt (section 5, pages 48-51 - Decompression)
- old_code/bse_multi_token_processor.py (working field mappings)
