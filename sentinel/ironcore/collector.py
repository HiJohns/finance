#!/usr/bin/env python3
import sqlite3
import time
import logging
import os
import sys
import yaml
from datetime import datetime

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s'
)
logger = logging.getLogger(__name__)

DB_PATH = os.path.join(os.path.dirname(__file__), 'ironcore.db')
CONFIG_PATH = os.path.join(os.path.dirname(__file__), 'config.yaml')

class ConfigManager:
    """Dynamic configuration manager for IronCore collector"""
    
    def __init__(self, config_path=CONFIG_PATH):
        self.config_path = config_path
        self.config = None
        self.last_modified = 0
        self.load_config()
    
    def load_config(self):
        """Load configuration from YAML file"""
        try:
            with open(self.config_path, 'r', encoding='utf-8') as f:
                self.config = yaml.safe_load(f)
            self.last_modified = os.path.getmtime(self.config_path)
            logger.info(f"Configuration loaded from {self.config_path}")
            return True
        except Exception as e:
            logger.error(f"Failed to load config: {e}")
            return False
    
    def reload_if_changed(self):
        """Reload config if file has been modified"""
        try:
            current_mtime = os.path.getmtime(self.config_path)
            if current_mtime > self.last_modified:
                logger.info("Config file changed, reloading...")
                return self.load_config()
        except Exception as e:
            logger.error(f"Failed to check config modification: {e}")
        return False
    
    def get_china_stocks(self):
        """Get China A-stock symbols from config"""
        if not self.config:
            return []
        assets = self.config.get('assets', {})
        china_assets = assets.get('china_power_grid', [])
        return [asset['symbol'] for asset in china_assets if asset.get('symbol')]
    
    def get_us_assets(self):
        """Get US asset symbols from config"""
        if not self.config:
            return []
        assets = self.config.get('assets', {})
        global_assets = assets.get('global_macro', [])
        return [asset['symbol'] for asset in global_assets if asset.get('symbol')]
    
    def get_collection_interval(self):
        """Get collection interval from config or environment"""
        if self.config and 'runtime' in self.config:
            minutes = self.config['runtime'].get('audit_interval_minutes', 10)
            return minutes * 60
        return int(os.environ.get('COLLECT_INTERVAL', 600))
    
    def get_timezone(self):
        """Get timezone from config"""
        if self.config and 'runtime' in self.config:
            return self.config['runtime'].get('timezone', 'Asia/Shanghai')
        return 'Asia/Shanghai'

def init_db():
    conn = sqlite3.connect(DB_PATH)
    cursor = conn.cursor()
    
    # Check if table exists and get columns
    cursor.execute("PRAGMA table_info(market_data)")
    existing_columns = {row[1] for row in cursor.fetchall()}
    
    if 'market_data' not in [t[0] for t in cursor.execute("SELECT name FROM sqlite_master WHERE type='table'")]:  # Table doesn't exist
        cursor.execute('''
            CREATE TABLE IF NOT EXISTS market_data (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                timestamp TEXT NOT NULL,
                symbol TEXT NOT NULL,
                price REAL,
                volume REAL,
                turnover_rate REAL,
                created_at TEXT DEFAULT CURRENT_TIMESTAMP
            )
        ''')
    else:
        # Table exists, add turnover_rate if missing
        if 'turnover_rate' not in existing_columns:
            try:
                cursor.execute('ALTER TABLE market_data ADD COLUMN turnover_rate REAL')
                logger.info("Added turnover_rate column to market_data table")
            except sqlite3.OperationalError as e:
                logger.warning(f"Column turnover_rate may already exist: {e}")
    
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
        # Remove .SS or .SZ suffix for efinance
        clean_symbol = symbol
        if symbol.endswith('.SS'):
            clean_symbol = symbol[:-3]
        elif symbol.endswith('.SZ'):
            clean_symbol = symbol[:-3]
        
        df = efinance.stock.get_quote(clean_symbol)
        if df is not None and not df.empty:
            latest = df.iloc[-1]
            # Try to get turnover rate, fallback to 0 if not available
            turnover = 0.0
            try:
                # Common field names for turnover rate in Chinese stock data
                turnover = float(latest.get('换手率', 0))
                if turnover == 0:
                    turnover = float(latest.get('turnover', 0))
            except (ValueError, TypeError) as e:
                logger.warning(f"Failed to parse turnover rate for {symbol}: {e}, using default 0.0")
                turnover = 0.0
            
            return {
                'symbol': symbol,  # Keep original symbol with suffix
                'price': float(latest.get('最新价', 0)),
                'volume': float(latest.get('成交量', 0)),
                'turnover_rate': turnover
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
                'INSERT INTO market_data (timestamp, symbol, price, volume, turnover_rate) VALUES (?, ?, ?, ?, ?)',
                (timestamp, data['symbol'], data.get('price'), data.get('volume'), data.get('turnover_rate', 0))
            )
    conn.commit()
    logger.info(f"Saved {len(data_list)} records to DB")

def is_market_closed(config_manager):
    try:
        import pytz
        
        tz = pytz.timezone(config_manager.get_timezone())
        now = datetime.now(tz)
        
        weekday = now.weekday()
        if weekday >= 5:
            return True
        
        return False
    except Exception as e:
        logger.error(f"Failed to check market status: {e}")
        return False  # Default to open if we can't determine

def run_collection(config_manager):
    if is_market_closed(config_manager):
        logger.info("Market is closed on weekend, skipping collection")
        return []
    
    logger.info("Starting data collection...")
    
    conn = init_db()
    
    efinance = try_import_efinance()
    yfinance = try_import_yfinance()
    
    all_data = []
    
    # Get dynamic asset lists from config
    china_stocks = config_manager.get_china_stocks()
    us_assets = config_manager.get_us_assets()
    
    logger.info(f"Configured China stocks: {china_stocks}")
    logger.info(f"Configured US assets: {us_assets}")
    
    if efinance:
        for symbol in china_stocks:
            data = fetch_china_a_stock(efinance, symbol)
            if data:
                logger.info(f"Fetched {data['symbol']}: price={data['price']}, volume={data['volume']}")
                all_data.append(data)
    
    if yfinance:
        for symbol in us_assets:
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
    config_manager = ConfigManager()
    interval = config_manager.get_collection_interval()
    logger.info(f"Collector starting with {interval}s interval")
    
    iteration = 0
    
    while True:
        try:
            # Check for config changes every 10 iterations
            iteration += 1
            if iteration % 10 == 0:
                if config_manager.reload_if_changed():
                    interval = config_manager.get_collection_interval()
                    logger.info(f"Interval updated to {interval}s")
            
            run_collection(config_manager)
        except Exception as e:
            logger.error(f"Collection error: {e}")
        
        time.sleep(interval)

if __name__ == '__main__':
    main()
