"""
BSE Dual Feed with Full Order Book
===================================

Tests simultaneous reception from:
- Port 26001: Equity Cash (CM) - Stocks like RELIANCE, TCS
- Port 26002: Derivatives (F&O) - SENSEX/BANKEX Options/Futures

Includes FULL ORDER BOOK (Best 5 Bid/Ask levels):
- bid_prices, bid_qtys, bid_orders
- ask_prices, ask_qtys, ask_orders

Saves both to separate CSV files with all fields.
"""

import sys
import socket
import struct
import csv
import threading
import queue
from pathlib import Path
from datetime import datetime
from typing import Dict, List, Optional

# Add src to path
sys.path.insert(0, str(Path(__file__).parent))

from equity_cash_fetcher import BSEEquityCashFetcher
from contract_manager import BSEContractManager

# Global queues for data collection
cm_queue = queue.Queue()
fno_queue = queue.Queue()

# Stop flag
stop_flag = threading.Event()

# Network config
MULTICAST_IP = "239.1.2.5"
CM_PORT = 26001   # Equity Cash
FNO_PORT = 26002  # Derivatives
API_URL = "http://192.168.102.166:2060/v1/sftp-files?api_type=erp"


def create_multicast_socket(multicast_ip: str, port: int) -> socket.socket:
    """Create UDP multicast socket."""
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    sock.bind(('', port))
    
    # Join multicast group
    mreq = struct.pack("4sl", socket.inet_aton(multicast_ip), socket.INADDR_ANY)
    sock.setsockopt(socket.IPPROTO_IP, socket.IP_ADD_MEMBERSHIP, mreq)
    
    # Set timeout for graceful shutdown
    sock.settimeout(1.0)
    
    return sock


def decode_header(packet: bytes) -> dict:
    """Decode packet header."""
    header = {}
    header['format_id'] = struct.unpack('>H', packet[4:6])[0]
    header['msg_type'] = struct.unpack('<H', packet[8:10])[0]
    
    hour = struct.unpack('<H', packet[20:22])[0]
    minute = struct.unpack('<H', packet[22:24])[0]
    second = struct.unpack('<H', packet[24:26])[0]
    ms = struct.unpack('<H', packet[26:28])[0]
    
    if hour < 24 and minute < 60 and second < 60:
        header['timestamp'] = f"{hour:02d}:{minute:02d}:{second:02d}.{ms:03d}"
    else:
        header['timestamp'] = datetime.now().strftime('%H:%M:%S.%f')[:-3]
    
    return header


def parse_order_book(record_bytes: bytes) -> Dict:
    """
    Parse 5-level order book depth (bid and ask).
    
    Structure:
    - Starts at offset 104
    - INTERLEAVED: Bid1, Ask1, Bid2, Ask2, ...
    - Each level: [Price 4B][Qty 4B][Orders 4B][Unknown 4B] = 16 bytes
    - Total: 5 levels × 32 bytes = 160 bytes (offset 104-263)
    """
    order_book = {
        'bid_prices': [],
        'bid_qtys': [],
        'bid_orders': [],
        'ask_prices': [],
        'ask_qtys': [],
        'ask_orders': []
    }
    
    if len(record_bytes) < 264:
        return order_book
    
    for i in range(5):
        bid_base = 104 + (i * 32)
        ask_base = bid_base + 16
        
        # Parse BID
        bid_price = struct.unpack('<i', record_bytes[bid_base:bid_base+4])[0] / 100.0
        bid_qty = struct.unpack('<i', record_bytes[bid_base+4:bid_base+8])[0]
        bid_orders = struct.unpack('<i', record_bytes[bid_base+8:bid_base+12])[0]
        
        # Parse ASK
        ask_price = struct.unpack('<i', record_bytes[ask_base:ask_base+4])[0] / 100.0
        ask_qty = struct.unpack('<i', record_bytes[ask_base+4:ask_base+8])[0]
        ask_orders = struct.unpack('<i', record_bytes[ask_base+8:ask_base+12])[0]
        
        if bid_qty > 0:
            order_book['bid_prices'].append(str(bid_price))
            order_book['bid_qtys'].append(str(bid_qty))
            order_book['bid_orders'].append(str(bid_orders))
        
        if ask_qty > 0:
            order_book['ask_prices'].append(str(ask_price))
            order_book['ask_qtys'].append(str(ask_qty))
            order_book['ask_orders'].append(str(ask_orders))
    
    return order_book


