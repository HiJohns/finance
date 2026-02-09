# strategy_lab/weight_engine.py

import pandas as pd
import numpy as np


def update_weights(current_weights: pd.Series, losses: pd.Series, epsilon: float) -> pd.Series:
    """Updates the weights using the multiplicative weights update (MWU) algorithm."""
    # Calculate the updated weights
    updated_weights = current_weights * np.exp(-epsilon * losses)

    # Normalize the weights to sum to 1
    updated_weights = updated_weights / updated_weights.sum()

    return updated_weights


if __name__ == "__main__":
    # Example usage
    current_weights = pd.Series([0.2, 0.2, 0.2, 0.2, 0.2], index=["AMD", "UBS", "USO", "GLD", "SLV"])
    losses = pd.Series([0.01, 0.02, 0.03, 0.04, 0.05], index=["AMD", "UBS", "USO", "GLD", "SLV"])
    epsilon = 0.1

    updated_weights = update_weights(current_weights, losses, epsilon)

    print("Current Weights:\n", current_weights)
    print("Losses:\n", losses)
    print("Updated Weights:\n", updated_weights)
