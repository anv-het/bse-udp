"""
BSE Packet Receiver Module
===========================

Phase 2 + Phase 3 Implementation: Receives raw UDP packets from BSE NFCAST feed,
validates, filters, decodes, decompresses, normalizes, and saves market data.

Key Features:
- Continuous packet reception with configurable timeout
- Packet validation (size, header structure, message type)
- Token extraction from market data records
- **Phase 3: Full packet decoding (header + base values)**
- **Phase 3: NFCAST differential decompression (Open/High/Low + Best 5)**
- **Phase 3: Quote normalization and symbol resolution**
- **Phase 3: JSON/CSV output with timestamped files**
- Raw packet storage (.bin files)
- Token metadata storage (JSON)
- Comprehensive statistics tracking

Packet Structure (from BSE_Final_Analysis_Report.md):
- Leading zeros: 0x00000000 (bytes 0-3)
- Format ID: 0x0124 (300B) or 0x022C (556B) at offset 4-5 (Big-Endian)
- Type field: 0x07E4 (2020) or 0x07E5 (2021) at offset 8-9 (Little-Endian)
- Market data records: Start at offset 36, every 64 bytes
- Token: First 4 bytes of each record (Little-Endian uint32)

Phase 3 Pipeline:
1. Validate packet → 2. Decode header + records → 3. Decompress differentials →
4. Collect quotes (normalize + resolve symbols) → 5. Save to JSON/CSV

BSE Market Hours: 9:00 AM - 3:30 PM IST (Mon-Fri)
"""

import struct
import logging
import socket
import json
from datetime import datetime
from pathlib import Path
from typing import List, Dict, Optional, Tuple

# Phase 3 imports
from decoder import PacketDecoder
from decompressor import NFCASTDecompressor
from data_collector import MarketDataCollector
from saver import DataSaver

logger = logging.getLogger(__name__)


