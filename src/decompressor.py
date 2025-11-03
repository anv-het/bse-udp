"""
BSE NFCAST Data Normalizer Module
==================================

Normalizes BSE NFCAST market data from decoder (types 2020/2021).
Converts prices from paise to Rupees and structures data for the collector.

IMPORTANT DISCOVERY:
- BSE production feed (239.1.2.5:26002) sends UNCOMPRESSED data!
- The decoder already extracts all fields correctly (confirmed with live market data)
- This module acts as a pass-through normalizer, not a decompressor
- Original compression algorithm from manual (pages 48-55) is NOT used in current feed

Phase 3: Data Normalization
- Convert prices from paise → Rupees (divide by 100)
- Structure order book levels (5 bid + 5 ask)
- Pass through volume, sequence numbers, and other metadata
- Maintain statistics for monitoring

Validated with Live Data (Token 873830 - Sensex Nov 2025 Futures):
- LTP: ₹84,530.00 (confirmed accurate)
- Volume: 10,960 contracts
- OHLC prices: All correct
- Best 5 bid/ask levels: Full order book depth working
- Sequence numbers: Incrementing by 1 per tick

Author: BSE Integration Team
Phase: Phase 3 - Decoding & Normalization
"""

import struct
import logging
from typing import Dict, List, Optional, Tuple
import time

logger = logging.getLogger(__name__)


