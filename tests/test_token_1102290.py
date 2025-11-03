"""
Test script to read live UDP packets and display parsed data for token 1102290.
This allows cross-checking parsed values against actual market data.

Usage:
    python tests/test_token_1102290.py

Press Ctrl+C to stop.
"""

import sys
import os
import struct
import socket
import json
from datetime import datetime

# Add src to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

from connection import BSEMulticastConnection
from decoder import PacketDecoder


def load_config():
    """Load configuration from config.json"""
    config_path = os.path.join(os.path.dirname(__file__), '..', 'config.json')
    with open(config_path, 'r') as f:
        return json.load(f)


def load_token_details():
    """Load token details for symbol lookup"""
    token_path = os.path.join(os.path.dirname(__file__), '..', 'data', 'tokens', 'token_details.json')
    with open(token_path, 'r') as f:
        return json.load(f)


def format_price(value_in_paise):
    """Convert paise to rupees and format"""
    if value_in_paise is None or value_in_paise == 0:
        return "N/A"
    return f"₹{value_in_paise / 100.0:,.2f}"


def format_volume(volume):
    """Format volume with commas"""
    if volume is None or volume == 0:
        return "N/A"
    return f"{volume:,}"


def print_packet_header(packet):
    """Print packet header information"""
    print("\n" + "="*80)
    print("PACKET HEADER")
    print("="*80)
    
    # Leading zeros (Big-Endian)
    leading_zeros = struct.unpack('>I', packet[0:4])[0]
    print(f"Leading Zeros:  0x{leading_zeros:08X}")
    
    # Format ID (Big-Endian)
    format_id = struct.unpack('>H', packet[4:6])[0]
    print(f"Format ID:      0x{format_id:04X}")
    
    # Message type (Little-Endian)
    msg_type = struct.unpack('<H', packet[8:10])[0]
    print(f"Message Type:   {msg_type}")
    
    # Timestamp (Little-Endian)
    hour = struct.unpack('<H', packet[20:22])[0]
    minute = struct.unpack('<H', packet[22:24])[0]
    second = struct.unpack('<H', packet[24:26])[0]
    
    if hour < 24 and minute < 60 and second < 60:
        print(f"Packet Time:    {hour:02d}:{minute:02d}:{second:02d}")
    else:
        print(f"Packet Time:    INVALID ({hour}:{minute}:{second})")
    
    print(f"Packet Size:    {len(packet)} bytes")


def print_token_data(record_data, token_details):
    """Print detailed information for token 1102290"""
    token = record_data['token']
    token_str = str(token)
    
    print("\n" + "="*80)
    print(f"TOKEN {token} DATA")
    print("="*80)
    
    # Token info from contract master
    if token_str in token_details:
        info = token_details[token_str]
        print(f"\n📋 CONTRACT DETAILS:")
        print(f"   Symbol:       {info.get('symbol', 'N/A')}")
        print(f"   Expiry:       {info.get('expiry', 'N/A')}")
        print(f"   Option Type:  {info.get('option_type', 'N/A')}")
        print(f"   Strike:       {info.get('strike', 'N/A')}")
    else:
        print(f"\n⚠️  Token not found in contract master")
    
    # Uncompressed fields (from decoder)
    print(f"\n📊 UNCOMPRESSED FIELDS (from decoder.py):")
    print(f"   Token:        {record_data['token']}")
    print(f"   LTP:          {format_price(record_data.get('ltp'))}")
    print(f"   LTQ:          {format_volume(record_data.get('ltq'))}")
    print(f"   Volume:       {format_volume(record_data.get('volume'))}")
    print(f"   Prev Close:   {format_price(record_data.get('prev_close'))}")
    
    print(f"\n📈 OHLC (from decoder.py):")
    print(f"   Open:         {format_price(record_data.get('open'))}")
    print(f"   High:         {format_price(record_data.get('high'))}")
    print(f"   Low:          {format_price(record_data.get('low'))}")
    print(f"   Close:        {format_price(record_data.get('close'))}")
    
    # Raw hex dump of record (first 64 bytes)
    print(f"\n🔍 RAW RECORD DATA (first 64 bytes):")
    raw_record = record_data.get('raw_record', b'')
    if raw_record:
        for i in range(0, min(64, len(raw_record)), 16):
            hex_str = ' '.join(f'{b:02X}' for b in raw_record[i:i+16])
            ascii_str = ''.join(chr(b) if 32 <= b < 127 else '.' for b in raw_record[i:i+16])
            print(f"   {i:04d}: {hex_str:<48} {ascii_str}")
    
    # Key offsets for manual verification
    print(f"\n🔑 KEY FIELD OFFSETS (for manual verification):")
    print(f"   Token (0-3):      {struct.unpack('<I', raw_record[0:4])[0] if len(raw_record) >= 4 else 'N/A'}")
    print(f"   LTP (4-7):        {struct.unpack('<i', raw_record[4:8])[0] if len(raw_record) >= 8 else 'N/A'} paise = {format_price(struct.unpack('<i', raw_record[4:8])[0] if len(raw_record) >= 8 else 0)}")
    print(f"   Open (8-11):      {struct.unpack('<i', raw_record[8:12])[0] if len(raw_record) >= 12 else 'N/A'} paise = {format_price(struct.unpack('<i', raw_record[8:12])[0] if len(raw_record) >= 12 else 0)}")
    print(f"   High (12-15):     {struct.unpack('<i', raw_record[12:16])[0] if len(raw_record) >= 16 else 'N/A'} paise = {format_price(struct.unpack('<i', raw_record[12:16])[0] if len(raw_record) >= 16 else 0)}")
    print(f"   Low (16-19):      {struct.unpack('<i', raw_record[16:20])[0] if len(raw_record) >= 20 else 'N/A'} paise = {format_price(struct.unpack('<i', raw_record[16:20])[0] if len(raw_record) >= 20 else 0)}")
    print(f"   Close (20-23):    {struct.unpack('<i', raw_record[20:24])[0] if len(raw_record) >= 24 else 'N/A'} paise = {format_price(struct.unpack('<i', raw_record[20:24])[0] if len(raw_record) >= 24 else 0)}")
    print(f"   Volume (24-27):   {struct.unpack('<i', raw_record[24:28])[0] if len(raw_record) >= 28 else 'N/A'}")
    print(f"   LTQ (28-31):      {struct.unpack('<i', raw_record[28:32])[0] if len(raw_record) >= 32 else 'N/A'}")


