# BSE UDP Market Data Reader - Architecture Guide

## Visual Architecture Diagrams

### 1. System Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        BSE MARKET DATA NETWORK                           │
│                                                                           │
│   Production:  227.0.0.21:12996 (Equity NFCAST)                         │
│   Simulation:  226.1.0.1:11401  (Equity test feed)                      │
│                                                                           │
│   Protocol: UDP Multicast (IGMPv2)                                       │
│   Packet Format: BSE Proprietary Binary (564 bytes)                      │
└──────────────────────────────────┬──────────────────────────────────────┘
                                   │
                                   │ UDP Multicast Stream
                                   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                     APPLICATION LAYER (Python 3.8+)                      │
│                                                                           │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    MAIN ORCHESTRATOR                             │   │
│  │                     (src/main.py)                                │   │
│  │                                                                   │   │
│  │  • Signal handling (Ctrl+C)                                      │   │
│  │  • Component initialization                                      │   │
│  │  • Logging configuration                                         │   │
│  │  • Error recovery                                                │   │
│  └────────────────┬────────────────────────────────────────────────┘   │
│                   │                                                      │
│                   │ Coordinates                                          │
│                   ▼                                                      │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                  CONNECTION MANAGER                              │   │
│  │                  (src/connection.py)                             │   │
│  │                                                                   │   │
│  │  • UDP socket creation                                           │   │
│  │  • Multicast group join (IGMP)                                   │   │
│  │  • Socket buffer tuning (2MB)                                    │   │
│  │  • Network error handling                                        │   │
│  └────────────────┬────────────────────────────────────────────────┘   │
│                   │                                                      │
│                   │ Returns socket                                       │
│                   ▼                                                      │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                  PACKET RECEIVER                                 │   │
│  │               (src/packet_receiver.py)                           │   │
│  │                                                                   │   │
│  │  • Receive loop (sock.recvfrom)                                  │   │
│  │  • Size validation (564 bytes)                                   │   │
│  │  • Statistics tracking                                           │   │
│  │  • Optional raw packet saving                                    │   │
│  └────────────────┬────────────────────────────────────────────────┘   │
│                   │                                                      │
│                   │ Raw packets                                          │
│                   ▼                                                      │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                   BINARY DECODER                                 │   │
│  │                   (src/decoder.py)                               │   │
│  │                                                                   │   │
│  │  • Header parsing (36 bytes)                                     │   │
│  │    - Format ID (0x0234)                                          │   │
│  │    - Timestamp (HH:MM:SS)                                        │   │
│  │    - Record size detection (66 bytes)                            │   │
│  │  • Record parsing (8 × 66 bytes)                                 │   │
│  │    - Token, LTP, Prev Close, Close Rate                          │   │
│  │    - Volume, Trades                                              │   │
│  │  • Compression detection                                         │   │
│  │  • Paise → Rupees conversion (format 0x0234)                     │   │
│  └────────────────┬────────────────────────────────────────────────┘   │
│                   │                                                      │
│                   │ Decoded records                                      │
│                   ▼                                                      │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                MARKET DEPTH DECOMPRESSOR                         │   │
│  │                 (src/decompressor.py)                            │   │
│  │                                                                   │   │
│  │  • Uncompressed bypass (format 0x0234)                           │   │
│  │  • Differential decompression (compressed)                       │   │
│  │    - Open/High/Low extraction                                    │   │
│  │    - 5 bid levels + 5 ask levels                                 │   │
│  │  • Paise → Rupees conversion                                     │   │
│  └────────────────┬────────────────────────────────────────────────┘   │
│                   │                                                      │
│                   │ Decompressed data                                    │
│                   ▼                                                      │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                   DATA COLLECTOR                                 │   │
│  │                (src/data_collector.py)                           │   │
│  │                                                                   │   │
│  │  • Token → Symbol mapping                                        │   │
│  │  • Load token database (29k contracts)                           │   │
│  │  • Data validation (LTP > 0, Vol > 0)                            │   │
│  │  • Quote normalization                                           │   │
│  └────────────────┬────────────────────────────────────────────────┘   │
│                   │                                                      │
│                   │ Normalized quotes                                    │
│                   ▼                                                      │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │                    OUTPUT WRITER                                 │   │
│  │                     (src/saver.py)                               │   │
│  │                                                                   │   │
│  │  • JSON output (newline-delimited)                               │   │
│  │  • CSV output (append mode)                                      │   │
│  │  • File rotation (daily)                                         │   │
│  │  • Error handling                                                │   │
│  └────────────────┬────────────────────────────────────────────────┘   │
│                   │                                                      │
└───────────────────┼──────────────────────────────────────────────────────┘
                    │
                    │ Writes to disk
                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         FILE STORAGE                                     │
