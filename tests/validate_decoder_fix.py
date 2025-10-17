"""
Test script to validate decoder fixes against known CSV data.
Expected values from 20251017_quotes.csv for token 878192:
- LTP: 8556380.16 paise (855638016 centipaise)
- Volume: 403374080
- Prev Close: 838860.8 paise
"""

import sys
import os
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..', 'src'))

from decoder import PacketDecoder
from decompressor import NFCASTDecompressor
import struct

def test_with_real_packet():
    """Test decoder with real packet from raw_packets folder."""
    
    # Find a type 2020 packet
    packet_dir = os.path.join(os.path.dirname(__file__), '..', 'data', 'raw_packets')
    packet_files = [f for f in os.listdir(packet_dir) if 'type2020' in f]
    
    if not packet_files:
        print("❌ No type 2020 packets found")
        return
    
    packet_path = os.path.join(packet_dir, packet_files[0])
    print(f"Testing with packet: {packet_files[0]}")
    
    # Read packet
    with open(packet_path, 'rb') as f:
        packet_data = f.read()
    
    print(f"Packet size: {len(packet_data)} bytes")
    
    # Decode
    decoder = PacketDecoder()
    result = decoder.decode_packet(packet_data)
    
    if not result:
        print("❌ Decoder returned None")
        return
    
    print(f"\n{'='*80}")
    print(f"DECODER RESULTS")
    print(f"{'='*80}")
    
    # Debug: print the structure
    print(f"Result keys: {result.keys()}")
    print()
    
    # Check what structure we got
    if 'header' in result:
        header = result['header']
        print(f"Header keys: {header.keys()}")
        print(f"Format ID: {header.get('format_id', 'N/A')}")
        print(f"Message Type: {header.get('msg_type', 'N/A')}")
        print(f"Timestamp: {header.get('timestamp', 'N/A')}")
    
    if 'records' in result:
        records = result['records']
        print(f"Records parsed: {len(records)}")
    else:
        print("No records found in result")
        return
    
    if not records:
        print("No records were decoded!")
        return
    
    # Look for token 878192 or just show first few records
    target_token = 878192
    found_target = False
    
    print(f"\n{'='*80}")
    print(f"SAMPLE RECORDS (looking for token {target_token})")
    print(f"{'='*80}")
    
    for i, record in enumerate(records[:5]):  # First 5 records
        token = record['token']
        ltp = record['ltp']
        ltq = record['ltq']
        volume = record['volume']
        close_rate = record['close_rate']
        num_trades = record['num_trades']
        
        # Convert paise to rupees for readability
        ltp_rs = ltp / 100.0
        close_rs = close_rate / 100.0
        
        print(f"\nRecord {i+1}:")
        print(f"  Token: {token}")
        print(f"  LTP: {ltp} paise (₹{ltp_rs:,.2f})")
        print(f"  LTQ: {ltq}")
        print(f"  Volume: {volume:,}")
        print(f"  Close Rate: {close_rate} paise (₹{close_rs:,.2f})")
        print(f"  Trades: {num_trades}")
        
        if token == target_token:
            found_target = True
            print(f"  ✅ FOUND TARGET TOKEN!")
            print(f"  Expected LTP: ~8556380 paise (₹85,563.80)")
            print(f"  Expected Volume: ~403,374,080")
            
            # Check if values are in reasonable range
            ltp_diff = abs(ltp - 8556380) / 8556380 * 100
            vol_diff = abs(volume - 403374080) / 403374080 * 100 if volume > 0 else 100
            
            if ltp_diff < 10:  # Within 10%
                print(f"  ✅ LTP within 10% of expected ({ltp_diff:.1f}% diff)")
            else:
                print(f"  ❌ LTP NOT in expected range ({ltp_diff:.1f}% diff)")
            
            if vol_diff < 10:  # Within 10%
                print(f"  ✅ Volume within 10% of expected ({vol_diff:.1f}% diff)")
            else:
                print(f"  ❌ Volume NOT in expected range ({vol_diff:.1f}% diff)")
    
    if not found_target:
        print(f"\n⚠️  Target token {target_token} not found in first 5 records")
        print(f"   Total records in packet: {len(records)}")
        
        # Check if it exists anywhere in the packet
        all_tokens = [r['token'] for r in records]
        if target_token in all_tokens:
            idx = all_tokens.index(target_token)
            record = records[idx]
            print(f"\n✅ Found token {target_token} at record index {idx}")
            print(f"   LTP: {record['ltp']} paise (₹{record['ltp']/100:,.2f})")
            print(f"   Volume: {record['volume']:,}")

def test_manual_parsing():
    """Manually parse a packet to debug field extraction."""
    
    packet_dir = os.path.join(os.path.dirname(__file__), '..', 'data', 'raw_packets')
    packet_files = [f for f in os.listdir(packet_dir) if 'type2020' in f]
    
    if not packet_files:
        return
    
    packet_path = os.path.join(packet_dir, packet_files[0])
    
    with open(packet_path, 'rb') as f:
        packet_data = f.read()
    
    print(f"\n{'='*80}")
    print(f"MANUAL PACKET PARSING")
    print(f"{'='*80}")
    
    # Skip 36-byte header
    header = packet_data[:36]
    data_section = packet_data[36:]
    
    print(f"Packet total size: {len(packet_data)} bytes")
    print(f"Header size: 36 bytes")
    print(f"Data section size: {len(data_section)} bytes")
    
    # Try to parse first record manually
    if len(data_section) >= 76:
        record_bytes = data_section[:76]
        
        print(f"\nFirst 76 bytes (uncompressed section):")
        print(f"Hex: {record_bytes.hex()}")
        
        # Parse fields
        token = struct.unpack('<I', record_bytes[0:4])[0]
        num_trades = struct.unpack('>I', record_bytes[4:8])[0]
        volume = struct.unpack('>q', record_bytes[8:16])[0]
        close_rate = struct.unpack('>i', record_bytes[60:64])[0]
        ltq = struct.unpack('>q', record_bytes[64:72])[0]
        ltp = struct.unpack('>i', record_bytes[72:76])[0]
        
        print(f"\nManually parsed fields:")
        print(f"  Token: {token} (Little-Endian)")
        print(f"  Num Trades: {num_trades} (Big-Endian)")
        print(f"  Volume: {volume:,} (Big-Endian)")
        print(f"  Close Rate: {close_rate} paise = ₹{close_rate/100:,.2f} (Big-Endian)")
        print(f"  LTQ: {ltq:,} (Big-Endian)")
        print(f"  LTP: {ltp} paise = ₹{ltp/100:,.2f} (Big-Endian)")
        
        # Check if values look reasonable
        print(f"\nValue validation:")
        if token > 0 and token < 10000000:
            print(f"  ✅ Token looks valid")
        else:
            print(f"  ❌ Token looks invalid")
        
        if ltp > 0 and ltp < 100000000:  # Less than 1 crore paise
            print(f"  ✅ LTP looks reasonable")
        else:
            print(f"  ⚠️  LTP might be incorrect")
        
        if volume >= 0 and volume < 1000000000:  # Less than 100 crore
            print(f"  ✅ Volume looks reasonable")
        else:
            print(f"  ⚠️  Volume might be incorrect")

if __name__ == '__main__':
    print("\n" + "="*80)
    print("TESTING DECODER FIXES")
    print("="*80)
    
    test_manual_parsing()
    print("\n")
    test_with_real_packet()
    
    print("\n" + "="*80)
    print("TEST COMPLETE")
    print("="*80)
