#!/usr/bin/env python3
import sqlite3
import time
import logging
import os
import sys

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s'
)
logger = logging.getLogger(__name__)

DB_PATH = os.path.join(os.path.dirname(__file__), 'ironcore.db')

CHINA_A_STOCKS = [
    '600406',  # 国电南瑞
    '002028',  # 思源电气
    '002270',  # 华明装备
    '688676',  # 金盘科技
    '159326',  # 电网设备ETF
]

US_ASSETS = [
    'SRVR',    # 全球数据中心REIT
    'DX-Y.NYB', # DXY
    '^VIX',    # VIX
]

def init_db():
    conn = sqlite3.connect(DB_PATH)
    cursor = conn.cursor()
    cursor.execute('''
        CREATE TABLE IF NOT EXISTS market_data (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp TEXT NOT NULL,
            symbol TEXT NOT NULL,
            price REAL,
            volume REAL,
            created_at TEXT DEFAULT CURRENT_TIMESTAMP
        )
    ''')
    cursor.execute('''
        CREATE INDEX IF NOT EXISTS idx_symbol_timestamp 
        ON market_data(symbol, timestamp)
    ''')
    conn.commit()
    return conn

def try_import_efinance():
    try:
        import efinance
        return efinance
    except ImportError:
        logger.warning("efinance not installed, installing...")
        os.system(f"{sys.executable} -m pip install efinance -q")
        try:
            import efinance
            return efinance
        except ImportError:
            logger.error("Failed to install efinance")
            return None

def try_import_yfinance():
    try:
        import yfinance
        return yfinance
    except ImportError:
        logger.warning("yfinance not installed, installing...")
        os.system(f"{sys.executable} -m pip install yfinance -q")
        try:
            import yfinance
            return yfinance
        except ImportError:
            logger.error("Failed to install yfinance")
            return None

def fetch_china_a_stock(efinance, symbol):
    try:
        df = efinance.stock.get_quote(symbol)
        if df is not None and not df.empty:
            latest = df.iloc[-1]
            return {
                'symbol': f"{symbol}.SH" if symbol.startswith('6') else f"{symbol}.SZ",
                'price': float(latest.get('最新价', 0)),
                'volume': float(latest.get('成交量', 0))
            }
    except Exception as e:
        logger.error(f"Failed to fetch {symbol}: {e}")
    return None

def fetch_us_asset(yfinance, symbol):
    try:
        ticker = yfinance.Ticker(symbol)
        hist = ticker.history(period="1d")
        if not hist.empty:
            latest = hist.iloc[-1]
            return {
                'symbol': symbol,
                'price': float(latest['Close']),
                'volume': float(latest['Volume'])
            }
    except Exception as e:
        logger.error(f"Failed to fetch {symbol}: {e}")
    return None

def save_to_db(conn, data_list):
    cursor = conn.cursor()
    timestamp = time.strftime('%Y-%m-%d %H:%M:%S')
    for data in data_list:
        if data:
            cursor.execute(
                'INSERT INTO market_data (timestamp, symbol, price, volume) VALUES (?, ?, ?, ?)',
                (timestamp, data['symbol'], data.get('price'), data.get('volume'))
            )
    conn.commit()
    logger.info(f"Saved {len(data_list)} records to DB")

def run_collection():
    logger.info("Starting data collection...")
    
    conn = init_db()
    
    efinance = try_import_efinance()
    yfinance = try_import_yfinance()
    
    all_data = []
    
    if efinance:
        for symbol in CHINA_A_STOCKS:
            data = fetch_china_a_stock(efinance, symbol)
            if data:
                logger.info(f"Fetched {data['symbol']}: price={data['price']}, volume={data['volume']}")
                all_data.append(data)
    
    if yfinance:
        for symbol in US_ASSETS:
            data = fetch_us_asset(yfinance, symbol)
            if data:
                logger.info(f"Fetched {data['symbol']}: price={data['price']}, volume={data['volume']}")
                all_data.append(data)
    
    if all_data:
        save_to_db(conn, all_data)
    else:
        logger.warning("No data collected!")
    
    conn.close()
    return all_data

def main():
    interval = int(os.environ.get('COLLECT_INTERVAL', 600))
    logger.info(f"Collector starting with {interval}s interval")
    
    while True:
        try:
            run_collection()
        except Exception as e:
            logger.error(f"Collection error: {e}")
        
        time.sleep(interval)

if __name__ == '__main__':
    main()