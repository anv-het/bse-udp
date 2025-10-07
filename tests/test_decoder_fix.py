"""
Test if decoder correctly handles format 0x0234 (564-byte) packets
"""
import struct
from pathlib import Path

# Read a test packet
packet_file = Path("data/raw_packets/20251006_115104_524083_type2020_packet.bin")
with open(packet_file, "rb") as f:
    packet = f.read()

print(f"Packet size: {len(packet)} bytes")

# Check format ID with Little-Endian
format_id_le = struct.unpack('<H', packet[4:6])[0]
print(f"Format ID (LE): 0x{format_id_le:04x} ({format_id_le})")

# Check format ID with Big-Endian (old way)
format_id_be = struct.unpack('>H', packet[4:6])[0]
print(f"Format ID (BE): 0x{format_id_be:04x} ({format_id_be}) - WRONG!")

# Determine record size
record_size = 66 if format_id_le == 0x0234 else 64
print(f"\nRecord size: {record_size} bytes")

# Calculate number of records
header_size = 36
data_size = len(packet) - header_size
num_records = data_size // record_size
print(f"Number of records: {num_records} (header={header_size}, data={data_size})")

# Parse first record (offset 36)
print(f"\n=== First Record (66 bytes) ===")
record_start = 36
record_bytes = packet[record_start:record_start+66]

# Token (0-3, LE)
token = struct.unpack('<I', record_bytes[0:4])[0]
print(f"Token: {token}")

# Fields at 4-7, 8-11, 12-15 (LE uint32, paise)
field1 = struct.unpack('<I', record_bytes[4:8])[0]
field2 = struct.unpack('<I', record_bytes[8:12])[0]
field3 = struct.unpack('<I', record_bytes[12:16])[0]
print(f"Field at 4-7: {field1} paise = {field1/100:.2f} Rupees (likely LTP)")
print(f"Field at 8-11: {field2} paise = {field2/100:.2f} Rupees")
print(f"Field at 12-15: {field3} paise = {field3/100:.2f} Rupees")

# Volume at 16-19 (LE uint32)
volume = struct.unpack('<I', record_bytes[16:20])[0]
print(f"Volume: {volume}")

print(f"\n✓ If token={token} and LTP around 380-476 Rupees, decoder fix is working!")
print(f"✓ Expected token: 861201 (SENSEX CE 82700 option)")
