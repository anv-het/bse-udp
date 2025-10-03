"""
BSE Packet Decoder Module
==========================

Decodes BSE NFCAST packets (300-byte and 556-byte formats).
Parses packet headers and uncompressed market data fields.

Phase 3: Full Packet Decoding
- Parse header (36 bytes): leading zeros, format ID, message type, timestamp
- Parse market data records (64 bytes each): token and uncompressed fields
- Extract: token, num_trades, volume, close_rate, LTQ, LTP (uncompressed base values)
- Returns structured data for decompressor to process compressed fields

Protocol Details (from BSE_Final_Analysis_Report.md):
- Leading zeros: 0x00000000 at offset 0-3
- Format ID: 0x0124 (300B) or 0x022C (556B) at offset 4-5 (Big-Endian)
- Message type: 2020/2021 at offset 8-9 (Little-Endian)
- Timestamp: HH:MM:SS at offsets 20-25 (3x uint16 Big-Endian)
- Records: Start at offset 36, every 64 bytes, up to 6 instruments
- Token: Little-Endian uint32 at record start
- Uncompressed fields: Big-Endian int32 (prices in paise)

Author: BSE Integration Team
Phase: Phase 3 - Decoding & Decompression
"""

import struct
import logging
from typing import Dict, List, Optional
from datetime import datetime

logger = logging.getLogger(__name__)


