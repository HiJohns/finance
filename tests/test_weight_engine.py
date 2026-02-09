import unittest
import pandas as pd
import numpy as np
from strategy_lab.weight_engine import update_weights

class TestWeightEngine(unittest.TestCase):

    def test_update_weights(self):
        current_weights = pd.Series([0.2, 0.2, 0.2, 0.2, 0.2], index=["AMD", "UBS", "USO", "GLD", "SLV"])
        losses = pd.Series([0.01, 0.02, 0.03, 0.04, 0.05], index=["AMD", "UBS", "USO", "GLD", "SLV"])
        epsilon = 0.1

        updated_weights = update_weights(current_weights, losses, epsilon)

        expected_weights = pd.Series([0.200400, 0.200200, 0.199999, 0.199800, 0.199600], index=["AMD", "UBS", "USO", "GLD", "SLV"])
        np.testing.assert_almost_equal(updated_weights.values, [0.200400, 0.200200, 0.199999, 0.199800, 0.199600], decimal=5)

if __name__ == '__main__':
    unittest.main()