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
            
            # Parse records at FIXED 264-byte intervals
            # CONFIRMED: Each record slot is exactly 264 bytes (null-padded if smaller)
            # Pattern: 36 (header) + N×264 (records) = total packet size
            records = []
            record_slot_size = 264
            
            logger.debug(f"Packet {packet_size}B → {num_records} records at 264-byte intervals")
            
            for record_index in range(num_records):
                # Calculate record offset (fixed 264-byte slots)
                record_offset = 36 + (record_index * record_slot_size)
                
                # Read exactly 264 bytes for this record slot
                record_bytes = packet[record_offset:record_offset + record_slot_size]
                
                if len(record_bytes) < record_slot_size:
                    logger.warning(f"Record {record_index} incomplete: {len(record_bytes)}/{record_slot_size} bytes")
                    break
                
                # Parse the record (will handle null padding internally)
                record = self._parse_record(record_bytes, record_offset)
                if record:
                    records.append(record)
                    self.stats['records_decoded'] += 1
                    logger.debug(f"Decoded record {record_index}: token={record['token']}, "
                               f"ltp={record['ltp']}, offset={record_offset}")
                else:
                    self.stats['empty_records'] += 1
            
            self.stats['packets_decoded'] += 1
            # logger.info(f"Successfully decoded packet: {len(records)} records extracted")
            
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
            
            # Timestamp (offsets 20-25, Big-Endian uint16 each - as per BSE header spec)
            hour = struct.unpack('>H', packet[20:22])[0]
            minute = struct.unpack('>H', packet[22:24])[0]
            second = struct.unpack('>H', packet[24:26])[0]
            
            # Validate timestamp values before using them
            if hour > 23 or minute > 59 or second > 59:
                logger.warning(f"Invalid timestamp values: {hour:02d}:{minute:02d}:{second:02d} - using current time")
                timestamp = datetime.now()
            else:
                # Create timestamp with current date, parsed time, and current microseconds
                # Note: BSE header doesn't include milliseconds, so we use system time microseconds
                now = datetime.now()
                timestamp = now.replace(hour=hour, minute=minute, second=second)
            logger.debug(f"Timestamp: {timestamp.strftime('%H:%M:%S.%f')[:-3]}")  # Show milliseconds
            
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
        Parse market data record with uncompressed fields.
        
        Record Structure (CONFIRMED from live data analysis):
        
        RECORDS ARE VARIABLE LENGTH:
        - Full records (with order book depth): 264 bytes
        - Minimal records (no depth): 64-108 bytes
        - Pattern: 36 (header) + N×264 (records) = 300, 564, 828, 1092, 1356, 1620
        - Max 6 records per message type 2020
        
        ✅ CONFIRMED OFFSETS (from live market data):
        Offset 0-3:   Token (uint32 LE) ✓ CONFIRMED
        Offset 4-7:   Open Price (int32 LE, paise) ✓ CONFIRMED
        Offset 8-11:  Previous Close (int32 LE, paise) ✓ CONFIRMED
        Offset 12-15: High Price (int32 LE, paise) ✓ CONFIRMED
        Offset 16-19: Low Price (int32 LE, paise) ✓ CONFIRMED
        Offset 20-23: Unknown Field (int32 LE) ❓ NOT close price!
        Offset 24-27: Volume (int32 LE) ✓ CONFIRMED
        Offset 28-31: Turnover in Lakhs (uint32 LE) ✓ CONFIRMED - Traded Value / 100,000
        Offset 32-35: Lot Size (uint32 LE) ✓ CONFIRMED - Contract lot size
        Offset 36-39: LTP - Last Traded Price (int32 LE, paise) ✓ CONFIRMED
        Offset 40-43: Unknown Field (uint32 LE) ❓ Always zero
        Offset 44-47: Market Sequence Number (uint32 LE) ✓ CONFIRMED - Increments by 1 per tick
        Offset 84-87: ATP - Average Traded Price (int32 LE, paise) ✓ CONFIRMED
        Offset 104-107: Best Bid Price (int32 LE, paise) ✓ CONFIRMED - Also Order Book Bid Level 1
        Offset 104-263: 5-Level Order Book (160 bytes) ✓ CONFIRMED - Interleaved Bid/Ask
        
        All prices are in paise (divide by 100 for rupees).
        All integer fields use Little-Endian byte order.
        """
        # Minimum record size based on your confirmed fields
        # We need at least 40 bytes to read LTP at offset 36-39
        if len(record_bytes) < 40:
            logger.warning(f"Record too short: {len(record_bytes)} < 40 bytes (minimum for LTP)")
            return None
        
        try:
            # Token (offset 0-3, Little-Endian ⚠️ - ONLY field that's Little-Endian!)
            token = struct.unpack('<I', record_bytes[0:4])[0]
            
            # Skip empty slots (token = 0)
            if token == 0:
                logger.debug(f"Empty record slot at offset {offset}")
                return None
            
            logger.debug(f"Raw record bytes (first 70): {record_bytes[:70].hex()}")
            
            # Parse uncompressed fields - CONFIRMED structure (99% sure)
            # Field offsets verified from live market data analysis:
            # Offset +4:  Open Price - 4 bytes, Little-Endian ✓ CONFIRMED
            # Offset +8:  Prev Close - 4 bytes, Little-Endian ✓ CONFIRMED
            # Offset +12: High Price - 4 bytes, Little-Endian ✓ CONFIRMED
            # Offset +16: Low Price - 4 bytes, Little-Endian ✓ CONFIRMED
            # LTP location: UNKNOWN - still needs to be identified
            # All prices are in paise (divide by 100 for rupees)
            
            # Offset +4: Open Price (4 bytes, Little-Endian) ✓ CONFIRMED
            open_price = struct.unpack('<i', record_bytes[4:8])[0]
            
            # Offset +8: Prev Close Price (4 bytes, Little-Endian) ✓ CONFIRMED
            prev_close = struct.unpack('<i', record_bytes[8:12])[0]
            
            # Offset +12: High Price (4 bytes, Little-Endian) ✓ CONFIRMED
            high_price = struct.unpack('<i', record_bytes[12:16])[0]
            
            # Offset +16: Low Price (4 bytes, Little-Endian) ✓ CONFIRMED
            low_price = struct.unpack('<i', record_bytes[16:20])[0]
            
            # Offset +20: Field 20-23 (4 bytes, Little-Endian) - ⚠️ NOT Close Price!
            # Live data shows values like ₹15.33 when LTP is ₹84,535 - clearly wrong
            # Possible interpretations: Change from prev close? Points? Different field?
            # TODO: Investigate actual meaning of this field
            field_20_23 = struct.unpack('<i', record_bytes[20:24])[0]
            
            # Offset +24: Volume (4 bytes, int32 Little-Endian) ✓ CONFIRMED
            volume = struct.unpack('<i', record_bytes[24:28])[0]
            
            # Offset +28: Total Turnover in Lakhs ✓ CONFIRMED
            # Total Turnover = Traded Value / 1,00,000 (standard F&O field)
            # Example: 8,728 lakhs = ₹872,800,000 traded value
            turnover_lakhs = struct.unpack('<I', record_bytes[28:32])[0] if len(record_bytes) >= 32 else None
            
            # Offset +32: Lot Size ✓ CONFIRMED
            # Contract lot size (e.g., 20 for Sensex futures)
            # Used to calculate actual quantity in contracts
            lot_size = struct.unpack('<I', record_bytes[32:36])[0] if len(record_bytes) >= 36 else None
            
            # Offset +36: LTP - Last Traded Price (4 bytes, Little-Endian) ✓ CONFIRMED
            ltp = struct.unpack('<i', record_bytes[36:40])[0]
            
            # UNKNOWN FIELDS 40-47 (for manual identification)
            # Offset +40: Always zero so far - flags/padding?
            unknown_40_43 = struct.unpack('<I', record_bytes[40:44])[0] if len(record_bytes) >= 44 else None
            
            # Offset +44: Market Sequence Number ✓ CONFIRMED
            # Packet/market sequence number - increments by 1 with each tick
            # Critical for detecting missing/out-of-order UDP packets
            # Used to maintain data integrity in UDP multicast stream
            sequence_number = struct.unpack('<I', record_bytes[44:48])[0] if len(record_bytes) >= 48 else None
            
            # Optional fields (only present in longer records):
            # Offset +84: ATP - Average Traded Price (4 bytes, Little-Endian) ✓ CONFIRMED
            atp = struct.unpack('<i', record_bytes[84:88])[0] if len(record_bytes) >= 88 else 0
            
            # Offset +104: Best Bid Price ✓ CONFIRMED
            # This is also the first level of order book (Bid Level 1)
            bid_price = struct.unpack('<i', record_bytes[104:108])[0] if len(record_bytes) >= 108 else 0
            
            # 📊 ORDER BOOK PARSING - ✅ STRUCTURE DISCOVERED!
            # Order book uses 16-byte blocks per level (5 bids + 5 asks)
            # Block structure: [Price 4B][Qty 4B][Flag 4B][Unknown 4B]
            # BID/ASK INTERLEAVED: Bid1, Ask1, Bid2, Ask2, Bid3, Ask3, Bid4, Ask4, Bid5, Ask5
            # Starts at offset 104 (Bid1 price = Best Bid)
            # Total: 160 bytes for 10 levels (5 bid + 5 ask)
            order_book = None
            if len(record_bytes) >= 264:
                order_book = self._parse_order_book(record_bytes)
            
            # Fields still to find:
            ltq = 0  # Last Traded Quantity - Location TBD
            num_trades = 0  # Number of Trades - Location TBD
            
            # Record length determines what fields are available
            record_length = len(record_bytes)
            has_depth = record_length >= 264  # Full order book depth
            
            # Best ask from order book (first ask level price)
            ask_price = 0
            if order_book and order_book['asks']:
                ask_price = int(order_book['asks'][0]['price'] * 100)  # Convert back to paise
            
            # Compression starts after uncompressed section
            compressed_offset = offset + record_length
            
            logger.debug(f"Parsed record: token={token}, len={record_length}B, "
                       f"ltp={ltp} (Rs.{ltp/100:.2f}), "
                       f"open={open_price}, high={high_price}, low={low_price}, prev_close={prev_close}, "
                       f"volume={volume}, turnover={turnover_lakhs} lakhs, lot_size={lot_size}, "
                       f"atp={atp}, bid={bid_price}, ask={ask_price}, seq={sequence_number}, "
                       f"has_depth={has_depth}, order_book={bool(order_book)}")
            
            return {
                'token': token,
                'num_trades': num_trades,  # Location TBD
                'volume': volume,  # ✓ CONFIRMED (offset 24-27)
                'field_20_23': field_20_23,  # paise - Unknown field (NOT close price!)
                'prev_close': prev_close,  # paise - Previous close ✓ CONFIRMED
                'ltq': ltq,  # Location TBD
                'ltp': ltp,  # paise - Last Traded Price ✓ CONFIRMED (offset 36-39)
                'atp': atp,  # paise - Average Traded Price ✓ CONFIRMED (offset 84-87)
                'bid': bid_price,  # paise - Best Bid Price ✓ CONFIRMED (offset 104-107)
                'ask': ask_price,  # paise - Best Ask (from order book first level)
                'open': open_price,  # paise - Open price ✓ CONFIRMED
                'high': high_price,  # paise - High price ✓ CONFIRMED
                'low': low_price,   # paise - Low price ✓ CONFIRMED
                'order_book': order_book,  # ✅ 5-level bid/ask depth (offsets 104-263)
                'compressed_offset': compressed_offset,
                # ✅ CONFIRMED FIELDS:
                'turnover_lakhs': turnover_lakhs,  # ✓ Total turnover in lakhs (offset 28-31)
                'lot_size': lot_size,  # ✓ Contract lot size (offset 32-35)
                'sequence_number': sequence_number,  # ✓ Market sequence number (offset 44-47)
                # ❓ UNKNOWN FIELDS:
                'unknown_40_43': unknown_40_43,  # Always zero?
            }
            
        except struct.error as e:
            logger.error(f"Record parsing error at offset {offset}: {e}")
            return None
    
    def _parse_order_book(self, record_bytes: bytes) -> Optional[Dict]:
        """
        Parse 5-level order book depth (bid and ask).
        
        ✅ STRUCTURE CONFIRMED from live data analysis:
        - Starts at offset 104 (NOT 108!)
        - INTERLEAVED structure: Bid1, Ask1, Bid2, Ask2, Bid3, Ask3, Bid4, Ask4, Bid5, Ask5
        - Each bid/ask uses 16-byte block: [Price 4B][Qty 4B][Flag 4B][Unknown 4B]
        - Total: 5 levels × 32 bytes/level = 160 bytes (offset 104-263)
        
        Block structure:
        Bytes 0-3:   Price (int32 LE) in paise
        Bytes 4-7:   Quantity (int32 LE)
        Bytes 8-11:  Flag (int32 LE) - usually 1
        Bytes 12-15: Unknown (int32 LE) - usually 0
        
        Interleaved pattern per level:
        - Bid block (16 bytes)
        - Ask block (16 bytes)
        = 32 bytes per level
        
        Args:
            record_bytes: Full record bytes (must be ≥264 bytes)
        
        Returns:
            Dictionary with 'bids' and 'asks' arrays, or None if invalid
        """
        if len(record_bytes) < 264:
            logger.debug(f"Record too short for order book: {len(record_bytes)} < 264 bytes")
            return None
        
        try:
            order_book = {
                'bids': [],
                'asks': []
            }
            
            # Parse 5 levels (interleaved bid/ask pairs)
            for i in range(5):
                # Each level occupies 32 bytes (16 bid + 16 ask)
                bid_base = 104 + (i * 32)  # Bid block
                ask_base = bid_base + 16    # Ask block immediately follows
                
                # Parse BID (offset pattern: Price, Qty, Flag, Unknown)
                bid_price = struct.unpack('<i', record_bytes[bid_base:bid_base+4])[0]
                bid_qty = struct.unpack('<i', record_bytes[bid_base+4:bid_base+8])[0]
                bid_flag = struct.unpack('<i', record_bytes[bid_base+8:bid_base+12])[0]
                bid_unknown = struct.unpack('<i', record_bytes[bid_base+12:bid_base+16])[0]
                
                # Parse ASK (offset pattern: Price, Qty, Flag, Unknown)
                ask_price = struct.unpack('<i', record_bytes[ask_base:ask_base+4])[0]
                ask_qty = struct.unpack('<i', record_bytes[ask_base+4:ask_base+8])[0]
                ask_flag = struct.unpack('<i', record_bytes[ask_base+8:ask_base+12])[0]
                ask_unknown = struct.unpack('<i', record_bytes[ask_base+12:ask_base+16])[0]
                
                # Validate bid (skip if invalid)
                if bid_qty > 0 and bid_price > 0:
                    order_book['bids'].append({
                        'price': bid_price / 100.0,  # Convert paise to rupees
                        'quantity': bid_qty,
                        'flag': bid_flag,  # Unknown purpose (usually 1)
                        'unknown': bid_unknown  # Usually 0, sometimes other values
                    })
                else:
                    logger.debug(f"Invalid bid level {i+1}: qty={bid_qty}, price={bid_price}")
                
                # Validate ask (skip if invalid)
                if ask_qty > 0 and ask_price > 0:
                    order_book['asks'].append({
                        'price': ask_price / 100.0,  # Convert paise to rupees
                        'quantity': ask_qty,
                        'flag': ask_flag,  # Unknown purpose (usually 1)
                        'unknown': ask_unknown  # Usually 0, sometimes other values
                    })
                else:
                    logger.debug(f"Invalid ask level {i+1}: qty={ask_qty}, price={ask_price}")
            
            logger.debug(f"Parsed order book: {len(order_book['bids'])} bids, "
                        f"{len(order_book['asks'])} asks")
            
            return order_book
            
        except struct.error as e:
            logger.error(f"Order book parsing error: {e}")
            return None
    
    def _get_num_records(self, packet_size: int) -> int:
        """
        Determine number of records based on packet size.
        
        CONFIRMED PATTERN (from empirical analysis):
        - Each record occupies EXACTLY 264 bytes (fixed slot size)
        - Actual data varies (64-264 bytes), rest is NULL-padded
        - Pattern: 36 (header) + N×264 (records)
        
        Packet sizes and record counts:
        - 300 bytes = 36 + 1×264 → 1 record
        - 564 bytes = 36 + 2×264 → 2 records
        - 828 bytes = 36 + 3×264 → 3 records
        - 1092 bytes = 36 + 4×264 → 4 records
        - 1356 bytes = 36 + 5×264 → 5 records
        - 1620 bytes = 36 + 6×264 → 6 records (max for msg type 2020)
        """
        header_size = 36
        record_slot_size = 264  # Fixed slot size per record
        
        if packet_size < header_size:
            logger.warning(f"Packet size {packet_size} < header size {header_size}")
            return 0
        
        data_size = packet_size - header_size
        num_records = data_size // record_slot_size
        
        logger.debug(f"Packet {packet_size}B → {num_records} records "
                   f"(36 header + {num_records}×264 slots)")
        
        return num_records
    
    def get_stats(self) -> Dict:
        """Get decoder statistics."""
        return self.stats.copy()
    
    def reset_stats(self):
        """Reset statistics counters."""
        for key in self.stats:
            self.stats[key] = 0
        logger.info("Decoder statistics reset")