def main():
    """Main test loop"""
    print("="*80)
    print("BSE LIVE UDP PACKET READER - TOKEN 1102290")
    print("="*80)
    print("\nPress Ctrl+C to stop\n")
    
    # Load configuration and token details
    config = load_config()
    token_details = load_token_details()
    
    TARGET_TOKEN = 1102290
    
    # Get token info if available
    if str(TARGET_TOKEN) in token_details:
        info = token_details[str(TARGET_TOKEN)]
        print(f"🎯 Target Token: {TARGET_TOKEN}")
        print(f"   Symbol: {info.get('symbol', 'N/A')}")
        print(f"   Expiry: {info.get('expiry', 'N/A')}")
        print(f"   Type: {info.get('option_type', 'N/A')}")
        print(f"   Strike: {info.get('strike', 'N/A')}")
    else:
        print(f"🎯 Target Token: {TARGET_TOKEN} (not found in contract master)")
    
    # Create connection and decoder
    print(f"\n📡 Connecting to multicast group...")
    print(f"   Address: {config['multicast_group']}")
    print(f"   Port: {config['multicast_port']}")
    
    connection = BSEMulticastConnection(config)
    sock = connection.connect()
    decoder = PacketDecoder()
    
    print("✅ Connected! Waiting for packets...\n")
    
    packets_received = 0
    token_found_count = 0
    last_status_time = datetime.now()
    
    try:
        while True:
            try:
                # Receive packet (1 second timeout)
                packet, addr = sock.recvfrom(config['buffer_size'])
                packets_received += 1
                
                # Decode packet
                decoded_records = decoder.decode(packet)
                
                if decoded_records is None:
                    continue
                
                # Check each record for our target token
                found_in_this_packet = False
                for record in decoded_records:
                    if record['token'] == TARGET_TOKEN:
                        if not found_in_this_packet:
                            # Print packet header once per packet
                            print_packet_header(packet)
                            found_in_this_packet = True
                        
                        # Print token data
                        print_token_data(record, token_details)
                        token_found_count += 1
                        
                        print(f"\n⏰ System Time: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
                        print(f"📊 Stats: Total packets: {packets_received}, Token found: {token_found_count} times")
                        print("\n" + "="*80)
                        print("Waiting for next packet with token 1102290...")
                        print("="*80 + "\n")
                
                # Print status every 30 seconds if token not found
                if not found_in_this_packet:
                    now = datetime.now()
                    if (now - last_status_time).total_seconds() >= 30:
                        print(f"⏱️  Still waiting... (received {packets_received} packets, found token {token_found_count} times)")
                        last_status_time = now
                
            except socket.timeout:
                # Normal timeout, continue
                now = datetime.now()
                if (now - last_status_time).total_seconds() >= 30:
                    print(f"⏱️  Still waiting for packets... (received {packets_received} packets, found token {token_found_count} times)")
                    last_status_time = now
                continue
                
    except KeyboardInterrupt:
        print("\n\n" + "="*80)
        print("STOPPED BY USER")
        print("="*80)
        print(f"Total packets received: {packets_received}")
        print(f"Token 1102290 found: {token_found_count} times")
        print("="*80)
    
    finally:
        connection.disconnect()
        print("\n✅ Socket closed")


if __name__ == "__main__":
    main()
