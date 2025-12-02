"""
BSE Live Token Monitor - Real-Time Tick Display
=================================================

Monitors specific tokens with LIVE tick-by-tick updates.
Shows real-time price changes and order book updates.

Usage:
- Reliance (CM): python test_live_token.py --token 500325 --port 26001 --ticks 50
- SENSEX Fut (F&O): python test_live_token.py --token 1102290 --port 26002 --ticks 50

Features:
- Live tick display with price change highlighting
- Per-token CSV file with all ticks
- Order book depth (Best 5 Bid/Ask)
- Simple token→symbol mapping (only symbol needed)
"""

import sys
import socket
import struct
import csv
import os
import argparse
from pathlib import Path
from datetime import datetime
from typing import Dict, Optional

# Add src to path (tests/ is sibling to src/)
sys.path.insert(0, str(Path(__file__).parent.parent / 'src'))

# ============================================================================
# SIMPLE TOKEN MAPPING - Only symbol needed
# ============================================================================

def load_cm_symbols() -> Dict[int, str]:
    """Load CM token → symbol mapping from BhavCopy (simple)."""
    from equity_cash_fetcher import BSEEquityCashFetcher
    
    fetcher = BSEEquityCashFetcher()
    csv_content = fetcher.fetch_bhavcopy()
    if csv_content:
        fetcher.parse_bhavcopy(csv_content)
    
    # Simple mapping: token → symbol only
    symbols = {}
    for token, info in fetcher.contracts.items():
        symbols[int(token)] = info.get('symbol', f'TOKEN_{token}')
    
    return symbols


def load_fno_symbols() -> Dict[int, str]:
    """Load F&O token → symbol mapping from local Contract CSV file."""
    import csv
    from pathlib import Path
    
    # Find the latest contract file in data/tokens/
    tokens_dir = Path(__file__).parent.parent / 'data' / 'tokens'
    
    # Look for BSE_EQD_CONTRACT_*.csv files (without _fetched suffix)
    pattern = "BSE_EQD_CONTRACT_*.csv"
    files = [f for f in tokens_dir.glob(pattern) if '_fetched' not in f.name]
    
    if not files:
        print(f"   ❌ No contract files found in {tokens_dir}")
        return {}
    
    # Sort to get latest (filename has date DDMMYYYY)
    latest_file = sorted(files)[-1]
    print(f"   📂 Loading from: {latest_file.name}")
    
    # Parse CSV - columns are:
    # 0: Token, 3: Symbol, 4: Expiry, 5: Strike, 6: Option Type (CE/PE)
    symbols = {}
    with open(latest_file, 'r', encoding='utf-8') as f:
        reader = csv.reader(f)
        for row in reader:
            if len(row) < 7:
                continue
            
            try:
                token = int(row[0])
                symbol = row[3]  # e.g., RELIANCE
                expiry = row[4]  # e.g., 24-DEC-2025
                strike = row[5]  # e.g., 154000 (in paise, divide by 100)
                opt_type = row[6]  # CE or PE
                
                # Format: RELIANCE 1540 PE 24-DEC
                if strike and opt_type:
                    # Convert strike from paise to rupees for display
                    strike_val = int(strike) // 100 if strike.isdigit() else strike
                    # Shorten expiry: 24-DEC-2025 → 24-DEC
                    short_expiry = '-'.join(expiry.split('-')[:2]) if expiry else ''
                    symbols[token] = f"{symbol} {strike_val} {opt_type} {short_expiry}"
                elif expiry:
                    short_expiry = '-'.join(expiry.split('-')[:2]) if expiry else ''
                    symbols[token] = f"{symbol} FUT {short_expiry}"
                else:
                    symbols[token] = symbol
                    
            except (ValueError, IndexError):
                continue
    
    return symbols


# ============================================================================
# PACKET DECODING
# ============================================================================

