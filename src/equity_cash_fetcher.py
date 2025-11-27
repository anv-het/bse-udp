"""
BSE Equity Cash Contract Fetcher
=================================

Fetches BhavCopy CSV for Equity Cash segment (port 26001).
Maps stock tokens to their symbols (TCS, RELIANCE, etc.)

Features:
- Daily auto-update from BSE API
- Auto-cleanup of old files (keeps last 2 days)
- Token→Symbol mapping for equity cash segment

API: http://192.168.102.166:2060/v1/sftp-files?api_type=erp
File: BhavCopy_BSE_CM_0_0_0_YYYYMMDD_F_0000.csv
"""

import os
import re
import requests
import base64
import csv
import io
import logging
from datetime import datetime, timedelta
from pathlib import Path
from typing import Dict, Optional, List

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class BSEEquityCashFetcher:
    """
    Fetches and parses BhavCopy CSV for Equity Cash segment.
    Maps stock tokens (5xxxxx) to symbols.
    
    Features:
    - Auto-fetch from BSE API (Base64 encoded CSV)
    - Auto-cleanup of old files (configurable days to keep)
    - Simple token→symbol mapping
    """
    
    # File pattern: BhavCopy_BSE_CM_0_0_0_YYYYMMDD_F_0000.csv or BhavCopy_BSE_CM_DDMMYYYY.csv
    FILE_PATTERN = re.compile(r'BhavCopy_BSE_CM_.*\.csv$', re.IGNORECASE)
    
    def __init__(self, api_url: str = "http://192.168.102.166:2060/v1/sftp-files", 
                 data_dir: str = None):
        self.api_url = api_url
        self.contracts = {}
        
        # Set data directory (default: data/tokens)
        if data_dir:
            self.data_dir = Path(data_dir)
        else:
            self.data_dir = Path(__file__).parent.parent / 'data' / 'tokens'
        
        self.stats = {
            'total_contracts': 0,
            'last_fetch': None,
            'file_date': None,
            'files_deleted': 0
        }
    
    def get_bhavcopy_files(self) -> List[Path]:
        """Get all BhavCopy CSV files in data directory, sorted by date (newest first)."""
        if not self.data_dir.exists():
            return []
        
        files = []
        for f in self.data_dir.iterdir():
            if f.is_file() and self.FILE_PATTERN.match(f.name):
                files.append(f)
        
        # Sort by modification time (newest first)
        files.sort(key=lambda x: x.stat().st_mtime, reverse=True)
        return files
    
    def get_latest_file(self) -> Optional[Path]:
        """Get the most recent BhavCopy CSV file."""
        files = self.get_bhavcopy_files()
        return files[0] if files else None
    
    def delete_old_files(self, keep_days: int = 2) -> int:
        """
        Delete BhavCopy files older than keep_days.
        
        Args:
            keep_days: Number of days to keep (default: 2)
        
        Returns:
            Number of files deleted
        """
        files = self.get_bhavcopy_files()
        
        if len(files) <= keep_days:
            logger.info(f"📂 BhavCopy: {len(files)} files found, keeping all (threshold: {keep_days})")
            return 0
        
        # Keep the newest 'keep_days' files, delete the rest
        files_to_delete = files[keep_days:]
        deleted_count = 0
        
        for f in files_to_delete:
            try:
                f.unlink()
                logger.info(f"🗑️  Deleted old BhavCopy: {f.name}")
                deleted_count += 1
            except Exception as e:
                logger.error(f"❌ Failed to delete {f.name}: {e}")
        
        self.stats['files_deleted'] = deleted_count
        logger.info(f"🧹 BhavCopy cleanup: Deleted {deleted_count} old files, kept {keep_days}")
        return deleted_count
    
    def file_exists_for_date(self, date_str: str) -> bool:
        """Check if BhavCopy file exists for given date (DD-MM-YYYY format)."""
        # Convert DD-MM-YYYY to YYYYMMDD or DDMMYYYY
        parts = date_str.split('-')
        date_yyyymmdd = f"{parts[2]}{parts[1]}{parts[0]}"
        date_ddmmyyyy = f"{parts[0]}{parts[1]}{parts[2]}"
        
        for f in self.get_bhavcopy_files():
            if date_yyyymmdd in f.name or date_ddmmyyyy in f.name:
                return True
        return False
    
    def update_contracts(self, date_str: str = None, keep_days: int = 2) -> bool:
        """
        Update BhavCopy contracts: fetch new file, cleanup old ones.
        
        Args:
            date_str: Date in DD-MM-YYYY format (default: yesterday)
            keep_days: Number of days to keep
        
        Returns:
            True if successful
        """
        # Default to yesterday
        if date_str is None:
            yesterday = datetime.now() - timedelta(days=1)
            date_str = yesterday.strftime('%d-%m-%Y')
        
        logger.info(f"🔄 Updating BhavCopy contracts for: {date_str}")
        
        # Check if file already exists
        if self.file_exists_for_date(date_str):
            logger.info(f"✅ BhavCopy for {date_str} already exists, loading from cache")
            return self.load_latest_contracts()
        
        # Fetch new file
        parts = date_str.split('-')
        date_yyyymmdd = f"{parts[2]}{parts[1]}{parts[0]}"
        save_path = self.data_dir / f'BhavCopy_BSE_CM_{date_yyyymmdd}.csv'
        
        success = self.fetch_and_load(date_str, str(save_path))
        
        if success:
            # Cleanup old files
            self.delete_old_files(keep_days)
        
        return success
    
    def load_latest_contracts(self) -> bool:
        """Load contracts from the latest BhavCopy CSV file."""
        latest_file = self.get_latest_file()
        
        if not latest_file:
            logger.warning("⚠️ No BhavCopy files found")
            return False
        
        logger.info(f"📂 Loading BhavCopy from: {latest_file.name}")
        
        try:
            with open(latest_file, 'r', encoding='utf-8') as f:
                csv_content = f.read()
            
            self.parse_bhavcopy(csv_content)
            return True
        except Exception as e:
            logger.error(f"❌ Failed to load BhavCopy: {e}")
            return False
    
    def fetch_bhavcopy(self, date_str: str = None) -> Optional[str]:
        """
        Fetch BhavCopy CSV from API.
        
        Args:
            date_str: Date in DD-MM-YYYY format (e.g., "25-11-2025")
                     If None, uses yesterday's date
        
        Returns:
            CSV content as string, or None if failed
        """
        # Use yesterday's date if not provided
        if date_str is None:
            yesterday = datetime.now() - timedelta(days=1)
            date_str = yesterday.strftime('%d-%m-%Y')
        
        # Convert DD-MM-YYYY to YYYYMMDD for filename
        parts = date_str.split('-')
        file_date = f"{parts[2]}{parts[1]}{parts[0]}"  # YYYYMMDD
        
        # Get month name for path
        month_names = ['JAN', 'FEB', 'MAR', 'APR', 'MAY', 'JUN', 
                       'JUL', 'AUG', 'SEP', 'OCT', 'NOV', 'DEC']
        month = month_names[int(parts[1]) - 1]
        year = parts[2]
        
        # Construct file path and name
        # Path: EQ/Common/NOV-2025/25-11-2025/BhavCopy_BSE_CM_0_0_0_20251125_F_0000.csv
        file_path = f"EQ/Common/{month}-{year}/{date_str}/BhavCopy_BSE_CM_0_0_0_{file_date}_F_0000.csv"
        file_name = f"BhavCopy_BSE_CM_0_0_0_{file_date}_F_0000.csv"
        
        logger.info(f"📥 Fetching BhavCopy for date: {date_str}")
        logger.info(f"   File: {file_name}")
        logger.info(f"   Path: {file_path}")
        
        # API request
        payload = {
            "path": file_path,
            "file_name": file_name
        }
        
        headers = {
            'Content-Type': 'application/json'
        }
        
        params = {
            'api_type': 'erp'
        }
        
        try:
            response = requests.post(
                self.api_url,
                json=payload,
                headers=headers,
                params=params,
                timeout=30
            )
            response.raise_for_status()
            
            # Parse response
            data = response.json()
            
            logger.info(f"   Response type: {type(data)}")
            logger.info(f"   Response keys: {list(data.keys()) if isinstance(data, dict) else 'N/A'}")
            
            # Get Base64 encoded CSV
            csv_base64 = None
            
            if isinstance(data, dict):
                # Try different keys
                for key in ['csv', 'data', 'content', 'file', 'base64', 'result']:
                    if key in data:
                        value = data[key]
                        if isinstance(value, str) and len(value) > 100:
                            csv_base64 = value
                            logger.info(f"   Found CSV in key: {key}")
                            break
                        elif isinstance(value, dict):
                            # Nested dict
                            for subkey, subvalue in value.items():
                                if isinstance(subvalue, str) and len(subvalue) > 100:
                                    csv_base64 = subvalue
                                    logger.info(f"   Found CSV in key: {key}.{subkey}")
                                    break
                
                if csv_base64 is None:
                    # Log full response for debugging
                    logger.error(f"❌ No CSV content found in response")
                    logger.error(f"   Full response: {str(data)[:1000]}")
                    return None
            elif isinstance(data, str):
                # Response is directly a string (Base64 or CSV)
                csv_base64 = data
            else:
                logger.error(f"❌ Unexpected response type: {type(data)}")
                return None
            
            # Decode Base64
            try:
                csv_content = base64.b64decode(csv_base64).decode('utf-8')
            except Exception:
                # Maybe it's already plain CSV
                csv_content = csv_base64
            
            logger.info(f"✅ Fetched {len(csv_content):,} bytes of CSV data")
            self.stats['last_fetch'] = datetime.now().isoformat()
            self.stats['file_date'] = date_str
            
            return csv_content
            
        except requests.exceptions.RequestException as e:
            logger.error(f"❌ API request failed: {e}")
            return None
        except Exception as e:
            logger.error(f"❌ Error fetching BhavCopy: {e}")
            return None
    
    def parse_bhavcopy(self, csv_content: str):
        """
        Parse BhavCopy CSV and build token mapping.
        
        Expected columns may include:
        - TckrSymb (Ticker Symbol)
        - FinInstrmId (Token ID)
        - ISIN
        - TradDt (Trade Date)
        - OpnPric, HghPric, LwPric, ClsPric
        - TtlTradgVol, TtlTrnovr
        """
        logger.info("📋 Parsing BhavCopy CSV...")
        
        # Reset contracts
        self.contracts = {}
        
        # Parse CSV
        csv_file = io.StringIO(csv_content)
        
        # Try to detect delimiter
        first_line = csv_content.split('\n')[0]
        delimiter = ',' if ',' in first_line else '|'
        
        reader = csv.DictReader(csv_file, delimiter=delimiter)
        
        # Log headers
        if reader.fieldnames:
            logger.info(f"   Headers: {reader.fieldnames[:10]}...")
        
        for row in reader:
            try:
                # Try different column names for token
                token = None
                for col in ['FinInstrmId', 'ScripCode', 'Token', 'SCRIP_CD', 'scrip_code']:
                    if col in row and row[col]:
                        try:
                            token = int(row[col])
                            break
                        except ValueError:
                            continue
                
                if token is None:
                    continue
                
                # Try different column names for symbol
                symbol = None
                for col in ['TckrSymb', 'Symbol', 'SCRIP_NAME', 'scrip_name', 'SC_NAME']:
                    if col in row and row[col]:
                        symbol = row[col].strip()
                        break
                
                if symbol is None:
                    symbol = f"TOKEN_{token}"
                
                # Get ISIN if available
                isin = row.get('ISIN', row.get('isin', ''))
                
                # Get company name if available
                company_name = row.get('FinInstrmNm', row.get('SC_NAME', row.get('SCRIP_NAME', '')))
                
                # Store contract
                self.contracts[str(token)] = {
                    'symbol': symbol,
                    'isin': isin,
                    'company_name': company_name,
                    'token': token,
                    'segment': 'EQ'
                }
                
            except Exception as e:
                logger.debug(f"Error parsing row: {e}")
                continue
        
        self.stats['total_contracts'] = len(self.contracts)
        logger.info(f"✅ Parsed {len(self.contracts):,} equity contracts")
        
        # Show sample contracts
        if self.contracts:
            sample = list(self.contracts.items())[:5]
            logger.info("   Sample contracts:")
            for token, info in sample:
                logger.info(f"      {token}: {info['symbol']} ({info.get('company_name', '')[:30]})")
    
    def save_csv(self, csv_content: str, output_path: str):
        """Save CSV to file."""
        with open(output_path, 'w', encoding='utf-8') as f:
            f.write(csv_content)
        logger.info(f"💾 Saved CSV to: {output_path}")
    
    def get_contract(self, token: int) -> Optional[Dict]:
        """Get contract info for a token."""
        return self.contracts.get(str(token))
    
    def fetch_and_load(self, date_str: str = None, save_path: str = None) -> bool:
        """
        Fetch BhavCopy and load contracts.
        
        Args:
            date_str: Date in DD-MM-YYYY format
            save_path: Optional path to save CSV
        
        Returns:
            True if successful
        """
        csv_content = self.fetch_bhavcopy(date_str)
        
        if csv_content is None:
            return False
        
        if save_path:
            self.save_csv(csv_content, save_path)
        
        self.parse_bhavcopy(csv_content)
        
        return True


