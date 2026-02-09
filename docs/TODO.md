## System Beacon Todo List

Based on the README.md, here's a todo list for the finance system:

### Module 1: Python Strategy Lab (The Lab)

- [x] Implement Risk Profiler:
  - [x] Calculate annualized volatility
  - [x] Calculate Maximum Drawdown (MDD)
  - [x] Calculate Beta value relative to S&P 500
- [x] Implement MWU Dynamic Adjustment Weight Engine:
  - [x] Implement the Multiplication Weights Update (MWU) algorithm
  - [x] Automatically reduce weights on assets with systemic drops (Loss increase)
  - [x] Shift weights to cash or low volatility assets (e.g., UBS/German ETF)

### Module 2: Go Risk Sentinel (The Sentinel)

- [x] Implement Real-time Stop Loss and Water Level Monitoring:
  - [x] Poll market data API
  - [x] Implement hard stop-loss logic (e.g., 15% drawdown)
  - [x] Implement technical level monitoring (e.g., price below 200-day moving average)
  - [ ] Output instant desktop notifications or emails
- [x] Implement Whale Anomaly Detection:
  - [x] Monitor SLV (Silver) and GLD (Gold) for Volume-Price Divergence (VPD)
  - [x] Identify if major players are retreating based on volume spikes and price stagnation

### Module 3: Macro and Financial Audit (The Auditor)

- [x] Implement Mag 7 Input-Output Ratio Monitoring (AI-ROI Tracker):
  - [x] Parse financial statements (10-Q/10-K)
  - [x] Calculate Capex Intensity (Capital Expenditure / Total Revenue)
  - [x] Calculate FCF Yield (Free Cash Flow Yield)
  - [x] Monitor the burning money efficiency of Mag 7
  - [x] Trigger pre-warning reduction in tech stocks if Capex growth > Revenue growth

### Module 4: Execution Module (The Exec)

- [x] Implement Integer Programming Execution (Discrete Optimizer):
  - [x] Use `cvxpy` to solve the Integer Knapsack problem
  - [x] Calculate the actual number of shares to buy that are closest to the target proportion, under A-share (100 shares per lot) and high-priced U.S. stock restrictions
