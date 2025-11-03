"""
Test script to read live UDP packets and display parsed data for Sensex November Futures (Token 861384).
This allows cross-checking parsed values against actual market data.

Usage:
    python test_token_1102290.py

Press Ctrl+C to stop.
"""

import sys
import os
import struct
import socket
import json
from datetime import datetime

# Add src to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__)))

from connection import BSEMulticastConnection
from decoder import PacketDecoder


def load_config():
    """Load configuration from config.json"""
    config_path = os.path.join(os.path.dirname(__file__),'config.json')
    with open(config_path, 'r') as f:
        return json.load(f)


def load_token_details():
    """Load token details for symbol lookup"""
    token_path = os.path.join(os.path.dirname(__file__), 'tokens', 'token_details.json')
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
    """Print compact packet header information"""
    # Format ID (Big-Endian)
    format_id = struct.unpack('>H', packet[4:6])[0]
    
    # Message type (Little-Endian)
    msg_type = struct.unpack('<H', packet[8:10])[0]
    
    # Timestamp (Little-Endian)
    hour = struct.unpack('<H', packet[20:22])[0]
    minute = struct.unpack('<H', packet[22:24])[0]
    second = struct.unpack('<H', packet[24:26])[0]
    
    time_str = f"{hour:02d}:{minute:02d}:{second:02d}" if hour < 24 and minute < 60 and second < 60 else "INVALID"
    
    print(f"\n📦 Packet: FormatID=0x{format_id:04X} | MsgType={msg_type} | Time={time_str} | Size={len(packet)}B")


def print_token_data(record_data, token_details):
    """Print comprehensive information including order book"""
    token = record_data['token']
    token_str = str(token)
    
    # Token info from contract master
    symbol_info = "Unknown"
    if token_str in token_details:
        info = token_details[token_str]
        symbol_info = f"{info.get('symbol', 'N/A')} [{info.get('expiry', 'N/A')}]"
        if info.get('option_type'):
            symbol_info += f" {info.get('option_type')} {info.get('strike', '')}"
    
    # Get all field values
    ltp = record_data.get('ltp', 0)
    atp = record_data.get('atp', 0)
    bid = record_data.get('bid', 0)
    ask = record_data.get('ask', 0)
    open_p = record_data.get('open', 0)
    high_p = record_data.get('high', 0)
    low_p = record_data.get('low', 0)
    prev_close = record_data.get('prev_close', 0)
    field_20_23 = record_data.get('field_20_23', 0)
    volume = record_data.get('volume', 0)
    
    # Unknown fields
    field_28 = record_data.get('unknown_28_31', 0)
    field_32 = record_data.get('unknown_32_35', 0)
    field_40 = record_data.get('unknown_40_43', 0)
    sequence_num = record_data.get('sequence_number', 0)
    
    # Order book
    order_book = record_data.get('order_book')
    
    print(f"\n{'='*100}")
    print(f"🎯 Token {token} | {symbol_info}")
    print(f"{'='*100}")
    
    # CONFIRMED FIELDS
    print(f"\n✅ PRICE FIELDS:")
    print(f"   LTP (36-39):       ₹{ltp/100:>12,.2f}  | Volume (24-27): {volume:>12,}")
    print(f"   Open (4-7):        ₹{open_p/100:>12,.2f}  | High (12-15):  ₹{high_p/100:>12,.2f}")
    print(f"   Low (16-19):       ₹{low_p/100:>12,.2f}  | Prev Close:    ₹{prev_close/100:>12,.2f}")
    
    if atp > 0:
        print(f"   ATP (84-87):       ₹{atp/100:>12,.2f}  (Average Traded Price)")
    
    print(f"\n⚠️  UNKNOWN FIELDS:")
    print(f"   Field 20-23:       ₹{field_20_23/100:>12,.2f}  (NOT Close - too small!)")
    print(f"   Bid (104-107):     ₹{bid/100:>12,.2f}  (Best Bid)")
    print(f"   Sequence (44-47):  {sequence_num:>15,}  (✅ Increments by 1)")
    
    if field_28 > 0:
        print(f"   Offset 28-31:      {field_28:>15,}  (Turnover in lakhs)")
    if field_32 > 0:
        print(f"   Offset 32-35:      {field_32:>15,}  (Trade count?)")
    
    # ORDER BOOK DISPLAY
    if order_book and (order_book['bids'] or order_book['asks']):
        print(f"\n📊 ORDER BOOK DEPTH (5 Levels):")
        print(f"{'':>4}{'BID QTY':>10}  {'BID PRICE':>12}  |  {'ASK PRICE':>12}  {'ASK QTY':<10}")
        print(f"   {'-'*59}")
        
        # Display up to 5 levels
        max_levels = max(len(order_book['bids']), len(order_book['asks']))
        for i in range(min(max_levels, 5)):
            # Bid side
            if i < len(order_book['bids']):
                bid_level = order_book['bids'][i]
                bid_qty_str = f"{bid_level['quantity']:,}"
                bid_price_str = f"₹{bid_level['price']:,.2f}"
            else:
                bid_qty_str = "-"
                bid_price_str = "-"
            
            # Ask side
            if i < len(order_book['asks']):
                ask_level = order_book['asks'][i]
                ask_qty_str = f"{ask_level['quantity']:,}"
                ask_price_str = f"₹{ask_level['price']:,.2f}"
            else:
                ask_qty_str = "-"
                ask_price_str = "-"
            
            print(f"   {bid_qty_str:>10}  {bid_price_str:>12}  |  {ask_price_str:>12}  {ask_qty_str:<10}")
        
        # Calculate total depth
        total_bid_qty = sum(b['quantity'] for b in order_book['bids'])
        total_ask_qty = sum(a['quantity'] for a in order_book['asks'])
        print(f"   {'-'*59}")
        print(f"   {total_bid_qty:>10,}  {'TOTAL':>12}  |  {'TOTAL':>12}  {total_ask_qty:<10,}")
        
        # Best bid/ask spread
        if order_book['bids'] and order_book['asks']:
            best_bid = order_book['bids'][0]['price']
            best_ask = order_book['asks'][0]['price']
            spread = best_ask - best_bid
            spread_pct = (spread / best_bid * 100) if best_bid > 0 else 0
            print(f"\n   💰 Spread: ₹{spread:.2f} ({spread_pct:.3f}%)")
    
    print(f"\n{'='*100}")