class PacketReceiver:
    """
    Receives and processes UDP packets from BSE NFCAST multicast feed.
    
    Responsibilities:
    - Continuous packet reception loop
    - Packet validation and filtering for types 2020/2021
    - Token extraction from market data records
    - Storage of raw packets and extracted metadata
    - Statistics tracking and logging
    """
    
    # Message types we're interested in (from BSE NFCAST Manual page 22)
    MSG_TYPE_MARKET_PICTURE = 2020  # 0x07E4 in Little-Endian
    MSG_TYPE_MARKET_PICTURE_COMPLEX = 2021  # 0x07E5 in Little-Endian
    
    # Packet format identifiers (Big-Endian at offset 4-5)
    FORMAT_300B = 0x0124  # 300-byte format
    FORMAT_556B = 0x022C  # 556-byte format
    
    # Expected packet sizes
    VALID_PACKET_SIZES = [300, 556]
    
    # Header constants
    HEADER_SIZE = 36
    RECORD_SIZE = 64
    RECORD_OFFSETS = [36, 100, 164, 228, 292, 356]  # Up to 6 instruments per packet
    
    def __init__(self, sock: socket.socket, config: dict, token_map: Dict[str, dict]):
        """
        Initialize packet receiver with Phase 3 components.
        
        Args:
            sock: Connected UDP multicast socket
            config: Configuration dictionary with storage paths and limits
            token_map: Dictionary mapping token IDs to contract details (for symbol resolution)
        """
        self.socket = sock
        self.config = config
        
        # Setup storage directories
        self.raw_packets_dir = Path(config.get('raw_packets_dir', 'data/raw_packets'))
        self.processed_json_dir = Path(config.get('processed_json_dir', 'data/processed_json'))
        
        self.raw_packets_dir.mkdir(parents=True, exist_ok=True)
        self.processed_json_dir.mkdir(parents=True, exist_ok=True)
        
        # Storage limits
        self.store_limit = config.get('store_limit', 100)
        self.stored_count = 0
        
        # Statistics
        self.stats = {
            'packets_received': 0,
            'packets_valid': 0,
            'packets_invalid': 0,
            'packets_2020': 0,
            'packets_2021': 0,
            'packets_other': 0,
            'tokens_extracted': 0,
            'bytes_received': 0,
            'errors': 0,
            # Phase 3 stats
            'packets_decoded': 0,
            'packets_decompressed': 0,
            'quotes_collected': 0,
            'quotes_saved': 0
        }
        
        # Tokens storage file
        self.tokens_file = self.processed_json_dir / 'tokens.json'
        
        # Phase 3: Initialize decoder, decompressor, collector, saver
        self.decoder = PacketDecoder()
        self.decompressor = NFCASTDecompressor()
        self.collector = MarketDataCollector(token_map)
        self.saver = DataSaver(output_dir='data')
        
        logger.info(f"PacketReceiver initialized - storing up to {self.store_limit} packets")
        logger.info(f"Raw packets: {self.raw_packets_dir}")
        logger.info(f"Processed JSON: {self.processed_json_dir}")
        logger.info(f"Phase 3 pipeline enabled: decode → decompress → collect → save")
        logger.info(f"Token map loaded: {len(token_map)} tokens")
    
    def receive_loop(self, max_packets: Optional[int] = None):
        """
        Main packet reception loop.
        
        Continuously receives packets from UDP socket, validates them,
        filters for message types 2020/2021, extracts tokens, and stores data.
        
        Args:
            max_packets: Optional limit on number of packets to receive (for testing)
        
        The loop continues until:
        - max_packets reached (if specified)
        - Keyboard interrupt (Ctrl+C)
        - Fatal socket error
        """
        logger.info("Starting packet reception loop...")
        logger.info("BSE Market Hours: 9:00 AM - 3:30 PM IST (Mon-Fri)")
        logger.info("Press Ctrl+C to stop")
        
        try:
            while True:
                # Check if we've reached the storage limit
                if max_packets and self.stats['packets_received'] >= max_packets:
                    logger.info(f"Reached max packets limit: {max_packets}")
                    break
                
                try:
                    # Receive packet (2000-byte buffer as per BSE specification)
                    # Timeout is set on the socket from configuration
                    packet, addr = self.socket.recvfrom(2000)
                    
                    self.stats['packets_received'] += 1
                    self.stats['bytes_received'] += len(packet)
                    
                    # Log every 10th packet to avoid flooding logs
                    if self.stats['packets_received'] % 10 == 0:
                        logger.info(
                            f"Packets received: {self.stats['packets_received']}, "
                            f"Valid: {self.stats['packets_valid']}, "
                            f"2020: {self.stats['packets_2020']}, "
                            f"2021: {self.stats['packets_2021']}"
                        )
                    
                    # Process the packet
                    self._process_packet(packet, addr)
                
                except socket.timeout:
                    # Timeout waiting for packet - this is normal
                    logger.debug("Socket timeout - waiting for packets...")
                    logger.info(
                        f"No packets received for {self.config.get('timeout', 30)}s. "
                        "Check: Market hours (9AM-3:30PM IST), network connectivity, IGMPv2 enabled"
                    )
                    continue
                
                except socket.error as e:
                    self.stats['errors'] += 1
                    logger.error(f"Socket error: {e}")
                    # On fatal errors, break the loop
                    if e.errno in [9, 10038]:  # Bad file descriptor, socket not open
                        logger.error("Fatal socket error - stopping receiver")
                        break
                    continue
        
        except KeyboardInterrupt:
            logger.info("Received keyboard interrupt - stopping receiver")
        
        finally:
            self._print_statistics()
    
    def _process_packet(self, packet: bytes, addr: Tuple[str, int]):
        """
        Process a received packet: validate, filter, extract tokens, decode, decompress, and save.
        
        Phase 2 + Phase 3 Pipeline:
        1. Validate packet structure
        2. Filter for message types 2020/2021
        3. Extract tokens (Phase 2 compatibility)
        4. **Decode packet → extract header + base values (Phase 3)**
        5. **Decompress differentials → full OHLC + Best 5 (Phase 3)**
        6. **Collect quotes → normalize + resolve symbols (Phase 3)**
        7. **Save to JSON/CSV (Phase 3)**
        8. Store raw packet + metadata (Phase 2 compatibility)
        
        Args:
            packet: Raw packet bytes
            addr: Source address tuple (ip, port)
        """
        # Validate packet
        if not self._validate_packet(packet):
            self.stats['packets_invalid'] += 1
            return
        
        # Extract message type
        msg_type = self._extract_message_type(packet)
        
        # Filter for message types 2020 or 2021
        if msg_type == self.MSG_TYPE_MARKET_PICTURE:
            self.stats['packets_2020'] += 1
        elif msg_type == self.MSG_TYPE_MARKET_PICTURE_COMPLEX:
            self.stats['packets_2021'] += 1
        else:
            self.stats['packets_other'] += 1
            logger.debug(f"Ignoring packet with message type: {msg_type}")
            return
        
        self.stats['packets_valid'] += 1
        
        # Extract tokens from the packet (Phase 2 - for metadata storage)
        tokens = self._extract_tokens(packet)
        
        if tokens:
            self.stats['tokens_extracted'] += len(tokens)
            logger.debug(f"Extracted {len(tokens)} tokens from packet: {tokens}")
        
        # ========== PHASE 3 PIPELINE ==========
        try:
            # Step 1: Decode packet → extract header + base values
            decoded_data = self.decoder.decode_packet(packet)
            if not decoded_data or not decoded_data.get('records'):
                logger.debug("Packet decoded but no records found")
                return
            
            self.stats['packets_decoded'] += 1
            logger.debug(f"Decoded {len(decoded_data['records'])} records from packet")
            
            # Step 2: Decompress each record → full OHLC + Best 5
            decompressed_records = []
            for record in decoded_data['records']:
                decompressed = self.decompressor.decompress_record(packet, record)
                if decompressed:
                    decompressed_records.append(decompressed)
            
            if not decompressed_records:
                logger.debug("No records successfully decompressed")
                return
            
            self.stats['packets_decompressed'] += 1
            logger.debug(f"Decompressed {len(decompressed_records)} records")
            
            # Step 3: Collect quotes → normalize + resolve symbols
            quotes = self.collector.collect_quotes(decoded_data['header'], decompressed_records)
            if not quotes:
                logger.debug("No quotes collected from decompressed records")
                return
            
            self.stats['quotes_collected'] += len(quotes)
            logger.debug(f"Collected {len(quotes)} normalized quotes")
            
            # Step 4: Save to JSON/CSV
            save_success = self.saver.save_quotes(quotes, save_json=True, save_csv=True)
            if save_success:
                self.stats['quotes_saved'] += len(quotes)
                logger.info(f"✓ Phase 3 pipeline complete: {len(quotes)} quotes saved")
            else:
                logger.warning("Failed to save some quotes")
        
        except Exception as e:
            logger.error(f"Error in Phase 3 pipeline: {e}", exc_info=True)
            self.stats['errors'] += 1
        # ========== END PHASE 3 PIPELINE ==========
        
        # Store packet if under limit (Phase 2 compatibility)
        if self.stored_count < self.store_limit:
            self._store_packet(packet, msg_type, tokens, addr)
            self.stored_count += 1
        elif self.stored_count == self.store_limit:
            logger.warning(f"Reached storage limit of {self.store_limit} packets - no longer storing")
            self.stored_count += 1  # Increment to avoid logging again
    
    def _validate_packet(self, packet: bytes) -> bool:
        """
        Validate packet structure.
        
        Checks:
        - Minimum size (at least header size)
        - Expected packet size (300 or 556 bytes)
        - Leading zeros (first 4 bytes should be 0x00000000)
        - Format ID validity
        
        Args:
            packet: Raw packet bytes
        
        Returns:
            True if packet is valid, False otherwise
        """
        # Check minimum size
        if len(packet) < self.HEADER_SIZE:
            logger.debug(f"Packet too small: {len(packet)} bytes (need at least {self.HEADER_SIZE})")
            return False
        
        # Check expected size (300 or 556 bytes)
        if len(packet) not in self.VALID_PACKET_SIZES:
            logger.debug(f"Unexpected packet size: {len(packet)} bytes (expected 300 or 556)")
            return False
        
        # Check leading zeros (offset 0-3 should be 0x00000000)
        # This is a key identifier from BSE_Final_Analysis_Report.md
        leading_bytes = packet[0:4]
        if leading_bytes != b'\x00\x00\x00\x00':
            logger.debug(f"Invalid leading bytes: {leading_bytes.hex()} (expected 00000000)")
            return False
        
        # Check format ID (offset 4-5, Big-Endian)
        format_id = struct.unpack('>H', packet[4:6])[0]
        if format_id not in [self.FORMAT_300B, self.FORMAT_556B]:
            logger.debug(f"Unknown format ID: 0x{format_id:04X}")
            return False
        
        return True
    
    def _extract_message_type(self, packet: bytes) -> int:
        """
        Extract message type from packet header.
        
        Message type is at offset 8-9 (2 bytes, Little-Endian).
        Note: This is LITTLE-ENDIAN despite most of the packet being Big-Endian.
        
        Args:
            packet: Raw packet bytes
        
        Returns:
            Message type as integer (e.g., 2020, 2021)
        """
        # Extract type field at offset 8-9 (Little-Endian uint16)
        # This is 0x07E4 (2020) or 0x07E5 (2021)
        msg_type = struct.unpack('<H', packet[8:10])[0]
        return msg_type
    
    def _extract_tokens(self, packet: bytes) -> List[int]:
        """
        Extract token IDs from market data records.
        
        Market data records start at offset 36 and repeat every 64 bytes.
        Each record starts with a 4-byte token ID (Little-Endian uint32).
        Token ID of 0 indicates an empty slot.
        
        Args:
            packet: Raw packet bytes
        
        Returns:
            List of valid (non-zero) token IDs
        """
        tokens = []
        
        # Parse records at fixed offsets
        for offset in self.RECORD_OFFSETS:
            # Check if we have enough data for this record
            if offset + 4 > len(packet):
                break
            
            # Extract token (Little-Endian uint32)
            token = struct.unpack('<I', packet[offset:offset+4])[0]
            
            # Token 0 indicates empty slot - skip it
            if token != 0:
                tokens.append(token)
        
        return tokens
    
    def _store_packet(self, packet: bytes, msg_type: int, tokens: List[int], addr: Tuple[str, int]):
        """
        Store raw packet and extracted metadata.
        
        Stores:
        1. Raw packet as .bin file in data/raw_packets/
        2. Token metadata as JSON entry in data/processed_json/tokens.json
        
        Args:
            packet: Raw packet bytes
            msg_type: Message type (2020 or 2021)
            tokens: List of extracted token IDs
            addr: Source address (ip, port)
        """
        timestamp = datetime.now()
        
        # Generate filenames with timestamp
        timestamp_str = timestamp.strftime('%Y%m%d_%H%M%S_%f')
        
        # Store raw packet as .bin file
        raw_filename = f"{timestamp_str}_type{msg_type}_packet.bin"
        raw_path = self.raw_packets_dir / raw_filename
        
        try:
            with open(raw_path, 'wb') as f:
                f.write(packet)
            logger.debug(f"Stored raw packet: {raw_filename}")
        except Exception as e:
            logger.error(f"Failed to store raw packet: {e}")
            self.stats['errors'] += 1
        
        # Store token metadata as JSON
        metadata = {
            'timestamp': timestamp.isoformat(),
            'msg_type': msg_type,
            'packet_size': len(packet),
            'tokens': tokens,
            'source_ip': addr[0],
            'source_port': addr[1],
            'raw_file': raw_filename
        }
        
        try:
            # Append to tokens.json file
            with open(self.tokens_file, 'a') as f:
                json.dump(metadata, f)
                f.write('\n')  # Newline-delimited JSON for easy parsing
            
            logger.info(f"Stored packet metadata: {len(tokens)} tokens, type {msg_type}")
        except Exception as e:
            logger.error(f"Failed to store token metadata: {e}")
            self.stats['errors'] += 1
    
    def _print_statistics(self):
        """Print comprehensive statistics about received packets and Phase 3 pipeline."""
        logger.info("=" * 70)
        logger.info("PACKET RECEIVER STATISTICS (Phase 2 + Phase 3)")
        logger.info("=" * 70)
        logger.info(f"Packets Received:     {self.stats['packets_received']:,}")
        logger.info(f"Packets Valid:        {self.stats['packets_valid']:,}")
        logger.info(f"Packets Invalid:      {self.stats['packets_invalid']:,}")
        logger.info(f"Packets Type 2020:    {self.stats['packets_2020']:,}")
        logger.info(f"Packets Type 2021:    {self.stats['packets_2021']:,}")
        logger.info(f"Packets Other Types:  {self.stats['packets_other']:,}")
        logger.info(f"Tokens Extracted:     {self.stats['tokens_extracted']:,}")
        logger.info(f"Bytes Received:       {self.stats['bytes_received']:,}")
        logger.info("-" * 70)
        logger.info("Phase 3 Pipeline:")
        logger.info(f"Packets Decoded:      {self.stats['packets_decoded']:,}")
        logger.info(f"Packets Decompressed: {self.stats['packets_decompressed']:,}")
        logger.info(f"Quotes Collected:     {self.stats['quotes_collected']:,}")
        logger.info(f"Quotes Saved:         {self.stats['quotes_saved']:,}")
        logger.info("-" * 70)
        logger.info(f"Errors:               {self.stats['errors']:,}")
        logger.info(f"Packets Stored (raw): {min(self.stored_count, self.store_limit):,}")
        logger.info("=" * 70)
        
        # Print Phase 3 component statistics
        logger.info("\nPhase 3 Component Statistics:")
        logger.info("-" * 70)
        
        decoder_stats = self.decoder.get_stats()
        logger.info("Decoder:")
        for key, value in decoder_stats.items():
            logger.info(f"  {key}: {value:,}")
        
        decompressor_stats = self.decompressor.get_stats()
        logger.info("Decompressor:")
        for key, value in decompressor_stats.items():
            logger.info(f"  {key}: {value:,}")
        
        collector_stats = self.collector.get_stats()
        logger.info("Data Collector:")
        for key, value in collector_stats.items():
            logger.info(f"  {key}: {value:,}")
        
        saver_stats = self.saver.get_stats()
        logger.info("Data Saver:")
        for key, value in saver_stats.items():
            logger.info(f"  {key}: {value:,}")
        
        logger.info("=" * 70)


# For testing the module independently
if __name__ == '__main__':
    print("PacketReceiver module - Phase 2 implementation")
    print("Use via main.py for full functionality")
