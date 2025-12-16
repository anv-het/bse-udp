"""
BSE Live Greeks Monitor - DIRECT UDP FEED (Python)
===================================================

Real-time Greeks calculation from LIVE BSE UDP multicast feeds.
Reads F&O token data + Index spot prices directly from UDP (before CSV save).

Uses py_vollib for accurate Black-Scholes Greeks calculations.

================================================================================
INSTALLATION
================================================================================

pip install py_vollib scipy numpy

================================================================================
USAGE
================================================================================

1. Monitor SENSEX CE 84900 (Token 1146822):
   python test_live_greeks_udp.py --token 1146822 --ticks 10

2. Monitor SENSEX PE 84900 (Token 1149680):
   python test_live_greeks_udp.py --token 1149680 --ticks 20

3. Custom multicast IPs and ports:
   python test_live_greeks_udp.py --token 1146822 --foip 239.1.2.5 --foport 26002 --indexip 239.1.2.5 --indexport 26001 --ticks 50

4. Custom risk-free rate:
   python test_live_greeks_udp.py --token 1146822 --rate 0.07 --ticks 10

PARAMETERS:
  --token int       Token ID to monitor (required, e.g., 1146822)
  --ticks int       Maximum ticks to capture (default: 100)
  --rate float      Risk-free rate (default: 0.065 = 6.5%)
  --foip string     F&O multicast IP (default: "239.1.2.5")
  --foport int      F&O UDP port (default: 26002)
  --indexip string  Index multicast IP (default: "239.1.2.5")
  --indexport int   Index UDP port (default: 26001)

OUTPUT:
  - Live terminal display with market data + Greeks
  - CSV file: data/output/YYYYMMDD_HHMMSS_TOKEN_greeks_udp_live.csv

================================================================================
"""

import sys
import socket
import struct
import csv
import os
import time
import threading
import argparse
from pathlib import Path
from datetime import datetime, timedelta
from typing import Dict, Optional, Tuple
from dataclasses import dataclass

# Try to import py_vollib (best library for Greeks)
try:
    from py_vollib.black_scholes import black_scholes
    from py_vollib.black_scholes.implied_volatility import implied_volatility
    from py_vollib.black_scholes.greeks.analytical import delta, gamma, theta, vega, rho
    VOLLIB_AVAILABLE = True
except ImportError:
    VOLLIB_AVAILABLE = False
    print("⚠️  py_vollib not installed. Install with: pip install py_vollib scipy numpy")

# ============================================================================
# CONFIGURATION
# ============================================================================

DEFAULT_FO_MULTICAST_IP = "239.1.2.5"
DEFAULT_INDEX_MULTICAST_IP = "239.1.2.5"
DEFAULT_FO_PORT = 26002
DEFAULT_INDEX_PORT = 26001
DEFAULT_RISK_FREE_RATE = 0.065  # 6.5% Indian T-Bills
HEADER_SIZE = 36
RECORD_SIZE = 264
INDEX_RECORD_SIZE = 40
BUFFER_SIZE = 2048

# ============================================================================
# DATA STRUCTURES
# ============================================================================

@dataclass
class OptionGreeks:
    implied_volatility: float = 0.0
    delta: float = 0.0
    gamma: float = 0.0
    theta: float = 0.0
    vega: float = 0.0
    rho: float = 0.0
    vanna: float = 0.0
    vomma: float = 0.0
    charm: float = 0.0

@dataclass
class FOTickData:
    timestamp: datetime
    token: int
    symbol: str
    contract: str
    option_type: str
    strike: float
    expiry: str
    ltp: float
    open: float
    high: float
    low: float
    prev_close: float
    atp: float
    volume: int
    turnover_lakhs: int
    seq_num: int
    spot_price: float
    greeks: OptionGreeks
    calc_time_ms: float

@dataclass
class TokenInfo:
    symbol: str
    contract: str
    expiry: str
    option_type: str
    strike: float

# ============================================================================
# GLOBAL STATE
# ============================================================================

token_map: Dict[int, TokenInfo] = {}
spot_prices: Dict[str, float] = {}
spot_prices_lock = threading.Lock()

# ============================================================================
# TOKEN MAP LOADING
# ============================================================================

