"""
BSE Unified Token Mapper
========================

Maps token IDs to symbols for both:
- Equity Cash (port 26001): BhavCopy CSV
- Derivatives/F&O (port 26002): Contract Master CSV

NO dependency on token_details.json - uses API-based CSV files only.

Usage:
    from token_mapper import BSETokenMapper
    
    mapper = BSETokenMapper()
    mapper.update_all()  # Fetch/update both CSVs, cleanup old files
    
    symbol = mapper.get_symbol(500325)  # Returns "RELIANCE"
    symbol = mapper.get_symbol(873830)  # Returns "SENSEX_FUT_..."
"""

import logging
from datetime import datetime, timedelta
from pathlib import Path
from typing import Dict, Optional

from contract_manager import BSEContractManager
from equity_cash_fetcher import BSEEquityCashFetcher

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class BSETokenMapper:
    """
    Unified token→symbol mapper for BSE market data.
    
    Combines:
    - BhavCopy CSV (Equity Cash, port 26001) - ~4,689 stocks
    - Contract Master CSV (F&O, port 26002) - ~40,000 derivatives
    
    Features:
    - Auto-update from BSE API
    - Auto-cleanup of old files (keeps last 2 days)
    - Simple get_symbol(token) interface
    """
    
    def __init__(self, api_url: str = "http://192.168.102.166:2060/v1/sftp-files",
                 data_dir: str = None):
        """
        Initialize token mapper.
        
        Args:
            api_url: BSE API endpoint
            data_dir: Directory for CSV files (default: data/tokens)
        """
        self.api_url = api_url
        
        if data_dir:
            self.data_dir = Path(data_dir)
        else:
            self.data_dir = Path(__file__).parent.parent / 'data' / 'tokens'
        
        # Initialize both fetchers
        self.equity_fetcher = BSEEquityCashFetcher(api_url, str(self.data_dir))
        self.contract_manager = BSEContractManager(api_url, str(self.data_dir))
        
        # Combined token map: token (str) → symbol (str)
        self.token_map: Dict[str, str] = {}
        
        self.stats = {
            'equity_tokens': 0,
            'fo_tokens': 0,
            'total_tokens': 0,
            'last_update': None
        }
    
    def update_all(self, date_str: str = None, keep_days: int = 2) -> bool:
        """
        Update both Equity and F&O token mappings.
        
        Args:
            date_str: Date in DD-MM-YYYY format (default: yesterday)
            keep_days: Number of days to keep files
        
        Returns:
            True if at least one source updated successfully
        """
        if date_str is None:
            yesterday = datetime.now() - timedelta(days=1)
            date_str = yesterday.strftime('%d-%m-%Y')
        
        logger.info(f"🔄 Updating all token mappings for: {date_str}")
        
        success_count = 0
        
        # Update Equity Cash (BhavCopy)
        logger.info("📊 Updating Equity Cash tokens...")
        if self.equity_fetcher.update_contracts(date_str, keep_days):
            success_count += 1
            # Load the contracts into memory after update
            self.equity_fetcher.load_latest_contracts()
            self._load_equity_tokens()
        
        # Update F&O (Contract Master)
        logger.info("📊 Updating F&O tokens...")
        if self.contract_manager.update_contracts(date_str, keep_days):
            success_count += 1
            # Load the contracts into memory after update
            self.contract_manager.load_latest_contracts()
            self._load_fo_tokens()
        
        self.stats['last_update'] = datetime.now().isoformat()
        self.stats['total_tokens'] = len(self.token_map)
        
        logger.info(f"✅ Token mapper updated: {self.stats['total_tokens']:,} total tokens")
        logger.info(f"   Equity: {self.stats['equity_tokens']:,} | F&O: {self.stats['fo_tokens']:,}")
        
        return success_count > 0
    
    def load_from_cache(self) -> bool:
        """
        Load tokens from existing CSV files (no API fetch).
        
        Returns:
            True if at least one source loaded successfully
        """
        logger.info("📂 Loading tokens from cached CSV files...")
        
        success_count = 0
        
        # Load Equity Cash
        if self.equity_fetcher.load_latest_contracts():
            success_count += 1
            self._load_equity_tokens()
        
        # Load F&O
        if self.contract_manager.load_latest_contracts():
            success_count += 1
            self._load_fo_tokens()
        
        self.stats['total_tokens'] = len(self.token_map)
        logger.info(f"✅ Loaded {self.stats['total_tokens']:,} tokens from cache")
        
        return success_count > 0
    
    def _load_equity_tokens(self):
        """Load tokens from equity fetcher into unified map."""
        count = 0
        for token_str, info in self.equity_fetcher.contracts.items():
            symbol = info.get('symbol', f'TOKEN_{token_str}')
            self.token_map[token_str] = symbol
            count += 1
        
        self.stats['equity_tokens'] = count
        logger.info(f"   Loaded {count:,} equity tokens")
    
    def _load_fo_tokens(self):
        """Load tokens from contract manager into unified map."""
        count = 0
        for token_str, info in self.contract_manager.contracts.items():
            # Build F&O symbol with details
            symbol = self._build_fo_symbol(info)
            self.token_map[token_str] = symbol
            count += 1
        
        self.stats['fo_tokens'] = count
        logger.info(f"   Loaded {count:,} F&O tokens")
    
    def _build_fo_symbol(self, info: Dict) -> str:
        """
        Build descriptive symbol for F&O contract.
        
        Examples:
            SENSEX FUT 18-Sep-2025
            SENSEX CE 85000 18-Sep-2025
            BANKEX PE 57000 18-Sep-2025
        """
        underlying = info.get('underlying', info.get('symbol', 'UNKNOWN'))
        inst_type = info.get('inst_type', '')
        
        # Futures
        if 'FUT' in inst_type.upper() or inst_type.upper() in ['IF', 'SF']:
            expiry = info.get('expiry_display', info.get('expiry', ''))
            return f"{underlying}_FUT_{expiry}"
        
        # Options
        option_type = info.get('option_type', '')
        strike = info.get('strike', '')
        expiry = info.get('expiry_display', info.get('expiry', ''))
        
        if option_type:
            return f"{underlying}_{option_type}_{strike}_{expiry}"
        
        # Default
        return f"{underlying}_{inst_type}"
    
    def get_symbol(self, token: int) -> Optional[str]:
        """
        Get symbol for a token.
        
        Args:
            token: Token ID (integer)
        
        Returns:
            Symbol string or None if not found
        """
        return self.token_map.get(str(token))
    
    def get_symbols_batch(self, tokens: list) -> Dict[int, str]:
        """
        Get symbols for multiple tokens.
        
        Args:
            tokens: List of token IDs
        
        Returns:
            Dict mapping token → symbol (only for found tokens)
        """
        result = {}
        for token in tokens:
            symbol = self.get_symbol(token)
            if symbol:
                result[token] = symbol
        return result
    
    def is_equity(self, token: int) -> bool:
        """Check if token is from Equity Cash segment."""
        return str(token) in self.equity_fetcher.contracts
    
    def is_fo(self, token: int) -> bool:
        """Check if token is from F&O segment."""
        return str(token) in self.contract_manager.contracts
    
    def get_segment(self, token: int) -> Optional[str]:
        """
        Get segment for a token.
        
        Returns:
            'EQ' for Equity Cash, 'FO' for Derivatives, None if not found
        """
        if self.is_equity(token):
            return 'EQ'
        elif self.is_fo(token):
            return 'FO'
        return None
    
    def __contains__(self, token: int) -> bool:
        """Check if token exists in mapper."""
        return str(token) in self.token_map
    
    def __len__(self) -> int:
        """Return total number of mapped tokens."""
        return len(self.token_map)