def create_multicast_socket(multicast_ip: str, port: int) -> socket.socket:
    """Create UDP multicast socket."""
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    sock.bind(('', port))
    
    mreq = struct.pack("4sl", socket.inet_aton(multicast_ip), socket.INADDR_ANY)
    sock.setsockopt(socket.IPPROTO_IP, socket.IP_ADD_MEMBERSHIP, mreq)
    sock.settimeout(0.1)  # Short timeout for responsive updates
    
    return sock


def parse_order_book(record_bytes: bytes) -> Dict:
    """Parse 5-level order book depth."""
    ob = {
        'bid_prices': [], 'bid_qtys': [], 'bid_orders': [],
        'ask_prices': [], 'ask_qtys': [], 'ask_orders': []
    }
    
    if len(record_bytes) < 264:
        return ob
    
    for i in range(5):
        bid_base = 104 + (i * 32)
        ask_base = bid_base + 16
        
        # BID
        bid_price = struct.unpack('<i', record_bytes[bid_base:bid_base+4])[0] / 100.0
        bid_qty = struct.unpack('<i', record_bytes[bid_base+4:bid_base+8])[0]
        bid_orders = struct.unpack('<i', record_bytes[bid_base+8:bid_base+12])[0]
        
        # ASK
        ask_price = struct.unpack('<i', record_bytes[ask_base:ask_base+4])[0] / 100.0
        ask_qty = struct.unpack('<i', record_bytes[ask_base+4:ask_base+8])[0]
        ask_orders = struct.unpack('<i', record_bytes[ask_base+8:ask_base+12])[0]
        
        if bid_qty > 0:
            ob['bid_prices'].append(bid_price)
            ob['bid_qtys'].append(bid_qty)
            ob['bid_orders'].append(bid_orders)
        
        if ask_qty > 0:
            ob['ask_prices'].append(ask_price)
            ob['ask_qtys'].append(ask_qty)
            ob['ask_orders'].append(ask_orders)
    
    return ob


def decode_record(record_bytes: bytes) -> Dict:
    """Decode 264-byte record with full market data."""
    return {
        'token': struct.unpack('<I', record_bytes[0:4])[0],
        'open': struct.unpack('<i', record_bytes[4:8])[0] / 100.0,
        'prev_close': struct.unpack('<i', record_bytes[8:12])[0] / 100.0,
        'high': struct.unpack('<i', record_bytes[12:16])[0] / 100.0,
        'low': struct.unpack('<i', record_bytes[16:20])[0] / 100.0,
        'volume': struct.unpack('<i', record_bytes[24:28])[0],
        'turnover_lakhs': struct.unpack('<I', record_bytes[28:32])[0],
        'lot_size': struct.unpack('<I', record_bytes[32:36])[0],
        'ltp': struct.unpack('<i', record_bytes[36:40])[0] / 100.0,
        'seq': struct.unpack('<I', record_bytes[44:48])[0] if len(record_bytes) >= 48 else 0,
        'atp': struct.unpack('<i', record_bytes[84:88])[0] / 100.0 if len(record_bytes) >= 88 else 0,
        'order_book': parse_order_book(record_bytes)
    }


# ============================================================================
# LIVE DISPLAY
# ============================================================================

def clear_screen():
    """Clear terminal screen."""
    os.system('cls' if os.name == 'nt' else 'clear')


def format_change(current: float, previous: float) -> str:
    """Format price change with color indicators."""
    if previous == 0:
        return "  NEW"
    
    change = current - previous
    pct = (change / previous) * 100
    
    if change > 0:
        return f" ▲ +{change:.2f} (+{pct:.2f}%)"
    elif change < 0:
        return f" ▼ {change:.2f} ({pct:.2f}%)"
    else:
        return f"   ━ 0.00 (0.00%)"


