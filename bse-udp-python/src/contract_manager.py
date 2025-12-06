"""
BSE Contract Manager - Daily Contract File Management
======================================================

Manages daily BSE contract files:
1. Fetches yesterday's contract file (API doesn't have today's file)
2. Deletes old files (older than 2 days)
3. Checks if file exists before downloading
4. Provides latest contract file to decoder

Example:
    Today: 27-11-2025
    Download: 26-11-2025 (yesterday)
    Keep: 26-11-2025, 25-11-2025 (2 days)
    Delete: 24-11-2025 and older
"""

import os
import logging
from datetime import datetime, timedelta
from pathlib import Path
from typing import Optional
from bse_contract_fetcher import BSEContractFetcher

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class BSEContractManager:
    """
    Manages daily BSE contract files with automatic cleanup.
    """
    
    def __init__(self, api_url: str, storage_dir: str = None):
        """
        Initialize contract manager.
        
        Args:
            api_url: BSE API endpoint
            storage_dir: Directory to store contract files (default: data/tokens/)
        """
        self.api_url = api_url
        self.fetcher = BSEContractFetcher(api_url)
        
        # Set storage directory
        if storage_dir is None:
            self.storage_dir = Path(__file__).parent.parent / 'data' / 'tokens'
        else:
            self.storage_dir = Path(storage_dir)
        
        # Create directory if not exists
        self.storage_dir.mkdir(parents=True, exist_ok=True)
        
        # Contracts dictionary - populated after loading
        self.contracts = {}
        
        logger.info(f"Contract Manager initialized")
        logger.info(f"Storage: {self.storage_dir}")
    
    def get_file_path(self, date_str: str, with_suffix: bool = True) -> Path:
        """
        Get file path for a date.
        
        Args:
            date_str: Date in DD-MM-YYYY format
            with_suffix: If True, use _fetched suffix (for new downloads)
        
        Returns:
            Path to contract file
        """
        file_date = date_str.replace("-", "")
        if with_suffix:
            filename = f"BSE_EQD_CONTRACT_{file_date}_fetched.csv"
        else:
            filename = f"BSE_EQD_CONTRACT_{file_date}.csv"
        return self.storage_dir / filename
    
    def file_exists(self, date_str: str) -> bool:
        """
        Check if contract file exists for a date.
        Checks both API-fetched (_fetched.csv) and manually downloaded (.csv) files.
        
        Args:
            date_str: Date in DD-MM-YYYY format
        
        Returns:
            True if file exists (either format)
        """
        # Check for API-fetched file first
        file_path_fetched = self.get_file_path(date_str, with_suffix=True)
        if file_path_fetched.exists():
            logger.info(f"✅ File exists: {file_path_fetched.name}")
            return True
        
        # Check for manually downloaded file (without _fetched suffix)
        file_path_manual = self.get_file_path(date_str, with_suffix=False)
        if file_path_manual.exists():
            logger.info(f"✅ File exists: {file_path_manual.name}")
            return True
        
        logger.info(f"❌ File missing: BSE_EQD_CONTRACT_{date_str.replace('-', '')}.csv (both formats)")
        return False
    
    def fetch_contract_file(self, date_str: str) -> bool:
        """
        Fetch contract file for a date.
        
        Args:
            date_str: Date in DD-MM-YYYY format
        
        Returns:
            True if successful
        """
        file_path = self.get_file_path(date_str)
        
        try:
            logger.info(f"📥 Fetching contract file for {date_str}...")
            
            # Fetch from API
            csv_content = self.fetcher.fetch_latest_csv(date_str)
            
            # Save to file
            self.fetcher.save_csv(csv_content, str(file_path))
            
            # Parse to verify
            self.fetcher.parse_csv_content(csv_content)
            
            logger.info(f"✅ Downloaded: {file_path.name}")
            logger.info(f"   Contracts: {self.fetcher.stats['total_contracts']}")
            
            return True
            
        except Exception as e:
            logger.error(f"❌ Failed to fetch {date_str}: {e}")
            return False
    
    def delete_old_files(self, keep_days: int = 2):
        """
        Delete contract files older than specified days.
        
        Args:
            keep_days: Number of recent days to keep (default: 2)
        """
        logger.info(f"🗑️  Cleaning up old files (keeping last {keep_days} days)...")
        
        # Calculate cutoff date
        cutoff_date = datetime.now() - timedelta(days=keep_days)
        
        # Find all contract files
        pattern = "BSE_EQD_CONTRACT_*_fetched.csv"
        files = list(self.storage_dir.glob(pattern))
        
        deleted_count = 0
        
        for file_path in files:
            try:
                # Extract date from filename
                # Format: BSE_EQD_CONTRACT_DDMMYYYY_fetched.csv
                filename = file_path.stem  # Remove .csv
                date_part = filename.split('_')[3]  # Get DDMMYYYY
                
                # Parse date
                file_date = datetime.strptime(date_part, '%d%m%Y')
                
                # Check if older than cutoff
                if file_date < cutoff_date:
                    logger.info(f"   Deleting: {file_path.name} ({file_date.strftime('%d-%m-%Y')})")
                    file_path.unlink()
                    deleted_count += 1
                else:
                    logger.info(f"   Keeping: {file_path.name} ({file_date.strftime('%d-%m-%Y')})")
                    
            except Exception as e:
                logger.warning(f"   Error processing {file_path.name}: {e}")
        
        if deleted_count > 0:
            logger.info(f"✅ Deleted {deleted_count} old file(s)")
        else:
            logger.info(f"✅ No old files to delete")
    
    def get_latest_file(self) -> Optional[Path]:
        """
        Get the latest available contract file.
        
        Returns:
            Path to latest file or None
        """
        # Find all contract files - both API-fetched and manually downloaded
        # API creates: BSE_EQD_CONTRACT_DDMMYYYY_fetched.csv
        # Manual:      BSE_EQD_CONTRACT_DDMMYYYY.csv
        pattern_fetched = "BSE_EQD_CONTRACT_*_fetched.csv"
        pattern_manual = "BSE_EQD_CONTRACT_????????.csv"  # 8 digits = DDMMYYYY
        
        files_fetched = list(self.storage_dir.glob(pattern_fetched))
        files_manual = [f for f in self.storage_dir.glob(pattern_manual) if '_fetched' not in f.name]
        
        files = files_fetched + files_manual
        
        if not files:
            logger.warning("❌ No contract files found")
            return None
        
        # Sort by date in filename (DDMMYYYY format)
        # Extract date from filename for proper sorting
        def extract_date(f):
            name = f.stem  # e.g., BSE_EQD_CONTRACT_01122025 or BSE_EQD_CONTRACT_01122025_fetched
            parts = name.split('_')
            for part in parts:
                if part.isdigit() and len(part) == 8:
                    # Convert DDMMYYYY to YYYYMMDD for sorting
                    return part[4:8] + part[2:4] + part[0:2]
            return "00000000"
        
        latest_file = sorted(files, key=extract_date)[-1]
        
        logger.info(f"📂 Latest file: {latest_file.name}")
        return latest_file
    
    def update_contracts(self, date_str: str = None, keep_days: int = 2) -> bool:
        """
        Main update function - fetch contract file and cleanup.
        
        Process:
        1. Calculate target date (default: yesterday, API doesn't have today's file)
        2. Check if file exists
        3. Download if not exists
        4. Delete old files (older than keep_days)
        
        Args:
            date_str: Date in DD-MM-YYYY format (default: yesterday)
            keep_days: Number of days to keep files (default: 2)
        
        Returns:
            True if update successful
        """
        logger.info("=" * 80)
        logger.info("DAILY CONTRACT UPDATE")
        logger.info("=" * 80)
        
        # Calculate target date (yesterday if not specified)
        today = datetime.now()
        if date_str is None:
            yesterday = today - timedelta(days=1)
            target_date_str = yesterday.strftime("%d-%m-%Y")
        else:
            target_date_str = date_str
        
        logger.info(f"Today: {today.strftime('%d-%m-%Y')}")
        logger.info(f"Fetching: {target_date_str}")
        
        # Check if file exists
        if self.file_exists(target_date_str):
            logger.info("✅ File already exists - skipping download")
            success = True
        else:
            # Download contract file
            success = self.fetch_contract_file(target_date_str)
        
        if success:
            # Delete old files
            self.delete_old_files(keep_days=keep_days)
            
            logger.info("=" * 80)
            logger.info("✅ CONTRACT UPDATE COMPLETE")
            logger.info("=" * 80)
        else:
            logger.error("=" * 80)
            logger.error("❌ CONTRACT UPDATE FAILED")
            logger.error("=" * 80)
        
        return success
    
    def load_latest_contracts(self):
        """
        Load the latest contract file into memory.
        
        Returns:
            BSEContractFetcher with loaded contracts
        """
        latest_file = self.get_latest_file()
        
        if latest_file is None:
            raise FileNotFoundError("No contract files available")
        
        logger.info(f"📖 Loading contracts from: {latest_file.name}")
        
        # Read file
        with open(latest_file, 'r', encoding='utf-8') as f:
            csv_content = f.read()
        
        # Parse into fetcher
        self.fetcher.parse_csv_content(csv_content)
        
        # Populate contracts dict for token_mapper compatibility
        self.contracts = self.fetcher.contracts.copy()
        
        logger.info(f"✅ Loaded {self.fetcher.stats['total_contracts']} contracts")
        
        return self.fetcher