def decode_record_full(record_bytes: bytes) -> dict:
    """Decode 264-byte record with full order book."""
    data = {}
    data['token'] = struct.unpack('<I', record_bytes[0:4])[0]
    data['open'] = struct.unpack('<i', record_bytes[4:8])[0] / 100.0
    data['prev_close'] = struct.unpack('<i', record_bytes[8:12])[0] / 100.0
    data['high'] = struct.unpack('<i', record_bytes[12:16])[0] / 100.0
    data['low'] = struct.unpack('<i', record_bytes[16:20])[0] / 100.0
    data['volume'] = struct.unpack('<i', record_bytes[24:28])[0]
    data['turnover_lakhs'] = struct.unpack('<I', record_bytes[28:32])[0]
    data['lot_size'] = struct.unpack('<I', record_bytes[32:36])[0]
    data['ltp'] = struct.unpack('<i', record_bytes[36:40])[0] / 100.0
    data['atp'] = struct.unpack('<i', record_bytes[84:88])[0] / 100.0 if len(record_bytes) >= 88 else 0
    
    # Parse order book
    data['order_book'] = parse_order_book(record_bytes)
    
    return data


def cm_receiver_thread(multicast_ip: str, port: int, token_map: Dict):
    """Thread to receive CM (Equity Cash) data from port 26001."""
    print(f"📡 CM Thread: Starting on {multicast_ip}:{port}")
    
    try:
        sock = create_multicast_socket(multicast_ip, port)
    except Exception as e:
        print(f"❌ CM Thread: Socket error - {e}")
        return
    
    packets_received = 0
    records_decoded = 0
    
    try:
        while not stop_flag.is_set():
            try:
                packet, addr = sock.recvfrom(2048)
                packets_received += 1
                
                header = decode_header(packet)
                packet_size = len(packet)
                header_size = 36
                payload_size = packet_size - header_size
                
                if payload_size % 264 != 0:
                    continue
                
                record_size = 264
                num_records = payload_size // 264
                
                for i in range(num_records):
                    offset = header_size + (i * record_size)
                    record_bytes = packet[offset:offset+record_size]
                    record = decode_record_full(record_bytes)
                    
                    token = record['token']
                    if token == 0 or token == 1:
                        continue
                    
                    # Get contract info
                    contract = token_map.get(str(token), {})
                    ob = record['order_book']
                    
                    # Build quote with order book
                    quote = {
                        'timestamp': datetime.now().strftime('%Y-%m-%d') + ' ' + header['timestamp'],
                        'token': token,
                        'symbol': contract.get('symbol', f'TOKEN_{token}'),
                        'company': contract.get('company_name', '')[:30] if contract.get('company_name') else '',
                        'isin': contract.get('isin', ''),
                        'ltp': record['ltp'],
                        'open': record['open'],
                        'high': record['high'],
                        'low': record['low'],
                        'prev_close': record['prev_close'],
                        'atp': record['atp'],
                        'volume': record['volume'],
                        'turnover_lakhs': record['turnover_lakhs'],
                        # Order book fields
                        'bid_prices': '|'.join(ob['bid_prices']),
                        'bid_qtys': '|'.join(ob['bid_qtys']),
                        'bid_orders': '|'.join(ob['bid_orders']),
                        'ask_prices': '|'.join(ob['ask_prices']),
                        'ask_qtys': '|'.join(ob['ask_qtys']),
                        'ask_orders': '|'.join(ob['ask_orders'])
                    }
                    
                    cm_queue.put(quote)
                    records_decoded += 1
                
            except socket.timeout:
                continue
            except Exception as e:
                print(f"❌ CM Thread Error: {e}")
                break
                
    finally:
        sock.close()
        print(f"📡 CM Thread: Stopped - {packets_received} packets, {records_decoded} records")


