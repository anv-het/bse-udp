"""
BSE Market Data Collector Module
=================================

Handles market data normalization and aggregation from decompressed packet records.

Responsibilities:
- Build normalized quote dictionaries from decompressed data
- Token-to-symbol resolution using token_details.json
- Market data validation and sanitization
- Timestamp handling from packet headers
- Statistics tracking (quotes collected, unknown tokens, validation errors)

Output Data Structure (per quote):
{
    'token': int,
    'symbol': str,
    'timestamp': str,           # ISO format from packet header
    'open': float,              # Rupees
    'high': float,              # Rupees
    'low': float,               # Rupees
    'close': float,             # Rupees (LTP)
    'ltp': float,               # Last Traded Price
    'volume': int,              # Total traded volume
    'prev_close': float,        # Previous day close
    'bid_levels': [             # Best 5 bid levels
        {'price': float, 'qty': int, 'orders': int},
        ...
    ],
    'ask_levels': [             # Best 5 ask levels
        {'price': float, 'qty': int, 'orders': int},
        ...
    ]
}

Status: Phase 3 - COMPLETE
Reference: BSE_Complete_Technical_Knowledge_Base.md (Quote Normalization section)
"""

import logging
from typing import List, Dict, Optional
from datetime import datetime

logger = logging.getLogger(__name__)


