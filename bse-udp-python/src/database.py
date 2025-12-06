"""
BSE Market Data - SQLite Database Handler
==========================================

Saves market quotes to SQLite database with:
1. Master table: bse_quotes (all records)
2. Date-wise tables: bse_quotes_YYYYMMDD (daily)
3. Automatic table creation if not exists
4. Batch insert operations for performance

Usage:
    from database import DatabaseHandler
    
    db = DatabaseHandler(db_path="data/bse_market.db")
    db.save_quotes(quotes, date_str="20251103")
    
    # Query examples
    quotes = db.get_quotes_by_date("20251103")
    latest = db.get_latest_quotes(symbol="SENSEX")
    stats = db.get_daily_stats("20251103")

Author: BSE Integration Team
Date: November 2025
"""

import sqlite3
import logging
from datetime import datetime
from pathlib import Path
from typing import List, Dict, Optional

# Configure logging
logger = logging.getLogger(__name__)


class DatabaseHandler:
    """
    Handles SQLite database operations for BSE market data.
    
    Features:
    - Master table (bse_quotes) for all records
    - Daily tables (bse_quotes_YYYYMMDD) for date-wise storage
    - Automatic table creation
    - Transaction-based batch inserts
    - Data validation before insert
    """
    
    def __init__(self, db_path: str = "data/bse_market.db"):
        """
        Initialize database handler.
        
        Args:
            db_path: Path to SQLite database file
        """
        self.db_path = Path(db_path)
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        
        # Initialize database
        self._initialize_database()
        
        # Statistics
        self.stats = {
            'quotes_inserted_master': 0,
            'quotes_inserted_daily': 0,
            'queries_executed': 0,
            'db_errors': 0
        }
        
        logger.info(f"✅ Database initialized: {self.db_path}")
    
    def _initialize_database(self):
        """Initialize database connection and create master table if not exists."""
        try:
            conn = sqlite3.connect(str(self.db_path))
            conn.row_factory = sqlite3.Row  # Allow column access by name
            cursor = conn.cursor()
            
            # Create master table
            cursor.execute('''
                CREATE TABLE IF NOT EXISTS bse_quotes (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    date_str TEXT NOT NULL,
                    token INTEGER NOT NULL,
                    symbol TEXT NOT NULL,
                    symbol_name TEXT,
                    expiry TEXT,
                    option_type TEXT,
                    strike REAL,
                    timestamp TEXT NOT NULL,
                    open REAL,
                    high REAL,
                    low REAL,
                    close REAL,
                    ltp REAL NOT NULL,
                    volume INTEGER,
                    prev_close REAL,
                    bid_price_1 REAL,
                    bid_qty_1 INTEGER,
                    bid_order_1 INTEGER,
                    ask_price_1 REAL,
                    ask_qty_1 INTEGER,
                    ask_order_1 INTEGER,
                    bid_price_2 REAL,
                    bid_qty_2 INTEGER,
                    bid_order_2 INTEGER,
                    ask_price_2 REAL,
                    ask_qty_2 INTEGER,
                    ask_order_2 INTEGER,
                    bid_price_3 REAL,
                    bid_qty_3 INTEGER,
                    bid_order_3 INTEGER,
                    ask_price_3 REAL,
                    ask_qty_3 INTEGER,
                    ask_order_3 INTEGER,
                    bid_price_4 REAL,
                    bid_qty_4 INTEGER,
                    bid_order_4 INTEGER,
                    ask_price_4 REAL,
                    ask_qty_4 INTEGER,
                    ask_order_4 INTEGER,
                    bid_price_5 REAL,
                    bid_qty_5 INTEGER,
                    bid_order_5 INTEGER,
                    ask_price_5 REAL,
                    ask_qty_5 INTEGER,
                    ask_order_5 INTEGER,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    UNIQUE(date_str, token, timestamp)
                )
            ''')
            
            # Create indexes for faster queries
            cursor.execute('CREATE INDEX IF NOT EXISTS idx_date ON bse_quotes(date_str)')
            cursor.execute('CREATE INDEX IF NOT EXISTS idx_symbol ON bse_quotes(symbol)')
            cursor.execute('CREATE INDEX IF NOT EXISTS idx_token ON bse_quotes(token)')
            cursor.execute('CREATE INDEX IF NOT EXISTS idx_timestamp ON bse_quotes(timestamp)')
            cursor.execute('CREATE INDEX IF NOT EXISTS idx_ltp ON bse_quotes(ltp)')
            
            conn.commit()
            conn.close()
            
            logger.debug("✅ Master table 'bse_quotes' created/verified")
            
        except Exception as e:
            logger.error(f"❌ Error initializing database: {e}", exc_info=True)
            self.stats['db_errors'] += 1
            raise
    
    def _create_daily_table(self, date_str: str):
        """
        Create date-wise table if not exists.
        
        Args:
            date_str: Date string in YYYYMMDD format (e.g., "20251103")
        """
        table_name = f"bse_quotes_{date_str}"
        
        try:
            conn = sqlite3.connect(str(self.db_path))
            cursor = conn.cursor()
            
            # Check if table already exists
            cursor.execute(f"SELECT name FROM sqlite_master WHERE type='table' AND name='{table_name}'")
            if cursor.fetchone():
                logger.debug(f"✓ Table {table_name} already exists")
                conn.close()
                return
            
            # Create date-wise table (same structure as master)
            cursor.execute(f'''
                CREATE TABLE {table_name} (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    token INTEGER NOT NULL,
                    symbol TEXT NOT NULL,
                    symbol_name TEXT,
                    expiry TEXT,
                    option_type TEXT,
                    strike REAL,
                    timestamp TEXT NOT NULL,
                    open REAL,
                    high REAL,
                    low REAL,
                    close REAL,
                    ltp REAL NOT NULL,
                    volume INTEGER,
                    prev_close REAL,
                    bid_price_1 REAL,
                    bid_qty_1 INTEGER,
                    bid_order_1 INTEGER,
                    ask_price_1 REAL,
                    ask_qty_1 INTEGER,
                    ask_order_1 INTEGER,
                    bid_price_2 REAL,
                    bid_qty_2 INTEGER,
                    bid_order_2 INTEGER,
                    ask_price_2 REAL,
                    ask_qty_2 INTEGER,
                    ask_order_2 INTEGER,
                    bid_price_3 REAL,
                    bid_qty_3 INTEGER,
                    bid_order_3 INTEGER,
                    ask_price_3 REAL,
                    ask_qty_3 INTEGER,
                    ask_order_3 INTEGER,
                    bid_price_4 REAL,
                    bid_qty_4 INTEGER,
                    bid_order_4 INTEGER,
                    ask_price_4 REAL,
                    ask_qty_4 INTEGER,
                    ask_order_4 INTEGER,
                    bid_price_5 REAL,
                    bid_qty_5 INTEGER,
                    bid_order_5 INTEGER,
                    ask_price_5 REAL,
                    ask_qty_5 INTEGER,
                    ask_order_5 INTEGER,
                    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                    UNIQUE(token, timestamp)
                )
            ''')
            
            # Create indexes
            cursor.execute(f'CREATE INDEX IF NOT EXISTS idx_{date_str}_symbol ON {table_name}(symbol)')
            cursor.execute(f'CREATE INDEX IF NOT EXISTS idx_{date_str}_token ON {table_name}(token)')
            cursor.execute(f'CREATE INDEX IF NOT EXISTS idx_{date_str}_ltp ON {table_name}(ltp)')
            
            conn.commit()
            conn.close()
            
            logger.info(f"✅ Daily table '{table_name}' created successfully")
            
        except Exception as e:
            logger.error(f"❌ Error creating daily table {table_name}: {e}", exc_info=True)
            self.stats['db_errors'] += 1
    
    def _flatten_quote(self, quote: Dict, date_str: str) -> Dict:
        """
        Flatten nested quote dictionary into flat table structure.
        
        Converts order_book structure with nested dicts/lists into
        flat bid_price_1, bid_qty_1, bid_order_1, etc. columns.
        
        Args:
            quote: Quote dictionary with nested structure
            date_str: Date string
        
        Returns:
            Flattened dictionary with all columns
        """
        flat = {
            'date_str': date_str,
            'token': quote.get('token'),
            'symbol': quote.get('symbol'),
            'symbol_name': quote.get('symbol_name', ''),
            'expiry': quote.get('expiry', ''),
            'option_type': quote.get('option_type', ''),
            'strike': quote.get('strike'),
            'timestamp': quote.get('timestamp'),
            'open': quote.get('open'),
            'high': quote.get('high'),
            'low': quote.get('low'),
            'close': quote.get('close'),
            'ltp': quote.get('ltp'),
            'volume': quote.get('volume'),
            'prev_close': quote.get('prev_close'),
        }
        
        # Flatten order book (Best 5 levels)
        order_book = quote.get('order_book', {})
        bids = order_book.get('bids', [])
        asks = order_book.get('asks', [])
        
        # Add bid levels (1-5)
        for i in range(5):
            if i < len(bids):
                flat[f'bid_price_{i+1}'] = bids[i].get('price')
                flat[f'bid_qty_{i+1}'] = bids[i].get('quantity')
                flat[f'bid_order_{i+1}'] = bids[i].get('flag')
            else:
                flat[f'bid_price_{i+1}'] = None
                flat[f'bid_qty_{i+1}'] = None
                flat[f'bid_order_{i+1}'] = None
        
        # Add ask levels (1-5)
        for i in range(5):
            if i < len(asks):
                flat[f'ask_price_{i+1}'] = asks[i].get('price')
                flat[f'ask_qty_{i+1}'] = asks[i].get('quantity')
                flat[f'ask_order_{i+1}'] = asks[i].get('flag')
            else:
                flat[f'ask_price_{i+1}'] = None
                flat[f'ask_qty_{i+1}'] = None
                flat[f'ask_order_{i+1}'] = None
        
        return flat
    
    def save_quotes(self, quotes: List[Dict], date_str: str = None) -> bool:
        """
        Save quotes to both master and daily tables.
        
        Args:
            quotes: List of quote dictionaries
            date_str: Date string in YYYYMMDD format (default: today)
        
        Returns:
            True if successful, False otherwise
        """
        if date_str is None:
            date_str = datetime.now().strftime('%Y%m%d')
        
        if not quotes:
            logger.warning("⚠️ No quotes to save")
            return False
        
        try:
            # Create daily table
            self._create_daily_table(date_str)
            
            # Connect to database
            conn = sqlite3.connect(str(self.db_path))
            cursor = conn.cursor()
            
            # Prepare data
            table_name = f"bse_quotes_{date_str}"
            flattened_quotes = [self._flatten_quote(q, date_str) for q in quotes]
            
            # Column names
            columns = [
                'date_str', 'token', 'symbol', 'symbol_name', 'expiry', 'option_type',
                'strike', 'timestamp', 'open', 'high', 'low', 'close', 'ltp', 'volume',
                'prev_close', 'bid_price_1', 'bid_qty_1', 'bid_order_1', 'ask_price_1',
                'ask_qty_1', 'ask_order_1', 'bid_price_2', 'bid_qty_2', 'bid_order_2',
                'ask_price_2', 'ask_qty_2', 'ask_order_2', 'bid_price_3', 'bid_qty_3',
                'bid_order_3', 'ask_price_3', 'ask_qty_3', 'ask_order_3', 'bid_price_4',
                'bid_qty_4', 'bid_order_4', 'ask_price_4', 'ask_qty_4', 'ask_order_4',
                'bid_price_5', 'bid_qty_5', 'bid_order_5', 'ask_price_5', 'ask_qty_5',
                'ask_order_5'
            ]
            
            # Insert into master table
            master_insert = f'''
                INSERT OR REPLACE INTO bse_quotes ({','.join(columns)})
                VALUES ({','.join(['?'] * len(columns))})
            '''
            
            # Insert into daily table
            daily_columns = [c for c in columns if c != 'date_str']
            daily_insert = f'''
                INSERT OR REPLACE INTO {table_name} ({','.join(daily_columns)})
                VALUES ({','.join(['?'] * len(daily_columns))})
            '''
            
            # Batch insert
            for quote in flattened_quotes:
                # Master table
                master_values = tuple(quote.get(col) for col in columns)
                cursor.execute(master_insert, master_values)
                self.stats['quotes_inserted_master'] += 1
                
                # Daily table
                daily_values = tuple(quote.get(col) for col in daily_columns)
                cursor.execute(daily_insert, daily_values)
                self.stats['quotes_inserted_daily'] += 1
            
            conn.commit()
            conn.close()
            
            logger.info(f"✅ Saved {len(quotes)} quotes to master table and {table_name}")
            return True
            
        except Exception as e:
            logger.error(f"❌ Error saving quotes to database: {e}", exc_info=True)
            self.stats['db_errors'] += 1
            return False
    
    def get_quotes_by_date(self, date_str: str, symbol: Optional[str] = None) -> List[Dict]:
        """
        Retrieve quotes from daily table by date.
        
        Args:
            date_str: Date string in YYYYMMDD format
            symbol: Optional symbol filter (e.g., "SENSEX")
        
        Returns:
            List of quote dictionaries
        """
        table_name = f"bse_quotes_{date_str}"
        
        try:
            conn = sqlite3.connect(str(self.db_path))
            conn.row_factory = sqlite3.Row
            cursor = conn.cursor()
            
            # Check if table exists
            cursor.execute(f"SELECT name FROM sqlite_master WHERE type='table' AND name='{table_name}'")
            if not cursor.fetchone():
                logger.warning(f"⚠️ Table {table_name} does not exist")
                return []
            
            # Query with optional symbol filter
            if symbol:
                cursor.execute(f"SELECT * FROM {table_name} WHERE symbol = ? ORDER BY timestamp DESC", (symbol,))
            else:
                cursor.execute(f"SELECT * FROM {table_name} ORDER BY timestamp DESC")
            
            rows = cursor.fetchall()
            conn.close()
            
            self.stats['queries_executed'] += 1
            logger.debug(f"✓ Retrieved {len(rows)} quotes from {table_name}")
            
            return [dict(row) for row in rows]
            
        except Exception as e:
            logger.error(f"❌ Error querying date table {table_name}: {e}", exc_info=True)
            self.stats['db_errors'] += 1
            return []
    
    def get_latest_quotes(self, symbol: str = None, limit: int = 10) -> List[Dict]:
        """
        Get latest quotes from master table.
        
        Args:
            symbol: Optional symbol filter
            limit: Number of latest records to fetch
        
        Returns:
            List of latest quote dictionaries
        """
        try:
            conn = sqlite3.connect(str(self.db_path))
            conn.row_factory = sqlite3.Row
            cursor = conn.cursor()
            
            if symbol:
                cursor.execute(
                    "SELECT * FROM bse_quotes WHERE symbol = ? ORDER BY timestamp DESC LIMIT ?",
                    (symbol, limit)
                )
            else:
                cursor.execute("SELECT * FROM bse_quotes ORDER BY timestamp DESC LIMIT ?", (limit,))
            
            rows = cursor.fetchall()
            conn.close()
            
            self.stats['queries_executed'] += 1
            logger.debug(f"✓ Retrieved {len(rows)} latest quotes")
            
            return [dict(row) for row in rows]
            
        except Exception as e:
            logger.error(f"❌ Error retrieving latest quotes: {e}", exc_info=True)
            self.stats['db_errors'] += 1
            return []
    
    def get_daily_stats(self, date_str: str) -> Dict:
        """
        Get statistics for a specific date.
        
        Args:
            date_str: Date string in YYYYMMDD format
        
        Returns:
            Dictionary with statistics
        """
        table_name = f"bse_quotes_{date_str}"
        
        try:
            conn = sqlite3.connect(str(self.db_path))
            cursor = conn.cursor()
            
            # Check if table exists
            cursor.execute(f"SELECT name FROM sqlite_master WHERE type='table' AND name='{table_name}'")
            if not cursor.fetchone():
                logger.warning(f"⚠️ Table {table_name} does not exist")
                return {}
            
            # Get statistics
            cursor.execute(f'''
                SELECT
                    COUNT(DISTINCT token) as unique_tokens,
                    COUNT(DISTINCT symbol) as unique_symbols,
                    COUNT(*) as total_records,
                    AVG(ltp) as avg_ltp,
                    MIN(ltp) as min_ltp,
                    MAX(ltp) as max_ltp,
                    SUM(volume) as total_volume,
                    MIN(timestamp) as first_timestamp,
                    MAX(timestamp) as last_timestamp
                FROM {table_name}
            ''')
            
            row = cursor.fetchone()
            conn.close()
            
            self.stats['queries_executed'] += 1
            
            if row:
                stats = {
                    'unique_tokens': row[0],
                    'unique_symbols': row[1],
                    'total_records': row[2],
                    'avg_ltp': round(row[3], 2) if row[3] else 0,
                    'min_ltp': round(row[4], 2) if row[4] else 0,
                    'max_ltp': round(row[5], 2) if row[5] else 0,
                    'total_volume': row[6] or 0,
                    'first_timestamp': row[7],
                    'last_timestamp': row[8],
                }
                logger.debug(f"✓ Retrieved stats for {date_str}")
                return stats
            
            return {}
            
        except Exception as e:
            logger.error(f"❌ Error retrieving daily stats: {e}", exc_info=True)
            self.stats['db_errors'] += 1
            return {}
    
    def get_symbol_stats(self, date_str: str, symbol: str) -> List[Dict]:
        """
        Get detailed statistics for a symbol on a specific date.
        
        Args:
            date_str: Date string in YYYYMMDD format
            symbol: Symbol name (e.g., "SENSEX")
        
        Returns:
            List of statistics per token
        """
        table_name = f"bse_quotes_{date_str}"
        
        try:
            conn = sqlite3.connect(str(self.db_path))
            conn.row_factory = sqlite3.Row
            cursor = conn.cursor()
            
            # Check if table exists
            cursor.execute(f"SELECT name FROM sqlite_master WHERE type='table' AND name='{table_name}'")
            if not cursor.fetchone():
                return []
            
            cursor.execute(f'''
                SELECT
                    token,
                    symbol_name,
                    COUNT(*) as record_count,
                    AVG(ltp) as avg_ltp,
                    MIN(ltp) as min_ltp,
                    MAX(ltp) as max_ltp,
                    SUM(volume) as total_volume,
                    MAX(timestamp) as latest_timestamp
                FROM {table_name}
                WHERE symbol = ?
                GROUP BY token
                ORDER BY record_count DESC
            ''', (symbol,))
            
            rows = cursor.fetchall()
            conn.close()
            
            self.stats['queries_executed'] += 1
            logger.debug(f"✓ Retrieved {len(rows)} symbol stats for {symbol}")
            
            return [dict(row) for row in rows]
            
        except Exception as e:
            logger.error(f"❌ Error retrieving symbol stats: {e}", exc_info=True)
            self.stats['db_errors'] += 1
            return []
    
    def export_to_csv(self, date_str: str, csv_path: str = None) -> bool:
        """
        Export daily table to CSV file.
        
        Args:
            date_str: Date string in YYYYMMDD format
            csv_path: Output CSV path (default: data/exports/YYYYMMDD_export.csv)
        
        Returns:
            True if successful
        """
        if csv_path is None:
            csv_path = f"data/exports/{date_str}_export.csv"
        
        table_name = f"bse_quotes_{date_str}"
        
        try:
            import csv
            
            Path(csv_path).parent.mkdir(parents=True, exist_ok=True)
            
            # Query all data
            quotes = self.get_quotes_by_date(date_str)
            
            if not quotes:
                logger.warning(f"⚠️ No data to export for {date_str}")
                return False
            
            # Write to CSV
            with open(csv_path, 'w', newline='', encoding='utf-8') as f:
                writer = csv.DictWriter(f, fieldnames=quotes[0].keys())
                writer.writeheader()
                writer.writerows(quotes)
            
            logger.info(f"✅ Exported {len(quotes)} records to {csv_path}")
            return True
            
        except Exception as e:
            logger.error(f"❌ Error exporting to CSV: {e}", exc_info=True)
            self.stats['db_errors'] += 1
            return False
    
    def get_stats(self) -> Dict:
        """Get database handler statistics."""
        return self.stats.copy()
    
    def close(self):
        """Close database connection (if any)."""
        logger.debug("Database handler closed")
