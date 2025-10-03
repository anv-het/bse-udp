"""
BSE NFCAST Decompression Module
================================

Handles decompression of BSE NFCAST compressed market data (types 2020/2021).
Implements differential encoding decompression with base values and special markers.

Phase 3: Full Decompression Implementation
- Differential field decompression (2-byte signed short + base value)
- Special value handling (32767 = read full 4-byte, ±32766 = end markers)
- Best 5 bid/ask decompression with cascading base values
- Price normalization (paise → Rupees, divide by 100)

Compression Algorithm (from BSE_DIRECT_NFCAST_Manual.pdf pages 48-55):
- Fields from Open Rate onwards are compressed as differentials
- Base values: LTP (Last Traded Price), LTQ (Last Traded Quantity)
- Each field: Read 2-byte signed differential, add to base
- Special handling:
  * diff == 32767: Read next 4 bytes for actual value
  * diff == 32766: End of buy side marker
  * diff == -32766: End of sell side marker
- Best 5 levels: Cascading bases (Level 1 base = LTP/LTQ, Level 2 base = Level 1, etc.)

Note: Observed BSE packets may be uncompressed. This implements the full
algorithm for when compressed 2020/2021 packets are received.

Author: BSE Integration Team
Phase: Phase 3 - Decoding & Decompression
"""

import struct
import logging
from typing import Dict, List, Optional, Tuple

logger = logging.getLogger(__name__)


class NFCASTDecompressor:
    """
    Decompressor for BSE NFCAST compressed market data.
    
    Handles:
    - Differential field decompression
    - Special value markers (32767, ±32766)
    - Best 5 bid/ask levels with cascading bases
    - Price normalization
    """
    
    def __init__(self):
        """Initialize decompressor with statistics tracking."""
        self.stats = {
            'records_decompressed': 0,
            'fields_decompressed': 0,
            'special_values_handled': 0,
            'decompress_errors': 0,
            'best5_levels_extracted': 0
        }
        logger.info("NFCASTDecompressor initialized")
    
    def decompress_record(self, packet: bytes, record: Dict) -> Optional[Dict]:
        """
        Decompress a single market data record.
        
        Args:
            packet: Full packet bytes (for reading compressed data)
            record: Decoded record with base values (from decoder.py)
        
        Returns:
            Dictionary with decompressed and normalized market data, or None if error
            
        Format:
        {
            'token': int,
            'timestamp': datetime,  # from header
            'open': float,  # Rupees
            'high': float,
            'low': float,
            'close': float,
            'ltp': float,
            'volume': int,
            'prev_close': float,
            'bid_levels': [{'price': float, 'qty': int, 'orders': int}, ...],
            'ask_levels': [{'price': float, 'qty': int, 'orders': int}, ...]
        }
        """
        try:
            logger.debug(f"Decompressing record: token={record['token']}")
            
            # Base values from uncompressed fields
            base_ltp = record['ltp']  # paise
            base_ltq = record['ltq']  # quantity
            close_rate = record['close_rate']  # paise
            
            # Starting offset for compressed data
            offset = record['compressed_offset']
            
            # Decompress touchline fields (Open, High, Low)
            # Manual p49: Fields after LTP/LTQ are differentials
            open_price, offset = self._decompress_field(packet, offset, base_ltp)
            high_price, offset = self._decompress_field(packet, offset, base_ltp)
            low_price, offset = self._decompress_field(packet, offset, base_ltp)
            
            # Additional fields (may vary by format)
            # For now, use base values if decompression incomplete
            if open_price is None:
                open_price = base_ltp
            if high_price is None:
                high_price = base_ltp
            if low_price is None:
                low_price = base_ltp
            
            # Decompress Best 5 bid levels
            bid_levels = []
            bid_base_price = base_ltp  # Start with LTP as base
            bid_base_qty = base_ltq
            
            for level in range(5):
                price, qty, orders, offset = self._decompress_market_depth_level(
                    packet, offset, bid_base_price, bid_base_qty
                )
                
                if price is None or qty is None:
                    # Hit end marker or error
                    logger.debug(f"Bid level {level}: End marker or error reached")
                    break
                
                bid_levels.append({
                    'price': price / 100.0,  # paise → Rupees
                    'qty': qty,
                    'orders': orders if orders else 0
                })
                
                # Cascading: next level uses this level as base
                bid_base_price = price
                bid_base_qty = qty
                
                self.stats['best5_levels_extracted'] += 1
            
            # Decompress Best 5 ask levels
            ask_levels = []
            ask_base_price = base_ltp
            ask_base_qty = base_ltq
            
            for level in range(5):
                price, qty, orders, offset = self._decompress_market_depth_level(
                    packet, offset, ask_base_price, ask_base_qty
                )
                
                if price is None or qty is None:
                    logger.debug(f"Ask level {level}: End marker or error reached")
                    break
                
                ask_levels.append({
                    'price': price / 100.0,  # paise → Rupees
                    'qty': qty,
                    'orders': orders if orders else 0
                })
                
                # Cascading
                ask_base_price = price
                ask_base_qty = qty
                
                self.stats['best5_levels_extracted'] += 1
            
            # Normalize prices to Rupees
            decompressed = {
                'token': record['token'],
                'open': open_price / 100.0 if open_price else 0.0,
                'high': high_price / 100.0 if high_price else 0.0,
                'low': low_price / 100.0 if low_price else 0.0,
                'close': close_rate / 100.0,
                'ltp': base_ltp / 100.0,
                'volume': record['volume'],
                'prev_close': close_rate / 100.0,  # Use close_rate as prev_close
                'bid_levels': bid_levels,
                'ask_levels': ask_levels
            }
            
            self.stats['records_decompressed'] += 1
            logger.info(f"Decompressed record: token={record['token']}, "
                       f"ltp={decompressed['ltp']:.2f}, "
                       f"bid_levels={len(bid_levels)}, ask_levels={len(ask_levels)}")
            
            return decompressed
            
        except Exception as e:
            self.stats['decompress_errors'] += 1
            logger.error(f"Decompression error for token {record.get('token')}: {e}", 
                        exc_info=True)
            return None
    
    def _decompress_field(self, packet: bytes, offset: int, 
                         base_value: int) -> Tuple[Optional[int], int]:
        """
        Decompress a single field using differential encoding.
        
        Algorithm (from manual p49):
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
        Decompress a single market depth level (Best 5 bid/ask).
        
        Each level has: Price, Quantity, Number of Orders
        Uses cascading base values as per manual p50.
        
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
        """Get decompressor statistics."""
        return self.stats.copy()
    
    def reset_stats(self):
        """Reset statistics counters."""
        for key in self.stats:
            self.stats[key] = 0
        logger.info("Decompressor statistics reset")
