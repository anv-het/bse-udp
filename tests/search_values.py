"""
Search for known values in packet bytes
"""

import struct

# Read packet
with open('d:\\bse\\data\\raw_packets\\20251017_102330_714968_type2020_packet.bin', 'rb') as f:
    data = f.read()

print("="*100)
print("SEARCHING FOR KNOWN VALUES IN PACKET")
print("="*100)
print()

# Expected values from CSV for token 878192
expected_ltp = 8556380  # paise (might be stored differently)
expected_volume = 403374080

print(f"Searching for LTP ~8,556,380 paise")
print(f"Searching for Volume ~403,374,080")
print()

# Search for volume (it's a large number, might be 8 bytes)
# Try Little-Endian int64
volume_bytes_le = struct.pack('<q', expected_volume)
if volume_bytes_le in data:
    offset = data.index(volume_bytes_le)
    print(f"✅ Found volume {expected_volume:,} (Little-Endian int64) at offset {offset}")
else:
    print(f"❌ Volume {expected_volume:,} not found as Little-Endian int64")

# Try Big-Endian int64
volume_bytes_be = struct.pack('>q', expected_volume)
if volume_bytes_be in data:
    offset = data.index(volume_bytes_be)
    print(f"✅ Found volume {expected_volume:,} (Big-Endian int64) at offset {offset}")
else:
    print(f"❌ Volume {expected_volume:,} not found as Big-Endian int64")

# Try Little-Endian int32 (maybe it's 32-bit?)
volume_bytes_le32 = struct.pack('<i', expected_volume)
if volume_bytes_le32 in data:
    offset = data.index(volume_bytes_le32)
    print(f"✅ Found volume {expected_volume:,} (Little-Endian int32) at offset {offset}")
else:
    print(f"❌ Volume {expected_volume:,} not found as Little-Endian int32")

print()

# Search for LTP
# Try Little-Endian int32
ltp_bytes_le = struct.pack('<i', expected_ltp)
if ltp_bytes_le in data:
    offset = data.index(ltp_bytes_le)
    print(f"✅ Found LTP {expected_ltp:,} paise (Little-Endian int32) at offset {offset}")
else:
    print(f"❌ LTP {expected_ltp:,} paise not found as Little-Endian int32")

# Try Big-Endian int32
ltp_bytes_be = struct.pack('>i', expected_ltp)
if ltp_bytes_be in data:
    offset = data.index(ltp_bytes_be)
    print(f"✅ Found LTP {expected_ltp:,} paise (Big-Endian int32) at offset {offset}")
else:
    print(f"❌ LTP {expected_ltp:,} paise not found as Big-Endian int32")

# Try in centipaise (multiply by 100)
ltp_centipaise = expected_ltp * 100
ltp_cp_le = struct.pack('<i', ltp_centipaise)
if ltp_cp_le in data:
    offset = data.index(ltp_cp_le)
    print(f"✅ Found LTP {ltp_centipaise:,} centipaise (Little-Endian int32) at offset {offset}")
else:
    print(f"❌ LTP {ltp_centipaise:,} centipaise not found as Little-Endian int32")

print()
print("="*100)
print("BYTE-BY-BYTE SCAN FOR LARGE VALUES")
print("="*100)
print()

# Scan through packet looking for values in reasonable ranges
record_start = 36
for offset in range(record_start, min(record_start + 200, len(data) - 8), 1):
    # Try reading as int32 Little-Endian
    val_le_i32 = struct.unpack('<i', data[offset:offset+4])[0]
    val_be_i32 = struct.unpack('>i', data[offset:offset+4])[0]
    val_le_i64 = struct.unpack('<q', data[offset:offset+8])[0]
    val_be_i64 = struct.unpack('>q', data[offset:offset+8])[0]
    
    # Check if any value is close to our expected LTP (within 10%)
    if 7700000 < abs(val_le_i32) < 9400000:
        print(f"Offset {offset:3d}: int32-LE = {val_le_i32:>12,} ← Possible LTP!")
    
    if 7700000 < abs(val_be_i32) < 9400000:
        print(f"Offset {offset:3d}: int32-BE = {val_be_i32:>12,} ← Possible LTP!")
    
    # Check if any value is close to our expected volume
    if 350000000 < abs(val_le_i64) < 450000000:
        print(f"Offset {offset:3d}: int64-LE = {val_le_i64:>12,} ← Possible Volume!")
    
    if 350000000 < abs(val_be_i64) < 450000000:
        print(f"Offset {offset:3d}: int64-BE = {val_be_i64:>12,} ← Possible Volume!")