def display_tick(data: Dict, symbol: str, tick_num: int, prev_ltp: float):
    """Display single tick with live formatting."""
    now = datetime.now().strftime('%H:%M:%S.%f')[:-3]
    ob = data['order_book']
    
    # Build display
    change_str = format_change(data['ltp'], prev_ltp)
    
    print(f"\n{'═'*80}")
    print(f"  TICK #{tick_num:<6}  │  {now}  │  Token: {data['token']}  │  {symbol}")
    print(f"{'═'*80}")
    
    print(f"\n  💰 LTP: ₹{data['ltp']:,.2f}{change_str}")
    print(f"  ────────────────────────────────────────────────────────────────────────")
    print(f"  Open: ₹{data['open']:,.2f}  │  High: ₹{data['high']:,.2f}  │  Low: ₹{data['low']:,.2f}  │  Prev: ₹{data['prev_close']:,.2f}")
    print(f"  ATP:  ₹{data['atp']:,.2f}  │  Volume: {data['volume']:,}  │  Turnover: ₹{data['turnover_lakhs']:,}L  │  Seq: {data['seq']}")
    
    # Order Book
    print(f"\n  📚 ORDER BOOK")
    print(f"  {'─'*72}")
    print(f"  {'BID':^34} │ {'ASK':^34}")
    print(f"  {'Price':>12}  {'Qty':>8}  {'Ord':>5} │ {'Price':>12}  {'Qty':>8}  {'Ord':>5}")
    print(f"  {'─'*72}")
    
    max_levels = max(len(ob['bid_prices']), len(ob['ask_prices']), 5)
    for i in range(min(max_levels, 5)):
        bid_str = ""
        ask_str = ""
        
        if i < len(ob['bid_prices']):
            bid_str = f"₹{ob['bid_prices'][i]:>10,.2f}  {ob['bid_qtys'][i]:>8,}  {ob['bid_orders'][i]:>5}"
        else:
            bid_str = f"{'--':>12}  {'--':>8}  {'--':>5}"
        
        if i < len(ob['ask_prices']):
            ask_str = f"₹{ob['ask_prices'][i]:>10,.2f}  {ob['ask_qtys'][i]:>8,}  {ob['ask_orders'][i]:>5}"
        else:
            ask_str = f"{'--':>12}  {'--':>8}  {'--':>5}"
        
        print(f"  {bid_str} │ {ask_str}")
    
    print(f"  {'─'*72}")


# ============================================================================
# CSV WRITER
# ============================================================================

def create_csv_writer(token: int, symbol: str):
    """Create CSV file for token with headers."""
    today = datetime.now().strftime('%Y%m%d')
    safe_symbol = symbol.replace(' ', '_').replace('/', '-')[:20]
    filename = f"data/processed_csv/{today}_{token}_{safe_symbol}_ticks.csv"
    
    Path("data/processed_csv").mkdir(parents=True, exist_ok=True)
    
    fieldnames = [
        'timestamp', 'token', 'symbol',
        'ltp', 'open', 'high', 'low', 'prev_close', 'atp',
        'volume', 'turnover_lakhs', 'lot_size', 'seq',
        'bid_prices', 'bid_qtys', 'bid_orders',
        'ask_prices', 'ask_qtys', 'ask_orders'
    ]
    
    f = open(filename, 'w', newline='', encoding='utf-8')
    writer = csv.DictWriter(f, fieldnames=fieldnames)
    writer.writeheader()
    
    return f, writer, filename


def write_tick_to_csv(writer, data: Dict, symbol: str):
    """Write single tick to CSV."""
    ob = data['order_book']
    
    row = {
        'timestamp': datetime.now().strftime('%Y-%m-%d %H:%M:%S.%f')[:-3],
        'token': data['token'],
        'symbol': symbol,
        'ltp': data['ltp'],
        'open': data['open'],
        'high': data['high'],
        'low': data['low'],
        'prev_close': data['prev_close'],
        'atp': data['atp'],
        'volume': data['volume'],
        'turnover_lakhs': data['turnover_lakhs'],
        'lot_size': data['lot_size'],
        'seq': data['seq'],
        'bid_prices': '|'.join(str(p) for p in ob['bid_prices']),
        'bid_qtys': '|'.join(str(q) for q in ob['bid_qtys']),
        'bid_orders': '|'.join(str(o) for o in ob['bid_orders']),
        'ask_prices': '|'.join(str(p) for p in ob['ask_prices']),
        'ask_qtys': '|'.join(str(q) for q in ob['ask_qtys']),
        'ask_orders': '|'.join(str(o) for o in ob['ask_orders'])
    }
    
    writer.writerow(row)


