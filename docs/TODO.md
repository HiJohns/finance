## System Beacon Todo List

Based on the README.md, here's a todo list for the finance system:

### Module 1: Python Strategy Lab (The Lab)

- [x] Implement Risk Profiler:
  - [x] Calculate annualized volatility
  - [x] Calculate Maximum Drawdown (MDD)
  - [x] Calculate Beta value relative to S&P 500
- [ ] Implement MWU Dynamic Adjustment Weight Engine:
  - [ ] Implement the Multiplication Weights Update (MWU) algorithm
  - [ ] Automatically reduce weights on assets with systemic drops (Loss increase)
  - [ ] Shift weights to cash or low volatility assets (e.g., UBS/German ETF)

### Module 2: Go Risk Sentinel (The Sentinel)

- [ ] Implement Real-time Stop Loss and Water Level Monitoring:
  - [ ] Poll market data API
  - [ ] Implement hard stop-loss logic (e.g., 15% drawdown)
  - [ ] Implement technical level monitoring (e.g., price below 200-day moving average)
  - [ ] Output instant desktop notifications or emails
- [ ] Implement Whale Anomaly Detection:
  - [ ] Monitor SLV (Silver) and GLD (Gold) for Volume-Price Divergence (VPD)
  - [ ] Identify if major players are retreating based on volume spikes and price stagnation

### Module 3: Macro and Financial Audit (The Auditor)

- [ ] Implement Mag 7 Input-Output Ratio Monitoring (AI-ROI Tracker):
  - [ ] Parse financial statements (10-Q/10-K)
  - [ ] Calculate Capex Intensity (Capital Expenditure / Total Revenue)
  - [ ] Calculate FCF Yield (Free Cash Flow Yield)
  - [ ] Monitor the burning money efficiency of Mag 7
  - [ ] Trigger pre-warning reduction in tech stocks if Capex growth > Revenue growth

### Module 4: Execution Module (The Exec)

- [ ] Implement Integer Programming Execution (Discrete Optimizer):
  - [ ] Use `cvxpy` to solve the Integer Knapsack problem
  - [ ] Calculate the actual number of shares to buy that are closest to the target proportion, under A-share (100 shares per lot) and high-priced U.S. stock restrictions
