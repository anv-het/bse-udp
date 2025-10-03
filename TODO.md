# BSE UDP Market Data Reader - TODO List

## 📋 Phase 1: Project Base Setup and Connection ✅ COMPLETE

### Structure & Setup
- [x] Create project folder structure
- [x] Create `src/` directory with all module files
- [x] Create `data/` subdirectories (raw_packets, processed_json, processed_csv)
- [x] Create `tests/` directory
- [x] Consolidate documentation in `docs/`
- [x] Create `.venv` virtual environment
- [x] Generate `requirements.txt` (Phase 1: standard library only)
- [x] Copy/verify all docs in `docs/` folder

### Configuration
- [x] Create `config.json` with multicast parameters
- [x] Set simulation environment as default (226.1.0.1:11401)
- [x] Document production environment (227.0.0.21:12996)
- [x] Configure buffer size (2000 bytes per BSE spec)
- [x] Add logging configuration

### Core Implementation
- [x] `connection.py`: Complete UDP multicast implementation
  - [x] Socket creation (IPPROTO_UDP)
  - [x] SO_REUSEADDR option
  - [x] Port binding
  - [x] IGMPv2 multicast group join (IP_ADD_MEMBERSHIP)
  - [x] Receive buffer configuration (2000 bytes)
  - [x] Error handling with logging
  - [x] Context manager support
  - [x] Disconnect/cleanup logic
- [x] `main.py`: Application orchestration
  - [x] Configuration loading from JSON
  - [x] Connection establishment
  - [x] Infinite receive loop
  - [x] Packet logging (size, address)
  - [x] Statistics tracking
  - [x] Graceful shutdown (Ctrl+C)
  - [x] Comprehensive logging

### Testing
- [x] `tests/test_connection.py`: Complete unit tests (10 tests)
  - [x] Initialization tests
  - [x] Connection tests with mocks
  - [x] Error handling tests
  - [x] Context manager tests
  - [x] Factory function tests

### Documentation
- [x] `README.md`: Complete project documentation
  - [x] "What We Did" section (Phase 1 accomplishments)
  - [x] "What We Will Do" section (future phases)
  - [x] Installation instructions
  - [x] Usage examples
  - [x] Project structure overview
  - [x] Technical details
  - [x] Troubleshooting guide
- [x] `TODO.md`: This file with checkboxes
- [x] `.github/copilot-instructions.md`: AI agent guidelines
- [x] Comprehensive docstrings in all modules
- [x] Inline code comments explaining protocol

### Verification
- [x] Test connection module: `python tests\test_connection.py` (10/10 passing)
- [x] Verify config.json loads correctly
- [x] Confirm all imports work
- [x] Check README completeness

---

## 📋 Phase 2: Packet Reception, Filtering & Storage ✅ COMPLETE

### Packet Receiver Implementation
- [x] Implement `packet_receiver.py` complete module (445 lines)
  - [x] PacketReceiver class with comprehensive functionality
  - [x] Socket configuration and initialization
  - [x] Continuous receive loop with timeout handling
  - [x] Packet validation logic
    - [x] Size validation (300 or 556 bytes)
    - [x] Leading zeros check (0x00000000)
    - [x] Format ID validation (0x0124=300B, 0x022C=556B, BE)
    - [x] Header structure validation
  - [x] Message type extraction (offset 8-9, LE)
  - [x] Filtering for types 2020/2021
    - [x] Type 2020: Market Picture (0x07E4)
    - [x] Type 2021: Market Picture Complex (0x07E5)
  - [x] Token extraction from records
    - [x] Parse records at offsets: 36, 100, 164, 228, 292, 356
    - [x] Extract LE uint32 token from record start
    - [x] Skip zero tokens (empty slots)
  - [x] Raw packet storage
    - [x] Create `data/raw_packets/` directory
    - [x] Save as timestamped .bin files (YYYYMMDD_HHMMSS_typeXXXX_packet.bin)
    - [x] Error handling for file operations
  - [x] Token metadata storage
    - [x] Create `data/processed_json/` directory
    - [x] Append to `tokens.json` (newline-delimited JSON)
    - [x] Include timestamp, msg_type, packet_size, tokens, source address
  - [x] Storage limit enforcement (configurable, default: 100)
  - [x] Statistics tracking
    - [x] Packets received/valid/invalid
    - [x] Packets by type (2020/2021/other)
    - [x] Tokens extracted
    - [x] Bytes received
    - [x] Errors encountered
  - [x] Comprehensive logging (DEBUG/INFO/ERROR levels)

