"""
Analyze raw packets to find correct LTP offset
Compare with known expected values to calibrate decoder
"""

import struct
import os
import glob
from datetime import datetime

# Known values from market at 14:22:23
KNOWN_VALUES = {
    861384: {  # SENSEX FUT
        'name': 'SENSEX 30-OCT FUT',
        'expected_ltp': 83847.00,  # Rupees
        'expected_paise': 8384700,  # Paise (x100)
    },
    878196: {  # SENSEX 83900 CE
        'name': 'SENSEX 83900 CE',
        'expected_ltp': 486.00,
        'expected_paise': 48600,
    },
    878015: {  # SENSEX 83800 PE
        'name': 'SENSEX 83800 PE',
        'expected_ltp': 340.00,
        'expected_paise': 34000,
    },
}

def analyze_packet(packet_path):
    """Analyze a single packet file"""
    with open(packet_path, 'rb') as f:
        packet = f.read()
    
    if len(packet) < 100:
        return None
    
    # Read header to get token from first record
    # Header is 36 bytes, then records start
    record_start = 36
    
    # Try to read first record token (Little-Endian at offset 0)
    if len(packet) < record_start + 4:
        return None
    
    token = struct.unpack('<I', packet[record_start:record_start+4])[0]
    
    # Skip empty records (token 0 or 1)
    if token <= 1:
        return None
    
    return {
        'path': packet_path,
        'token': token,
        'packet': packet,
        'record_start': record_start
    }

def search_for_value(data, value, start_offset=0):
    """Search for a 4-byte integer value in different encodings"""
    results = []
    
    # Search Big-Endian
    needle_be = struct.pack('>i', value)
    pos = data.find(needle_be, start_offset)
    if pos != -1:
        results.append(('big-endian signed', pos, value))
    
    # Search Little-Endian
    needle_le = struct.pack('<i', value)
    pos = data.find(needle_le, start_offset)
    if pos != -1:
        results.append(('little-endian signed', pos, value))
    
    # Try unsigned versions
    if value > 0:
        needle_be_u = struct.pack('>I', value)
        pos = data.find(needle_be_u, start_offset)
        if pos != -1:
            results.append(('big-endian unsigned', pos, value))
        
        needle_le_u = struct.pack('<I', value)
        pos = data.find(needle_le_u, start_offset)
        if pos != -1:
            results.append(('little-endian unsigned', pos, value))
    
    return results

def hex_dump(data, offset, length=16):
    """Create hex dump of data around offset"""
    start = max(0, offset - 8)
    end = min(len(data), offset + length + 8)
    
    result = []
    for i in range(start, end, 16):
        hex_part = ' '.join(f'{b:02x}' for b in data[i:min(i+16, end)])
        ascii_part = ''.join(chr(b) if 32 <= b < 127 else '.' for b in data[i:min(i+16, end)])
        marker = ' <-- HERE' if start <= offset < i+16 else ''
        result.append(f'  {i:04x}: {hex_part:<48} {ascii_part}{marker}')
    
    return '\n'.join(result)

def main():
    print("="*100)
    print("ANALYZING RAW PACKETS FOR CORRECT LTP OFFSET")
    print("="*100)
    print()
    
    # Find all captured packets from today
    packet_dir = 'data/raw_packets'
    today = datetime.now().strftime('%Y%m%d')
    pattern = f'{packet_dir}/{today}_*.bin'
    
    packets = glob.glob(pattern)
    print(f"Found {len(packets)} packets from today")
    print()
    
    # Analyze each packet
    analyzed = []
    for pkt_path in packets[:100]:  # Limit to first 100
        result = analyze_packet(pkt_path)
        if result and result['token'] in KNOWN_VALUES:
            analyzed.append(result)
    
    print(f"Found {len(analyzed)} packets with target tokens")
    print()
    
    # For each known token, find the value in the packet
    for token, info in KNOWN_VALUES.items():
        print("="*100)
        print(f"TOKEN {token}: {info['name']}")
        print(f"Expected LTP: ₹{info['expected_ltp']:.2f} = {info['expected_paise']} paise")
        print("="*100)
        print()
        
        # Find packets with this token
        token_packets = [p for p in analyzed if p['token'] == token]
        
        if not token_packets:
            print(f"⚠️  No packets found for token {token}")
            print()
            continue
        
        print(f"Analyzing {len(token_packets)} packets for this token...")
        print()
        
        # Take first packet for this token
        pkt = token_packets[0]
        packet = pkt['packet']
        record_start = pkt['record_start']
        
        print(f"Packet: {os.path.basename(pkt['path'])}")
        print(f"Record starts at byte {record_start}")
        print()
        
        # Search for expected paise value
        print(f"Searching for {info['expected_paise']} paise in different encodings:")
        print()
        
        results = search_for_value(packet, info['expected_paise'], record_start)
        
        if results:
            for encoding, position, value in results:
                offset_from_record = position - record_start
                print(f"✅ FOUND at byte {position} (offset +{offset_from_record} from record start)")
                print(f"   Encoding: {encoding}")
                print(f"   Value: {value} paise = ₹{value/100:.2f}")
                print()
                print("   Hex dump around this location:")
                print(hex_dump(packet, position))
                print()
        else:
            print(f"❌ Value {info['expected_paise']} NOT FOUND")
            print()
            
            # Try nearby values (±10%)
            print("Searching for nearby values (±10%):")
            for factor in [0.9, 0.95, 1.05, 1.10]:
                nearby = int(info['expected_paise'] * factor)
                results = search_for_value(packet, nearby, record_start)
                if results:
                    for encoding, position, value in results:
                        offset_from_record = position - record_start
                        print(f"  Found {value} at offset +{offset_from_record} ({encoding})")
            print()
            
            # Show what's currently being read at offset +63
            if len(packet) >= record_start + 67:
                current_be = struct.unpack('>i', packet[record_start+63:record_start+67])[0]
                current_le = struct.unpack('<i', packet[record_start+63:record_start+67])[0]
                print(f"Current decoder reads offset +63:")
                print(f"  Big-Endian:    {current_be:>10} paise = ₹{current_be/100:>10,.2f}")
                print(f"  Little-Endian: {current_le:>10} paise = ₹{current_le/100:>10,.2f}")
                print()
        
        # Show first 100 bytes of record for manual inspection
        print("First 100 bytes of record (hex):")
        print(hex_dump(packet[record_start:], 0, 100))
        print()

if __name__ == '__main__':
    main()
