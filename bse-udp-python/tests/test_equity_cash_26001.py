"""
Test Port 26001 - Equity Cash Decoder with BhavCopy Mapping
============================================================

Tests port 26001 (Equity Cash) with token mapping from BhavCopy CSV.
Now we can decode stock symbols like INFY, NTPC, TCS, etc.
"""

import sys
import socket
import struct
from pathlib import Path
from datetime import datetime

# Add src to path
sys.path.insert(0, str(Path(__file__).parent))

from equity_cash_fetcher import BSEEquityCashFetcher

def create_multicast_socket(multicast_ip: str, port: int) -> socket.socket:
    """Create UDP multicast socket."""
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    sock.bind(('', port))
    
    # Join multicast group
    mreq = struct.pack("4sl", socket.inet_aton(multicast_ip), socket.INADDR_ANY)
    sock.setsockopt(socket.IPPROTO_IP, socket.IP_ADD_MEMBERSHIP, mreq)
    
    # Set timeout
    sock.settimeout(10)
    
    return sock

def decode_header(packet: bytes) -> dict:
    """Decode packet header."""
    header = {}
    
    # Format ID (Big-Endian) at offset 4-5
    header['format_id'] = struct.unpack('>H', packet[4:6])[0]
    header['packet_size'] = header['format_id']
    
    # Message Type (Little-Endian) at offset 8-9
    header['msg_type'] = struct.unpack('<H', packet[8:10])[0]
    
    # Timestamp
    hour = struct.unpack('<H', packet[20:22])[0]
    minute = struct.unpack('<H', packet[22:24])[0]
    second = struct.unpack('<H', packet[24:26])[0]
    ms = struct.unpack('<H', packet[26:28])[0]
    
    if hour < 24 and minute < 60 and second < 60:
        header['timestamp'] = f"{hour:02d}:{minute:02d}:{second:02d}.{ms:03d}"
    else:
        header['timestamp'] = "INVALID"
    
    return header

def decode_record_264(record: bytes) -> dict:
    """Decode 264-byte record format."""
    data = {}
    
    # Token (Little-Endian)
    data['token'] = struct.unpack('<I', record[0:4])[0]
    
    # Market data (Little-Endian, paise → rupees)
    data['open'] = struct.unpack('<i', record[4:8])[0] / 100.0
    data['prev_close'] = struct.unpack('<i', record[8:12])[0] / 100.0
    data['high'] = struct.unpack('<i', record[12:16])[0] / 100.0
    data['low'] = struct.unpack('<i', record[16:20])[0] / 100.0
    data['volume'] = struct.unpack('<i', record[24:28])[0]
    data['turnover_lakhs'] = struct.unpack('<I', record[28:32])[0]
    data['lot_size'] = struct.unpack('<I', record[32:36])[0]
    data['ltp'] = struct.unpack('<i', record[36:40])[0] / 100.0
    data['sequence'] = struct.unpack('<I', record[44:48])[0]
    
    return data

def main():
    print("=" * 80)
    print("TEST PORT 26001 - EQUITY CASH WITH BHAVCOPY MAPPING")
    print("=" * 80)
    print(f"Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    
    # Configuration
    multicast_ip = "239.1.2.5"
    port = 26001  # Equity Cash port
    
    print(f"\n📡 Connecting to: {multicast_ip}:{port}")
    
    # Load BhavCopy for equity cash tokens
    print("\n📋 Loading Equity Cash BhavCopy...")
    api_url = "http://192.168.102.166:2060/v1/sftp-files"
    fetcher = BSEEquityCashFetcher(api_url)
    
    # Fetch yesterday's BhavCopy
    date_str = "26-11-2025"
    success = fetcher.fetch_and_load(date_str)
    
    if success:
        print(f"   ✅ Loaded {fetcher.stats['total_contracts']:,} equity contracts")
    else:
        print("   ❌ Failed to load BhavCopy, tokens won't be mapped")
    
    # Create socket
    try:
        sock = create_multicast_socket(multicast_ip, port)
        print(f"   ✅ Socket connected to port {port}")
    except Exception as e:
        print(f"   ❌ Socket error: {e}")
        return
    
    print("\n" + "=" * 80)
    print("📦 WAITING FOR PACKETS (timeout: 10 seconds)...")
    print("=" * 80)
    
    packets_received = 0
    unique_tokens = set()
    mapped_tokens = 0
    unmapped_tokens = 0
    
    try:
        while packets_received < 10:  # Capture 10 packets
            try:
                packet, addr = sock.recvfrom(2048)
                packets_received += 1
                
                # Decode header
                header = decode_header(packet)
                packet_size = len(packet)
                
                print(f"\n📦 PACKET #{packets_received}")
                print(f"   Size: {packet_size} bytes | Type: {header['msg_type']} | Time: {header['timestamp']}")
                
                # Calculate record size
                header_size = 36
                payload_size = packet_size - header_size
                
                # Determine record format
                if payload_size % 264 == 0:
                    record_size = 264
                    num_records = payload_size // 264
                else:
                    print(f"   ⚠️ Unknown record format (payload={payload_size} bytes)")
                    continue
                
                # Decode records
                for i in range(num_records):
                    offset = header_size + (i * record_size)
                    record_bytes = packet[offset:offset+record_size]
                    record = decode_record_264(record_bytes)
                    
                    token = record['token']
                    
                    # Skip empty records
                    if token == 0 or token == 1:
                        continue
                    
                    unique_tokens.add(token)
                    
                    # Get contract details from BhavCopy
                    contract = fetcher.get_contract(token)
                    
                    if contract:
                        mapped_tokens += 1
                        symbol = contract['symbol']
                        company = contract.get('company_name', '')[:25]
                        print(f"   ✅ {token}: {symbol:<10} ({company}) | LTP: ₹{record['ltp']:,.2f} | Vol: {record['volume']:,}")
                    else:
                        unmapped_tokens += 1
                        print(f"   ❌ {token}: UNKNOWN | LTP: ₹{record['ltp']:,.2f} | Vol: {record['volume']:,}")
                
            except socket.timeout:
                print("\n⏱️  Socket timeout - no more packets")
                break
                
    except KeyboardInterrupt:
        print("\n\n⚠️  Interrupted by user (Ctrl+C)")
    
    finally:
        sock.close()
    
    # Summary
    print("\n" + "=" * 80)
    print("📊 SUMMARY")
    print("=" * 80)
    print(f"Packets received: {packets_received}")
    print(f"Unique tokens: {len(unique_tokens)}")
    print(f"Mapped tokens: {mapped_tokens} ✅")
    print(f"Unmapped tokens: {unmapped_tokens} ❌")
    
    if unique_tokens:
        print(f"\nSample tokens: {sorted(list(unique_tokens)[:10])}")
    
    print("\n" + "=" * 80)
    print("✅ TEST COMPLETE")
    print("=" * 80)

if __name__ == '__main__':
    main()