### Main Application Update
- [x] Update `main.py` to integrate packet receiver
  - [x] Import PacketReceiver class
  - [x] Create receiver instance after connection
  - [x] Pass configuration (storage paths, limits, timeout)
  - [x] Call receiver.receive_loop()
  - [x] Update docstring (Phase 2 description)
  - [x] Update logging messages

### Configuration Update
- [x] Add Phase 2 parameters to `config.json`
  - [x] `store_limit`: Maximum packets to store (default: 100)
  - [x] `timeout`: Socket timeout in seconds (default: 30)
  - [x] Document BSE market hours (9:00 AM - 3:30 PM IST)

### Testing
- [x] `tests/test_packet_receiver.py`: Complete unit tests (15 tests)
  - [x] Packet validation tests (6 tests)
    - [x] Valid 300-byte packet
    - [x] Valid 556-byte packet
    - [x] Invalid size (too small)
    - [x] Invalid size (wrong size)
    - [x] Invalid leading bytes
    - [x] Invalid format ID
  - [x] Message type extraction tests (2 tests)
    - [x] Extract type 2020
    - [x] Extract type 2021
  - [x] Token extraction tests (3 tests)
    - [x] Extract single token
    - [x] Extract multiple tokens
    - [x] Skip zero tokens
  - [x] Packet processing tests (3 tests)
    - [x] Process type 2020 packet
    - [x] Process type 2021 packet
    - [x] Reject invalid packet
  - [x] Storage limit test (1 test)
    - [x] Enforce storage limit

### Documentation Update
- [x] Update `README.md` with Phase 2 accomplishments
  - [x] Add "Phase 2: Packet Reception, Filtering & Storage" section
  - [x] Document packet receiver implementation
  - [x] Update "What We Did" section
  - [x] Update "What We Will Do" for Phase 3
- [x] Update `TODO.md` (this file)
  - [x] Mark Phase 2 tasks as complete
  - [x] Detail Phase 3 tasks

### Verification
- [x] Run packet receiver tests: `python tests\test_packet_receiver.py` (15/15 passing)
- [x] Verify imports work correctly
- [x] Check config.json updates load
- [x] Confirm data directories are created
- [x] Validate storage functionality with mock packets

---

## 📋 Phase 3: Full Packet Decoding & Data Normalization ✅ COMPLETE

### Decoder Implementation
- [x] Implement `decoder.py` BSE packet parser (330 lines)
  - [x] 300-byte format parser
    - [x] Header parsing (36 bytes)
      - [x] Leading zeros validation (offset 0: 0x00000000)
      - [x] Format ID extraction (offset 4: 0x0124=300B, BE)
      - [x] Type field extraction (offset 8: 0x07E4=2020, LE)
      - [x] Timestamp parsing (offset 20-25: HH:MM:SS, BE)
    - [x] Market data record parsing (64-byte intervals)
      - [x] Token extraction (offset +0, 4 bytes, LE)
      - [x] Previous close (offset +8, 4 bytes, BE, paise)
      - [x] LTP (offset +20, 4 bytes, BE, paise)
      - [x] Volume (offset +24, 4 bytes, BE)
      - [x] Return base values for decompression
    - [x] Handle multiple instruments per packet (offsets: 36, 100, 164, 228)
    - [x] Skip empty slots (token=0)
  - [x] 556-byte format parser (6 instruments)
    - [x] Header parsing (same as 300B)
    - [x] Extended record parsing (offsets: 36, 100, 164, 228, 292, 356)
  - [x] Error handling for malformed packets
  - [x] Statistics tracking (parse errors, invalid headers, empty records)
  - [x] Comprehensive logging (DEBUG/INFO/ERROR)

