# BSE UDP Market Data Reader

A Python-based real-time market data feed parser for Bombay Stock Exchange (BSE) via UDP multicast using the **Direct NFCAST protocol** (low bandwidth interface).

## 🎯 Project Goal

Parse BSE market data packets received via UDP multicast, extract touchline data (OHLC, volume, bid/ask) and market depth (best 5 levels), and output to JSON/CSV formats for trading applications.

## 📊 Current Status: Phase 2 Complete ✅

### ✅ What We Did

#### **Phase 1: Project Structure & Connection** ✅

1. **Project Structure Created**
   - ✅ Complete directory structure established
   - ✅ Source code organization (`src/` directory)
   - ✅ Data directories for output (`data/raw_packets`, `data/processed_json`, `data/processed_csv`)
   - ✅ Test framework setup (`tests/` directory)
   - ✅ Documentation consolidated in `docs/`

2. **Virtual Environment Setup**
   - ✅ `.venv/` virtual environment configured
   - ✅ `requirements.txt` created (standard library only)
   - ✅ Python 3.8+ compatibility

3. **Configuration Management**
   - ✅ `config.json` created with multicast parameters
   - ✅ Simulation environment as default (226.1.0.1:11401) for safety
   - ✅ Production environment documented (227.0.0.21:12996)
   - ✅ Buffer size configured (2000 bytes as per BSE specification)

4. **UDP Multicast Connection Implementation** ⭐
   - ✅ `connection.py`: Complete UDP socket implementation
     - Socket creation with IPPROTO_UDP
     - SO_REUSEADDR option set for multiple listeners
     - Port binding on all interfaces
     - **IGMPv2 multicast group join** using IP_ADD_MEMBERSHIP
     - Receive buffer size configuration (2000 bytes)
     - Comprehensive error handling and logging

5. **Testing Framework**
   - ✅ `tests/test_connection.py`: 10 unit tests (all passing)
   - ✅ Mock-based tests (no network required)

#### **Phase 2: Packet Reception, Filtering & Storage** ✅

1. **Packet Receiver Implementation** ⭐
   - ✅ `packet_receiver.py`: Complete packet reception and processing (445 lines)
     - **Continuous receive loop** with configurable timeout
     - **Packet validation**: size (300/556 bytes), leading zeros (0x00000000), format ID
     - **Message type filtering**: 2020 (Market Picture), 2021 (Market Picture Complex)
     - **Token extraction**: Little-Endian uint32 from 64-byte records at offsets 36, 100, 164, 228...
     - **Raw packet storage**: .bin files in `data/raw_packets/` with timestamps
     - **Metadata storage**: JSON entries in `data/processed_json/tokens.json`
     - **Statistics tracking**: packets received/valid/invalid, tokens extracted, errors
     - **Storage limits**: Configurable max packets to store (default: 100)

2. **Main Application Updated** ⭐
   - ✅ `main.py`: Integrated packet receiver
     - Calls `PacketReceiver.receive_loop()` after connection
     - Passes configuration for storage paths and limits
     - Comprehensive logging and graceful shutdown
     - Phase 1 connection + Phase 2 reception in single workflow

3. **Configuration Extended**
   - ✅ Added `store_limit` parameter (default: 100 packets)
   - ✅ Added `timeout` parameter for socket operations (default: 30s)
   - ✅ Documented BSE market hours (9:00 AM - 3:30 PM IST)

4. **Testing Framework Extended**
   - ✅ `tests/test_packet_receiver.py`: 15 unit tests (all passing)
     - Packet validation tests (6 tests)
     - Message type extraction tests (2 tests)
     - Token extraction tests (3 tests)
     - Packet processing and filtering tests (3 tests)
     - Storage limit enforcement test (1 test)

### 🚀 What We Will Do (Next Phases)

#### **Phase 3: Packet Decoding & Decompression**
- [ ] Implement `decoder.py` for BSE proprietary formats
  - 300-byte format parser (fixed 64-byte instrument records)
  - 556-byte format parser
  - Header parsing (36 bytes with leading zeros, mixed endianness)
  - Market data record parsing (8 fields: open, close, high, low, LTP, volume, bid, ask)
  - Price field extraction (Big-Endian int32, divide by 100 for Rupees)
  - Token validation against `data/tokens/token_details.json`
- [ ] BOLTPLUS API authentication (`bse/auth.py`)
- [ ] Contract master synchronization
- [ ] WebSocket streaming interface
- [ ] Performance optimization (<1ms parsing target)

#### **Phase 4: Production Readiness**
- [ ] Comprehensive error recovery
- [ ] Monitoring and alerting
- [ ] Production environment testing
- [ ] Docker containerization
- [ ] Deployment documentation

## 🔧 Installation & Setup

### Prerequisites
- Python 3.8 or higher
- Windows OS (configured for Windows CMD)
- Network access to BSE multicast (or simulation environment)
- IGMPv2 multicast support on network interface

### Setup Steps

```cmd
REM 1. Clone repository (if applicable)
cd d:\bse

REM 2. Create and activate virtual environment
python -m venv .venv
call .venv\Scripts\activate.bat

REM 3. Install dependencies (Phase 1: standard library only)
pip install -r requirements.txt

REM 4. Verify configuration
type config.json

REM 5. Run connection test
python tests\test_connection.py
```

## 🚀 Running the Application

### Phase 1: Connection and Receive Loop

```cmd
REM Activate virtual environment
call .venv\Scripts\activate.bat

REM Run main application
python src\main.py
```

