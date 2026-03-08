import pandas as pd
import yfinance as yf
from datetime import datetime, timedelta

def get_percentile(ticker_list):
    results = []
    end_date = datetime.now()
    start_date = end_date - timedelta(days=180) # 最近半年
    
    for ticker in ticker_list:
        # 下载数据
        df = yf.download(ticker, start=start_date.strftime('%Y-%m-%d'))
        if df.empty: continue
        
        current_price = df['Adj Close'].iloc[-1]
        min_6m = df['Adj Close'].min()
        max_6m = df['Adj Close'].max()
        
        # 计算百分位
        percentile = (current_price - min_6m) / (max_6m - min_6m) * 100
        
        results.append({
            "代码": ticker,
            "现价": round(current_price, 2),
            "半年最低": round(min_6m, 2),
            "半年最高": round(max_6m, 2),
            "百分位": f"{round(percentile, 2)}%"
        })
    return pd.DataFrame(results)

# 你的目标池
my_pool = ["688676.SS", "002028.SZ", "159326.SZ"]
print(get_percentile(my_pool))
