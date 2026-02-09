# strategy_lab/risk_profiler.py
"""
Risk Profiler Module

This module calculates risk metrics (Volatility, MDD, Beta) for a list of tickers.
"""

import pandas as pd
import yfinance as yf
import numpy as np
from datetime import datetime, timedelta


def fetch_data(tickers: list[str], start_date: str, end_date: str) -> pd.DataFrame:
    """
    Fetch historical price data for given tickers.
    
    Args:
        tickers: List of ticker symbols
        start_date: Start date for data fetch
        end_date: End date for data fetch
        
    Returns:
        DataFrame with closing prices (MultiIndex handled automatically)
    """
    data = yf.download(tickers, start=start_date, end=end_date, progress=False)
    
    # Handle MultiIndex structure from yfinance
    if isinstance(data.columns, pd.MultiIndex):
        # Multiple tickers - extract Close prices
        data = data['Close']
    else:
        # Single ticker - data is already a Series or single-level DataFrame
        if len(tickers) == 1:
            data = data['Close'].to_frame(tickers[0])
        else:
            data = data['Close']
    
    return data


def calculate_annualized_volatility(returns: pd.Series) -> float:
    """
    Calculate annualized volatility from daily returns.
    
    Args:
        returns: Series of daily returns
        
    Returns:
        Annualized volatility as a float
    """
    daily_volatility = returns.std()
    annualized_volatility = daily_volatility * np.sqrt(252)
    return annualized_volatility


def calculate_maximum_drawdown(prices: pd.Series) -> float:
    """
    Calculate Maximum Drawdown (MDD) from price series.
    
    Args:
        prices: Series of prices
        
    Returns:
        Maximum drawdown as a negative float (e.g., -0.25 for 25% drawdown)
    """
    # Calculate cumulative returns
    cumulative_returns = (1 + prices.pct_change()).cumprod()
    
    # Calculate running maximum
    running_max = cumulative_returns.expanding(min_periods=1).max()
    
    # Calculate drawdown
    drawdown = (cumulative_returns - running_max) / running_max
    
    # Return minimum (most negative) drawdown
    return drawdown.min()


def calculate_beta(asset_returns: pd.Series, benchmark_returns: pd.Series) -> float:
    """
    Calculate Beta value relative to benchmark.
    
    Args:
        asset_returns: Series of asset daily returns
        benchmark_returns: Series of benchmark daily returns
        
    Returns:
        Beta value as a float
    """
    # Align the two series to ensure same dates
    aligned_data = pd.concat([asset_returns, benchmark_returns], axis=1).dropna()
    
    if len(aligned_data) < 2:
        return np.nan
    
    asset_ret = aligned_data.iloc[:, 0]
    benchmark_ret = aligned_data.iloc[:, 1]
    
    covariance = asset_ret.cov(benchmark_ret)
    variance = benchmark_ret.var()
    
    if variance == 0:
        return np.nan
    
    beta = covariance / variance
    return beta


def calculate_risk_metrics(
    tickers: list[str],
    benchmark_ticker: str = "^GSPC"
) -> pd.DataFrame:
    """
    Calculate risk metrics for all tickers.
    
    Args:
        tickers: List of ticker symbols to analyze
        benchmark_ticker: Benchmark ticker for Beta calculation (default: S&P 500)
        
    Returns:
        DataFrame with columns: Ticker, Volatility, MDD, Beta, sorted by Volatility
    """
    # Set date range: 1 year ago to today
    end_date = datetime.now().strftime('%Y-%m-%d')
    start_date = (datetime.now() - timedelta(days=365)).strftime('%Y-%m-%d')
    
    print(f"Fetching data from {start_date} to {end_date}...")
    
    # Fetch data for all tickers and benchmark
    all_tickers = tickers + [benchmark_ticker]
    price_data = fetch_data(all_tickers, start_date, end_date)
    
    if price_data.empty:
        print("Failed to fetch data.")
        return pd.DataFrame()
    
    # Calculate returns for all assets
    returns_data = price_data.pct_change().dropna()
    
    # Get benchmark returns
    if benchmark_ticker in returns_data.columns:
        benchmark_returns = returns_data[benchmark_ticker]
    else:
        print(f"Warning: Benchmark {benchmark_ticker} not found in data.")
        benchmark_returns = pd.Series()
    
    # Calculate metrics for each ticker
    results = []
    
    for ticker in tickers:
        if ticker not in price_data.columns:
            print(f"Warning: Data not found for {ticker}")
            continue
        
        try:
            # Get price and return series for this ticker
            prices = price_data[ticker].dropna()
            ticker_returns = returns_data[ticker].dropna()
            
            # Calculate metrics
            volatility = calculate_annualized_volatility(ticker_returns)
            mdd = calculate_maximum_drawdown(prices)
            
            # Calculate beta if benchmark data is available
            if not benchmark_returns.empty:
                beta = calculate_beta(ticker_returns, benchmark_returns)
            else:
                beta = np.nan
            
            results.append({
                'Ticker': ticker,
                'Volatility': volatility,
                'MDD': mdd,
                'Beta': beta
            })
            
        except Exception as e:
            print(f"Error calculating metrics for {ticker}: {e}")
            continue
    
    # Create DataFrame and sort by Volatility (descending)
    results_df = pd.DataFrame(results)
    
    if not results_df.empty:
        results_df = results_df.sort_values('Volatility', ascending=False).reset_index(drop=True)
        
        # Format numeric columns
        results_df['Volatility'] = results_df['Volatility'].apply(lambda x: f"{x:.2%}")
        results_df['MDD'] = results_df['MDD'].apply(lambda x: f"{x:.2%}")
        results_df['Beta'] = results_df['Beta'].apply(lambda x: f"{x:.2f}" if not pd.isna(x) else "N/A")
    
    return results_df


if __name__ == "__main__":
    # Example usage
    tickers = ["AMD", "UBS", "USO", "GLD", "SLV"]
    
    print(f"Calculating risk profile for: {', '.join(tickers)}\n")
    
    risk_table = calculate_risk_metrics(tickers)
    
    if not risk_table.empty:
        print("\nRisk Profile Summary (sorted by Volatility):")
        print("=" * 60)
        print(risk_table.to_string(index=False))
        print("=" * 60)
    else:
        print("No data available for analysis.")
