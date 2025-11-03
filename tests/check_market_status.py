#!/usr/bin/env python3
"""
Show what data is available from today's market session
and what happens if you run the test NOW (after market close)
"""

import pandas as pd
from datetime import datetime
from pathlib import Path

print("=" * 70)
print("BSE DATA COLLECTION - AFTER HOURS ANALYSIS")
print("=" * 70)

# Current time
now = datetime.now()
print(f"\n📅 Current time: {now.strftime('%Y-%m-%d %H:%M:%S')}")

# Market hours
market_open = now.replace(hour=9, minute=15, second=0)
market_close = now.replace(hour=15, minute=30, second=0)
is_market_hours = market_open <= now <= market_close

print(f"⏰ Market hours: 09:15 AM - 03:30 PM")
print(f"📊 Market status: {'🟢 OPEN' if is_market_hours else '🔴 CLOSED'}")

# Check today's data file
csv_file = Path('data/processed_csv/20251017_quotes.csv')

if not csv_file.exists():
    print(f"\n❌ No data file found: {csv_file}")
    print("   No data was collected during today's market hours")
else:
    print(f"\n✅ Data file exists: {csv_file}")
    print(f"   File size: {csv_file.stat().st_size:,} bytes")
    
    # Read data
    df = pd.read_csv(csv_file, low_memory=False)
    
    print(f"\n📈 DATA CAPTURED DURING TODAY'S MARKET HOURS:")
    print(f"   Total rows: {len(df):,}")
    print(f"   First update: {df.iloc[0]['timestamp']}")
    print(f"   Last update: {df.iloc[-1]['timestamp']}")
    
    # SENSEX Future analysis
    sensex = df[df['token'] == 861384]
    
    if len(sensex) > 0:
        last_row = sensex.iloc[-1]
        
        print(f"\n📊 SENSEX FUTURE (Token 861384):")
        print(f"   Total updates captured: {len(sensex):,}")
        print(f"   Last LTP: ₹{last_row['ltp']:,.2f}")
        print(f"   OHLC: Open=₹{last_row['open']:,.2f}, High=₹{last_row['high']:,.2f}, "
              f"Low=₹{last_row['low']:,.2f}, Close=₹{last_row['close']:,.2f}")
        print(f"   Volume: {last_row['volume']:,}")

print("\n" + "=" * 70)
print("WHAT HAPPENS IF YOU RUN THE TEST NOW?")
print("=" * 70)

if is_market_hours:
    print("\n✅ Market is OPEN - You will get LIVE real-time data!")
    print("   - Packets arriving every ~0.8 seconds")
    print("   - Live price updates")
    print("   - Active order book")
    print("\n💡 Run: python src/main.py")
else:
    print("\n❌ Market is CLOSED - You will get NO NEW DATA!")
    print("   - Socket connects successfully")
    print("   - But NO packets arrive from BSE")
    print("   - System waits forever (until Ctrl+C)")
    print("   - CSV/JSON files remain unchanged")
    
    # Time until next market open
    if now > market_close:
        next_open = now.replace(day=now.day+1, hour=9, minute=15, second=0)
    else:
        next_open = market_open
    
    wait_time = next_open - now
    hours = int(wait_time.total_seconds() // 3600)
    minutes = int((wait_time.total_seconds() % 3600) // 60)
    
    print(f"\n⏰ Next market session: {next_open.strftime('%Y-%m-%d %H:%M')}")
    print(f"   Time remaining: {hours}h {minutes}m")
    
    print("\n💡 OPTIONS:")
    print("   1. Wait for tomorrow's market hours (9:15 AM)")
    print("   2. Analyze existing data from today (see above)")
    print("   3. Switch to simulation feed in config.json")

print("\n" + "=" * 70)
