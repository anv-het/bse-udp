"""
Test with corrected understanding:
- Token: Little-Endian (4 bytes)
- All other fields: BIG-ENDIAN!
- Uncompressed section: 76 bytes
- Manual page 49: "Binary values are presented in big-endian byte order"
"""

import struct

# Read packet
with open('d:\\bse\\data\\raw_packets\\20251017_102330_714968_type2020_packet.bin', 'rb') as f:
    data = f.read()

print("="*100)
print("CORRECTED PARSING WITH BIG-ENDIAN")
print("="*100)
print()

# Record starts at offset 36
record_start = 36
record_bytes = data[record_start:record_start+76]

print(f"First 76 bytes (hex): {record_bytes.hex()}\n")

# Token (Little-Endian!)
token = struct.unpack('<I', record_bytes[0:4])[0]
print(f"Token (offset 0-3, Little-Endian):          {token}")
print()

# No of Trades (Big-Endian!)
num_trades = struct.unpack('>I', record_bytes[4:8])[0]
print(f"No of Trades (offset 4-7, Big-Endian):      {num_trades:,}")
print()

# Traded Volume (Big-Endian!)
volume = struct.unpack('>q', record_bytes[8:16])[0]
print(f"Traded Volume (offset 8-15, Big-Endian):    {volume:,}")
print()

# Traded Value (Big-Endian!)
value = struct.unpack('>q', record_bytes[16:24])[0]
print(f"Traded Value (offset 16-23, Big-Endian):    {value:,} paise = ₹{value/100:,.2f}")
print()

# Market Type (Big-Endian!)
market_type = struct.unpack('>H', record_bytes[28:30])[0]
print(f"Market Type (offset 28-29, Big-Endian):     {market_type}")
print()

# Session Number (Big-Endian!)
session = struct.unpack('>H', record_bytes[30:32])[0]
print(f"Session Number (offset 30-31, Big-Endian):  {session}")
print()

# No of Price Points (Big-Endian!)
price_points = struct.unpack('>H', record_bytes[50:52])[0]
print(f"No of Price Points (offset 50-51, Big-Endian): {price_points}")
print()

# Timestamp (Big-Endian!)
timestamp = struct.unpack('>Q', record_bytes[52:60])[0]
print(f"Timestamp (offset 52-59, Big-Endian):       {timestamp}")
print()

# Close Rate (Big-Endian!)
close_rate = struct.unpack('>i', record_bytes[60:64])[0]
print(f"Close Rate (offset 60-63, Big-Endian):      {close_rate:,} paise = ₹{close_rate/100:,.2f}")
print()

# LTQ - Last Traded Quantity (Big-Endian!)
ltq = struct.unpack('>q', record_bytes[64:72])[0]
print(f"LTQ (offset 64-71, Big-Endian):             {ltq:,}")
print()

# LTP - Last Traded Price (Big-Endian!)
ltp = struct.unpack('>i', record_bytes[72:76])[0]
print(f"LTP (offset 72-75, Big-Endian):             {ltp:,} paise = ₹{ltp/100:,.2f}")
print()

print("="*100)
print("COMPARISON WITH CSV DATA")
print("="*100)
print()
print("From 20251017_quotes.csv, token 878192:")
print("  Expected LTP: ~8,556,380 paise (₹85,563.80)")
print("  Expected Volume: ~403,374,080")
print()

if ltp > 100000:  # More than 1000 rupees
    print(f"✅ LTP looks reasonable: ₹{ltp/100:,.2f}")
else:
    print(f"❌ LTP too small: ₹{ltp/100:,.2f}")

if 100000000 < volume < 1000000000:  # Between 1 crore and 100 crore
    print(f"✅ Volume looks reasonable: {volume:,}")
else:
    print(f"⚠️  Volume unusual: {volume:,}")

print()
print("="*100)
print("TESTING LITTLE-ENDIAN AS ALTERNATIVE")
print("="*100)
print()

# Try Little-Endian for comparison
trades_le = struct.unpack('<I', record_bytes[4:8])[0]
volume_le = struct.unpack('<q', record_bytes[8:16])[0]
ltp_le = struct.unpack('<i', record_bytes[72:76])[0]

print(f"No of Trades (Little-Endian):    {trades_le:,}")
print(f"Volume (Little-Endian):          {volume_le:,}")
print(f"LTP (Little-Endian):             {ltp_le:,} paise = ₹{ltp_le/100:,.2f}")
print()

# Which looks more reasonable?
if abs(trades_le - 1000) < abs(num_trades - 1000):
    print("Trades: Little-Endian looks more reasonable")
else:
    print("Trades: Big-Endian looks more reasonable")

if 100000000 < volume_le < 1000000000:
    print("Volume: Little-Endian looks more reasonable")
elif 100000000 < volume < 1000000000:
    print("Volume: Big-Endian looks more reasonable")
else:
    print("Volume: Neither looks reasonable!")