def load_contract_master() -> int:
    """Load F&O contract master from CSV file."""
    import csv
    from pathlib import Path
    
    # Find latest contract file
    tokens_dir = Path(__file__).parent.parent / 'data' / 'tokens'
    pattern = "BSE_EQD_CONTRACT_*.csv"
    files = [f for f in tokens_dir.glob(pattern) if '_fetched' not in f.name]
    
    if not files:
        print(f"   ❌ No contract files found in {tokens_dir}")
        return 0
    
    latest_file = sorted(files)[-1]
    
    count = 0
    with open(latest_file, 'r', encoding='utf-8') as f:
        reader = csv.reader(f)
        line_num = 0
        
        for row in reader:
            line_num += 1
            
            # Skip header
            if line_num == 1 and row[0].startswith('FinInstrmId'):
                continue
            
            if len(row) < 20:
                continue
            
            try:
                token = int(row[0])
                if token == 0:
                    continue
                
                symbol = row[3]
                contract = row[18]
                expiry = row[4]
                opt_type = row[6]
                
                strike = 0.0
                if len(row) > 5 and row[5]:
                    try:
                        strike = float(row[5]) / 100.0  # Paise to Rupees
                    except:
                        pass
                
                token_map[token] = TokenInfo(
                    symbol=symbol,
                    contract=contract,
                    expiry=expiry,
                    option_type=opt_type,
                    strike=strike
                )
                count += 1
                
            except Exception as e:
                continue
    
    return count

def get_token_info(token: int) -> Optional[TokenInfo]:
    """Get token info from map."""
    return token_map.get(token)

# ============================================================================
# SPOT PRICE MANAGEMENT (Thread-Safe)
# ============================================================================

def update_spot_price(symbol: str, price: float):
    """Update spot price in thread-safe cache."""
    with spot_prices_lock:
        spot_prices[symbol] = price

def get_spot_price(symbol: str) -> Tuple[float, bool]:
    """Get spot price from cache."""
    with spot_prices_lock:
        price = spot_prices.get(symbol, 0.0)
        return price, (price > 0)

# ============================================================================
# GREEKS CALCULATION (Using py_vollib)
# ============================================================================

def calculate_implied_volatility(spot_price: float, strike_price: float, time_to_expiry: float, 
                                 risk_free_rate: float, market_price: float, is_call: bool) -> Tuple[float, bool]:
    """Calculate Implied Volatility using py_vollib."""
    if not VOLLIB_AVAILABLE:
        return 0.0, False
    
    if market_price <= 0 or spot_price <= 0 or strike_price <= 0 or time_to_expiry <= 0:
        return 0.0, False
    
    try:
        flag = 'c' if is_call else 'p'
        iv = implied_volatility(market_price, spot_price, strike_price, time_to_expiry, risk_free_rate, flag)
        
        # Validate IV
        if iv > 0 and iv < 3.0:  # IV between 0% and 300%
            return iv, True
        else:
            return 0.0, False
            
    except Exception as e:
        return 0.0, False

def calculate_greeks(spot_price: float, strike_price: float, time_to_expiry: float,
                     risk_free_rate: float, sigma: float, is_call: bool) -> OptionGreeks:
    """Calculate all Greeks using py_vollib."""
    greeks = OptionGreeks()
    
    if not VOLLIB_AVAILABLE:
        return greeks
    
    if spot_price <= 0 or strike_price <= 0 or time_to_expiry <= 0 or sigma <= 0:
        return greeks
    
    try:
        flag = 'c' if is_call else 'p'
        
        greeks.implied_volatility = sigma
        
        # Calculate standard Greeks
        greeks.delta = delta(flag, spot_price, strike_price, time_to_expiry, risk_free_rate, sigma)
        greeks.gamma = gamma(flag, spot_price, strike_price, time_to_expiry, risk_free_rate, sigma)
        greeks.theta = theta(flag, spot_price, strike_price, time_to_expiry, risk_free_rate, sigma) / 365.0  # Per day
        greeks.vega = vega(flag, spot_price, strike_price, time_to_expiry, risk_free_rate, sigma) / 100.0  # Per 1%
        greeks.rho = rho(flag, spot_price, strike_price, time_to_expiry, risk_free_rate, sigma) / 100.0  # Per 1%
        
        # Advanced Greeks (calculate manually)
        from scipy.stats import norm
        import math
        
        d1 = (math.log(spot_price / strike_price) + (risk_free_rate + 0.5 * sigma**2) * time_to_expiry) / (sigma * math.sqrt(time_to_expiry))
        d2 = d1 - sigma * math.sqrt(time_to_expiry)
        
        nd1 = norm.pdf(d1)
        sqrt_t = math.sqrt(time_to_expiry)
        
        # Vanna = dDelta/dVol = dVega/dSpot
        greeks.vanna = -(nd1 / spot_price) * (1 - d1 / (sigma * sqrt_t))
        
        # Vomma (Volga) = dVega/dVol
        greeks.vomma = greeks.vega * 100.0 * d1 * d2 / sigma
        
        # Charm = dDelta/dTime
        greeks.charm = -nd1 * (2 * risk_free_rate * time_to_expiry - d2 * sigma * sqrt_t) / (2 * time_to_expiry * sigma * sqrt_t)
        
    except Exception as e:
        print(f"   ⚠️  Greeks calculation error: {e}")
    
    return greeks

