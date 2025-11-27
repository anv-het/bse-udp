"""
BSE Contract Fetcher - Daily Updated CSV
=========================================

Fetches the latest BSE contract master CSV from API.
Updates daily with new contracts, expiries, and strikes.

API returns Base64 encoded CSV which we decode and use for token mapping.
"""

import base64
import requests
import csv
import io
import logging
from datetime import datetime
from typing import Dict, Optional
from pathlib import Path

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)


class BSEContractFetcher:
    """
    Fetches latest BSE contract CSV from API.
    
    API returns Base64 encoded CSV with all active contracts.
    This replaces manual CSV download and ensures always up-to-date data.
    """
    
    def __init__(self, api_url: str):
        """
        Initialize fetcher.
        
        Args:
            api_url: API endpoint URL (e.g., http://192.168.102.166:2060/v1/sftp-files)
        """
        self.api_url = api_url
        self.contracts = {}
        self.last_update = None
        self.stats = {
            'total_contracts': 0,
            'sensex_contracts': 0,
            'bankex_contracts': 0,
            'stock_contracts': 0,
            'futures': 0,
            'options': 0,
            'last_fetch': None,
            'file_date': None
        }
    
    def fetch_latest_csv(self, date_str: str = None) -> str:
        """
        Fetch latest BSE contract CSV from API.
        
        Args:
            date_str: Date string in DD-MM-YYYY format (e.g., "26-11-2025")
                     If None, uses today's date
        
        Returns:
            CSV content as string
        """
        # Use today's date if not provided
        if date_str is None:
            date_str = datetime.now().strftime("%d-%m-%Y")
        
        # Convert DD-MM-YYYY to DDMMYYYY for file name
        file_date = date_str.replace("-", "")
        
        # Construct API request
        file_path = f"FNO/Common/NOV-2025/{date_str}/BSE_EQD_CONTRACT_{file_date}.csv"
        file_name = f"BSE_EQD_CONTRACT_{file_date}.csv"
        
        payload = {
            "path": file_path,
            "file_name": file_name
        }
        
        headers = {
            'Content-Type': 'application/json'
        }
        
        # Add api_type parameter
        params = {
            'api_type': 'erp'
        }
        
        logger.info(f"Fetching contract CSV for date: {date_str}")
        logger.info(f"File: {file_name}")
        
        try:
            # Call API
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
            
            if data.get('status') != 'success':
                raise Exception(f"API returned error: {data.get('message')}")
            
            # Get Base64 encoded file content
            file_content_b64 = data['data']['file_content']
            
            logger.info(f"✅ Received Base64 content: {len(file_content_b64)} characters")
            
            # Decode Base64
            csv_bytes = base64.b64decode(file_content_b64)
            csv_content = csv_bytes.decode('utf-8')
            
            logger.info(f"✅ Decoded CSV: {len(csv_content)} characters, {csv_content.count(chr(10))} lines")
            
            # Update stats
            self.stats['last_fetch'] = datetime.now()
            self.stats['file_date'] = date_str
            
            return csv_content
            
        except requests.exceptions.RequestException as e:
            logger.error(f"❌ API request failed: {e}")
            raise
        except Exception as e:
            logger.error(f"❌ Failed to fetch CSV: {e}")
            raise
    
    def parse_csv_content(self, csv_content: str):
        """
        Parse CSV content and load contracts into memory.
        
        Args:
            csv_content: CSV string content
        """
        logger.info("Parsing CSV content...")
        
        # Reset contracts
        self.contracts = {}
        self.stats['total_contracts'] = 0
        self.stats['sensex_contracts'] = 0
        self.stats['bankex_contracts'] = 0
        self.stats['stock_contracts'] = 0
        self.stats['futures'] = 0
        self.stats['options'] = 0
        
        # Parse CSV
        csv_file = io.StringIO(csv_content)
        reader = csv.DictReader(csv_file)
        
        for row in reader:
            token = row.get('FinInstrmId')
            if not token or not token.isdigit():
                continue
            
            # Parse contract details
            symbol = row.get('TckrSymb', '').strip()
            instrument_name = row.get('FinInstrmNm', '').strip()
            expiry = row.get('XpryDt', '').strip()
            strike_paise = row.get('StrkPric', '').strip()
            option_type = row.get('OptnTp', '').strip()
            lot_size = row.get('MinLot', '').strip()
            contract_type = row.get('CtrctTp', '').strip()
            full_name = row.get('SctyLngNm', '').strip()
            
            # Convert strike from paise to rupees
            strike = 0.0
            if strike_paise and strike_paise.isdigit():
                strike = int(strike_paise) / 100.0
            
            # Convert lot size
            lot = 0
            if lot_size and lot_size.isdigit():
                lot = int(lot_size)
            
            # Store contract
            self.contracts[token] = {
                'token': int(token),
                'symbol': symbol,
                'instrument_name': instrument_name,
                'expiry': expiry,
                'strike': strike,
                'option_type': option_type,
                'lot_size': lot,
                'contract_type': contract_type,
                'full_name': full_name
            }
            
            # Update stats
            self.stats['total_contracts'] += 1
            
            if symbol == 'SENSEX':
                self.stats['sensex_contracts'] += 1
            elif symbol == 'BANKEX':
                self.stats['bankex_contracts'] += 1
            else:
                self.stats['stock_contracts'] += 1
            
            if option_type:  # CE or PE
                self.stats['options'] += 1
            else:  # Empty means futures
                self.stats['futures'] += 1
        
        self.last_update = datetime.now()
        
        logger.info(f"✅ Parsed {self.stats['total_contracts']} contracts")
        logger.info(f"   SENSEX: {self.stats['sensex_contracts']}")
        logger.info(f"   BANKEX: {self.stats['bankex_contracts']}")
        logger.info(f"   Stocks: {self.stats['stock_contracts']}")
        logger.info(f"   Options: {self.stats['options']}")
        logger.info(f"   Futures: {self.stats['futures']}")
    
    def save_csv(self, csv_content: str, output_path: str):
        """
        Save CSV content to file for backup/reference.
        
        Args:
            csv_content: CSV string content
            output_path: Path to save file
        """
        with open(output_path, 'w', encoding='utf-8') as f:
            f.write(csv_content)
        logger.info(f"✅ Saved CSV to: {output_path}")
    
    def fetch_and_load(self, date_str: str = None, save_path: str = None) -> Dict:
        """
        Fetch CSV from API and load into memory.
        
        Args:
            date_str: Date string in DD-MM-YYYY format (e.g., "26-11-2025")
            save_path: Optional path to save CSV file
        
        Returns:
            Statistics dictionary
        """
        # Fetch CSV
        csv_content = self.fetch_latest_csv(date_str)
        
        # Save if requested
        if save_path:
            self.save_csv(csv_content, save_path)
        
        # Parse and load
        self.parse_csv_content(csv_content)
        
        return self.stats
    
    def get_contract(self, token: int) -> Optional[Dict]:
        """
        Get contract details for a token.
        
        Args:
            token: BSE token ID
        
        Returns:
            Contract details or None if not found
        """
        return self.contracts.get(str(token))
    
    def get_stats(self) -> Dict:
        """Get fetcher statistics."""
        return self.stats