# Example usage
if __name__ == '__main__':
    print("\n" + "=" * 80)
    print("BSE CONTRACT MANAGER - DAILY UPDATE TEST")
    print("=" * 80)
    
    # Initialize manager
    api_url = "http://192.168.102.166:2060/v1/sftp-files"
    manager = BSEContractManager(api_url)
    
    # Run daily update
    success = manager.update_contracts()
    
    if success:
        print("\n" + "=" * 80)
        print("LOADING LATEST CONTRACTS")
        print("=" * 80)
        
        # Load latest contracts
        fetcher = manager.load_latest_contracts()
        
        # Test with sample tokens
        print("\n📋 TESTING TOKEN LOOKUP")
        print("-" * 80)
        
        test_tokens = [880622, 885255, 885890, 1117858]
        
        for token in test_tokens:
            contract = fetcher.get_contract(token)
            if contract:
                print(f"\n✅ Token {token}:")
                print(f"   {contract['symbol']:<10} {contract['full_name']}")
                if contract['expiry']:
                    print(f"   Expiry: {contract['expiry']:<15} Strike: ₹{contract['strike']:>10,.0f}")
                    print(f"   Type: {contract['option_type']:<4} Lot: {contract['lot_size']}")
            else:
                print(f"\n❌ Token {token}: NOT FOUND")
        
        print("\n" + "=" * 80)
        print("✅ CONTRACT MANAGER TEST COMPLETE")
        print("=" * 80)