def main():
    """Main test loop"""
    print("="*80)
    print("BSE LIVE FEED MONITOR - SENSEX NOVEMBER FUTURES (Token 861384)")
    print("="*80)
    print("Press Ctrl+C to stop\n")
    
    # Load configuration and token details
    config = load_config()
    token_details = load_token_details()
    
    TARGET_TOKEN = 873830  # SENSEX November Futures
    
    # Get token info if available
    if str(TARGET_TOKEN) in token_details:
        info = token_details[str(TARGET_TOKEN)]
        print(f"📍 Monitoring: {info.get('symbol', 'N/A')} | Expiry: {info.get('expiry', 'N/A')}\n")
    
    # Create connection and decoder
    print(f"📡 Connecting to BSE NFCAST: {config['multicast']['ip']}:{config['multicast']['port']}")
    
    connection = BSEMulticastConnection(
        multicast_ip=config['multicast']['ip'],
        port=config['multicast']['port'],
        buffer_size=config.get('buffer_size', 2048)
    )
    sock = connection.connect()
    decoder = PacketDecoder()
    
    print("✅ Connected! Listening for live data...\n")
    print("-" * 80)
    
    packets_received = 0
    token_found_count = 0
    last_status_time = datetime.now()
    
    try:
        while True:
            try:
                # Receive packet (1 second timeout)
                packet, addr = sock.recvfrom(config.get('buffer_size', 2048))
                packets_received += 1
                
                # Decode packet
                decoded_data = decoder.decode_packet(packet)
                
                if decoded_data is None:
                    continue
                
                # Get records from decoded data
                decoded_records = decoded_data.get('records', [])
                
                # Check each record for our target token
                found_in_this_packet = False
                for record in decoded_records:
                    if record['token'] == TARGET_TOKEN:
                        if not found_in_this_packet:
                            # Print packet header once per packet
                            print_packet_header(packet)
                            found_in_this_packet = True
                        
                        # Print token data (compact 2-3 line format)
                        print_token_data(record, token_details)
                        token_found_count += 1
                        
                        # Timestamp and stats
                        print(f"   ⏰ {datetime.now().strftime('%H:%M:%S')} | Packets: {packets_received:,} | Found: {token_found_count} times\n")
                
                # Print status every 60 seconds if token not found
                if not found_in_this_packet:
                    now = datetime.now()
                    if (now - last_status_time).total_seconds() >= 60:
                        print(f"⏳ Waiting... | Packets: {packets_received:,} | Token found: {token_found_count} times | {now.strftime('%H:%M:%S')}")
                        last_status_time = now
                
            except socket.timeout:
                # Normal timeout, continue
                now = datetime.now()
                if (now - last_status_time).total_seconds() >= 60:
                    print(f"⏳ Waiting... | Packets: {packets_received:,} | Token found: {token_found_count} times | {now.strftime('%H:%M:%S')}")
                    last_status_time = now
                continue
                
    except KeyboardInterrupt:
        print("\n" + "-" * 80)
        print(f"⏹️  STOPPED | Total Packets: {packets_received:,} | Token Found: {token_found_count} times")
        print("-" * 80)
    
    finally:
        connection.disconnect()
        print("✅ Disconnected\n")


if __name__ == "__main__":
    main()