# ============================================================================
# MAIN MONITOR
# ============================================================================

def monitor_token(token: int, port: int, multicast_ip: str = "239.1.2.5", max_ticks: int = 100):
    """Monitor a specific token with live display."""
    
    print("="*80)
    print("BSE LIVE TOKEN MONITOR")
    print("="*80)
    print(f"Token: {token}")
    print(f"Port:  {port} ({'CM - Equity' if port == 26001 else 'F&O - Derivatives'})")
    print(f"Time:  {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    
    # Load symbol mapping (simple - only symbol needed)
    print("\n📋 Loading symbol mapping...")
    if port == 26001:
        symbols = load_cm_symbols()
        print(f"   ✅ Loaded {len(symbols):,} CM symbols")
    else:
        symbols = load_fno_symbols()
        print(f"   ✅ Loaded {len(symbols):,} F&O symbols")
    
    symbol = symbols.get(token, f"TOKEN_{token}")
    print(f"   📊 Token {token} → {symbol}")
    
    # Create CSV file
    csv_file, csv_writer, csv_filename = create_csv_writer(token, symbol)
    print(f"\n💾 CSV File: {csv_filename}")
    
    # Connect
    print(f"\n📡 Connecting to {multicast_ip}:{port}...")
    sock = create_multicast_socket(multicast_ip, port)
    print("   ✅ Connected!")
    
    print(f"\n🚀 STARTING LIVE MONITOR (Press Ctrl+C to stop)")
    print(f"   Watching for token {token} ({symbol})")
    print(f"   Max ticks: {max_ticks}")
    print("="*80)
    
    tick_count = 0
    prev_ltp = 0.0
    last_seq = 0
    
    try:
        while tick_count < max_ticks:
            try:
                packet, _ = sock.recvfrom(2048)
                
                # Parse packet
                if len(packet) < 300:
                    continue
                
                num_records = (len(packet) - 36) // 264
                
                for i in range(num_records):
                    offset = 36 + (i * 264)
                    record_bytes = packet[offset:offset + 264]
                    
                    if len(record_bytes) < 264:
                        continue
                    
                    rec_token = struct.unpack('<I', record_bytes[0:4])[0]
                    
                    if rec_token == token:
                        data = decode_record(record_bytes)
                        
                        # Skip if same sequence (duplicate)
                        if data['seq'] == last_seq:
                            continue
                        
                        last_seq = data['seq']
                        tick_count += 1
                        
                        # Display live
                        display_tick(data, symbol, tick_count, prev_ltp)
                        prev_ltp = data['ltp']
                        
                        # Write to CSV
                        write_tick_to_csv(csv_writer, data, symbol)
                        csv_file.flush()
                        
                        print(f"\n  💾 Saved to CSV ({tick_count} ticks)")
                
            except socket.timeout:
                continue
                
    except KeyboardInterrupt:
        print("\n\n⏹ Stopped by user")
    
    finally:
        sock.close()
        csv_file.close()
        
        print("\n" + "="*80)
        print("📊 SUMMARY")
        print("="*80)
        print(f"Token:      {token}")
        print(f"Symbol:     {symbol}")
        print(f"Ticks:      {tick_count}")
        print(f"CSV File:   {csv_filename}")
        print("="*80)


# ============================================================================
# ENTRY POINT
# ============================================================================

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="BSE Live Token Monitor")
    parser.add_argument("--token", type=int, required=True, help="Token to monitor (e.g., 500325 for Reliance)")
    parser.add_argument("--port", type=int, default=26001, help="Port (26001=CM, 26002=F&O)")
    parser.add_argument("--ticks", type=int, default=100, help="Max ticks to capture")
    parser.add_argument("--ip", type=str, default="239.1.2.5", help="Multicast IP")
    
    args = parser.parse_args()
    
    monitor_token(
        token=args.token,
        port=args.port,
        multicast_ip=args.ip,
        max_ticks=args.ticks
    )
