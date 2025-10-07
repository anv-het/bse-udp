import struct

# Read raw packet
with open(r'd:\bse\data\raw_packets\20251006_115104_524083_type2020_packet.bin', 'rb') as f:
    data = f.read()

print(f'Packet size: {len(data)} bytes')
print(f'\n=== Header (first 36 bytes) ===')
print(f'Raw hex: {data[:36].hex()}')
print(f'Format ID at offset 4-5 (LE): {struct.unpack("<H", data[4:6])[0]:#06x} ({struct.unpack("<H", data[4:6])[0]})')
print(f'Time fields (offset 20-25):')
print(f'  Hour (LE):   {struct.unpack("<H", data[20:22])[0]}')
print(f'  Minute (LE): {struct.unpack("<H", data[22:24])[0]}')
print(f'  Second (LE): {struct.unpack("<H", data[24:26])[0]}')

print(f'\n=== First Record (offset 36-99, 64 bytes) ===')
rec = data[36:100]
token_le = struct.unpack('<I', rec[0:4])[0]
token_be = struct.unpack('>I', rec[0:4])[0]
print(f'Token bytes: {rec[0:4].hex()}')
print(f'  Little-Endian: {token_le}')
print(f'  Big-Endian: {token_be}')

print(f'\nNext fields (offset 4-20):')
print(f'Raw hex: {rec[4:20].hex()}')

# Try different interpretations for the LTP field (should be around 1000-5000 rupees = 100000-500000 paise)
print(f'\nTrying different field interpretations:')
print(f'Field at offset 4-7 (4 bytes):')
print(f'  LE uint32: {struct.unpack("<I", rec[4:8])[0]}')
print(f'  BE uint32: {struct.unpack(">I", rec[4:8])[0]}')
print(f'  LE int32: {struct.unpack("<i", rec[4:8])[0]}')
print(f'  BE int32: {struct.unpack(">i", rec[4:8])[0]}')
print(f'  LE float32: {struct.unpack("<f", rec[4:8])[0]}')
print(f'  BE float32: {struct.unpack(">f", rec[4:8])[0]}')

print(f'\nField at offset 8-11 (4 bytes):')
print(f'  LE uint32: {struct.unpack("<I", rec[8:12])[0]}')
print(f'  BE uint32: {struct.unpack(">I", rec[8:12])[0]}')
print(f'  LE int32: {struct.unpack("<i", rec[8:12])[0]}')
print(f'  BE int32: {struct.unpack(">i", rec[8:12])[0]}')