# ============================================================================
# UDP DECODERS
# ============================================================================

def decode_fo_record(record: bytes) -> Optional[FOTickData]:
    """Decode F&O record (message type 2020/2021)."""
    if len(record) < RECORD_SIZE:
        return None
    
    token = struct.unpack('<I', record[0:4])[0]
    if token == 0:
        return None
    
    info = get_token_info(token)
    if info is None:
        return None
    
    # Decode fields (Little-Endian, values in paise)
    open_price = struct.unpack('<i', record[4:8])[0] / 100.0
    prev_close = struct.unpack('<i', record[8:12])[0] / 100.0
    high = struct.unpack('<i', record[12:16])[0] / 100.0
    low = struct.unpack('<i', record[16:20])[0] / 100.0
    volume = struct.unpack('<i', record[24:28])[0]
    turnover_lakhs = struct.unpack('<I', record[28:32])[0]
    ltp = struct.unpack('<i', record[36:40])[0] / 100.0
    
    atp = 0.0
    if len(record) >= 88:
        atp = struct.unpack('<i', record[84:88])[0] / 100.0
    
    seq_num = 0
    if len(record) >= 48:
        seq_num = struct.unpack('<I', record[44:48])[0]
    
    return FOTickData(
        timestamp=datetime.now(),
        token=token,
        symbol=info.symbol,
        contract=info.contract,
        option_type=info.option_type,
        strike=info.strike,
        expiry=info.expiry,
        ltp=ltp,
        open=open_price,
        high=high,
        low=low,
        prev_close=prev_close,
        atp=atp,
        volume=volume,
        turnover_lakhs=turnover_lakhs,
        seq_num=seq_num,
        spot_price=0.0,
        greeks=OptionGreeks(),
        calc_time_ms=0.0
    )

def decode_index_record(record: bytes) -> Tuple[str, float, bool]:
    """Decode Index record (message type 2012)."""
    if len(record) < INDEX_RECORD_SIZE:
        return "", 0.0, False
    
    # Index code (2 bytes at offset 0, Little-Endian)
    index_code = struct.unpack('<H', record[0:2])[0]
    
    # LTP (Last Traded Price) - THE LIVE PRICE THAT CHANGES EVERY TICK!
    # Offset 20, 4 bytes, int32 Little-Endian, in paise (divide by 100)
    ltp = struct.unpack('<i', record[20:24])[0]
    index_value = ltp / 100.0
    
    if index_value <= 0:
        return "", 0.0, False
    
    # Map index codes to symbols
    index_symbol = ""
    if index_code == 1:
        index_symbol = "SENSEX"
    elif index_code == 12:  # Corrected from 46
        index_symbol = "BANKEX"
    elif index_code == 47:
        index_symbol = "SNSX50"
    elif index_code == 2:
        index_symbol = "BSE100"
    else:
        return "", 0.0, False
    
    return index_symbol, index_value, True

# ============================================================================
# MULTICAST SOCKETS
# ============================================================================