def fno_receiver_thread(multicast_ip: str, port: int, token_map: Dict):
    """Thread to receive F&O (Derivatives) data from port 26002."""
    print(f"📡 F&O Thread: Starting on {multicast_ip}:{port}")
    
    try:
        sock = create_multicast_socket(multicast_ip, port)
    except Exception as e:
        print(f"❌ F&O Thread: Socket error - {e}")
        return
    
    packets_received = 0
    records_decoded = 0
    
    try:
        while not stop_flag.is_set():
            try:
                packet, addr = sock.recvfrom(2048)
                packets_received += 1
                
                header = decode_header(packet)
                packet_size = len(packet)
                header_size = 36
                payload_size = packet_size - header_size
                
                if payload_size % 264 != 0:
                    continue
                
                record_size = 264
                num_records = payload_size // 264
                
                for i in range(num_records):
                    offset = header_size + (i * record_size)
                    record_bytes = packet[offset:offset+record_size]
                    record = decode_record_full(record_bytes)
                    
                    token = record['token']
                    if token == 0 or token == 1:
                        continue
                    
                    # Get contract info
                    contract = token_map.get(str(token), {})
                    ob = record['order_book']
                    
                    # Build quote with order book
                    quote = {
                        'timestamp': datetime.now().strftime('%Y-%m-%d') + ' ' + header['timestamp'],
                        'token': token,
                        'symbol': contract.get('symbol', 'UNKNOWN'),
                        'instrument_name': contract.get('instrument_name', ''),
                        'expiry': contract.get('expiry', ''),
                        'strike': contract.get('strike', ''),
                        'option_type': contract.get('option_type', ''),
                        'ltp': record['ltp'],
                        'open': record['open'],
                        'high': record['high'],
                        'low': record['low'],
                        'prev_close': record['prev_close'],
                        'atp': record['atp'],
                        'volume': record['volume'],
                        'turnover_lakhs': record['turnover_lakhs'],
                        # Order book fields
                        'bid_prices': '|'.join(ob['bid_prices']),
                        'bid_qtys': '|'.join(ob['bid_qtys']),
                        'bid_orders': '|'.join(ob['bid_orders']),
                        'ask_prices': '|'.join(ob['ask_prices']),
                        'ask_qtys': '|'.join(ob['ask_qtys']),
                        'ask_orders': '|'.join(ob['ask_orders'])
                    }
                    
                    fno_queue.put(quote)
                    records_decoded += 1
                
            except socket.timeout:
                continue
            except Exception as e:
                print(f"❌ F&O Thread Error: {e}")
                break
                
    finally:
        sock.close()
        print(f"📡 F&O Thread: Stopped - {packets_received} packets, {records_decoded} records")


def csv_writer_thread(output_dir: Path, segment: str, data_queue: queue.Queue, fieldnames: List[str]):
    """Thread to write quotes to CSV."""
    today = datetime.now().strftime('%Y%m%d')
    csv_path = output_dir / f"{today}_{segment}_quotes_full.csv"
    
    quotes_written = 0
    buffer = []
    
    try:
        with open(csv_path, 'w', newline='', encoding='utf-8') as f:
            writer = csv.DictWriter(f, fieldnames=fieldnames)
            writer.writeheader()
            
            while not stop_flag.is_set():
                try:
                    # Get quote with timeout
                    quote = data_queue.get(timeout=1.0)
                    buffer.append(quote)
                    
                    # Write in batches
                    if len(buffer) >= 100:
                        writer.writerows(buffer)
                        quotes_written += len(buffer)
                        f.flush()
                        print(f"💾 {segment}: Saved {quotes_written} quotes to {csv_path.name}")
                        buffer = []
                        
                except queue.Empty:
                    # Flush any remaining
                    if buffer:
                        writer.writerows(buffer)
                        quotes_written += len(buffer)
                        f.flush()
                        buffer = []
                    continue
            
            # Final flush
            if buffer:
                writer.writerows(buffer)
                quotes_written += len(buffer)
                print(f"💾 {segment}: Final save - {len(buffer)} quotes")
                
    except Exception as e:
        print(f"❌ {segment} Writer Error: {e}")
    
    return quotes_written