### Decompressor Implementation
- [x] Implement `decompressor.py` for NFCAST differential decompression (280 lines)
  - [x] Differential decompression algorithm
    - [x] Core `_decompress_field()` function
    - [x] Read 2-byte signed diff (Big-Endian)
    - [x] Special value handling:
      - [x] 32767 → read next 4 bytes for actual value
      - [x] ±32766 → end marker (return None)
    - [x] Calculate: actual_value = base_value + diff
  - [x] Decompress OHLC fields (Open/High/Low using LTP as base)
  - [x] Decompress Best 5 bid levels
    - [x] Cascading base values (Level 1 uses LTP, Level 2 uses Level 1, etc.)
    - [x] Price/qty/orders for each level
  - [x] Decompress Best 5 ask levels
    - [x] Same cascading logic as bid levels
  - [x] Price normalization (paise → Rupees, divide by 100)
  - [x] Statistics tracking (fields decompressed, special values, Best 5 levels extracted)
  - [x] Comprehensive logging and error handling

### Data Collector Implementation
- [x] Implement `data_collector.py` for quote normalization (270 lines)
  - [x] Load token database from `data/tokens/token_details.json`
  - [x] Token-to-symbol resolution
    - [x] Lookup token in token_map
    - [x] Build descriptive symbols for options (e.g., SENSEX_CE_86900_20250918)
    - [x] Handle unknown tokens (use "UNKNOWN")
  - [x] Timestamp creation from packet header (HH:MM:SS → ISO format)
  - [x] MarketQuote dictionary creation with fields:
    - [x] token, symbol, timestamp
    - [x] open, high, low, close (LTP)
    - [x] volume, prev_close
    - [x] bid_levels (array of {price, qty, orders})
    - [x] ask_levels (array of {price, qty, orders})
  - [x] Field validation
    - [x] Required fields check (token, ltp, volume)
    - [x] Price ranges (ltp > 0)
    - [x] Volume ranges (volume >= 0)
  - [x] Missing data handling
  - [x] Statistics tracking (quotes collected, unknown tokens, validation errors)

### Saver Implementation
- [x] Implement `saver.py` for data output (290 lines)
  - [x] JSON writer
    - [x] Newline-delimited JSON (one quote per line)
    - [x] File naming with timestamps (YYYYMMDD_quotes.json)
    - [x] Append mode support
    - [x] Create `data/processed_json/` directory
  - [x] CSV writer
    - [x] Header row generation (token, symbol, timestamp, OHLC, volume, bid/ask levels)
    - [x] Best 5 level flattening (bid_prices, bid_qtys, bid_orders, ask_prices, ask_qtys, ask_orders)
    - [x] File naming with timestamps (YYYYMMDD_quotes.csv)
    - [x] Append mode (header written once on creation)
    - [x] Create `data/processed_csv/` directory
  - [x] Convenience method `save_quotes()` for both formats
  - [x] Output directory management (automatic creation)
  - [x] Error handling for I/O operations
  - [x] Statistics tracking (files saved, quotes written, I/O errors)

### Integration
- [x] Update `packet_receiver.py` to use decoder, decompressor, collector, saver
  - [x] Import Phase 3 modules
  - [x] Initialize Phase 3 components in `__init__` with token_map
  - [x] Update `_process_packet()` with Phase 3 pipeline:
    - [x] Decode packet → extract header + base values
    - [x] Decompress records → full OHLC + Best 5
    - [x] Collect quotes → normalize + resolve symbols
    - [x] Save to JSON/CSV
  - [x] Add Phase 3 statistics to stats dict
  - [x] Update `_print_statistics()` to show Phase 3 component stats
  - [x] Maintain backward compatibility with Phase 2 (raw packet storage)