def create_multicast_socket(multicast_ip: str, port: int) -> socket.socket:
    """Create UDP multicast socket."""
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    
    # Bind to port
    sock.bind(('', port))
    
    # Join multicast group
    mreq = struct.pack("4sl", socket.inet_aton(multicast_ip), socket.INADDR_ANY)
    sock.setsockopt(socket.IPPROTO_IP, socket.IP_ADD_MEMBERSHIP, mreq)
    
    # Set buffer size
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 16 * 1024 * 1024)
    
    # Set timeout for Ctrl+C
    sock.settimeout(0.1)
    
    return sock

# ============================================================================
# INDEX FEED LISTENER (Background Thread)
# ============================================================================

def listen_index_feed(multicast_ip: str, port: int, stop_event: threading.Event):
    """Listen to index feed in background thread."""
    try:
        conn = create_multicast_socket(multicast_ip, port)
        print(f"   ✅ Index feed connected ({multicast_ip}:{port})")
        
        buffer = bytearray(BUFFER_SIZE)
        
        while not stop_event.is_set():
            try:
                n, _ = conn.recvfrom_into(buffer)
                
                if n < HEADER_SIZE + INDEX_RECORD_SIZE:
                    continue
                
                # Check message type (2012 = Index)
                msg_type = struct.unpack('<H', buffer[8:10])[0]
                if msg_type != 2012:
                    continue
                
                # Decode index record
                record = bytes(buffer[HEADER_SIZE:HEADER_SIZE + INDEX_RECORD_SIZE])
                symbol, price, ok = decode_index_record(record)
                if ok:
                    update_spot_price(symbol, price)
                    
            except socket.timeout:
                continue
            except Exception as e:
                continue
        
        conn.close()
        
    except Exception as e:
        print(f"❌ Index feed failed: {e}")

# ============================================================================
# DISPLAY FUNCTIONS
# ============================================================================

def format_change(current: float, previous: float) -> str:
    """Format price change with arrow."""
    if previous == 0:
        return "  NEW"
    
    change = current - previous
    pct = (change / previous) * 100
    
    if change > 0:
        return f" ▲ +{pct:.2f}%"
    elif change < 0:
        return f" ▼ {pct:.2f}%"
    else:
        return "   ━ 0.00%"

def get_moneyness(spot_price: float, strike_price: float, is_call: bool) -> str:
    """Get moneyness (ITM/ATM/OTM)."""
    diff = abs(spot_price - strike_price)
    if diff < 100:
        return "ATM"
    if is_call:
        return "ITM" if spot_price > strike_price else "OTM"
    else:
        return "ITM" if strike_price > spot_price else "OTM"

def display_tick_with_greeks(data: FOTickData, tick_num: int, prev_ltp: float):
    """Display tick with market data and Greeks."""
    now = data.timestamp.strftime("%H:%M:%S.%f")[:-3]
    change_str = format_change(data.ltp, prev_ltp)
    
    print("\n" + "=" * 80)
    print(f"  TICK #{tick_num:<6}  │  {now}  │  Token: {data.token}  │  {data.contract}")
    print("=" * 80)
    
    print(f"\n  💰 LTP: ₹{data.ltp:.2f}{change_str}")
    print("  " + "-" * 72)
    print(f"  Open: ₹{data.open:.2f}  │  High: ₹{data.high:.2f}  │  Low: ₹{data.low:.2f}  │  Prev: ₹{data.prev_close:.2f}")
    print(f"  ATP:  ₹{data.atp:.2f}  │  Volume: {data.volume}  │  Turnover: ₹{data.turnover_lakhs}L")
    
    # Option Details
    days_to_expiry = 0
    if data.expiry:
        try:
            expiry_date = datetime.strptime(data.expiry, "%Y-%m-%d")
            days_to_expiry = (expiry_date - datetime.now()).days
        except:
            pass
    
    moneyness = get_moneyness(data.spot_price, data.strike, data.option_type == "CE")
    intrinsic_value = 0.0
    if data.option_type == "CE":
        intrinsic_value = max(0, data.spot_price - data.strike)
    elif data.option_type == "PE":
        intrinsic_value = max(0, data.strike - data.spot_price)
    time_value = data.ltp - intrinsic_value
    
    print("\n  📋 OPTION DETAILS")
    print("  " + "-" * 72)
    print(f"  Symbol: {data.symbol}  │  Type: {data.option_type}  │  Strike: ₹{data.strike:.2f}  │  Expiry: {data.expiry}")
    print(f"  Spot Price: ₹{data.spot_price:.2f}  │  Moneyness: {moneyness}  │  Days to Expiry: {days_to_expiry}")
    print(f"  Intrinsic Value: ₹{intrinsic_value:.2f}  │  Time Value: ₹{time_value:.2f}")
    
    # Greeks
    print("\n  🎯 OPTION GREEKS")
    print("  " + "-" * 72)
    print(f"  Implied Volatility: {data.greeks.implied_volatility * 100:.2f}%")
    print()
    print(f"  Delta:         {data.greeks.delta:10.6f}   │   Gamma:         {data.greeks.gamma:10.6f}")
    print(f"  Theta:         {data.greeks.theta:10.2f}   │   Vega:          {data.greeks.vega:10.2f}")
    print(f"  Rho:           {data.greeks.rho:10.2f}")
    
    print("\n  🔬 ADVANCED GREEKS")
    print("  " + "-" * 72)
    print(f"  Vanna:  {data.greeks.vanna:10.6f}   │   Vomma:  {data.greeks.vomma:10.2f}   │   Charm:  {data.greeks.charm:10.6f}")
    
    print(f"\n  ⚡ Calculation Time: {data.calc_time_ms:.2f} ms")
    print("  " + "-" * 72)

