import yfinance as yf
import pandas as pd
import sys

# Configuration
TICKERS = ["DX-Y.NYB", "600406.SS", "002028.SZ", "002270.SZ", "688676.SS", "159326.SZ"]
START_DATE = "2021-01-01"
END_DATE = "2026-02-22"

if __name__ == "__main__":
    print("Date,Ticker,Open,High,Low,Close,AdjClose,Volume")
    for ticker in TICKERS:
        try:
            data = yf.download(ticker, start=START_DATE, end=END_DATE, progress=False)
            if data.empty: continue
            
            # Fix columns
            if isinstance(data.columns, pd.MultiIndex):
                data.columns = data.columns.droplevel(1)
            
            data = data.reset_index() # Turns index into 'Date' column
            
            for i in range(len(data)):
                date = data.iloc[i]['Date']
                date_str = date.strftime('%Y-%m-%d') if hasattr(date, 'strftime') else str(date)[:10]
                
                open_p = data.iloc[i]['Open']
                high_p = data.iloc[i]['High']
                low_p = data.iloc[i]['Low']
                close_p = data.iloc[i]['Close']
                adj_p = data.iloc[i]['Adj Close'] if 'Adj Close' in data.columns else close_p
                vol = data.iloc[i]['Volume']
                
                print(f"{date_str},{ticker},{open_p},{high_p},{low_p},{close_p},{adj_p},{vol}")
                
        except Exception as e:
            print(f"Error {ticker}: {e}", file=sys.stderr)