# Example usage
if __name__ == '__main__':
    print("=" * 80)
    print("BSE CONTRACT FETCHER TEST")
    print("=" * 80)
    
    # Initialize fetcher
    api_url = "http://192.168.102.166:2060/v1/sftp-files"
    fetcher = BSEContractFetcher(api_url)
    
    # Fetch latest contracts for today (26-11-2025)
    date_str = "26-11-2025"
    save_path = Path(__file__).parent.parent / 'docs' / f'BSE_EQD_CONTRACT_{date_str.replace("-", "")}_fetched.csv'
    
    try:
        stats = fetcher.fetch_and_load(date_str=date_str, save_path=str(save_path))
        
        print("\n✅ CONTRACT CSV FETCHED & LOADED")
        print("=" * 80)
        print(f"File Date: {stats['file_date']}")
        print(f"Last Fetch: {stats['last_fetch']}")
        print(f"Total Contracts: {stats['total_contracts']}")
        print(f"SENSEX: {stats['sensex_contracts']}")
        print(f"BANKEX: {stats['bankex_contracts']}")
        print(f"Stocks: {stats['stock_contracts']}")
        print(f"Options: {stats['options']}")
        print(f"Futures: {stats['futures']}")
        
        # Test token lookup
        print("\n" + "=" * 80)
        print("TESTING TOKEN LOOKUP")
        print("=" * 80)
        
        test_tokens = [880622, 885255, 885890, 1117858]
        
        for token in test_tokens:
            contract = fetcher.get_contract(token)
            if contract:
                print(f"\n✅ Token {token}:")
                print(f"   {contract['symbol']} {contract['expiry']} "
                      f"{contract['strike']:.0f} {contract['option_type']}")
                print(f"   Lot Size: {contract['lot_size']}")
            else:
                print(f"\n❌ Token {token}: NOT FOUND")
        
        print("\n" + "=" * 80)
        print("✅ TEST COMPLETE - Using API-fetched CSV!")
        print("=" * 80)
        
    except Exception as e:
        print(f"\n❌ ERROR: {e}")
        import traceback
        traceback.print_exc()
