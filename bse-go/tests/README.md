# BSE Go Test Utilities

This directory contains live market data monitoring utilities for BSE (Bombay Stock Exchange) market data.

## Available Test Tools

### 1. Live Token Monitor (`live_token_monitor/`)
Generic token monitor that can watch any specific token.

**Usage:**
```bash
# CM (Equity) - Token 500325 for Reliance
.\live_token_monitor.exe --token 500325 --port 26001 --ticks 50 --data ..\..\data

# FO (Derivatives) - Token 873830 for SENSEX Future
.\live_token_monitor.exe --token 873830 --port 26002 --ticks 50 --data ..\..\data
```

**Flags:**
- `--token` (required): Token to monitor (e.g., 500325 for Reliance)
- `--port` (default: 26001): Multicast port (26001=CM, 26002=FO)
- `--ticks` (default: 100): Maximum ticks to capture
- `--ip` (default: 239.1.2.5): Multicast IP
- `--data` (default: data): Data directory containing token files

### 2. Live Reliance Monitor (`live_reliance_monitor/`)
Dedicated monitor for Reliance Industries (BSE Token: 500325).

**Usage:**
```bash
.\live_reliance_monitor.exe --ticks 50 --data ..\..\data

# Or monitor any other CM token:
.\live_reliance_monitor.exe --token 532540 --ticks 50 --data ..\..\data  # TCS
```

**Flags:**
- `--token` (default: 500325): Token to monitor
- `--port` (default: 26001): CM port
- `--ticks` (default: 100): Maximum ticks to capture
- `--data` (default: data): Data directory containing token files

### 3. Live SENSEX Monitor (`live_sensex_monitor/`)
Multi-token monitor for all SENSEX-related contracts (futures and options).

**Usage:**
```bash
.\live_sensex_monitor.exe --ticks 100 --data ..\..\data

# Show all SENSEX tokens (not just active ones)
.\live_sensex_monitor.exe --ticks 100 --all --data ..\..\data
```

**Flags:**
- `--ticks` (default: 100): Maximum ticks to capture
- `--port` (default: 26002): FO port
- `--ip` (default: 239.1.2.5): Multicast IP
- `--data` (default: data): Data directory containing token files
- `--all`: Show all SENSEX tokens (including inactive)

## Output

All tools save data to CSV files in `data/processed_csv/`:
- Per-token files: `YYYYMMDD_TOKEN_SYMBOL_live.csv`
- Per-run files: `sensex_live_YYYYMMDD_HHMMSS.csv`

## Building

From each test directory:
```bash
go build -o <name>.exe .
```

Or build all from the bse-go root:
```bash
cd tests/live_token_monitor && go build -o live_token_monitor.exe .
cd ../live_reliance_monitor && go build -o live_reliance_monitor.exe .
cd ../live_sensex_monitor && go build -o live_sensex_monitor.exe .
```

## Token Files

The monitors require token master files in the data directory:
- **CM (Equity)**: `data/tokens/BhavCopy_BSE_CM_*.csv`
- **FO (Derivatives)**: `data/tokens/BSE_EQD_CONTRACT_*.csv`

If running from the tests directory, use `--data ..\..\data` to point to the correct location.

## Common Tokens

### CM (Cash Market) - Port 26001
| Token | Symbol | Company |
|-------|--------|---------|
| 500325 | RELIANCE | Reliance Industries Ltd. |
| 532540 | TCS | Tata Consultancy Services |
| 500180 | HDFCBANK | HDFC Bank Ltd. |
| 500209 | INFY | Infosys Ltd. |
| 500112 | SBIN | State Bank of India |

### FO (Derivatives) - Port 26002
Token IDs change with each expiry. Use the live_sensex_monitor to discover current tokens.

## Example Output

**Reliance Monitor:**
```
════════════════════════════════════════════════════════════════════════════════
  TICK #1       │  12:09:50.365  │  Token: 500325
  RELIANCE (RELIANCE INDUSTRIES LTD.)
════════════════════════════════════════════════════════════════════════════════

  💰 LTP: ₹1571.05  ━ +7.50 (+0.48% from prev close)
  ──────────────────────────────────────────────────────────────────────────
  Open: ₹1568.00  │  High: ₹1580.90  │  Low: ₹1562.35  │  Prev: ₹1563.55

  📚 ORDER BOOK
  ────────────────────────────────────────────────────────────────────────
                                 BID │                                ASK
         Price       Qty    Ord │        Price       Qty    Ord
  ────────────────────────────────────────────────────────────────────────
  ₹   1571.05        78      1 │ ₹   1571.60        57      4
  ₹   1571.00       597     14 │ ₹   1571.70        31      1
```

**SENSEX Monitor:**
```
[12:10:21.443] #5     ▲ SENSEX FUT 24-DEC-2025                   LTP: ₹  86385.00 ▲ +74.70 (+0.09%) Vol: 8600
[12:10:21.445] #6     ▲ SENSEX FUT 29-JAN-2026                   LTP: ₹  87008.95 ▲ +111.75 (+0.13%) Vol: 340
```
