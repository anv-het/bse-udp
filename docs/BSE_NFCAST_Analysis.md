# BSE Direct NFCAST Protocol Analysis

## Overview
Based on the BSE Direct NFCAST Manual V 5.0, this document analyzes the message format and structure for implementing a packet parser.

## Infrastructure Requirements
- **Multicast Network**: BSE multicast network with IP multicast capable router
- **Protocol**: IGMPv2 (or IGMPv3 with IGMPv2 compatibility mode)
- **Network Byte Order**: Big Endian byte order
- **Transport**: UDP datagrams (unreliable, self-contained packets)

## Message Categories

### Service Messages (Technical)
- **Time Broadcast**: Sent periodically (every minute) on multicast address
- **Auction Keep Alive**: Network message to keep spanning tree alive for auction broadcasts

### Data Messages (Functional - Product/Instrument Related)
- **Market Picture**: Snapshot of 5 price levels + statistical trade information
- **Product State Change**: BSE product state publishing (Session Broadcast)
- **Index Change**: Current index values, day's high/low/open/close (periodic)
- **Auction Market Picture**: 5 price levels of auction order book (defaulter/shortage auctions)
- **Close Price**: Close prices for all instruments (closing session + start of day)
- **Open Interest**: Open interest for derivatives contracts
- **RBI Reference Rate**: USD reference rate published by RBI
- **VaR Percentage**: Approximate applicable margin percentage for trades

## Key Message Types

### Message Type Field (Long - 4 bytes)
- **2020**: Market Picture (COMPRESSED)
- **2021**: Market Picture (Complex Instruments - COMPRESSED)

**⚠️ CRITICAL**: Only message types 2020 and 2021 use compression. All other messages can be read directly.

## UDP Packet Structure
```
┌─────────────────┐
│  MESSAGE TYPE   │ ← 4 bytes (Template ID)
├─────────────────┤
│    MESSAGE      │ ← Actual message data
│     DATA        │
└─────────────────┘
```

## Compression Details

### Compression Scope
- **ONLY** applies to message types **2020** and **2021**
- **NOT** the entire message - compression starts from **Open Rate field** onwards
- All fields before Open Rate are **uncompressed** (can be read directly)
- Uses **proprietary BSE compression algorithm**

### Compression Principle
- **Rate fields**: Use differential values (2 bytes) instead of full 4-byte values
- **Base Fields**: LTP (Last Traded Price) for prices, LTQ (Last Traded Quantity) for quantities
- **Savings**: 2 bytes saved per compressed field (4 bytes → 2 bytes for differences)

## Decompression Mechanism

### Base Fields
- **Price Base**: LTP (Last Traded Price) - used for all price-related fields
- **Quantity Base**: LTQ (Last Traded Quantity) - used for all quantity-related fields

### Decompression Algorithm
1. **Read 2 bytes** for compressed field
2. **Check special values**:
   - `32767`: Difference exceeds 2 bytes → read next 4 bytes for actual value
   - `32766`: End of buy side data (no more buy levels)
   - `-32766`: End of sell side data (no more sell levels)
3. **Calculate actual value**: Base Value + Difference Value

### Best 5 Structure Decompression

#### Reading Sequence
**Buy Side (5 levels)**:
1. Best Bid Rate1, Total Bid Qty1, No. of Bid Orders1, Implied Buy Qty1
2. Best Bid Rate2, Total Bid Qty2, No. of Bid Orders2, Implied Buy Qty2
3. Best Bid Rate3, Total Bid Qty3, No. of Bid Orders3, Implied Buy Qty3
4. Best Bid Rate4, Total Bid Qty4, No. of Bid Orders4, Implied Buy Qty4
5. Best Bid Rate5, Total Bid Qty5, No. of Bid Orders5, Implied Buy Qty5

**Sell Side (5 levels)**: Similar structure for offer rates

#### Dynamic Base Fields for Best 5
| Level | Price Base | Quantity Base | Orders Base | Implied Base |
|-------|------------|---------------|-------------|--------------|
| Level 1 | LTP | LTQ | LTQ | LTQ |
| Level 2 | Best Bid 1 | Best Bid Qty 1 | No of Orders 1 | Implied Buy Qty 1 |
| Level 3 | Best Bid 2 | Best Bid Qty 2 | No of Orders 2 | Implied Buy Qty 2 |
| Level 4 | Best Bid 3 | Best Bid Qty 3 | No of Orders 3 | Implied Buy Qty 3 |
| Level 5 | Best Bid 4 | Best Bid Qty 4 | No of Orders 4 | Implied Buy Qty 4 |

## Complete Packet Reading Sequence