# Test the mapper
if __name__ == '__main__':
    print("=" * 80)
    print("BSE UNIFIED TOKEN MAPPER TEST")
    print("=" * 80)
    print(f"Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    
    # Initialize mapper
    mapper = BSETokenMapper()
    
    # Try to load from cache first
    print("\n📂 Loading from cache...")
    if mapper.load_from_cache():
        print(f"✅ Loaded {len(mapper):,} tokens from cache")
    else:
        print("⚠️ Cache empty, fetching from API...")
        mapper.update_all()
    
    print(f"\n📊 Stats:")
    print(f"   Equity tokens: {mapper.stats['equity_tokens']:,}")
    print(f"   F&O tokens: {mapper.stats['fo_tokens']:,}")
    print(f"   Total tokens: {mapper.stats['total_tokens']:,}")
    
    # Test Equity tokens (port 26001)
    equity_tokens = [500325, 532540, 500209, 532555, 532762]
    
    print("\n" + "=" * 80)
    print("🔍 EQUITY CASH TOKENS (Port 26001)")
    print("=" * 80)
    
    for token in equity_tokens:
        symbol = mapper.get_symbol(token)
        segment = mapper.get_segment(token)
        if symbol:
            print(f"✅ {token}: {symbol} [{segment}]")
        else:
            print(f"❌ {token}: NOT FOUND")
    
    # Test F&O tokens (port 26002)
    fo_tokens = [873830, 1102290, 842364, 1114300, 1114301]
    
    print("\n" + "=" * 80)
    print("🔍 F&O TOKENS (Port 26002)")
    print("=" * 80)
    
    for token in fo_tokens:
        symbol = mapper.get_symbol(token)
        segment = mapper.get_segment(token)
        if symbol:
            print(f"✅ {token}: {symbol} [{segment}]")
        else:
            print(f"❌ {token}: NOT FOUND")
    
    # Test batch lookup
    all_tokens = equity_tokens + fo_tokens
    batch_results = mapper.get_symbols_batch(all_tokens)
    print(f"\n📦 Batch lookup: {len(batch_results)}/{len(all_tokens)} tokens found")
    
    print("\n" + "=" * 80)
    print("TEST COMPLETE")
    print("=" * 80)