class PacketDecoder:
    """
    Decoder for BSE NFCAST packet formats.
    
    Handles:
    - 300-byte format (4 instruments max)
    - 556-byte format (6 instruments max)
    - Header parsing with mixed endianness
    - Uncompressed field extraction
    """
    
    def __init__(self):
        """Initialize decoder with statistics tracking."""
        self.stats = {
            'packets_decoded': 0,
            'decode_errors': 0,
            'invalid_headers': 0,
            'empty_records': 0,
            'records_decoded': 0
        }
        logger.info("PacketDecoder initialized")
    
    def decode_packet(self, packet: bytes) -> Optional[Dict]:
        """
        Decode a BSE packet into structured data.
        
        Args:
            packet: Raw packet bytes (300 or 556 bytes)
        
        Returns:
            Dictionary with 'header' and 'records' lists, or None if invalid
            
        Format:
        {
            'header': {
                'format_id': int,
                'msg_type': int,
                'timestamp': datetime,
                'packet_size': int
            },
            'records': [
                {
                    'token': int,
                    'num_trades': int,
                    'volume': int,
                    'close_rate': int,  # paise
                    'ltq': int,
                    'ltp': int,  # paise (base for decompression)
                    'compressed_offset': int  # where compressed data starts
                },
                ...
            ]
        }
        """
        try:
            packet_size = len(packet)
            logger.debug(f"Decoding packet: size={packet_size} bytes")
            
            # Parse header (36 bytes)
            header = self._parse_header(packet)
            if not header:
                self.stats['invalid_headers'] += 1
                logger.warning(f"Invalid header in packet (size={packet_size})")
                return None
            
            header['packet_size'] = packet_size
            
            # Determine number of records based on packet size
            num_records = self._get_num_records(packet_size)
            logger.debug(f"Expected {num_records} records for {packet_size}B packet")
            
            # Parse records starting at offset 36
            records = []
            for i in range(num_records):
                offset = 36 + (i * 64)
                if offset + 64 > packet_size:
                    logger.debug(f"Record {i} would exceed packet size, stopping")
                    break
                
                record = self._parse_record(packet[offset:offset+64], offset)
                if record:
                    records.append(record)
                    self.stats['records_decoded'] += 1
                    logger.debug(f"Decoded record {i}: token={record['token']}, "
                               f"ltp={record['ltp']}")
                else:
                    self.stats['empty_records'] += 1
            
            self.stats['packets_decoded'] += 1
            logger.info(f"Successfully decoded packet: {len(records)} records extracted")
            
            return {
                'header': header,
                'records': records
            }
            
        except Exception as e:
            self.stats['decode_errors'] += 1
            logger.error(f"Decode error: {e}", exc_info=True)
            return None
    
    def _parse_header(self, packet: bytes) -> Optional[Dict]:
        """
        Parse 36-byte packet header.
        
        Header Structure (from BSE_Final_Analysis_Report.md):
        Offset 0-3:   Leading zeros (0x00000000)
        Offset 4-5:   Format ID (0x0124=300B, 0x022C=556B) Big-Endian
        Offset 8-9:   Message type (2020/2021) Little-Endian ⚠️
        Offset 20-21: Hour (Big-Endian uint16)
        Offset 22-23: Minute (Big-Endian uint16)
        Offset 24-25: Second (Big-Endian uint16)
        """
        if len(packet) < 36:
            logger.error("Packet too short for header (<36 bytes)")
            return None
        
        try:
            # Check leading zeros (offset 0-3)
            leading = struct.unpack('>I', packet[0:4])[0]  # Big-Endian
            if leading != 0x00000000:
                logger.warning(f"Leading zeros check failed: {leading:#010x}")
            
            # Format ID (offset 4-5, Big-Endian)
            format_id = struct.unpack('>H', packet[4:6])[0]
            logger.debug(f"Format ID: {format_id:#06x} ({format_id})")
            
            # Message type (offset 8-9, Little-Endian ⚠️)
            msg_type = struct.unpack('<H', packet[8:10])[0]
            logger.debug(f"Message type: {msg_type} (0x{msg_type:04x})")
            
            # Timestamp (offsets 20-25, Big-Endian uint16 each)
            hour = struct.unpack('>H', packet[20:22])[0]
            minute = struct.unpack('>H', packet[22:24])[0]
            second = struct.unpack('>H', packet[24:26])[0]
            
            # Create timestamp with current date and parsed time
            now = datetime.now()
            timestamp = now.replace(hour=hour, minute=minute, second=second, 
                                  microsecond=0)
            logger.debug(f"Timestamp: {timestamp.strftime('%H:%M:%S')}")
            
            return {
                'format_id': format_id,
                'msg_type': msg_type,
                'timestamp': timestamp
            }
            
        except struct.error as e:
            logger.error(f"Header parsing struct error: {e}")
            return None
    
    def _parse_record(self, record_bytes: bytes, offset: int) -> Optional[Dict]:
        """
        Parse 64-byte market data record (uncompressed fields only).
        
        Record Structure (from manual pages 48-55):
        Offset 0-3:   Token (Little-Endian uint32) ⚠️
        Offset 8-11:  Prev Close (Big-Endian int32, paise)
        Offset 20-23: LTP (Big-Endian int32, paise) - Uncompressed base value
        Offset 24-27: Volume (Big-Endian int32)
        
        Note: For compressed packets (2020/2021), fields from Open onwards
        are stored as differentials after the base fields.
        This function extracts the UNCOMPRESSED base values. Decompressor handles diffs.
        """
        if len(record_bytes) < 64:
            logger.warning(f"Record too short: {len(record_bytes)} < 64 bytes")
            return None
        
        try:
            # Token (offset 0-3, Little-Endian ⚠️)
            token = struct.unpack('<I', record_bytes[0:4])[0]
            
            # Skip empty slots (token = 0)
            if token == 0:
                logger.debug(f"Empty record slot at offset {offset}")
                return None
            
            # Parse uncompressed fields (Big-Endian int32, in paise)
            prev_close = struct.unpack('>i', record_bytes[8:12])[0]  # paise
            ltp = struct.unpack('>i', record_bytes[20:24])[0]  # paise (BASE)
            volume = struct.unpack('>i', record_bytes[24:28])[0]
            
            # Placeholder values
            num_trades = 0
            ltq = 0
            close_rate = prev_close
            compressed_offset = offset + 28
            
            logger.debug(f"Parsed record: token={token}, ltp={ltp} paise, "
                       f"volume={volume}, close_rate={close_rate}")
            
            return {
                'token': token,
                'num_trades': num_trades,
                'volume': volume,
                'close_rate': close_rate,  # paise
                'ltq': ltq,
                'ltp': ltp,  # paise (BASE for decompression)
                'compressed_offset': compressed_offset
            }
            
        except struct.error as e:
            logger.error(f"Record parsing error at offset {offset}: {e}")
            return None
    
    def _get_num_records(self, packet_size: int) -> int:
        """Determine number of records based on packet size."""
        if packet_size == 300:
            return 4
        elif packet_size == 556:
            return 6
        else:
            return max(0, (packet_size - 36) // 64)
    
    def get_stats(self) -> Dict:
        """Get decoder statistics."""
        return self.stats.copy()
    
    def reset_stats(self):
        """Reset statistics counters."""
        for key in self.stats:
            self.stats[key] = 0
        logger.info("Decoder statistics reset")