### Uncompressed Header (24 bytes)
```
Field Name          Type        Size    Action
Message Type        Long        4       Read 4 bytes
Reserved Field 1    Long        4       Read 4 bytes
Reserved Field 2    Long        4       Read 4 bytes  
Reserved Field 3    Short       2       Read 2 bytes
Hour               Short       2       Read 2 bytes
Minute             Short       2       Read 2 bytes
Second             Short       2       Read 2 bytes
Millisecond        Short       2       Read 2 bytes
Message Length     Short       2       Read 2 bytes
Number of Records  Short       2       Read 2 bytes
```

### Per-Record Structure (Repeats up to 6 times)

#### Uncompressed Section (up to Close Rate)
```
Instrument Code     Long/Long   4/8     Read 4 bytes (2020) or 8 bytes (2021)
No of Trades        Long        4       Read 4 bytes
Volume              Long Long   8       Read 8 bytes
Value               Long Long   8       Read 8 bytes
Trade Value Flag    Char        1       Read 1 byte
Trend               Char        1       Read 1 byte
Six Lakh Flag       Char        1       Read 1 byte
Reserved Field      Char        1       Read 1 byte
Market Type         Short       2       Read 2 bytes
Session Number      Short       2       Read 2 bytes
LTP Hour            Char        1       Read 1 byte
LTP Minute          Char        1       Read 1 byte
LTP Second          Char        1       Read 1 byte
LTP Millisecond     Char[3]     3       Read 3 bytes
Reserved Field      Char[2]     2       Read 2 bytes
Reserved Field      Short       2       Read 2 bytes
Reserved Field      Long Long   8       Read 8 bytes
No of Price Points  Short       2       Read 2 bytes
Timestamp           Long Long   8       Read 8 bytes
Close Rate          Long        4       Read 4 bytes
Last Trade Qty      Long Long   8       Read 8 bytes → SAVE AS QTY BASE
LTP                 Long        4       Read 4 bytes → SAVE AS RATE BASE
```

#### Compressed Section (starts from Open Rate)
```
Open Rate               Long    2    Apply decompression with Rate Base
Previous Close Rate     Long    2    Apply decompression with Rate Base
High Rate              Long    2    Apply decompression with Rate Base
Low Rate               Long    2    Apply decompression with Rate Base
Reserved Field         Long    2    Apply decompression with Rate Base
Indicative Equilibrium Price  Long  2  Apply decompression with Rate Base
Indicative Equilibrium Qty    Long Long  2  Apply decompression with Qty Base
Total Bid Qty          Long Long  2  Apply decompression with Qty Base
Total Offer Qty        Long Long  2  Apply decompression with Qty Base
Lower Circuit Limit    Long    2    Apply decompression with Rate Base
Upper Circuit Limit    Long    2    Apply decompression with Rate Base
Weighted Average       Long    2    Apply decompression with Rate Base
```

#### Best 5 Buy Structure (Compressed)
```
For each level (1-5) or until 32766 encountered:
Best Bid Rate          Long    2    Decompress with appropriate base
Total Bid Qty          Long Long  2  Decompress with appropriate base
No. of Bid Orders      Long    2    Decompress with appropriate base
Implied Buy Quantity   Long Long  2  Decompress with appropriate base
Reserved Field         Long    2    Decompress with appropriate base
```

#### Best 5 Sell Structure (Compressed)
```
For each level (1-5) or until -32766 encountered:
Best Offer Rate        Long    2    Decompress with appropriate base
Total Offer Qty        Long Long  2  Decompress with appropriate base
No. of Ask Orders      Long    2    Decompress with appropriate base
Implied Sell Quantity  Long Long  2  Decompress with appropriate base
Reserved Field         Long    2    Decompress with appropriate base
```

## Critical Implementation Details

### Buffer Size & Packet Processing
- **MTU Limit**: 1500 bytes maximum per UDP packet
- **Recommended Buffer**: **2000 bytes** for socket reads (as per manual)
- **Read Strategy**: Always request 2000 bytes in single read call
- **Packet Processing**: First 4 bytes = message type for filtering
- **Single Packet Rule**: One packet = one message only

### Special Values for Decompression
- **32767**: Difference exceeds 2 bytes → read next 4 bytes for actual value
- **32766**: End of buy side data (no more buy price levels)
- **-32766**: End of sell side data (no more sell price levels)

### Data Format Conversion
- **Byte Order**: Convert from Big Endian to Little Endian
- **Price Fields**: Values in paise (divide by 100 for rupees)
- **Compressed Fields**: Cannot be mapped to structure directly - must read byte by byte

### Network Characteristics
- **UDP Nature**: Unreliable, packets may be delayed/missing/duplicated
- **No Recovery**: No recovery mechanism defined
- **Self-Contained**: Each packet has complete information
- **No Dependencies**: No dependency across packets
- **Processing Rule**: Apply every packet received