def main():
    print("="*80)
    print("BSE DUAL FEED - FULL ORDER BOOK")
    print("="*80)
    print(f"Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    
    print(f"\n📡 Configuration:")
    print(f"   CM Port:  {CM_PORT} (Equity Cash - Stocks)")
    print(f"   F&O Port: {FNO_PORT} (Derivatives - Options/Futures)")
    
    # Output directory
    output_dir = Path("data/processed_csv")
    output_dir.mkdir(parents=True, exist_ok=True)
    print(f"   Output:   {output_dir}/")
    
    # Load CM token map
    print("\n📋 Loading CM Token Map (BhavCopy)...")
    cm_fetcher = BSEEquityCashFetcher()
    cm_csv = cm_fetcher.fetch_bhavcopy()
    if cm_csv:
        cm_fetcher.parse_bhavcopy(cm_csv)
    cm_tokens = cm_fetcher.contracts
    print(f"   ✅ CM Tokens: {len(cm_tokens):,}")
    
    # Load F&O token map
    print("\n📋 Loading F&O Token Map (Contract Master)...")
    fno_manager = BSEContractManager(API_URL)
    fno_fetcher = fno_manager.load_latest_contracts()
    fno_tokens = fno_fetcher.contracts
    print(f"   ✅ F&O Tokens: {len(fno_tokens):,}")
    
    # CSV field names
    cm_fields = [
        'timestamp', 'token', 'symbol', 'company', 'isin',
        'ltp', 'open', 'high', 'low', 'prev_close', 'atp',
        'volume', 'turnover_lakhs',
        'bid_prices', 'bid_qtys', 'bid_orders',
        'ask_prices', 'ask_qtys', 'ask_orders'
    ]
    
    fno_fields = [
        'timestamp', 'token', 'symbol', 'instrument_name', 'expiry', 'strike', 'option_type',
        'ltp', 'open', 'high', 'low', 'prev_close', 'atp',
        'volume', 'turnover_lakhs',
        'bid_prices', 'bid_qtys', 'bid_orders',
        'ask_prices', 'ask_qtys', 'ask_orders'
    ]
    
    print("\n" + "="*80)
    print("🚀 STARTING DUAL FEED WITH FULL ORDER BOOK...")
    print("="*80)
    print("Press Ctrl+C to stop\n")
    
    # Start threads
    threads = []
    
    # CM receiver
    cm_thread = threading.Thread(
        target=cm_receiver_thread,
        args=(MULTICAST_IP, CM_PORT, cm_tokens)
    )
    threads.append(cm_thread)
    
    # F&O receiver
    fno_thread = threading.Thread(
        target=fno_receiver_thread,
        args=(MULTICAST_IP, FNO_PORT, fno_tokens)
    )
    threads.append(fno_thread)
    
    # CM writer
    cm_writer = threading.Thread(
        target=csv_writer_thread,
        args=(output_dir, "CM", cm_queue, cm_fields)
    )
    threads.append(cm_writer)
    
    # F&O writer
    fno_writer = threading.Thread(
        target=csv_writer_thread,
        args=(output_dir, "FNO", fno_queue, fno_fields)
    )
    threads.append(fno_writer)
    
    # Start all
    for t in threads:
        t.start()
    
    # Run for 30 seconds
    start_time = datetime.now()
    try:
        while (datetime.now() - start_time).seconds < 30:
            # Print status every 5 seconds
            import time
            time.sleep(5)
            print(f"📊 Queues: CM={cm_queue.qsize()} | F&O={fno_queue.qsize()}")
    except KeyboardInterrupt:
        print("\n⏹ Stopping...")
    
    # Stop all
    stop_flag.set()
    
    # Wait for threads
    for t in threads:
        t.join(timeout=5)
    
    # Summary
    print("\n" + "="*80)
    print("📊 SUMMARY")
    print("="*80)
    
    today = datetime.now().strftime('%Y%m%d')
    cm_csv = output_dir / f"{today}_CM_quotes_full.csv"
    fno_csv = output_dir / f"{today}_FNO_quotes_full.csv"
    
    if cm_csv.exists():
        with open(cm_csv, 'r') as f:
            cm_count = sum(1 for _ in f) - 1
        print(f"CM Quotes:  {cm_count:,}")
    
    if fno_csv.exists():
        with open(fno_csv, 'r') as f:
            fno_count = sum(1 for _ in f) - 1
        print(f"F&O Quotes: {fno_count:,}")
    
    print(f"\nOutput Files:")
    print(f"   CM:  {cm_csv}")
    print(f"   F&O: {fno_csv}")
    
    print("\n" + "="*80)
    print("✅ DUAL FEED COMPLETE")
    print("="*80)


if __name__ == "__main__":
    main()