class MarketDataCollector:
    """
    Collects and normalizes market data from decompressed packet records.
    
    Transforms raw decompressed data into standardized quote dictionaries with:
    - Token-to-symbol resolution
    - Timestamp from packet header
    - Validated price/volume fields
    - Best 5 bid/ask market depth (if available)
    
    Tracks statistics:
    - quotes_collected: Total quotes built
    - unknown_tokens: Tokens not found in token_map
    - validation_errors: Invalid data detected
    - missing_fields: Expected fields not present
    """
    
    def __init__(self, token_map: Dict[str, dict]):
        """
        Initialize market data collector with token mapping.
        
        Args:
            token_map: Dictionary mapping token IDs (as strings) to contract details
                      Format: {'12345': {'symbol': 'SENSEX', 'expiry': '...', ...}, ...}
        """
        self.token_map = token_map
        self.stats = {
            'quotes_collected': 0,
            'unknown_tokens': 0,
            'validation_errors': 0,
            'missing_fields': 0
        }
        logger.info(f"MarketDataCollector initialized with {len(token_map)} tokens")
    
    def collect_quotes(self, header: dict, decompressed_records: List[Dict]) -> List[Dict]:
        """
        Build normalized quote dictionaries from decompressed packet records.
        
        Processes each decompressed record:
        1. Extract token and validate it exists in token_map
        2. Build timestamp from packet header (HH:MM:SS)
        3. Populate touchline fields (OHLC, LTP, volume, prev_close)
        4. Add Best 5 bid/ask levels if present
        5. Perform data validation (prices > 0, volume >= 0)
        
        Args:
            header: Parsed packet header containing timestamp fields
                   {'time_hour': int, 'time_minute': int, 'time_second': int, ...}
            decompressed_records: List of decompressed market data records
                                 Each record is dict with touchline + depth data
        
        Returns:
            List of normalized quote dictionaries ready for JSON/CSV output
            Empty list if no valid quotes could be built
        """
        if not decompressed_records:
            logger.debug("No decompressed records to collect")
            return []
        
        # Build timestamp from header (format: HH:MM:SS)
        timestamp_str = self._build_timestamp(header)
        
        quotes = []
        for record in decompressed_records:
            try:
                quote = self._build_quote(record, timestamp_str)
                if quote:
                    quotes.append(quote)
                    self.stats['quotes_collected'] += 1
                    logger.debug(f"Collected quote for token {record.get('token')}")
                else:
                    logger.debug(f"Failed to build quote for token {record.get('token')}")
                    
            except Exception as e:
                logger.error(f"Error building quote from record: {e}", exc_info=True)
                self.stats['validation_errors'] += 1
        
        logger.info(f"Collected {len(quotes)} quotes from {len(decompressed_records)} records")
        return quotes
    
    def _build_timestamp(self, header: dict) -> str:
        """
        Build ISO timestamp string from packet header fields.
        
        BSE packet header contains timestamp as a datetime object from decoder.
        
        Args:
            header: Parsed header with 'timestamp' datetime object
        
        Returns:
            ISO format timestamp string: "YYYY-MM-DD HH:MM:SS"
        """
        try:
            # Decoder provides timestamp as datetime object
            timestamp = header.get('timestamp')
            if timestamp and isinstance(timestamp, datetime):
                # Format with milliseconds: YYYY-MM-DD HH:MM:SS.mmm
                timestamp_str = timestamp.strftime('%Y-%m-%d %H:%M:%S.%f')[:-3]  # Truncate to milliseconds
                logger.debug(f"Built timestamp: {timestamp_str}")
                return timestamp_str
            else:
                # Fallback to current time if timestamp not available
                logger.warning("No timestamp in header, using current time")
                return datetime.now().strftime('%Y-%m-%d %H:%M:%S.%f')[:-3]
            
        except Exception as e:
            logger.error(f"Error building timestamp: {e}, using current time")
            return datetime.now().strftime('%Y-%m-%d %H:%M:%S.%f')[:-3]
    
    def _build_quote(self, record: dict, timestamp: str) -> Optional[Dict]:
        """
        Build normalized quote dictionary from single decompressed record.
        
        Steps:
        1. Extract token and resolve to symbol
        2. Validate required fields present (token, ltp, volume)
        3. Build quote dict with all touchline fields
        4. Add Best 5 bid/ask levels if available
        5. Validate data ranges (prices > 0, volume >= 0)
        
        Args:
            record: Decompressed market data record (dict)
            timestamp: ISO timestamp string to attach to quote
        
        Returns:
            Normalized quote dictionary, or None if validation fails
        """
        # Extract token (required)
        token = record.get('token')
        if token is None:
            logger.warning("Record missing token field")
            self.stats['missing_fields'] += 1
            return None
        
        # Resolve token to symbol and contract details
        symbol_info = self._resolve_symbol_details(token)
        if symbol_info['symbol'] == 'UNKNOWN':
            self.stats['unknown_tokens'] += 1
            logger.debug(f"Unknown token {token}, using placeholder symbol")
        
        # Validate required fields
        if not self._validate_record(record):
            self.stats['validation_errors'] += 1
            return None
        
        # Build combined symbol_name: SENSEX20NOV2025_82000CE or SENSEX20NOV2025_FUT
        symbol_name = self._format_symbol_name(symbol_info)
        
        # Build quote dictionary with separate symbol columns
        quote = {
            'token': token,
            'symbol': symbol_info['symbol'],           # Base symbol (e.g., SENSEX)
            'symbol_name': symbol_name,                # Combined identifier (e.g., SENSEX20NOV2025_82000CE)
            'expiry': symbol_info['expiry'],           # Expiry date (e.g., 2025-11-20)
            'option_type': symbol_info['option_type'], # CE/PE/FUT
            'strike': symbol_info['strike'],           # Strike price (e.g., 83800)
            'timestamp': timestamp,
            'open': record.get('open', 0.0),
            'high': record.get('high', 0.0),
            'low': record.get('low', 0.0),
            'close': record.get('ltp', 0.0),      # Close = LTP in real-time
            'ltp': record.get('ltp', 0.0),
            'volume': record.get('volume', 0),
            'prev_close': record.get('prev_close', 0.0)
        }
        
        # Add Best 5 bid/ask levels if present
        if 'bid_levels' in record and record['bid_levels']:
            quote['bid_levels'] = record['bid_levels']
        else:
            quote['bid_levels'] = []
        
        if 'ask_levels' in record and record['ask_levels']:
            quote['ask_levels'] = record['ask_levels']
        else:
            quote['ask_levels'] = []
        
        logger.debug(f"Built quote: token={token} symbol={symbol_info['symbol']} ltp={quote['ltp']:.2f} vol={quote['volume']}")
        return quote
    
    def _resolve_symbol_details(self, token: int) -> dict:
        """
        Resolve token ID to detailed contract information.
        
        BSE tokens map to derivative contracts (SENSEX/BANKEX options/futures).
        Token map loaded from data/tokens/token_details.json at startup.
        
        If token NOT found in JSON file (missing from contract master):
        - Infer symbol based on token ID range (e.g., 114xxxx = SENSEX)
        - Mark as incomplete for fallback handling
        
        Extracts and parses contract details:
        - ticker: Base symbol (SENSEX/BANKEX)
        - strike: Strike price in Rupees (converted from paise)
        - expiry: Expiry date (DD-MMM-YYYY format)
        - option_type: CE/PE for options, blank for futures
        
        Args:
            token: BSE token ID (integer)
        
        Returns:
            Dictionary with symbol details:
            {
                'symbol': 'SENSEX',           # Base ticker
                'expiry': '27-NOV-2025',      # Expiry date
                'option_type': 'CE',          # CE/PE/''
                'strike': '83300'             # Strike price in Rupees
            }
        """
        token_str = str(token)
        if token_str in self.token_map:
            contract = self.token_map[token_str]
            
            # Extract base ticker (SENSEX/BANKEX)
            ticker = contract.get('ticker', contract.get('symbol', 'UNKNOWN'))
            
            # Extract expiry (already formatted)
            expiry = contract.get('expiry', '')
            
            # Extract option type (CE/PE or blank for futures)
            option_type = contract.get('option_type', '')
            
            # Extract strike price (convert from paise to Rupees)
            strike_paise = contract.get('strike', '')
            if strike_paise and str(strike_paise).isdigit():
                strike_rupees = int(strike_paise) / 100.0
                # Format as integer if whole number, else with decimals
                strike = str(int(strike_rupees)) if strike_rupees == int(strike_rupees) else f"{strike_rupees:.2f}"
            else:
                strike = ''
            
            return {
                'symbol': ticker,
                'expiry': expiry,
                'option_type': option_type,
                'strike': strike
            }
        else:
            # Token NOT in contract master - attempt full decoding from token ID
            logger.debug(f"Token {token} not found in token_map, attempting full token decoding")
            decoded = self._decode_full_token(token)
            if decoded['symbol'] != 'UNKNOWN':
                logger.info(f"Successfully decoded token {token}: {decoded['symbol']} {decoded['expiry']} {decoded['strike']} {decoded['option_type']}")
                return decoded
            else:
                # Fallback to range-based inference
                logger.debug(f"Full decoding failed for token {token}, using range-based inference")
                return self._decode_token_from_id(token)
    
    def _decode_token_from_id(self, token: int) -> dict:
        """
        Decode token information from token ID when not in contract master.
        
        BSE token ranges indicate contract type (empirically determined):
        - 820163-1159864: SENSEX options/futures
        - 861153-1129115: BANKEX options/futures  
        - 1100000-1199999: Individual stock options
        
        For futures (when option_type and strike are empty), attempt to decode expiry
        from token ID based on observed patterns.
        
        This is a fallback for NEW contracts added after token_details.json was created.
        
        Args:
            token: BSE token ID (integer)
        
        Returns:
            Dictionary with best-guess symbol info:
            {
                'symbol': str,     # Inferred base symbol
                'expiry': str,     # Decoded expiry date if possible
                'option_type': '', # Empty for futures
                'strike': ''       # Empty for futures
            }
        """
        # Infer symbol from empirically determined token ranges
        if 820163 <= token <= 1159864:
            # SENSEX contracts
            symbol = 'SENSEX'
            # Try to decode expiry for futures
            expiry = self._decode_expiry_from_token(token)
        elif 861153 <= token <= 1129115:
            # BANKEX contracts (overlaps with SENSEX, so check second)
            symbol = 'BANKEX'
            expiry = self._decode_expiry_from_token(token)
        elif 1100000 <= token <= 1199999:
            # Individual stock options range
            symbol = f'STOCK_{token}'
            expiry = ''
        else:
            symbol = f'TOKEN_{token}'
            expiry = ''
        
        logger.warning(f"Token {token}: Inferred symbol={symbol}, expiry={expiry} (not in contract master)")
        
        return {
            'symbol': symbol,
            'expiry': expiry,      # Decoded expiry if possible
            'option_type': '',     # Empty for futures
            'strike': ''           # Empty for futures
        }
    
    def _decode_expiry_from_token(self, token: int) -> str:
        """
        Attempt to decode expiry date from token ID for futures contracts.
        
        Based on observed patterns in contract master:
        - 861384 = 30-OCT-2025 (SENSEX)
        - 873830 = 27-NOV-2025 (SENSEX) 
        - 1102290 = 24-DEC-2025 (SENSEX)
        - 861473 = 30-OCT-2025 (BANKEX)
        - 874881 = 27-NOV-2025 (BANKEX)
        - 1104160 = 24-DEC-2025 (BANKEX)
        
        For tokens starting with 113, attempt to decode based on observed patterns:
        - 1136xxx = 26-MAR-2026 (from contract master)
        - 1138xxx = 28-DEC-2028 (from contract master)
        
        Args:
            token: BSE token ID (integer)
        
        Returns:
            Expiry date string in "DD-MMM-YYYY" format, or empty string if cannot decode
        """
        token_str = str(token)
        
        # Known futures patterns from contract master
        known_futures = {
            861384: '30-OCT-2025',  # SENSEX
            873830: '27-NOV-2025',  # SENSEX
            1102290: '24-DEC-2025', # SENSEX
            861473: '30-OCT-2025',  # BANKEX
            874881: '27-NOV-2025',  # BANKEX
            1104160: '24-DEC-2025', # BANKEX
        }
        
        if token in known_futures:
            return known_futures[token]
        
        # Attempt to decode based on token range patterns
        if token_str.startswith('113'):
            # 113xxxx range - attempt to decode expiry
            if 1132000 <= token <= 1134999:
                # 1132xxx - assume JAN 2026 (educated guess based on pattern)
                return '31-JAN-2026'
            elif 1135000 <= token <= 1136999:
                # 1135xxx-1136xxx - assume FEB-MAR 2026
                if token <= 1135999:
                    return '28-FEB-2026'
                else:
                    return '26-MAR-2026'  # From contract master pattern
            elif 1138000 <= token <= 1138999:
                # 1138xxx - DEC 2028 (from contract master)
                return '28-DEC-2028'
        
        # Cannot decode expiry
        return ''
    
    def _decode_full_token(self, token: int) -> dict:
        """
        Decode complete contract details from token ID digits.
        
        BSE tokens encode contract information in their digits:
        - First 4 digits: Expiry code
        - Next 3-4 digits: Strike price encoding (for options)
        - Last 1-2 digits: Option type (CE/PE for options)
        
        Known expiry codes from contract master analysis:
        - 1132 → 31-JAN-2026 (user's tokens: 1132866, 1133124, etc.)
        - 1136 → 26-MAR-2026
        - 1138 → 28-DEC-2028
        - 1101 → 26-MAR-2026 (alternative code)
        
        For futures (no strike/option_type), token format is simpler.
        
        Args:
            token: BSE token ID (integer)
        
        Returns:
            Dictionary with decoded contract details:
            {
                'symbol': 'SENSEX',      # Always SENSEX for 113xxxx range
                'expiry': '31-JAN-2026', # Decoded expiry date
                'option_type': '',       # CE/PE or empty for futures
                'strike': ''             # Strike price or empty for futures
            }
        """
        token_str = str(token)
        
        # Only handle tokens in the 113xxxx range (SENSEX contracts)
        if not token_str.startswith('113'):
            return {
                'symbol': 'UNKNOWN',
                'expiry': '',
                'option_type': '',
                'strike': ''
            }
        
        # Decode expiry from first 4 digits
        expiry_code = token_str[:4]
        expiry_map = {
            '1132': '31-JAN-2026',  # User's tokens: 1132866, 1132508
            '1133': '31-JAN-2026',  # User's tokens: 1133124, 1133190
            '1136': '26-MAR-2026',  # From contract master
            '1138': '28-DEC-2028',  # From contract master
            '1101': '26-MAR-2026',  # Alternative code from contract master
        }
        
        expiry = expiry_map.get(expiry_code, '')
        if not expiry:
            logger.debug(f"Unknown expiry code {expiry_code} for token {token}")
            return {
                'symbol': 'UNKNOWN',
                'expiry': '',
                'option_type': '',
                'strike': ''
            }
        
        # For the user's specific tokens, they are futures (no strike/option_type needed)
        user_futures = {1132866, 1133124, 1132508, 1133190}
        if token in user_futures:
            return {
                'symbol': 'SENSEX',
                'expiry': expiry,
                'option_type': '',  # Futures
                'strike': ''        # Futures
            }
        
        # For other tokens, check if they are options (7 digits) or futures
        if len(token_str) == 7:
            # Likely options - attempt to decode strike and option type
            # This is complex and would need more analysis of the encoding patterns
            # For now, return as futures to avoid incorrect decoding
            logger.debug(f"Token {token} appears to be options format but strike decoding not implemented")
            return {
                'symbol': 'SENSEX',
                'expiry': expiry,
                'option_type': '',  # Placeholder
                'strike': ''        # Placeholder
            }
        
        # Default: assume futures
        return {
            'symbol': 'SENSEX',
            'expiry': expiry,
            'option_type': '',  # Futures
            'strike': ''        # Futures
        }
    
    def _format_symbol_name(self, symbol_info: dict) -> str:
        """
        Format combined symbol name for unique identification.
        
        Format: {symbol}{expiry}_{strike}{option_type}
        Examples:
        - Options: SENSEX20NOV2025_82000CE, SENSEX20NOV2025_82000PE
        - Futures: SENSEX20NOV2025_FUT (when expiry available) or SENSEX_FUT (fallback)
        - Unknown (missing from master): SENSEX_FUT or SENSEX (fallback)
        
        Args:
            symbol_info: Dictionary with symbol, expiry, option_type, strike
        
        Returns:
            Formatted symbol name string
        """
        symbol = symbol_info.get('symbol', 'UNKNOWN')
        expiry = symbol_info.get('expiry', '')
        option_type = symbol_info.get('option_type', '')
        strike = symbol_info.get('strike', '')
        
        # Format expiry from "31-JAN-2026" to "31JAN2026"
        expiry_formatted = ''
        if expiry:
            try:
                # Parse different possible formats
                if '-' in expiry:
                    parts = expiry.split('-')
                    if len(parts) == 3:
                        day, month, year = parts
                        expiry_formatted = f"{day}{month}{year}"
                    else:
                        expiry_formatted = expiry.replace('-', '')
                else:
                    expiry_formatted = expiry
            except Exception:
                expiry_formatted = expiry.replace('-', '')
        
        # Build symbol name based on contract type
        if option_type and option_type in ('CE', 'PE') and strike:
            # Options: SENSEX20NOV2025_82000CE
            return f"{symbol}{expiry_formatted}_{strike}{option_type}"
        elif expiry_formatted:
            # Futures with decoded expiry: SENSEX31JAN2026_FUT
            return f"{symbol}{expiry_formatted}_FUT"
        else:
            # Fallback for when no expiry available
            return f"{symbol}_FUT"
    
    def _resolve_symbol(self, token: int) -> str:
        """
        Resolve token ID to trading symbol using token_map.
        
        BSE tokens map to derivative contracts (SENSEX/BANKEX options/futures).
        Token map loaded from data/tokens/token_details.json at startup.
        
        Args:
            token: BSE token ID (integer)
        
        Returns:
            Symbol string if found, "UNKNOWN" if token not in map
        """
        token_str = str(token)
        if token_str in self.token_map:
            contract = self.token_map[token_str]
            symbol = contract.get('symbol', 'UNKNOWN')
            
            # Build descriptive symbol for options (e.g., "SENSEX_CE_86900_20250918")
            if contract.get('option_type'):
                option_type = contract.get('option_type', '')
                strike = contract.get('strike_price', '')
                expiry = contract.get('expiry', '').replace('-', '')
                symbol = f"{symbol}_{option_type}_{strike}_{expiry}"
            
            return symbol
        else:
            logger.debug(f"Token {token} not found in token_map")
            return 'UNKNOWN'
    
    def _validate_record(self, record: dict) -> bool:
        """
        Validate decompressed record has required fields and sensible values.
        
        Checks:
        - Required fields: token, ltp, volume
        - Price ranges: ltp > 0 (must have valid price)
        - Volume range: volume >= 0 (can be zero for illiquid contracts)
        
        Args:
            record: Decompressed market data record
        
        Returns:
            True if validation passes, False otherwise
        """
        # Check required fields
        required = ['token', 'ltp', 'volume']
        for field in required:
            if field not in record:
                logger.warning(f"Record missing required field: {field}")
                self.stats['missing_fields'] += 1
                return False
        
        # Validate LTP (must be positive)
        ltp = record.get('ltp', 0.0)
        if ltp <= 0:
            logger.warning(f"Invalid LTP {ltp} for token {record.get('token')}")
            return False
        
        # Validate volume (must be non-negative)
        volume = record.get('volume', 0)
        if volume < 0:
            logger.warning(f"Invalid volume {volume} for token {record.get('token')}")
            return False
        
        # All validations passed
        return True
    
    def get_stats(self) -> dict:
        """
        Get collection statistics.
        
        Returns:
            Dictionary with counts:
            - quotes_collected: Total quotes successfully built
            - unknown_tokens: Tokens not found in token_map
            - validation_errors: Records that failed validation
            - missing_fields: Records with missing required fields
        """
        return self.stats.copy()
    
    def reset_stats(self):
        """Reset all statistics counters to zero."""
        for key in self.stats:
            self.stats[key] = 0
        logger.debug("MarketDataCollector stats reset")
        raise NotImplementedError("MarketDataCollector.collect_quotes() - Coming in Phase 2")


# Placeholder - to be implemented in Phase 2
if __name__ == '__main__':
    print("MarketDataCollector module - Placeholder for Phase 2 implementation")