class NFCASTDecompressor:
    """
    Normalizer for BSE NFCAST market data (renamed from Decompressor).
    
    IMPORTANT: BSE production feed sends UNCOMPRESSED data!
    This class normalizes already-decoded data from decoder.py:
    - Converts prices from paise → Rupees (divide by 100)
    - Structures order book levels
    - Passes through metadata (volume, sequence numbers, etc.)
    
    The original compression algorithm from the manual is NOT used in the
    current production feed (239.1.2.5:26002). All data comes pre-decoded.
    """
    
    def __init__(self):
        """Initialize normalizer with statistics tracking."""
        self.stats = {
            'records_decompressed': 0,  # Keep name for backward compatibility
            'fields_decompressed': 0,   # No longer used
            'special_values_handled': 0,  # No longer used
            'decompress_errors': 0,
            'best5_levels_extracted': 0
        }
        logger.info("NFCASTDecompressor initialized (normalizer mode - data is NOT compressed)")
    
    def decompress_record(self, packet: bytes, record: Dict) -> Optional[Dict]:
        """
        Process a single market data record from the decoder.
        
        NOTE: BSE packets are NOT compressed in the current feed!
        The decoder already extracts all fields correctly (confirmed with live data).
        This method now acts as a pass-through normalizer, converting paise to Rupees
        and structuring the data for the collector.
        
        Args:
            packet: Full packet bytes (kept for compatibility, not used)
            record: Fully decoded record from decoder.py with all fields
        
        Returns:
            Dictionary with normalized market data, or None if error
            
        Format:
        {
            'token': int,
            'open': float,  # Rupees
            'high': float,
            'low': float,
            'close': float,
            'ltp': float,
            'volume': int,
            'prev_close': float,
            'atp': float,  # Average Traded Price
            'bid': float,  # Best bid price
            'ask': float,  # Best ask price
            'turnover_lakhs': int,
            'lot_size': int,
            'sequence_number': int,
            'bid_levels': [{'price': float, 'qty': int, 'flag': int}, ...],  # 5 levels
            'ask_levels': [{'price': float, 'qty': int, 'flag': int}, ...]   # 5 levels
        }
        """
        try:
            logger.debug(f"Processing record: token={record['token']}")
            
            # Extract order book (already parsed by decoder)
            order_book = record.get('order_book')
            bid_levels = []
            ask_levels = []
            
            if order_book:
                # Order book is already in correct format from decoder
                bid_levels = order_book.get('bids', [])
                ask_levels = order_book.get('asks', [])
                logger.debug(f"Order book: {len(bid_levels)} bids, {len(ask_levels)} asks")
            
            # All fields are already decoded and in paise - just normalize to Rupees
            normalized = {
                'token': record['token'],
                'open': record['open'] / 100.0,      # paise → Rupees
                'high': record['high'] / 100.0,      # paise → Rupees
                'low': record['low'] / 100.0,        # paise → Rupees
                'close': record['prev_close'] / 100.0,  # paise → Rupees (use prev_close as close)
                'ltp': record['ltp'] / 100.0,        # paise → Rupees
                'volume': record['volume'],           # Already in correct units
                'prev_close': record['prev_close'] / 100.0,  # paise → Rupees
                'atp': record.get('atp', 0) / 100.0 if record.get('atp') else 0.0,  # paise → Rupees
                'bid': record.get('bid', 0) / 100.0 if record.get('bid') else 0.0,  # paise → Rupees
                'ask': record.get('ask', 0) / 100.0 if record.get('ask') else 0.0,  # paise → Rupees
                'bid_levels': bid_levels,  # Already in Rupees from decoder
                'ask_levels': ask_levels,  # Already in Rupees from decoder
                # Additional fields
                'turnover_lakhs': record.get('turnover_lakhs', 0),
                'lot_size': record.get('lot_size', 0),
                'sequence_number': record.get('sequence_number', 0)
            }
            
            self.stats['records_decompressed'] += 1
            self.stats['best5_levels_extracted'] += len(bid_levels) + len(ask_levels)
            
            logger.info(f"Normalized record: token={record['token']}, "
                       f"ltp=₹{normalized['ltp']:.2f}, volume={normalized['volume']:,}, "
                       f"bid_levels={len(bid_levels)}, ask_levels={len(ask_levels)}")
            
            return normalized
            
        except Exception as e:
            self.stats['decompress_errors'] += 1
            logger.error(f"Normalization error for token {record.get('token')}: {e}", 
                        exc_info=True)
            return None
    
    def _decompress_field(self, packet: bytes, offset: int, 
                         base_value: int) -> Tuple[Optional[int], int]:
        """
        LEGACY METHOD - No longer used in production.
        
        BSE production feed sends uncompressed data. This method implemented
        the differential compression algorithm from the manual (pages 48-55),
        but it's NOT needed for the current feed.
        
        Kept for:
        1. Historical reference
        2. Potential future use if BSE enables compression
        3. Understanding the protocol specification
        
        Original Algorithm (from manual p49):
        1. Read 2-byte signed short differential
        2. If diff == 32767: Read next 4 bytes for actual value
        3. If diff == ±32766: End marker (return None)
        4. Otherwise: value = base + diff
        
        Args:
            packet: Full packet bytes
            offset: Current read offset
            base_value: Base value to add differential to
        
        Returns:
            (decompressed_value, new_offset) or (None, new_offset) if end marker/error
        """
        try:
            # Check bounds
            if offset + 2 > len(packet):
                logger.debug(f"Offset {offset} + 2 exceeds packet size {len(packet)}")
                return None, offset
            
            # Read 2-byte signed differential (Big-Endian)
            diff = struct.unpack('>h', packet[offset:offset+2])[0]
            offset += 2
            
            logger.debug(f"Diff at offset {offset-2}: {diff}")
            
            # Handle special values
            if diff == 32767:
                # Read full 4-byte value
                if offset + 4 > len(packet):
                    logger.warning(f"Cannot read 4-byte value at offset {offset}")
                    return None, offset
                
                value = struct.unpack('>I', packet[offset:offset+4])[0]
                offset += 4
                self.stats['special_values_handled'] += 1
                logger.debug(f"Special 32767: Read full value {value}")
                return value, offset
            
            elif diff == 32766 or diff == -32766:
                # End marker
                self.stats['special_values_handled'] += 1
                logger.debug(f"End marker: {diff}")
                return None, offset
            
            else:
                # Normal differential
                value = base_value + diff
                self.stats['fields_decompressed'] += 1
                logger.debug(f"Normal diff: base={base_value} + diff={diff} = {value}")
                return value, offset
                
        except struct.error as e:
            logger.error(f"Struct error at offset {offset}: {e}")
            return None, offset
    
    def _decompress_market_depth_level(self, packet: bytes, offset: int,
                                      base_price: int, base_qty: int) -> Tuple:
        """
        LEGACY METHOD - No longer used in production.
        
        BSE production feed sends uncompressed order book data. The decoder
        already parses the full 5-level order book correctly.
        
        Kept for:
        1. Historical reference
        2. Potential future use if BSE enables compression
        3. Understanding the protocol specification
        
        Original Algorithm (from manual p50):
        Each level: Price, Quantity, Number of Orders
        Uses cascading base values
        
        Returns:
            (price, qty, orders, new_offset) or (None, None, None, offset) if end
        """
        try:
            # Decompress price
            price, offset = self._decompress_field(packet, offset, base_price)
            if price is None:
                return None, None, None, offset
            
            # Decompress quantity
            qty, offset = self._decompress_field(packet, offset, base_qty)
            if qty is None:
                return None, None, None, offset
            
            # Decompress number of orders (if available)
            # May not always be present - handle gracefully
            orders, offset = self._decompress_field(packet, offset, 0)
            # If orders is None, just set to 0
            if orders is None:
                orders = 0
            
            logger.debug(f"Depth level: price={price}, qty={qty}, orders={orders}")
            return price, qty, orders, offset
            
        except Exception as e:
            logger.error(f"Error decompressing depth level at offset {offset}: {e}")
            return None, None, None, offset
    
    def get_stats(self) -> Dict:
        """Get normalizer statistics (kept as 'decompressor' for backward compatibility)."""
        return self.stats.copy()
    
    def reset_stats(self):
        """Reset statistics counters."""
        for key in self.stats:
            self.stats[key] = 0
        logger.info("Normalizer statistics reset")
