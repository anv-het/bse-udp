"""
BSE Dual Feed Test - CM (Equity Cash) + F&O (Derivatives)
==========================================================

Tests simultaneous reception from:
- Port 26001: Equity Cash (CM) - Stocks like SBIN, INFY, TCS
- Port 26002: Derivatives (F&O) - SENSEX/BANKEX Options/Futures

Saves both to separate CSV files:
- data/processed_csv/YYYYMMDD_CM_quotes.csv
- data/processed_csv/YYYYMMDD_FNO_quotes.csv

Uses threading to receive from both ports simultaneously.
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


def decode_record_264(record: bytes) -> dict:
    """Decode 264-byte record (CM format)."""
    data = {}
    data['token'] = struct.unpack('<I', record[0:4])[0]
    data['open'] = struct.unpack('<i', record[4:8])[0] / 100.0
    data['prev_close'] = struct.unpack('<i', record[8:12])[0] / 100.0
    data['high'] = struct.unpack('<i', record[12:16])[0] / 100.0
    data['low'] = struct.unpack('<i', record[16:20])[0] / 100.0
    data['volume'] = struct.unpack('<i', record[24:28])[0]
    data['turnover_lakhs'] = struct.unpack('<I', record[28:32])[0]
    data['lot_size'] = struct.unpack('<I', record[32:36])[0]
    data['ltp'] = struct.unpack('<i', record[36:40])[0] / 100.0
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
                    record = decode_record_264(record_bytes)
                    
                    token = record['token']
                    if token == 0 or token == 1:
                        continue
                    
                    # Get contract info
                    contract = token_map.get(str(token), {})
                    
                    # Build quote
                    quote = {
                        'timestamp': datetime.now().strftime('%Y-%m-%d') + ' ' + header['timestamp'],
                        'token': token,
                        'symbol': contract.get('symbol', f'TOKEN_{token}'),
                        'company': contract.get('company_name', '')[:30],
                        'isin': contract.get('isin', ''),
                        'ltp': record['ltp'],
                        'open': record['open'],
                        'high': record['high'],
                        'low': record['low'],
                        'prev_close': record['prev_close'],
                        'volume': record['volume'],
                        'turnover_lakhs': record['turnover_lakhs']
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
                    record = decode_record_264(record_bytes)
                    
                    token = record['token']
                    if token == 0 or token == 1:
                        continue
                    
                    # Get contract info
                    contract = token_map.get(str(token), {})
                    
                    # Build quote
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
                        'volume': record['volume'],
                        'turnover_lakhs': record['turnover_lakhs']
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


def save_cm_to_csv(quotes: List[Dict], output_dir: str):
    """Save CM quotes to CSV."""
    if not quotes:
        return
    
    date_str = datetime.now().strftime('%Y%m%d')
    csv_path = Path(output_dir) / f'{date_str}_CM_quotes.csv'
    
    # Check if file exists
    file_exists = csv_path.exists()
    
    fieldnames = ['timestamp', 'token', 'symbol', 'company', 'isin', 
                  'ltp', 'open', 'high', 'low', 'prev_close', 'volume', 'turnover_lakhs']
    
    with open(csv_path, 'a', newline='', encoding='utf-8') as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        
        if not file_exists:
            writer.writeheader()
        
        for quote in quotes:
            writer.writerow(quote)
    
    return str(csv_path)


def save_fno_to_csv(quotes: List[Dict], output_dir: str):
    """Save F&O quotes to CSV."""
    if not quotes:
        return
    
    date_str = datetime.now().strftime('%Y%m%d')
    csv_path = Path(output_dir) / f'{date_str}_FNO_quotes.csv'
    
    # Check if file exists
    file_exists = csv_path.exists()
    
    fieldnames = ['timestamp', 'token', 'symbol', 'instrument_name', 'expiry', 
                  'strike', 'option_type', 'ltp', 'open', 'high', 'low', 
                  'prev_close', 'volume', 'turnover_lakhs']
    
    with open(csv_path, 'a', newline='', encoding='utf-8') as f:
        writer = csv.DictWriter(f, fieldnames=fieldnames)
        
        if not file_exists:
            writer.writeheader()
        
        for quote in quotes:
            writer.writerow(quote)
    
    return str(csv_path)


def main():
    print("=" * 80)
    print("BSE DUAL FEED TEST - CM + F&O SIMULTANEOUS")
    print("=" * 80)
    print(f"Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    
    # Configuration
    multicast_ip = "239.1.2.5"
    cm_port = 26001   # Equity Cash
    fno_port = 26002  # Derivatives
    output_dir = "data/processed_csv"
    
    # Create output directory
    Path(output_dir).mkdir(parents=True, exist_ok=True)
    
    print(f"\n📡 Configuration:")
    print(f"   CM Port:  {cm_port} (Equity Cash - Stocks)")
    print(f"   F&O Port: {fno_port} (Derivatives - Options/Futures)")
    print(f"   Output:   {output_dir}/")
    
    # Load CM token map (BhavCopy)
    print("\n📋 Loading CM Token Map (BhavCopy)...")
    api_url = "http://192.168.102.166:2060/v1/sftp-files"
    cm_fetcher = BSEEquityCashFetcher(api_url)
    cm_fetcher.fetch_and_load("26-11-2025")
    cm_token_map = cm_fetcher.contracts
    print(f"   ✅ CM Tokens: {len(cm_token_map):,}")
    
    # Load F&O token map (Contract Master)
    print("\n📋 Loading F&O Token Map (Contract Master)...")
    fno_manager = BSEContractManager(api_url)
    fno_fetcher = fno_manager.load_latest_contracts()
    fno_token_map = fno_fetcher.contracts
    print(f"   ✅ F&O Tokens: {len(fno_token_map):,}")
    
    # Start receiver threads
    print("\n" + "=" * 80)
    print("🚀 STARTING DUAL FEED RECEIVERS...")
    print("=" * 80)
    print("Press Ctrl+C to stop\n")
    
    cm_thread = threading.Thread(
        target=cm_receiver_thread,
        args=(multicast_ip, cm_port, cm_token_map),
        daemon=True
    )
    
    fno_thread = threading.Thread(
        target=fno_receiver_thread,
        args=(multicast_ip, fno_port, fno_token_map),
        daemon=True
    )
    
    cm_thread.start()
    fno_thread.start()
    
    # Collect and save data
    cm_quotes = []
    fno_quotes = []
    cm_total = 0
    fno_total = 0
    last_save = datetime.now()
    save_interval = 5  # Save every 5 seconds
    
    try:
        duration = 30  # Run for 30 seconds
        start_time = datetime.now()
        
        while (datetime.now() - start_time).seconds < duration:
            # Collect CM quotes
            while not cm_queue.empty():
                quote = cm_queue.get_nowait()
                cm_quotes.append(quote)
                cm_total += 1
            
            # Collect F&O quotes
            while not fno_queue.empty():
                quote = fno_queue.get_nowait()
                fno_quotes.append(quote)
                fno_total += 1
            
            # Save periodically
            if (datetime.now() - last_save).seconds >= save_interval:
                if cm_quotes:
                    cm_csv = save_cm_to_csv(cm_quotes, output_dir)
                    print(f"💾 CM: Saved {len(cm_quotes)} quotes to {cm_csv}")
                    cm_quotes = []
                
                if fno_quotes:
                    fno_csv = save_fno_to_csv(fno_quotes, output_dir)
                    print(f"💾 F&O: Saved {len(fno_quotes)} quotes to {fno_csv}")
                    fno_quotes = []
                
                last_save = datetime.now()
                print(f"📊 Total: CM={cm_total:,} | F&O={fno_total:,}")
            
            # Small sleep to prevent CPU spin
            threading.Event().wait(0.1)
            
    except KeyboardInterrupt:
        print("\n\n⚠️  Interrupted by user (Ctrl+C)")
    
    finally:
        # Stop threads
        stop_flag.set()
        cm_thread.join(timeout=2)
        fno_thread.join(timeout=2)
        
        # Save remaining quotes
        if cm_quotes:
            save_cm_to_csv(cm_quotes, output_dir)
            print(f"💾 CM: Final save - {len(cm_quotes)} quotes")
        
        if fno_quotes:
            save_fno_to_csv(fno_quotes, output_dir)
            print(f"💾 F&O: Final save - {len(fno_quotes)} quotes")
    
    # Summary
    print("\n" + "=" * 80)
    print("📊 FINAL SUMMARY")
    print("=" * 80)
    print(f"Duration: {(datetime.now() - start_time).seconds} seconds")
    print(f"CM Quotes:  {cm_total:,} (Equity Cash)")
    print(f"F&O Quotes: {fno_total:,} (Derivatives)")
    print(f"\nOutput Files:")
    print(f"   CM:  {output_dir}/{datetime.now().strftime('%Y%m%d')}_CM_quotes.csv")
    print(f"   F&O: {output_dir}/{datetime.now().strftime('%Y%m%d')}_FNO_quotes.csv")
    
    print("\n" + "=" * 80)
    print("✅ TEST COMPLETE")
    print("=" * 80)


if __name__ == '__main__':
    main()