│                                                                           │
│  data/processed_json/20251006_quotes.json                                │
│  data/processed_csv/20251006_quotes.csv                                  │
│  data/raw_packets/*.bin (optional)                                       │
└─────────────────────────────────────────────────────────────────────────┘
```

---

### 2. Packet Processing Pipeline

```
┌──────────────────────────────────────────────────────────────────────┐
│                        PACKET LIFECYCLE                               │
└──────────────────────────────────────────────────────────────────────┘

Step 1: NETWORK RECEPTION
┌─────────────────────────────────────────────────────────────────┐
│  UDP Packet Arrives (564 bytes)                                 │
│                                                                  │
│  Source: 226.1.0.1:11401                                        │
│  Destination: Multicast Group                                   │
│  Transport: UDP (unreliable, no retransmission)                 │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
Step 2: SOCKET BUFFER
┌─────────────────────────────────────────────────────────────────┐
│  OS Kernel Buffer (2 MB)                                        │
│                                                                  │
│  [Packet 1][Packet 2][Packet 3]...[Packet N]                   │
│                                                                  │
│  • FIFO queue                                                   │
│  • Drops oldest if full (packet loss)                           │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           │ recvfrom(2000)
                           ▼
Step 3: APPLICATION BUFFER
┌─────────────────────────────────────────────────────────────────┐
│  packet = sock.recvfrom(2000)                                   │
│                                                                  │
│  [0x00][0x00][0x00][0x00][0x34][0x02][0xe4][0x07]...           │
│   564 bytes total                                               │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
Step 4: SIZE VALIDATION
┌─────────────────────────────────────────────────────────────────┐
│  if len(packet) != 564:                                         │
│      log_warning("Invalid size")                                │
│      return                                                     │
│                                                                  │
│  Stats: valid_packets += 1                                      │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
Step 5: HEADER DECODING (36 bytes)
┌─────────────────────────────────────────────────────────────────┐
│  Offset | Field          | Value                                │
│  -------|----------------|--------------------------------------│
│  0-3    | Leading zeros  | 0x00000000                           │
│  4-5    | Format ID (LE) | 0x0234 → 564                         │
│  6-7    | Type (LE)      | 0x07e4 → 2020                        │
│  20-21  | Hour (LE)      | 12                                   │
│  22-23  | Minute (LE)    | 27                                   │
│  24-25  | Second (LE)    | 8                                    │
│                                                                  │
│  Output: {'format_id': 564, 'timestamp': '12:27:08', ...}      │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
Step 6: RECORD SIZE DETERMINATION
┌─────────────────────────────────────────────────────────────────┐
│  if format_id == 0x0234:                                        │
│      record_size = 66                                           │
│      is_compressed = False                                      │
│  else:                                                          │
│      record_size = 64                                           │
│      is_compressed = True                                       │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
Step 7: RECORD PARSING (8 records)
┌─────────────────────────────────────────────────────────────────┐
│  For offset in [36, 102, 168, 234, 300, 366, 432, 498]:       │
│                                                                  │
│    Record bytes: packet[offset : offset + 66]                  │
│                                                                  │
│    Offset | Field       | Raw Bytes    | Parsed Value          │
│    -------|-------------|--------------|----------------------│
│    0-3    | Token (LE)  | 0x31 0x26 0x0D 0x00 | 861201        │
│    4-7    | LTP (LE)    | 0x65 0x94 0x00 0x00 | 38005 paise   │
│    8-11   | Prev Cl(LE) | 0xAB 0x95 0x00 0x00 | 38315 paise   │
│    12-15  | Close (LE)  | 0x01 0xBA 0x00 0x00 | 47617 paise   │
│    16-19  | Volume (LE) | 0xFA 0x91 0x00 0x00 | 37370         │
│    20-23  | Trades (LE) | 0x1A 0x00 0x00 0x00 | 26            │
│    24-65  | Compressed  | [42 bytes]          | For decomp    │
│                                                                  │
│    is_compressed = False (format 0x0234)                        │
│    ltp = 38005 / 100.0 = 380.05 Rupees                         │
│                                                                  │
│  Output: List[{token, ltp, volume, is_compressed, ...}]        │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
Step 8: DECOMPRESSION ROUTING
┌─────────────────────────────────────────────────────────────────┐
│  if not record['is_compressed']:                                │
│      # Format 0x0234: Already in Rupees                         │
│      return {                                                   │
│          'token': 861201,                                       │
│          'ltp': 380.05,        # Already converted              │
│          'volume': 37370,                                       │
│          'bid_levels': [],     # No market depth               │
│          'ask_levels': []                                       │
│      }                                                          │
│  else:                                                          │
│      # Compressed format: Apply differential decompression     │
│      open = ltp + decompress_value(offset + 24)                │
│      high = ltp + decompress_value(offset + 26)                │
│      low = ltp + decompress_value(offset + 28)                 │
│      # Extract 5 bid/ask levels...                             │
│      # Convert paise → Rupees                                  │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
Step 9: TOKEN MAPPING
┌─────────────────────────────────────────────────────────────────┐
│  token_map = load('data/tokens/token_details.json')            │
│                                                                  │
│  if '861201' in token_map:                                      │
│      info = token_map['861201']                                 │
│      quote = {                                                  │
│          'token': 861201,                                       │
│          'symbol': 'SENSEX',                                    │
│          'expiry': '2025-10-31',                                │
│          'option_type': 'CE',                                   │
│          'strike_price': '82700.00',                            │
│          'ltp': 380.05,                                         │
│          'volume': 37370                                        │
│      }                                                          │
│  else:                                                          │
│      quote['symbol'] = 'UNKNOWN'                                │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
Step 10: VALIDATION
┌─────────────────────────────────────────────────────────────────┐
│  if quote['ltp'] <= 0:                                          │
│      log_warning("Invalid LTP")                                 │
│      return None                                                │
│                                                                  │
│  if quote['volume'] < 0:                                        │
│      log_warning("Invalid volume")                              │
│      return None                                                │
│                                                                  │
│  ✓ Quote validated                                              │
└──────────────────────────┬──────────────────────────────────────┘
                           │
                           ▼
Step 11: OUTPUT WRITING
┌─────────────────────────────────────────────────────────────────┐
│  JSON (newline-delimited):                                      │
│  {"timestamp":"2025-10-06T12:27:08","token":861201,...}\n       │
│                                                                  │
│  CSV (append mode):                                             │
│  2025-10-06T12:27:08,861201,SENSEX,2025-10-31,CE,82700,380.05  │
│                                                                  │
│  Files:                                                         │
│    data/processed_json/20251006_quotes.json                     │
│    data/processed_csv/20251006_quotes.csv                       │
└─────────────────────────────────────────────────────────────────┘
```

---

### 3. Component Interaction Sequence

```
┌──────────┐  ┌────────────┐  ┌─────────┐  ┌──────────────┐  ┌────────────┐
│  main.py │  │connection.py│  │receiver │  │   decoder    │  │decompressor│
└────┬─────┘  └─────┬──────┘  └────┬────┘  └──────┬───────┘  └─────┬──────┘
     │              │              │               │                │
     │ setup()      │              │               │                │
     │─────────────>│              │               │                │
     │              │              │               │                │
     │          [Create socket]    │               │                │
     │          [Join multicast]   │               │                │
     │              │              │               │                │
     │<─────────────│              │               │                │
     │   socket     │              │               │                │
     │              │              │               │                │
     │ receive_loop()              │               │                │
     │────────────────────────────>│               │                │
     │              │              │               │                │
     │              │           [recvfrom]         │                │
     │              │              │               │                │
     │              │    decode_packet()           │                │
     │              │──────────────────────────────>│                │
     │              │              │               │                │
     │              │              │         [Parse header]         │
     │              │              │         [Parse records]        │
     │              │              │         [Set is_compressed]    │
     │              │              │               │                │
     │              │              │<──────────────│                │
     │              │              │   records     │                │
     │              │              │               │                │
     │              │       decompress_record()                     │
     │              │──────────────────────────────────────────────>│
     │              │              │               │                │
     │              │              │               │  [Check flag]  │
     │              │              │               │  [Bypass or    │
     │              │              │               │   decompress]  │
     │              │              │               │                │
     │              │              │<──────────────────────────────│
     │              │              │       decompressed_data        │
     │              │              │               │                │
     │         collect_quotes()    │               │                │
     │<────────────────────────────│               │                │
     │   normalized_quotes         │               │                │
     │              │              │               │                │
     │  save_to_json()             │               │                │
     │  save_to_csv()              │               │                │
     │              │              │               │                │
     │         [Write files]       │               │                │
     │              │              │               │                │
     │              │     [Loop continues...]      │                │
     │              │              │               │                │
```

---

### 4. Data Structure Transformations

```
┌─────────────────────────────────────────────────────────────────────┐
│                    STAGE 1: RAW UDP PACKET                           │
├─────────────────────────────────────────────────────────────────────┤
│  Type: bytes                                                         │
│  Size: 564 bytes                                                     │
│                                                                       │
│  [0x00][0x00][0x00][0x00][0x34][0x02]...[564 bytes total]          │
└───────────────────────────────┬─────────────────────────────────────┘
                                │ decoder.decode_packet()
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    STAGE 2: DECODED HEADER                           │
├─────────────────────────────────────────────────────────────────────┤
│  Type: Dict                                                          │
│                                                                       │
│  {                                                                   │
│    'format_id': 564,                                                 │
│    'timestamp': datetime(2025, 10, 6, 12, 27, 8),                   │
│    'record_size': 66                                                 │
│  }                                                                   │
└───────────────────────────────┬─────────────────────────────────────┘
                                │ decoder._parse_record()
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    STAGE 3: DECODED RECORDS                          │
├─────────────────────────────────────────────────────────────────────┤
│  Type: List[Dict]                                                    │
│                                                                       │
│  [                                                                   │
│    {                                                                 │
│      'token': 861201,                                                │
│      'ltp': 380.05,              # Rupees (converted from paise)    │
│      'close_rate': 382.35,       # Rupees                           │
│      'prev_close': 383.15,       # Rupees                           │
│      'volume': 37370,                                                │
│      'num_trades': 26,                                               │
│      'compressed_offset': 60,                                        │
│      'is_compressed': False      # Format 0x0234 flag               │
│    },                                                                │
│    {...},  # Record 2                                               │
│    {...}   # Records 3-8                                            │
│  ]                                                                   │
└───────────────────────────────┬─────────────────────────────────────┘
                                │ decompressor.decompress_record()
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                  STAGE 4: DECOMPRESSED DATA                          │
├─────────────────────────────────────────────────────────────────────┤
│  Type: Dict                                                          │
│                                                                       │
│  {                                                                   │
│    'token': 861201,                                                  │
│    'timestamp': datetime(2025, 10, 6, 12, 27, 8),                   │
│    'ltp': 380.05,                # Rupees                           │
│    'close': 382.35,              # Rupees                           │
│    'volume': 37370,                                                  │
│    'bid_levels': [],             # Empty for format 0x0234          │
│    'ask_levels': []              # Empty for format 0x0234          │
│  }                                                                   │
│                                                                       │
│  Note: For compressed formats, bid_levels and ask_levels would      │
│  contain 5 price/qty/order entries each                             │
└───────────────────────────────┬─────────────────────────────────────┘
                                │ data_collector.collect_quotes()
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                   STAGE 5: NORMALIZED QUOTE                          │
├─────────────────────────────────────────────────────────────────────┤
│  Type: Dict                                                          │
│                                                                       │
│  {                                                                   │
│    'timestamp': '2025-10-06T12:27:08',                              │
│    'token': 861201,                                                  │
│    'symbol': 'SENSEX',           # Mapped from token database       │
│    'expiry': '2025-10-31',       # Mapped                           │
│    'option_type': 'CE',          # Mapped                           │
│    'strike_price': '82700.00',   # Mapped                           │
│    'ltp': 380.05,                                                    │
│    'volume': 37370                                                   │
│  }                                                                   │
└───────────────────────────────┬─────────────────────────────────────┘
                                │ saver.save_to_json()
                                │ saver.save_to_csv()
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                   STAGE 6: PERSISTED OUTPUT                          │
├─────────────────────────────────────────────────────────────────────┤
│  JSON (newline-delimited):                                          │
│  {"timestamp":"2025-10-06T12:27:08","token":861201,"symbol":...}    │
│                                                                       │
│  CSV (append mode):                                                  │
│  2025-10-06T12:27:08,861201,SENSEX,2025-10-31,CE,82700.00,380.05   │
│                                                                       │
│  Files:                                                              │
│    data/processed_json/20251006_quotes.json                          │
│    data/processed_csv/20251006_quotes.csv                            │
└─────────────────────────────────────────────────────────────────────┘
```

---

### 5. Error Handling Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                        ERROR SCENARIOS                               │
└─────────────────────────────────────────────────────────────────────┘

Scenario 1: NETWORK ERROR
┌──────────────────┐
│ recvfrom() fails │
└────────┬─────────┘
         │
         ▼
┌──────────────────────────────┐
│ Log error: "Network timeout" │
│ Wait 1 second                │
│ Retry recvfrom()             │
└──────────────────────────────┘

Scenario 2: INVALID PACKET SIZE
┌─────────────────────────┐
│ len(packet) != 564      │
└────────┬────────────────┘
         │
         ▼
┌──────────────────────────────────────┐
│ Log warning: "Invalid size: X bytes" │
│ stats['invalid_size'] += 1           │
│ Save to error log                    │
│ Continue to next packet              │
└──────────────────────────────────────┘

Scenario 3: DECODING ERROR
┌───────────────────────────────┐
│ struct.unpack() raises error  │
└────────┬──────────────────────┘
         │
         ▼
┌──────────────────────────────────────────┐
│ Log error: "Decoding failed at offset X"│
│ Save raw packet to error directory      │
│ stats['decode_errors'] += 1             │
│ Return empty record list                │
└──────────────────────────────────────────┘

Scenario 4: TOKEN NOT FOUND
┌─────────────────────────────┐
│ token not in token_map      │
└────────┬────────────────────┘
         │
         ▼
┌──────────────────────────────────────────┐
│ Log warning: "Unknown token: X"          │
│ quote['symbol'] = 'UNKNOWN'              │
│ stats['unknown_tokens'] += 1             │
│ Continue processing (don't drop quote)   │
└──────────────────────────────────────────┘

Scenario 5: INVALID DATA
┌─────────────────────────────┐
│ ltp <= 0 or volume < 0      │
└────────┬────────────────────┘
         │
         ▼
┌──────────────────────────────────────────┐
│ Log warning: "Invalid LTP/Volume"        │
│ stats['invalid_data'] += 1               │
│ Drop quote (don't save)                  │
│ Continue to next record                  │
└──────────────────────────────────────────┘

Scenario 6: FILE WRITE ERROR
┌─────────────────────────────┐
│ open() or write() fails     │
└────────┬────────────────────┘
         │
         ▼
┌──────────────────────────────────────────┐
│ Log error: "Failed to write to X"        │
│ Retry with exponential backoff           │
│ If still fails, save to memory buffer    │
│ Alert operator (future enhancement)      │
└──────────────────────────────────────────┘

Scenario 7: SIGNAL INTERRUPT (Ctrl+C)
┌─────────────────────────────┐
│ SIGINT received             │
└────────┬────────────────────┘
         │
         ▼
┌──────────────────────────────────────────┐
│ signal_handler() called                  │
│ Set shutdown flag                        │
│ Complete current packet processing       │
│ Flush output buffers                     │
│ Log final statistics                     │
│ Close socket gracefully                  │
│ Exit with code 0                         │
└──────────────────────────────────────────┘
```

---

### 6. Configuration Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                    CONFIGURATION SOURCES                             │
└─────────────────────────────────────────────────────────────────────┘

Priority 1: COMMAND LINE ARGUMENTS (highest)
┌──────────────────────────────────────────┐
│ python src/main.py --multicast-ip X      │
│                    --port Y              │
│                    --log-level INFO      │
└──────────────────┬───────────────────────┘
                   │
                   ▼
Priority 2: ENVIRONMENT VARIABLES
┌──────────────────────────────────────────┐
│ BSE_MULTICAST_IP=226.1.0.1               │
│ BSE_MULTICAST_PORT=11401                 │
│ BSE_LOG_LEVEL=INFO                       │
└──────────────────┬───────────────────────┘
                   │
                   ▼
Priority 3: CONFIG FILE (config.json)
┌──────────────────────────────────────────┐
│ {                                        │
│   "multicast_ip": "226.1.0.1",          │
│   "multicast_port": 11401,              │
│   "log_level": "INFO"                   │
│ }                                        │
└──────────────────┬───────────────────────┘
                   │
                   ▼
Priority 4: HARDCODED DEFAULTS (lowest)
┌──────────────────────────────────────────┐
│ DEFAULT_MULTICAST_IP = "226.1.0.1"      │
│ DEFAULT_MULTICAST_PORT = 11401          │
│ DEFAULT_LOG_LEVEL = "INFO"              │
└──────────────────┬───────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      MERGED CONFIGURATION                            │
├─────────────────────────────────────────────────────────────────────┤
│  {                                                                   │
│    "multicast_ip": "226.1.0.1",        # From Priority 1            │
│    "multicast_port": 11401,            # From Priority 2            │
│    "buffer_size": 2000,                # From Priority 3            │
│    "log_level": "INFO",                # From Priority 4            │
│    "token_file": "data/tokens/token_details.json",                  │
│    "save_raw_packets": false                                        │
│  }                                                                   │
└─────────────────────────────────────────────────────────────────────┘
```

---

### 7. State Machine: Connection Lifecycle

```
┌─────────────────────────────────────────────────────────────────────┐
│                    CONNECTION STATE MACHINE                          │
└─────────────────────────────────────────────────────────────────────┘

                    ┌───────────────┐
                    │  INITIALIZED  │
                    └───────┬───────┘
                            │ setup_multicast_socket()
                            ▼
                    ┌───────────────┐
                    │  CONNECTING   │◄─────┐
                    └───────┬───────┘      │
                            │              │ Retry
                            │ Success      │ (network error)
                            ▼              │
                    ┌───────────────┐      │
           ┌───────►│   JOINED      │──────┘
           │        └───────┬───────┘
           │                │ receive_loop()
           │                ▼
           │        ┌───────────────┐
           │        │   RECEIVING   │
           │        └───────┬───────┘
           │                │
           │        ┌───────┴───────┐
           │        │               │
           │  Ctrl+C│               │ Timeout
           │        ▼               ▼
           │ ┌──────────────┐ ┌──────────────┐
           │ │  SHUTTING_   │ │   TIMEOUT    │
           │ │     DOWN     │ └──────┬───────┘
           │ └──────┬───────┘        │ Reconnect
           │        │                └──────────┐
           │        │ Cleanup                   │
           │        ▼                           │
           │ ┌──────────────┐                  │
           └─┤  DISCONNECTED├──────────────────┘
             └──────┬───────┘
                    │ Restart?
                    ▼
             ┌──────────────┐
             │   STOPPED    │
             └──────────────┘
```

---

## Performance Optimization Strategies

### 1. Memory Management
```python
# Batch processing to reduce memory
BATCH_SIZE = 100
quotes_buffer = []

for packet in packet_stream:
    quotes = process_packet(packet)
    quotes_buffer.extend(quotes)
    
    if len(quotes_buffer) >= BATCH_SIZE:
        saver.save_batch(quotes_buffer)
        quotes_buffer.clear()
```

### 2. CPU Optimization
```python
# Reduce struct.unpack calls by unpacking multiple fields at once
token, ltp, prev_close, close_rate, volume = struct.unpack(
    '<5I',  # 5 unsigned integers (20 bytes)
    packet[offset:offset+20]
)
```

### 3. I/O Optimization
```python
# Use buffered file writes
with open(filename, 'a', buffering=8192) as f:
    for quote in quotes:
        f.write(json.dumps(quote) + '\n')
```

### 4. Network Optimization
```python
# Increase socket buffer to reduce packet loss
sock.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 4 * 1024 * 1024)  # 4 MB
```

---

## Security Considerations

### 1. Network Security
- UDP multicast is **unencrypted** (plaintext)
- Requires VPN or direct BSE network connectivity
- No authentication at network level
- Packet spoofing possible (verify source IP)

### 2. Data Integrity
- No checksums in BSE protocol (UDP has basic checksum)
- Packet loss possible (UDP is unreliable)
- No sequence numbers (can't detect gaps)
- Validate data ranges (LTP > 0, Volume >= 0)

### 3. Application Security
- Run with minimal privileges (non-root user)
- Validate all configuration inputs
- Sanitize token database (prevent injection)
- Log security events (unauthorized access attempts)

---

## Scalability Considerations

### Horizontal Scaling
```
Load Balancer (Round-robin)
         │
    ┌────┴────┬────────┬────────┐
    │         │        │        │
Instance 1  Instance 2  Instance 3  Instance 4
  (Port      (Port      (Port      (Port
   11401)     11402)     11403)     11404)
    │         │        │        │
    └────┬────┴────────┴────────┘
         │
   Shared Storage
  (NFS or Database)
```

### Vertical Scaling
- Multi-threading: One thread per component
- Async I/O: Non-blocking socket reads
- Memory: Increase buffer sizes (2MB → 8MB)
- CPU: Optimize struct.unpack (use Cython)

---

## Monitoring & Observability

### Key Metrics
```python
{
  "packets_per_second": 500,
  "records_per_second": 4000,
  "packet_loss_rate": 0.01,  # 1%
  "decode_success_rate": 0.9999,  # 99.99%
  "unknown_token_rate": 0.15,  # 15%
  "avg_processing_time_ms": 2,
  "memory_usage_mb": 50,
  "cpu_usage_percent": 10
}
```

### Logging Best Practices
```python
# Structured logging
logger.info("Packet processed", extra={
    'packet_size': 564,
    'records_extracted': 6,
    'processing_time_ms': 1.5,
    'timestamp': '2025-10-06T12:27:08'
})
```

---

## Conclusion

This architecture guide provides visual representations and detailed explanations of the BSE UDP Market Data Reader system. For implementation details, refer to:
- `PROJECT_DOCUMENTATION.md` - Complete project documentation
- `docs/BSE_Complete_Technical_Knowledge_Base.md` - Protocol specifications
- Source code in `src/` directory

**Key Takeaways**:
1. **Pipeline Architecture**: UDP → Decoder → Decompressor → Collector → Saver
2. **Format 0x0234**: 564 bytes, 66-byte records, uncompressed (critical discovery)
3. **Compression Bypass**: Essential for correct price extraction
4. **Error Handling**: Graceful degradation at each stage
5. **Scalability**: Horizontal scaling via multiple instances