- [x] Update `main.py` to orchestrate Phase 3 pipeline
  - [x] Add `load_token_map()` function
  - [x] Load token_details.json on startup
  - [x] Pass token_map to PacketReceiver initialization
  - [x] Update logging to show Phase 3 status
  - [x] Update docstring (Phase 1-3 complete)
- [x] Error handling for entire pipeline (try/except in _process_packet)
- [x] Comprehensive logging throughout pipeline

### Documentation
- [x] Create `PHASE3_COMPLETE.md` with full details
  - [x] Module descriptions (decoder, decompressor, collector, saver)
  - [x] Data flow diagrams
  - [x] Binary parsing details (mixed endianness)
  - [x] NFCAST differential compression algorithm
  - [x] Statistics tracking examples
  - [x] File output formats (JSON, CSV)
  - [x] Known limitations
  - [x] Next steps (Phase 4-5)

### Testing
- [ ] Complete `tests/test_decoder.py` (PENDING)
  - [ ] Test 300B packet parsing with sample data
  - [ ] Test 556B packet parsing
  - [ ] Test mixed endianness handling
  - [ ] Test multi-instrument packets
  - [ ] Test empty slots (token=0)
  - [ ] Test malformed packets
- [ ] Complete `tests/test_decompressor.py` (PENDING)
  - [ ] Test differential decompression with known values
  - [ ] Test special value handling (32767, ±32766)
  - [ ] Test cascading base values
  - [ ] Test Best 5 bid/ask decompression
  - [ ] Test price normalization (paise → Rupees)
- [ ] Create sample packet files for testing
  - [ ] Generate mock 300B packets with known values
  - [ ] Generate mock 556B packets
  - [ ] Capture real BSE packets (if available)
  - [ ] Create synthetic test packets
- [ ] Integration tests
  - [ ] End-to-end pipeline test
  - [ ] File output verification
  - [ ] Performance benchmarks

### Documentation
- [ ] Update README.md with Phase 3 completion
- [ ] Document packet format details (all 8 fields)
- [ ] Add usage examples for decoder
- [ ] Create troubleshooting guide for parsing errors
- [ ] Update token resolution workflow
- [ ] Document CSV output format

---

## 📋 Phase 4: NFCAST Compression & Advanced Features ⏳ TODO

### Decompression (If Needed)
- [ ] Implement `decompressor.py` NFCAST compression handler
  - [ ] Detect compressed packets (message type 2020/2021)
  - [ ] Read uncompressed base values (LTP, LTQ)
  - [ ] Differential field decompression
    - [ ] Read 2-byte signed differential
    - [ ] Handle special values:
      - [ ] 32767: Read next 4 bytes for actual value
      - [ ] 32766: End of buy side marker
      - [ ] -32766: End of sell side marker
    - [ ] Add differential to base value
  - [ ] Best 5 bid/ask decompression
    - [ ] Level 1 decompression (base = LTP/LTQ)
    - [ ] Level 2+ decompression (cascading bases)
  - [ ] Statistics tracking
- [ ] Integrate decompressor into decoder
- [ ] Test with compressed packet samples

### BOLTPLUS API Integration
- [ ] Create `bse/auth.py` for authentication
  - [ ] Login endpoint implementation
  - [ ] Interactive login (2FA support)
  - [ ] Token management
  - [ ] Session handling
- [ ] Create `bse/api_client.py` for REST API
  - [ ] Market data subscription
  - [ ] Contract master download
  - [ ] Instrument search
  - [ ] Error handling
- [ ] Token database synchronization
  - [ ] Download contract master
  - [ ] Parse and validate
  - [ ] Update `token_details.json`
  - [ ] Incremental updates
- [ ] Multicast group assignment
  - [ ] Subscribe to specific tokens
  - [ ] Handle group changes

