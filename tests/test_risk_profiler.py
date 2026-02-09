import unittest
import pandas as pd
import numpy as np
from strategy_lab.risk_profiler import calculate_annualized_volatility, calculate_maximum_drawdown, calculate_beta

class TestRiskProfiler(unittest.TestCase):

    def test_calculate_annualized_volatility(self):
        data = pd.DataFrame([1, 2, 3, 4, 5])
        volatility = calculate_annualized_volatility(data)
        self.assertAlmostEqual(volatility, 5.3327, places=4)

    def test_calculate_maximum_drawdown(self):
        data = pd.DataFrame([1, 2, 1.5, 3, 2])
        mdd = calculate_maximum_drawdown(data)
        self.assertAlmostEqual(mdd, -0.3333, places=4)

    def test_calculate_beta(self):
        asset_returns = pd.Series([0.01, 0.02, 0.03, 0.04, 0.05])
        benchmark_returns = pd.Series([0.005, 0.01, 0.015, 0.02, 0.025])
        beta = calculate_beta(asset_returns, benchmark_returns)
        self.assertAlmostEqual(beta, 2.0, places=4)

if __name__ == '__main__':
    unittest.main()