**Expected Output:**
```
======================================================================
🚀 BSE UDP Market Data Reader - Phase 1
======================================================================
...
✅ CONNECTION ESTABLISHED TO BSE NFCAST
======================================================================
✓ Connected to: 226.1.0.1:11401
✓ Segment: Equity
✓ Environment: simulation
✓ Buffer Size: 2000 bytes
✓ Protocol: IGMPv2 multicast
======================================================================

📡 Starting packet receive loop...
📦 Packet #10: Size=300 bytes, From=226.1.0.1:11401, Total=3,000 bytes received
...
```

**To Stop:** Press `Ctrl+C` for graceful shutdown

## 📁 Project Structure

```
bse_udp_reader/
├── .venv/                          # Virtual environment
├── src/
│   ├── __init__.py                # Package initialization
│   ├── connection.py              # ✅ UDP multicast connection (COMPLETE)
│   ├── main.py                    # ✅ Main application (COMPLETE)
│   ├── packet_receiver.py         # ⏳ Placeholder for Phase 2
│   ├── decoder.py                 # ⏳ Placeholder for Phase 2
│   ├── decompressor.py            # ⏳ Placeholder for Phase 3
│   ├── data_collector.py          # ⏳ Placeholder for Phase 2
│   └── saver.py                   # ⏳ Placeholder for Phase 2
├── data/
│   ├── tokens/
│   │   └── token_details.json     # ~29k BSE derivatives contracts
│   ├── raw_packets/               # For raw packet dumps
│   ├── processed_json/            # For JSON output
│   └── processed_csv/             # For CSV output
├── docs/
│   ├── BSE_Complete_Technical_Knowledge_Base.md  # Primary reference
│   ├── BSE_Final_Analysis_Report.md              # Packet analysis
│   ├── BSE_NFCAST_Analysis.md                    # Protocol notes
│   ├── BOLTPLUS Connectivity Manual V1.14.1.pdf  # Connection specs
│   └── BSE_DIRECT_NFCAST_Manual.pdf              # Protocol specs
├── tests/
│   ├── test_connection.py         # ✅ Connection tests (COMPLETE)
│   ├── test_decoder.py            # ⏳ Placeholder for Phase 2
│   └── test_decompressor.py       # ⏳ Placeholder for Phase 3
├── .github/
│   └── copilot-instructions.md    # AI agent guidelines
├── config.json                    # ✅ Configuration (COMPLETE)
├── requirements.txt               # ✅ Dependencies (COMPLETE)
├── README.md                      # ✅ This file (COMPLETE)
└── TODO.md                        # ✅ Task checklist (COMPLETE)
```

## 🔍 Key Technical Details

### BSE Protocol Deviations
⚠️ **Important:** BSE packets do NOT follow standard NFCAST format!

- **Leading Zeros:** Packets start with `0x00000000` (not message type 2020/2021)
- **Mixed Endianness:** Token = Little-Endian, Prices = Big-Endian
- **Fixed Offsets:** Instruments at 64-byte intervals (36, 100, 164, 228...)
- **Field Mismatches:** Official docs have incorrect field names (see analysis reports)

### Network Configuration
- **Simulation:** 226.1.0.1:11401 (Equity NFCAST)
- **Production:** 227.0.0.21:12996 (Equity NFCAST)
- **Protocol:** IGMPv2 multicast
- **Buffer Size:** 2000 bytes (BSE recommendation)
- **Netting Interval:** 0.80 seconds (BSE aggregates updates)

### Market Hours
- **BSE Trading Hours:** 9:00 AM - 3:30 PM IST
- **Note:** Test data outside market hours may be stale

## 🧪 Testing

### Run All Tests
```cmd
call .venv\Scripts\activate.bat
python -m unittest discover tests
```

### Run Specific Test
```cmd
python tests\test_connection.py
```

### Test Coverage (Phase 1)
- ✅ Connection module: 100% coverage
- ⏳ Decoder module: Planned for Phase 2
- ⏳ Decompressor module: Planned for Phase 3

## 📚 Documentation

### Primary References
1. **`docs/BSE_Complete_Technical_Knowledge_Base.md`** - Start here! 850-line comprehensive guide
2. **`docs/BSE_Final_Analysis_Report.md`** - Real packet analysis and discoveries
3. **`.github/copilot-instructions.md`** - AI agent guidelines

### Key Sections to Read
- Packet structure (BSE Complete KB, lines 76-178)
- Connection setup (BSE Complete KB, lines 241-266)
- Parser implementation (BSE Complete KB, lines 384-647)
- Common pitfalls (Copilot Instructions)

## 🐛 Troubleshooting

### "Connection failed" Error
- Verify network access to BSE multicast network
- Check IGMPv2 multicast support: `netsh interface ipv4 show joins`
- Try simulation environment first (226.1.0.1:11401)
- Verify config.json has correct IP/port

### "No packets received"
- Ensure BSE market is open (9:00 AM - 3:30 PM IST)
- Check firewall settings for UDP port
- Verify multicast routing is enabled
- Use Wireshark to verify packets are arriving

### Import Errors
- Ensure virtual environment is activated: `call .venv\Scripts\activate.bat`
- Verify you're running from project root: `cd d:\bse`
- Check Python version: `python --version` (need 3.8+)

## 📝 License

This project is part of the xts-pythonclient-api-sdk repository (symphonyfintech).

## 👥 Authors

BSE Integration Team - October 2025

## 🔗 Quick Links

- [TODO List](TODO.md) - Detailed task checklist
- [Technical Knowledge Base](docs/BSE_Complete_Technical_Knowledge_Base.md)
- [AI Agent Instructions](.github/copilot-instructions.md)

---

**Phase 1 Status:** ✅ **COMPLETE** - Connection established, ready for Phase 2 implementation!
"# bse-udp" 
