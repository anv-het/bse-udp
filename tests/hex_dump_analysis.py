"""
Hex dump analysis of BSE packets to determine correct byte structure
"""

import struct
import os

def hex_dump_packet(packet_path):
    """Hex dump a packet with byte-by-byte analysis"""
    
    with open(packet_path, 'rb') as f:
        data = f.read()
    
    print(f"\n{'='*100}")
    print(f"PACKET: {os.path.basename(packet_path)}")
    print(f"SIZE: {len(data)} bytes")
    print(f"{'='*100}\n")
    
    # Header section
    print("HEADER SECTION (First 36 bytes):")
    print("-" * 100)
    header = data[:36]
    
    print(f"Bytes 0-35 (Hex):  {header.hex()}")
    print()
    
    # Try different endianness for message type (offset 0-3)
    msg_type_le = struct.unpack('<I', data[0:4])[0]
    msg_type_be = struct.unpack('>I', data[0:4])[0]
    print(f"Message Type (Little-Endian): {msg_type_le}")
    print(f"Message Type (Big-Endian):    {msg_type_be}")
    print()
    
    # Try timestamp fields (offsets 14-21 according to manual)
    if len(data) >= 22:
        hour_le = struct.unpack('<H', data[14:16])[0]
        min_le = struct.unpack('<H', data[16:18])[0]
        sec_le = struct.unpack('<H', data[18:20])[0]
        ms_le = struct.unpack('<H', data[20:22])[0]
        
        hour_be = struct.unpack('>H', data[14:16])[0]
        min_be = struct.unpack('>H', data[16:18])[0]
        sec_be = struct.unpack('>H', data[18:20])[0]
        ms_be = struct.unpack('>H', data[20:22])[0]
        
        print("Timestamp (Little-Endian):")
        print(f"  Offset 14-15 (Hour):        {hour_le}")
        print(f"  Offset 16-17 (Minute):      {min_le}")
        print(f"  Offset 18-19 (Second):      {sec_le}")
        print(f"  Offset 20-21 (Millisecond): {ms_le}")
        print()
        
        print("Timestamp (Big-Endian):")
        print(f"  Offset 14-15 (Hour):        {hour_be}")
        print(f"  Offset 16-17 (Minute):      {min_be}")
        print(f"  Offset 18-19 (Second):      {sec_be}")
        print(f"  Offset 20-21 (Millisecond): {ms_be}")
        print()
    
    # Number of records (offset 26-27)
    if len(data) >= 28:
        num_records_le = struct.unpack('<H', data[26:28])[0]
        num_records_be = struct.unpack('>H', data[26:28])[0]
        print(f"Number of Records (Little-Endian): {num_records_le}")
        print(f"Number of Records (Big-Endian):    {num_records_be}")
        print()
    
    # FIRST RECORD SECTION (starts at offset 36)
    print("\n" + "="*100)
    print("FIRST RECORD (Starting at offset 36):")
    print("="*100 + "\n")
    
    if len(data) >= 36 + 76:
        record_start = 36
        
        # Token (Little-Endian uint32)
        token_le = struct.unpack('<I', data[record_start:record_start+4])[0]
        token_be = struct.unpack('>I', data[record_start:record_start+4])[0]
        
        print(f"Token at offset {record_start} (Little-Endian): {token_le}")
        print(f"Token at offset {record_start} (Big-Endian):    {token_be}")
        print()
        
        # Try to find recognizable fields
        print("Trying different field interpretations:")
        print("-" * 100)
        
        # Manual says: Instrument Code (4/8 bytes), then No of Trades (4), then Volume (8)
        # Let's try reading as if it's token (4 LE) + trades (4 BE) + volume (8 BE)
        
        print("\nInterpretation A: Token(4 LE) + Trades(4 BE) + Volume(8 BE)")
        trades_be = struct.unpack('>I', data[record_start+4:record_start+8])[0]
        volume_be = struct.unpack('>q', data[record_start+8:record_start+16])[0]
        print(f"  Token: {token_le}")
        print(f"  Trades: {trades_be}")
        print(f"  Volume: {volume_be}")
        print()
        
        # Try Little-Endian for trades/volume
        print("Interpretation B: Token(4 LE) + Trades(4 LE) + Volume(8 LE)")
        trades_le = struct.unpack('<I', data[record_start+4:record_start+8])[0]
        volume_le = struct.unpack('<q', data[record_start+8:record_start+16])[0]
        print(f"  Token: {token_le}")
        print(f"  Trades: {trades_le}")
        print(f"  Volume: {volume_le}")
        print()
        
        # Try old code offsets (LTP at +20)
        print("Interpretation C: Old Code Offsets (LTP at +20, Volume at +24)")
        ltp_old_le = struct.unpack('<I', data[record_start+20:record_start+24])[0]
        volume_old_le = struct.unpack('<I', data[record_start+24:record_start+28])[0]
        print(f"  Token: {token_le}")
        print(f"  LTP at +20 (LE): {ltp_old_le} paise = ₹{ltp_old_le/100:.2f}")
        print(f"  Volume at +24 (LE): {volume_old_le}")
        print()
        
        # Try manual offsets (Close Rate at +60, LTQ at +64, LTP at +72)
        print("Interpretation D: Manual Offsets (Close at +60, LTQ at +64, LTP at +72)")
        close_be = struct.unpack('>i', data[record_start+60:record_start+64])[0]
        ltq_be = struct.unpack('>q', data[record_start+64:record_start+72])[0]
        ltp_be = struct.unpack('>i', data[record_start+72:record_start+76])[0]
        print(f"  Token: {token_le}")
        print(f"  Close Rate at +60 (BE): {close_be} paise = ₹{close_be/100:.2f}")
        print(f"  LTQ at +64 (BE): {ltq_be}")
        print(f"  LTP at +72 (BE): {ltp_be} paise = ₹{ltp_be/100:.2f}")
        print()
        
        # Show first 100 bytes in hex for manual inspection
        print("\n" + "="*100)
        print("HEX DUMP OF FIRST RECORD (bytes 36-135):")
        print("="*100)
        first_100 = data[36:136]
        for i in range(0, len(first_100), 16):
            chunk = first_100[i:i+16]
            offset_label = f"{36+i:03d}-{36+i+15:03d}"
            hex_str = ' '.join(f"{b:02x}" for b in chunk)
            ascii_str = ''.join(chr(b) if 32 <= b < 127 else '.' for b in chunk)
            print(f"{offset_label}: {hex_str:<48} {ascii_str}")

if __name__ == '__main__':
    # Test with first available packet
    packet_dir = 'd:\\bse\\data\\raw_packets'
    packet_files = [f for f in os.listdir(packet_dir) if 'type2020' in f]
    
    if packet_files:
        packet_path = os.path.join(packet_dir, packet_files[0])
        hex_dump_packet(packet_path)
    else:
        print("No type 2020 packets found!")