# Test the fetcher
if __name__ == '__main__':
    print("=" * 80)
    print("BSE EQUITY CASH BHAVCOPY FETCHER TEST")
    print("=" * 80)
    print(f"Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    
    # Initialize fetcher
    api_url = "http://192.168.102.166:2060/v1/sftp-files"
    fetcher = BSEEquityCashFetcher(api_url)
    
    # Update contracts (auto-fetches if needed, auto-cleanup old files)
    date_str = "26-11-2025"
    
    print(f"\n📥 Updating BhavCopy contracts for: {date_str}")
    print(f"   Data directory: {fetcher.data_dir}")
    
    success = fetcher.update_contracts(date_str, keep_days=2)
    
    if success:
        print(f"\n✅ SUCCESS! Loaded {fetcher.stats['total_contracts']:,} equity contracts")
        print(f"   Files deleted: {fetcher.stats['files_deleted']}")
        
        # Test some tokens from port 26001
        test_tokens = [532966, 543350, 500209, 532181, 532555, 532762, 500325]
        
        print("\n" + "=" * 80)
        print("🔍 TESTING TOKENS FROM PORT 26001 (EQUITY CASH)")
        print("=" * 80)
        
        for token in test_tokens:
            contract = fetcher.get_contract(token)
            if contract:
                print(f"\n✅ Token {token}:")
                print(f"   Symbol: {contract['symbol']}")
                print(f"   Company: {contract.get('company_name', 'N/A')[:40]}")
            else:
                print(f"\n❌ Token {token}: NOT FOUND")
    else:
        print("\n❌ FAILED to fetch/load BhavCopy")
    
    print("\n" + "=" * 80)
    print("TEST COMPLETE")
    print("=" * 80)