# ============================================================================
# CSV WRITER
# ============================================================================

class CSVWriter:
    def __init__(self, token: int, symbol: str):
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        safe_symbol = symbol.replace(" ", "_").replace("/", "-")
        
        output_dir = Path(__file__).parent.parent / 'data' / 'output'
        output_dir.mkdir(parents=True, exist_ok=True)
        
        self.filename = output_dir / f"{timestamp}_{token}_{safe_symbol}_greeks_udp_live.csv"
        self.file = open(self.filename, 'w', newline='', encoding='utf-8')
        self.writer = csv.writer(self.file)
        
        # Write header
        headers = [
            'timestamp', 'token', 'symbol', 'contract', 'option_type', 'strike_price', 'expiry_date',
            'ltp', 'prev_close', 'open', 'high', 'low', 'volume', 'value', 'atp',
            'spot_price', 'days_to_expiry', 'implied_volatility',
            'delta', 'gamma', 'theta', 'vega', 'rho',
            'vanna', 'vomma', 'charm',
            'intrinsic_value', 'time_value', 'moneyness', 'calc_time_ms'
        ]
        self.writer.writerow(headers)
    
    def write_tick(self, data: FOTickData):
        """Write tick data to CSV."""
        days_to_expiry = 0
        if data.expiry:
            try:
                expiry_date = datetime.strptime(data.expiry, "%Y-%m-%d")
                days_to_expiry = (expiry_date - datetime.now()).days
            except:
                pass
        
        moneyness = get_moneyness(data.spot_price, data.strike, data.option_type == "CE")
        intrinsic_value = 0.0
        if data.option_type == "CE":
            intrinsic_value = max(0, data.spot_price - data.strike)
        elif data.option_type == "PE":
            intrinsic_value = max(0, data.strike - data.spot_price)
        time_value = data.ltp - intrinsic_value
        
        row = [
            data.timestamp.strftime("%Y-%m-%d %H:%M:%S.%f")[:-3],
            data.token,
            data.symbol,
            data.contract,
            data.option_type,
            f"{data.strike:.2f}",
            data.expiry,
            f"{data.ltp:.2f}",
            f"{data.prev_close:.2f}",
            f"{data.open:.2f}",
            f"{data.high:.2f}",
            f"{data.low:.2f}",
            data.volume,
            data.turnover_lakhs,
            f"{data.atp:.2f}",
            f"{data.spot_price:.2f}",
            days_to_expiry,
            f"{data.greeks.implied_volatility:.6f}",
            f"{data.greeks.delta:.6f}",
            f"{data.greeks.gamma:.6f}",
            f"{data.greeks.theta:.2f}",
            f"{data.greeks.vega:.2f}",
            f"{data.greeks.rho:.2f}",
            f"{data.greeks.vanna:.6f}",
            f"{data.greeks.vomma:.2f}",
            f"{data.greeks.charm:.6f}",
            f"{intrinsic_value:.2f}",
            f"{time_value:.2f}",
            moneyness,
            f"{data.calc_time_ms:.2f}"
        ]
        self.writer.writerow(row)
        self.file.flush()
    
    def close(self):
        """Close CSV file."""
        self.file.close()