### Advanced Features
- [ ] WebSocket streaming interface
  - [ ] Real-time quote streaming
  - [ ] Client subscription management
  - [ ] Backpressure handling
- [ ] Market depth parsing (Best 5 bid/ask)
  - [ ] Order book reconstruction
  - [ ] Depth aggregation
- [ ] Performance optimization
  - [ ] Target: <1ms packet parsing
  - [ ] Profiling and benchmarking
  - [ ] Memory optimization
  - [ ] Multi-threading (if needed)

### Testing
- [ ] Complete `tests/test_decompressor.py`
  - [ ] Test differential decompression
  - [ ] Test special value handling
  - [ ] Test Best 5 decompression
  - [ ] Test cascading bases
- [ ] API integration tests
  - [ ] Authentication flow
  - [ ] Subscription flow
  - [ ] Token database sync
- [ ] Performance tests
  - [ ] Parsing speed benchmarks
  - [ ] Memory usage profiling
  - [ ] Load testing

---

## 📋 Phase 5: Production Readiness & Deployment ⏳ TODO

### Production Features
- [ ] Comprehensive error recovery
  - [ ] Reconnection logic
  - [ ] Packet loss detection
  - [ ] Automatic failover
- [ ] Monitoring and alerting
  - [ ] Prometheus metrics
  - [ ] Health check endpoints
  - [ ] Alert rules
- [ ] Configuration management
  - [ ] Environment-based configs
  - [ ] Secret management
  - [ ] Runtime configuration updates
- [ ] Logging enhancements
  - [ ] Structured logging (JSON)
  - [ ] Log rotation
  - [ ] ELK integration

### Deployment
- [ ] Docker containerization
  - [ ] Dockerfile creation
  - [ ] Multi-stage builds
  - [ ] Image optimization
- [ ] Docker Compose setup
  - [ ] Multi-container orchestration
  - [ ] Network configuration
  - [ ] Volume management
- [ ] Kubernetes manifests (optional)
  - [ ] Deployment specs
  - [ ] Service definitions
  - [ ] ConfigMaps/Secrets
- [ ] CI/CD pipeline
  - [ ] Automated testing
  - [ ] Build automation
  - [ ] Deployment automation

### Testing & Validation
- [ ] Production environment testing
  - [ ] Connect to production multicast (227.0.0.21:12996)
  - [ ] Validate during market hours
  - [ ] Compare with official BSE data
- [ ] Stress testing
  - [ ] High packet rate handling
  - [ ] Memory leak detection
  - [ ] Long-running stability
- [ ] Security audit
  - [ ] Code review
  - [ ] Dependency scanning
  - [ ] Vulnerability assessment

### Documentation
- [ ] Deployment guide
  - [ ] Server requirements
  - [ ] Network configuration
  - [ ] Installation steps
- [ ] Operations manual
  - [ ] Monitoring guide
  - [ ] Troubleshooting procedures
  - [ ] Maintenance tasks
- [ ] API documentation
  - [ ] Endpoint reference
  - [ ] Usage examples
  - [ ] WebSocket protocol
- [ ] Architecture diagrams
  - [ ] Component overview
  - [ ] Data flow diagrams
  - [ ] Network topology

---

## 🎯 Summary

- **Phase 1:** ✅ **COMPLETE** (Structure, Connection, Receive Loop)
- **Phase 2:** ✅ **COMPLETE** (Packet Reception, Filtering, Token Extraction, Storage)
- **Phase 3:** ⏳ **TODO** (Full Packet Decoding, Data Normalization, CSV Output)
- **Phase 4:** ⏳ **TODO** (Decompression, API Integration, Advanced Features)
- **Phase 5:** ⏳ **TODO** (Production Readiness, Deployment)

**Next Step:** Begin Phase 3 implementation with `decoder.py` for full 8-field parsing.

---

**Last Updated:** October 3, 2025  
**Phase 1 Completion Date:** October 3, 2025  
**Phase 2 Completion Date:** October 3, 2025  
**Current Phase:** Phase 3 Planning
