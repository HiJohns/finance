# execution_layer/discrete_optimizer.py
"""
Discrete Portfolio Optimizer

This module solves the integer portfolio allocation problem using cvxpy.
It minimizes the squared error between actual and target weights while
ensuring the number of shares are positive integers.
"""

import cvxpy as cp
import numpy as np
import pandas as pd
from typing import Dict, Tuple


def optimize_portfolio(
    total_amount: float,
    target_weights: Dict[str, float],
    prices: Dict[str, float]
) -> Tuple[Dict[str, int], float, float]:
    """
    Optimize portfolio allocation using integer programming.
    
    Args:
        total_amount: Total amount available for investment (in USD)
        target_weights: Dictionary mapping asset tickers to target weights (e.g., {'USO': 0.4, 'GLD': 0.6})
        prices: Dictionary mapping asset tickers to current unit prices
        
    Returns:
        Tuple of (shares_dict, invested_amount, remaining_cash)
        - shares_dict: Dictionary mapping tickers to number of shares to buy
        - invested_amount: Total amount invested
        - remaining_cash: Remaining cash after purchase
    """
    
    # Validate inputs
    if abs(sum(target_weights.values()) - 1.0) > 1e-6:
        raise ValueError(f"Target weights must sum to 1.0, got {sum(target_weights.values())}")
    
    if not all(ticker in prices for ticker in target_weights):
        missing = set(target_weights.keys()) - set(prices.keys())
        raise ValueError(f"Missing prices for tickers: {missing}")
    
    # Prepare data
    tickers = list(target_weights.keys())
    n = len(tickers)
    
    # Convert to numpy arrays (maintain order)
    w_target = np.array([target_weights[t] for t in tickers])
    p = np.array([prices[t] for t in tickers])
    
    # Define decision variable: integer number of shares for each asset
    shares = cp.Variable(n, integer=True)
    
    # Calculate actual weights based on shares and prices
    # weight_i = (shares_i * price_i) / total_amount
    # We use total_amount as the denominator for the target comparison
    
    # Objective: Minimize sum of squared errors between actual and target weights
    # actual_weights = (shares * prices) / total_amount
    actual_values = cp.multiply(shares, p)
    actual_weights = actual_values / total_amount
    
    # Squared error objective
    objective = cp.Minimize(cp.sum_squares(actual_weights - w_target))
    
    # Constraints:
    # 1. Total cost cannot exceed available amount
    # 2. Shares must be non-negative integers (already defined as integer variable)
    # 3. Shares must be >= 0
    
    constraints = [
        cp.sum(actual_values) <= total_amount,  # Cannot exceed budget
        shares >= 0  # Non-negative shares
    ]
    
    # Solve the problem
    problem = cp.Problem(objective, constraints)
    
    # Use SCIP solver for mixed-integer quadratic programming
    try:
        result = problem.solve(solver=cp.SCIP)
        shares_value = np.array(shares.value).astype(int)
        shares_value = np.maximum(shares_value, 0)  # Ensure non-negative
    except Exception as e:
        print(f"Warning: SCIP solver failed ({e}). Using fallback method.")
        # Fallback: Relax integer constraint, solve, then round
        shares_relax = cp.Variable(n)
        actual_values_relax = cp.multiply(shares_relax, p)
        actual_weights_relax = actual_values_relax / total_amount
        objective_relax = cp.Minimize(cp.sum_squares(actual_weights_relax - w_target))
        constraints_relax = [
            cp.sum(actual_values_relax) <= total_amount,
            shares_relax >= 0
        ]
        problem_relax = cp.Problem(objective_relax, constraints_relax)
        problem_relax.solve()
        shares_value = np.round(shares_relax.value).astype(int)
        shares_value = np.maximum(shares_value, 0)  # Ensure non-negative
    
    # Calculate results
    shares_dict = {ticker: int(shares_value[i]) for i, ticker in enumerate(tickers)}
    
    # Calculate invested amount and remaining cash
    invested_amount = sum(shares_dict[t] * prices[t] for t in tickers)
    remaining_cash = total_amount - invested_amount
    
    # Calculate actual weights
    actual_weights_dict = {
        t: (shares_dict[t] * prices[t]) / total_amount 
        for t in tickers
    }
    
    return shares_dict, invested_amount, remaining_cash


def print_optimization_results(
    total_amount: float,
    target_weights: Dict[str, float],
    prices: Dict[str, float],
    shares_dict: Dict[str, int],
    invested_amount: float,
    remaining_cash: float
):
    """
    Print formatted optimization results.
    
    Args:
        total_amount: Total amount available
        target_weights: Target weight dictionary
        prices: Price dictionary
        shares_dict: Resulting shares dictionary
        invested_amount: Amount invested
        remaining_cash: Remaining cash
    """
    print("\n" + "=" * 80)
    print("PORTFOLIO OPTIMIZATION RESULTS")
    print("=" * 80)
    
    print(f"\nTotal Available Amount: ${total_amount:,.2f}")
    print(f"Total Invested: ${invested_amount:,.2f}")
    print(f"Remaining Cash: ${remaining_cash:,.2f}")
    
    print("\n" + "-" * 80)
    print(f"{'Ticker':<10} {'Price':>12} {'Target %':>10} {'Shares':>10} {'Cost':>14} {'Actual %':>10}")
    print("-" * 80)
    
    for ticker in sorted(target_weights.keys()):
        price = prices[ticker]
        target_pct = target_weights[ticker] * 100
        shares = shares_dict[ticker]
        cost = shares * price
        actual_pct = (cost / total_amount) * 100 if total_amount > 0 else 0
        
        print(f"{ticker:<10} ${price:>10.2f} {target_pct:>9.2f}% {shares:>10,} ${cost:>12,.2f} {actual_pct:>9.2f}%")
    
    print("-" * 80)
    
    # Calculate weight errors
    print("\nWeight Errors:")
    total_error = 0
    for ticker in sorted(target_weights.keys()):
        target_w = target_weights[ticker]
        actual_w = (shares_dict[ticker] * prices[ticker]) / total_amount
        error = actual_w - target_w
        total_error += error ** 2
        print(f"  {ticker}: Target={target_w:.4f}, Actual={actual_w:.4f}, Error={error:.4f}")
    
    print(f"\nSum of Squared Errors: {total_error:.8f}")
    print("=" * 80)


if __name__ == "__main__":
    # Example usage
    
    # Input parameters
    total_amount = 10000.0  # $10,000 USD
    
    target_weights = {
        'USO': 0.4,  # 40% in USO
        'GLD': 0.6   # 60% in GLD
    }
    
    # Current market prices (example values)
    prices = {
        'USO': 85.50,  # WTI Crude Oil
        'GLD': 185.25  # Gold ETF
    }
    
    print("Discrete Portfolio Optimizer")
    print(f"\nInput Parameters:")
    print(f"  Total Amount: ${total_amount:,.2f}")
    print(f"  Target Weights: {target_weights}")
    print(f"  Current Prices: {prices}")
    
    # Run optimization
    shares_dict, invested_amount, remaining_cash = optimize_portfolio(
        total_amount=total_amount,
        target_weights=target_weights,
        prices=prices
    )
    
    # Print results
    print_optimization_results(
        total_amount=total_amount,
        target_weights=target_weights,
        prices=prices,
        shares_dict=shares_dict,
        invested_amount=invested_amount,
        remaining_cash=remaining_cash
    )
