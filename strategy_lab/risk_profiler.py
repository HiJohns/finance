# strategy_lab/risk_profiler.py

import pandas as pd
import yfinance as yf
import numpy as np

def calculate_annualized_volatility(data: pd.DataFrame) -> float:
    """Calculates the annualized volatility of the given data."""
    daily_returns = data.pct_change().dropna()
    volatility = daily_returns.std().mean() * np.sqrt(252)
    return volatility


def calculate_maximum_drawdown(data: pd.DataFrame) -> float:
    """Calculates the maximum drawdown of the given data."""
    cumulative_returns = (1 + data.pct_change()).cumprod()
    peak = cumulative_returns.expanding(min_periods=1).max()
    drawdown = (cumulative_returns - peak) / peak
    mdd = drawdown.min().min()
    return mdd


def calculate_beta(asset_returns: pd.Series, benchmark_returns: pd.Series) -> float:
    """Calculates the beta value relative to the benchmark."""
    covariance = asset_returns.cov(benchmark_returns)
    variance = benchmark_returns.var()
    beta = covariance / variance
    return beta


def fetch_data(tickers: list[str], start_date: str, end_date: str) -> pd.DataFrame:
    data = yf.download(tickers, start=start_date, end=end_date)
    if len(tickers) > 1:
        data = data['Close']
    else:
        data = data['Close']
    return data


if __name__ == "__main__":
    # Example usage
    tickers = ["AMD", "UBS", "USO", "GLD", "SLV"]
    benchmark_ticker = "^GSPC"  # S&P 500
    start_date = "2025-01-01"
    end_date = "2025-12-31"

    print(f"Calculating risk profile for {tickers}")

    data = fetch_data(tickers, start_date, end_date)
    benchmark_data = fetch_data([benchmark_ticker], start_date, end_date)

    if isinstance(data, pd.DataFrame) and isinstance(benchmark_data, pd.DataFrame):
        # Calculate returns for beta calculation
        asset_returns = data[tickers[0]].pct_change().dropna() # Using the first ticker for example
        benchmark_returns = benchmark_data[benchmark_ticker].pct_change().dropna()

        volatility = calculate_annualized_volatility(data)
        mdd = calculate_maximum_drawdown(data)
        beta = calculate_beta(asset_returns, benchmark_returns)

        print(f"Volatility: {volatility:.4f}")
        print(f"Maximum Drawdown: {mdd:.4f}")
        print(f"Beta: {beta:.4f}")
    else:
        print("Failed to fetch data.")