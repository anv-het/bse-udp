"""
BSE Data Saver Module
=====================

Handles saving processed market data to JSON and CSV files.

Responsibilities:
- Save normalized quotes to JSON (processed_json/)
- Save normalized quotes to CSV (processed_csv/)
- Create timestamped output files
- Manage output directories
- Support append mode for continuous data collection
- Statistics tracking (files saved, quotes written, I/O errors)

Output File Formats:

JSON (processed_json/YYYYMMDD_quotes.json):
- Newline-delimited JSON (one quote per line)
- Each line is valid JSON object
- Easy to stream and process incrementally

CSV (processed_csv/YYYYMMDD_quotes.csv):
- Header row with column names
- One quote per row
- Best 5 bid/ask levels as comma-separated subfields

Status: Phase 3 - COMPLETE
Reference: BSE_Complete_Technical_Knowledge_Base.md (Data Output section)
"""

import logging
import json
import csv
import os
from typing import List, Dict
from datetime import datetime
from pathlib import Path

logger = logging.getLogger(__name__)


class DataSaver:
    """
    Saves processed market data to JSON and CSV files.
    
    Creates timestamped output files in configured directories:
    - processed_json/: Newline-delimited JSON for streaming
    - processed_csv/: CSV with headers for analysis
    
    Features:
    - Automatic directory creation
    - Append mode (add to existing files)
    - Statistics tracking (files saved, quotes written, errors)
    - Configurable output formats
    
    File naming convention:
    - YYYYMMDD_quotes.json (e.g., 20250120_quotes.json)
    - YYYYMMDD_quotes.csv (e.g., 20250120_quotes.csv)
    """
    
    def __init__(self, output_dir: str = 'data'):
        """
        Initialize data saver with output directory.
        
        Creates subdirectories if they don't exist:
        - {output_dir}/processed_json/
        - {output_dir}/processed_csv/
        
        Args:
            output_dir: Base directory for output files (default: 'data')
        """
        self.output_dir = Path(output_dir)
        self.json_dir = self.output_dir / 'processed_json'
        self.csv_dir = self.output_dir / 'processed_csv'
        
        # Create directories
        self._create_directories()
        
        # Statistics
        self.stats = {
            'json_files_saved': 0,
            'csv_files_saved': 0,
            'quotes_written_json': 0,
            'quotes_written_csv': 0,
            'io_errors': 0
        }
        
        logger.info(f"DataSaver initialized - JSON: {self.json_dir}, CSV: {self.csv_dir}")
    
    def _create_directories(self):
        """
        Create output directories if they don't exist.
        
        Creates:
        - processed_json/
        - processed_csv/
        
        Logs creation or uses existing directories.
        """
        try:
            self.json_dir.mkdir(parents=True, exist_ok=True)
            logger.debug(f"JSON output directory ready: {self.json_dir}")
            
            self.csv_dir.mkdir(parents=True, exist_ok=True)
            logger.debug(f"CSV output directory ready: {self.csv_dir}")
            
        except Exception as e:
            logger.error(f"Error creating output directories: {e}", exc_info=True)
            self.stats['io_errors'] += 1
    
    def save_to_json(self, quotes: List[Dict], date_str: str = None) -> bool:
        """
        Save quotes to newline-delimited JSON file.
        
        Output format (one quote per line):
        {"token": 842364, "symbol": "SENSEX", "ltp": 86500.0, ...}
        {"token": 842365, "symbol": "BANKEX", "ltp": 52000.0, ...}
        
        File is appended if it exists (supports continuous collection).
        
        Args:
            quotes: List of normalized quote dictionaries
            date_str: Optional date string (YYYYMMDD), defaults to today
        
        Returns:
            True if save successful, False if error occurred
        """
        if not quotes:
            logger.debug("No quotes to save to JSON")
            return True
        
        # Generate filename
        if date_str is None:
            date_str = datetime.now().strftime('%Y%m%d')
        filename = self.json_dir / f"{date_str}_quotes.json"
        
        try:
            # Append mode - add new quotes to existing file
            with open(filename, 'a', encoding='utf-8') as f:
                for quote in quotes:
                    # Write each quote as single JSON line
                    json_line = json.dumps(quote, ensure_ascii=False)
                    f.write(json_line + '\n')
                    self.stats['quotes_written_json'] += 1
            
            self.stats['json_files_saved'] += 1
            logger.info(f"Saved {len(quotes)} quotes to JSON: {filename}")
            return True
            
        except Exception as e:
            logger.error(f"Error saving to JSON {filename}: {e}", exc_info=True)
            self.stats['io_errors'] += 1
            return False
    
    def save_to_csv(self, quotes: List[Dict], date_str: str = None) -> bool:
        """
        Save quotes to CSV file with headers.
        
        CSV columns:
        - token, symbol, timestamp
        - open, high, low, close, ltp, volume, prev_close
        - bid_prices, bid_qtys, bid_orders (comma-separated Best 5)
        - ask_prices, ask_qtys, ask_orders (comma-separated Best 5)
        
        File is appended if it exists (header written only on creation).
        
        Args:
            quotes: List of normalized quote dictionaries
            date_str: Optional date string (YYYYMMDD), defaults to today
        
        Returns:
            True if save successful, False if error occurred
        """
        if not quotes:
            logger.debug("No quotes to save to CSV")
            return True
        
        # Generate filename
        if date_str is None:
            date_str = datetime.now().strftime('%Y%m%d')
        filename = self.csv_dir / f"{date_str}_quotes.csv"
        
        # Check if file exists (determines if header needed)
        file_exists = filename.exists()
        
        try:
            with open(filename, 'a', newline='', encoding='utf-8') as f:
                # Define CSV columns
                fieldnames = [
                    'token', 'symbol', 'symbol_name', 'expiry', 'option_type', 'strike', 'timestamp',
                    'open', 'high', 'low', 'close', 'ltp', 'volume', 'prev_close',
                    'bid_prices', 'bid_qtys', 'bid_orders',
                    'ask_prices', 'ask_qtys', 'ask_orders'
                ]
                
                writer = csv.DictWriter(f, fieldnames=fieldnames, extrasaction='ignore')
                
                # Write header if new file
                if not file_exists:
                    writer.writeheader()
                    logger.debug(f"CSV header written to {filename}")
                
                # Write quotes
                for quote in quotes:
                    # Flatten Best 5 levels to comma-separated strings
                    csv_row = self._flatten_quote_for_csv(quote)
                    writer.writerow(csv_row)
                    self.stats['quotes_written_csv'] += 1
            
            self.stats['csv_files_saved'] += 1
            logger.info(f"Saved {len(quotes)} quotes to CSV: {filename}")
            return True
            
        except Exception as e:
            logger.error(f"Error saving to CSV {filename}: {e}", exc_info=True)
            self.stats['io_errors'] += 1
            return False
    
    def _flatten_quote_for_csv(self, quote: Dict) -> Dict:
        """
        Flatten quote dictionary for CSV output.
        
        Converts Best 5 bid/ask levels (list of dicts) into comma-separated strings:
        - bid_levels → bid_prices, bid_qtys, bid_orders
        - ask_levels → ask_prices, ask_qtys, ask_orders
        
        Example:
        bid_levels: [{'price': 86500, 'qty': 100, 'orders': 5}, ...]
        → bid_prices: "86500,86499,86498,86497,86496"
        → bid_qtys: "100,200,300,150,250"
        → bid_orders: "5,10,8,6,12"
        
        Args:
            quote: Normalized quote dictionary with bid_levels/ask_levels
        
        Returns:
            Flattened dictionary suitable for CSV writing
        """
        csv_row = {
            'token': quote.get('token'),
            'symbol': quote.get('symbol'),
            'symbol_name': quote.get('symbol_name', ''),
            'expiry': quote.get('expiry', ''),
            'option_type': quote.get('option_type', ''),
            'strike': quote.get('strike', ''),
            'timestamp': quote.get('timestamp'),
            'open': quote.get('open'),
            'high': quote.get('high'),
            'low': quote.get('low'),
            'close': quote.get('close'),
            'ltp': quote.get('ltp'),
            'volume': quote.get('volume'),
            'prev_close': quote.get('prev_close')
        }
        
        # Flatten bid levels
        bid_levels = quote.get('bid_levels', [])
        if bid_levels:
            csv_row['bid_prices'] = ','.join(str(level['price']) for level in bid_levels)
            csv_row['bid_qtys'] = ','.join(str(level['quantity']) for level in bid_levels)
            csv_row['bid_orders'] = ','.join(str(level.get('flag', 0)) for level in bid_levels)
        else:
            csv_row['bid_prices'] = ''
            csv_row['bid_qtys'] = ''
            csv_row['bid_orders'] = ''
        
        # Flatten ask levels
        ask_levels = quote.get('ask_levels', [])
        if ask_levels:
            csv_row['ask_prices'] = ','.join(str(level['price']) for level in ask_levels)
            csv_row['ask_qtys'] = ','.join(str(level['quantity']) for level in ask_levels)
            csv_row['ask_orders'] = ','.join(str(level.get('flag', 0)) for level in ask_levels)
        else:
            csv_row['ask_prices'] = ''
            csv_row['ask_qtys'] = ''
            csv_row['ask_orders'] = ''
        
        return csv_row
    
    def save_quotes(self, quotes: List[Dict], save_json: bool = True, save_csv: bool = True) -> bool:
        """
        Save quotes to both JSON and CSV (convenience method).
        
        Args:
            quotes: List of normalized quote dictionaries
            save_json: If True, save to JSON file
            save_csv: If True, save to CSV file
        
        Returns:
            True if all enabled saves successful, False if any failed
        """
        success = True
        
        if save_json:
            if not self.save_to_json(quotes):
                success = False
        
        if save_csv:
            if not self.save_to_csv(quotes):
                success = False
        
        return success
    
    def get_stats(self) -> dict:
        """
        Get saver statistics.
        
        Returns:
            Dictionary with counts:
            - json_files_saved: Number of JSON save operations
            - csv_files_saved: Number of CSV save operations
            - quotes_written_json: Total quotes written to JSON
            - quotes_written_csv: Total quotes written to CSV
            - io_errors: File I/O errors encountered
        """
        return self.stats.copy()
    
    def reset_stats(self):
        """Reset all statistics counters to zero."""
        for key in self.stats:
            self.stats[key] = 0
        logger.debug("DataSaver stats reset")
    
    def save_json(self, quotes: List[Dict], filename: str = None):
        """Placeholder for JSON saving."""
        raise NotImplementedError("DataSaver.save_json() - Coming in Phase 2")
    
    def save_csv(self, quotes: List[Dict], filename: str = None):
        """Placeholder for CSV saving."""
        raise NotImplementedError("DataSaver.save_csv() - Coming in Phase 2")
    
    def save_raw_packet(self, packet: bytes, metadata: dict = None):
        """Placeholder for raw packet saving."""
        raise NotImplementedError("DataSaver.save_raw_packet() - Coming in Phase 2")


# Placeholder - to be implemented in Phase 2
if __name__ == '__main__':
    print("DataSaver module - Placeholder for Phase 2 implementation")