# ============================================================================
# MAIN MONITOR
# ============================================================================

def monitor_live_greeks(target_token: int, fo_ip: str, fo_port: int, index_ip: str, 
                       index_port: int, risk_free_rate: float, max_ticks: int):
    """Main monitoring function."""
    print("=" * 80)
    print("BSE LIVE GREEKS MONITOR - DIRECT UDP FEED (Python)")
    print("=" * 80)
    print(f"Token:      {target_token}")
    print(f"Risk Rate:  {risk_free_rate * 100:.2f}%")
    print(f"Date/Time:  {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print("=" * 80)
    
    # Get token info
    info = get_token_info(target_token)
    if info is None:
        print(f"❌ Token {target_token} not found in contract master")
        return
    
    print(f"\n📊 Token {target_token} → {info.contract}")
    print(f"   Symbol:     {info.symbol}")
    if info.expiry:
        print(f"   Expiry:     {info.expiry}")
    if info.option_type and info.option_type != "XX":
        print(f"   Type:       {info.option_type}")
    if info.strike > 0:
        print(f"   Strike:     ₹{info.strike:.2f}")
    
    # Create CSV writer
    csv_writer = CSVWriter(target_token, info.contract)
    print(f"\n💾 CSV File: {csv_writer.filename}")
    
    # Start index feed listener (background thread)
    stop_event = threading.Event()
    
    print(f"\n📡 Connecting to feeds...")
    print(f"   Index Feed: {index_ip}:{index_port}")
    print(f"   F&O Feed:   {fo_ip}:{fo_port}")
    
    index_thread = threading.Thread(target=listen_index_feed, args=(index_ip, index_port, stop_event))
    index_thread.daemon = True
    index_thread.start()
    
    time.sleep(1)  # Let index feed start
    
    # Connect to F&O multicast
    try:
        fo_conn = create_multicast_socket(fo_ip, fo_port)
        print(f"   ✅ F&O feed connected")
    except Exception as e:
        print(f"❌ F&O feed failed: {e}")
        stop_event.set()
        return
    
    # Wait for spot price
    print(f"\n⏳ Waiting for {info.symbol} spot price from index feed...")
    spot_found = False
    for i in range(50):
        time.sleep(0.2)
        spot_price, ok = get_spot_price(info.symbol)
        if ok:
            print(f"   ✅ {info.symbol} spot price: ₹{spot_price:.2f}")
            spot_found = True
            break
    
    if not spot_found:
        print(f"   ⚠️  No spot price yet, will use when available")
    
    print(f"\n🚀 STARTING LIVE GREEKS MONITOR (Press Ctrl+C to stop)")
    print(f"   Watching for token {target_token} ({info.contract})")
    print(f"   Max ticks: {max_ticks}")
    print("=" * 80)
    
    buffer = bytearray(BUFFER_SIZE)
    tick_count = 0
    prev_ltp = 0.0
    last_seq = 0
    packets_received = 0
    
    try:
        while tick_count < max_ticks:
            try:
                n, _ = fo_conn.recvfrom_into(buffer)
                packets_received += 1
                
                if n < HEADER_SIZE + RECORD_SIZE:
                    continue
                
                # Validate message type
                msg_type = struct.unpack('<H', buffer[8:10])[0]
                if msg_type not in [2020, 2021]:
                    continue
                
                # Parse records
                num_records = (n - HEADER_SIZE) // RECORD_SIZE
                if num_records > 8:
                    num_records = 8
                
                for i in range(num_records):
                    offset = HEADER_SIZE + (i * RECORD_SIZE)
                    record = bytes(buffer[offset:offset + RECORD_SIZE])
                    
                    data = decode_fo_record(record)
                    if data is None:
                        continue
                    
                    # Only process target token
                    if data.token != target_token:
                        continue
                    
                    # Skip duplicates
                    if data.seq_num == last_seq:
                        continue
                    last_seq = data.seq_num
                    
                    # Get latest spot price
                    spot_price, ok = get_spot_price(info.symbol)
                    if not ok or spot_price <= 0:
                        continue
                    
                    data.spot_price = spot_price
                    
                    # Calculate time to expiry
                    time_to_expiry = 0.0
                    if info.expiry:
                        try:
                            expiry_date = datetime.strptime(info.expiry, "%Y-%m-%d")
                            days = (expiry_date - datetime.now()).days
                            time_to_expiry = max(days / 365.0, 0.001)  # Min 0.001 years
                        except:
                            time_to_expiry = 0.05  # Default 18 days
                    
                    if time_to_expiry <= 0:
                        continue
                    
                    # Calculate Greeks
                    start_time = time.time()
                    
                    # Step 1: Calculate IV from market price
                    is_call = (info.option_type == "CE")
                    iv, iv_ok = calculate_implied_volatility(
                        spot_price, info.strike, time_to_expiry, 
                        risk_free_rate, data.ltp, is_call
                    )
                    
                    if not iv_ok or iv <= 0:
                        continue
                    
                    # Step 2: Calculate all Greeks with IV
                    greeks = calculate_greeks(
                        spot_price, info.strike, time_to_expiry,
                        risk_free_rate, iv, is_call
                    )
                    
                    calc_time = (time.time() - start_time) * 1000.0  # Convert to ms
                    
                    data.greeks = greeks
                    data.calc_time_ms = calc_time
                    
                    tick_count += 1
                    
                    # Display tick with Greeks
                    display_tick_with_greeks(data, tick_count, prev_ltp)
                    prev_ltp = data.ltp
                    
                    # Write to CSV
                    csv_writer.write_tick(data)
                
                # Show progress every 100 packets if no ticks yet
                if tick_count == 0 and packets_received % 100 == 0:
                    print(f"   ⏳ Searching... {packets_received} packets scanned, waiting for token {target_token}...")
                    
            except socket.timeout:
                continue
            except KeyboardInterrupt:
                print("\n\n⚠️  Interrupted by user")
                break
            except Exception as e:
                continue
        
    finally:
        # Summary
        print("\n\n" + "=" * 80)
        print("📊 SUMMARY")
        print("=" * 80)
        print(f"Token:      {target_token}")
        print(f"Symbol:     {info.contract}")
        print(f"Ticks:      {tick_count}")
        print(f"Packets:    {packets_received}")
        print(f"CSV File:   {csv_writer.filename}")
        print("=" * 80)
        
        csv_writer.close()
        fo_conn.close()
        stop_event.set()

# ============================================================================
# MAIN
# ============================================================================

def main():
    parser = argparse.ArgumentParser(description='BSE Live Greeks Monitor - Python')
    parser.add_argument('--token', type=int, required=True, help='Token ID to monitor (e.g., 1146822)')
    parser.add_argument('--ticks', type=int, default=100, help='Max ticks to capture (default: 100)')
    parser.add_argument('--rate', type=float, default=DEFAULT_RISK_FREE_RATE, help='Risk-free rate (default: 0.065)')
    parser.add_argument('--foip', type=str, default=DEFAULT_FO_MULTICAST_IP, help='F&O multicast IP')
    parser.add_argument('--foport', type=int, default=DEFAULT_FO_PORT, help='F&O UDP port')
    parser.add_argument('--indexip', type=str, default=DEFAULT_INDEX_MULTICAST_IP, help='Index multicast IP')
    parser.add_argument('--indexport', type=int, default=DEFAULT_INDEX_PORT, help='Index UDP port')
    
    args = parser.parse_args()
    
    if not VOLLIB_AVAILABLE:
        print("\n⚠️  ERROR: py_vollib is required for Greeks calculations")
        print("Install with: pip install py_vollib scipy numpy")
        sys.exit(1)
    
    # Load token maps from CSV files
    print("\n📂 Loading contract master...")
    count = load_contract_master()
    print(f"   ✅ Loaded {count} F&O contracts")
    
    monitor_live_greeks(
        args.token, args.foip, args.foport, args.indexip, args.indexport, 
        args.rate, args.ticks
    )

if __name__ == '__main__':
    main()
