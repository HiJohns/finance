package main

import (
	"bufio"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"

	_ "github.com/mattn/go-sqlite3"
	"gonum.org/v1/gonum/stat"
)

// --- Configuration ---
const (
	dbPath            = "./strategy_lab/ironcore_history.db"
	pythonScriptPath  = "./strategy_lab/data_fetcher.py"
	pythonInterpreter = "./venv/bin/python"
	benchmarkTicker   = "DX-Y.NYB"
	volatilityWindow  = 20
	correlationWindow = 30
	holdingPeriod     = 5
	triggerThreshold  = 1.5
	frictionCost      = 0.0015 // 0.15%
	numAssetsToPick   = 3
)

var assetPool = []string{"600406.SS", "002028.SZ", "002270.SZ", "688676.SS", "159326.SZ"}

// --- Data Structures ---
type DailyData struct {
	Date   string
	Ticker string
	Close  float64
}

type Trade struct {
	EntryDate  string
	ExitDate   string
	Ticker     string
	EntryPrice float64
	ExitPrice  float64
	Return     float64
}

// --- Main Application Logic ---
func main() {
	// 1. Prepare data
	log.Println("Preparing database...")
	err := prepareData()
	if err != nil {
		log.Fatalf("Failed to prepare data: %v", err)
	}

	// 2. Load data from the database
	log.Println("Loading historical data...")
	data, err := loadData()
	if err != nil {
		log.Fatalf("Failed to load data: %v", err)
	}
	log.Printf("Loaded %d records.", len(data))

	// 3. Prepare data for backtesting
	dataByTicker := make(map[string][]DailyData)
	for _, record := range data {
		dataByTicker[record.Ticker] = append(dataByTicker[record.Ticker], record)
	}
	for ticker := range dataByTicker {
		sort.Slice(dataByTicker[ticker], func(i, j int) bool {
			return dataByTicker[ticker][i].Date < dataByTicker[ticker][j].Date
		})
	}

	// 4. Run the backtest
	log.Println("Running backtest...")
	trades := runBacktest(dataByTicker)
	log.Printf("Backtest complete. Found %d trade opportunities.", len(trades))

	// 5. Generate and print the report
	if len(trades) > 0 {
		printReport(trades)
	} else {
		log.Println("No trades were executed.")
	}
}

// --- Data Preparation ---
func prepareData() error {
	// Remove old database file
	os.Remove(dbPath)

	// Execute python script to get data
	cmd := exec.Command(pythonInterpreter, pythonScriptPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	// Create new database and table
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	createTableSQL := `CREATE TABLE daily_prices (
		date TEXT,
		Ticker TEXT,
		open REAL,
		high REAL,
		low REAL,
		close REAL,
		adj_close REAL,
		volume REAL
	);`
	if _, err = db.Exec(createTableSQL); err != nil {
		return err
	}

	// Read CSV from python script and insert into DB
	reader := csv.NewReader(bufio.NewReader(stdout))
	// Read header
	header, err := reader.Read()
	if err != nil {
		return err
	}
	log.Printf("CSV Header from Python: %v", header)

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("INSERT INTO daily_prices(date, Ticker, open, high, low, close, adj_close, volume) VALUES(?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		_, err = stmt.Exec(line[0], line[1], toFloat(line[2]), toFloat(line[3]), toFloat(line[4]), toFloat(line[5]), toFloat(line[6]), toFloat(line[7]))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func toFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// --- Database Operations ---
func loadData() ([]DailyData, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query("SELECT date, Ticker, close FROM daily_prices ORDER BY date ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var data []DailyData
	for rows.Next() {
		var record DailyData
		if err := rows.Scan(&record.Date, &record.Ticker, &record.Close); err != nil {
			return nil, err
		}
		data = append(data, record)
	}
	return data, nil
}

// --- Backtesting Engine ---
func runBacktest(dataByTicker map[string][]DailyData) []Trade {
	var trades []Trade
	benchmarkData := dataByTicker[benchmarkTicker]

	// Pre-calculate benchmark returns
	benchmarkReturns := getDailyReturns(benchmarkData)

	// Iterate through each possible trading day for the benchmark
	// The loop should be over the returns, not the prices.
	for i := volatilityWindow; i < len(benchmarkReturns); i++ {
		// --- Trigger Condition ---
		// Calculate 20-day volatility
		pastReturns := benchmarkReturns[i-volatilityWindow : i]
		volatility := stat.StdDev(pastReturns, nil)

		// Check if the current day's return exceeds the threshold
		if benchmarkReturns[i] > triggerThreshold*volatility {
			// The date for return i is the date of price i+1
			triggerDate := benchmarkData[i+1].Date
			log.Printf("Trigger detected on %s. DXY return: %.4f, Volatility: %.4f", triggerDate, benchmarkReturns[i], volatility)

			// --- Filtering Condition ---
			correlations := calculateCorrelations(triggerDate, dataByTicker, benchmarkData)

			// Sort assets by correlation (lowest first)
			sort.Slice(correlations, func(j, k int) bool {
				return correlations[j].value < correlations[k].value
			})

			// --- Execution ---
			for k := 0; k < numAssetsToPick && k < len(correlations); k++ {
				pickedTicker := correlations[k].ticker
				assetData := dataByTicker[pickedTicker]

				// Find entry and exit points for the trade
				entryIdx := findDateIndex(assetData, triggerDate)
				if entryIdx == -1 {
					log.Printf("Could not find data for ticker %s on %s", pickedTicker, triggerDate)
					continue
				}

				exitIdx := entryIdx + holdingPeriod
				if exitIdx < len(assetData) {
					entryPrice := assetData[entryIdx].Close
					exitPrice := assetData[exitIdx].Close

					// Calculate return after friction costs
					tradeReturn := (exitPrice/entryPrice - 1) - frictionCost

					trades = append(trades, Trade{
						EntryDate:  triggerDate,
						ExitDate:   assetData[exitIdx].Date,
						Ticker:     pickedTicker,
						EntryPrice: entryPrice,
						ExitPrice:  exitPrice,
						Return:     tradeReturn,
					})
				}
			}
		}
	}
	return trades
}

type CorrelationData struct {
	ticker string
	value  float64
}

func calculateCorrelations(triggerDate string, dataByTicker map[string][]DailyData, benchmarkData []DailyData) []CorrelationData {
	correlations := []CorrelationData{}
	triggerIdx := findDateIndex(benchmarkData, triggerDate)
	if triggerIdx < correlationWindow {
		return correlations
	}
	corrStartDate := benchmarkData[triggerIdx-correlationWindow].Date
	benchmarkSlice := getPriceSlice(benchmarkData, corrStartDate, triggerDate)
	benchmarkReturns := getDailyReturns(benchmarkSlice)

	for _, ticker := range assetPool {
		assetData := dataByTicker[ticker]
		assetSlice := getPriceSlice(assetData, corrStartDate, triggerDate)
		if len(assetSlice) < correlationWindow {
			continue
		}
		assetReturns := getDailyReturns(assetSlice)
		if len(assetReturns) == len(benchmarkReturns) {
			corr := stat.Correlation(assetReturns, benchmarkReturns, nil)
			correlations = append(correlations, CorrelationData{ticker: ticker, value: corr})
		}
	}
	return correlations
}

// --- Performance Metrics ---
func printReport(trades []Trade) {
	var totalReturn float64
	var winningTrades int
	var returns []float64
	for _, trade := range trades {
		totalReturn += trade.Return
		if trade.Return > 0 {
			winningTrades++
		}
		returns = append(returns, trade.Return)
	}
	meanReturn := totalReturn / float64(len(trades))
	winRate := float64(winningTrades) / float64(len(trades))
	sharpeRatio := calculateSharpeRatio(returns)
	maxDrawdown := calculateMaxDrawdown(returns)

	fmt.Println("\n--- Backtest Report ---")
	fmt.Printf("Total Trades: %d\n", len(trades))
	fmt.Printf("Mean Return per Trade: %.2f%%\n", meanReturn*100)
	fmt.Printf("Win Rate: %.2f%%\n", winRate*100)
	fmt.Printf("Sharpe Ratio: %.2f\n", sharpeRatio)
	fmt.Printf("Maximum Drawdown: %.2f%%\n", maxDrawdown*100)
	fmt.Println("-----------------------")
}

func calculateSharpeRatio(returns []float64) float64 {
	if len(returns) < 2 {
		return 0.0
	}
	mean, stdDev := stat.MeanStdDev(returns, nil)
	if stdDev == 0 {
		return 0.0
	}
	return mean / stdDev * math.Sqrt(252) // Annualized
}

func calculateMaxDrawdown(returns []float64) float64 {
	var peak, maxDrawdown, portfolioValue float64 = 1.0, 0.0, 1.0
	for _, r := range returns {
		portfolioValue *= (1 + r)
		if portfolioValue > peak {
			peak = portfolioValue
		}
		drawdown := (peak - portfolioValue) / peak
		if drawdown > maxDrawdown {
			maxDrawdown = drawdown
		}
	}
	return -maxDrawdown
}

// --- Utility Functions ---
func getDailyReturns(data []DailyData) []float64 {
	var returns []float64
	for i := 1; i < len(data); i++ {
		if data[i-1].Close != 0 {
			returns = append(returns, (data[i].Close/data[i-1].Close)-1)
		} else {
			returns = append(returns, 0)
		}
	}
	return returns
}

func findDateIndex(data []DailyData, date string) int {
	for i, d := range data {
		if d.Date == date {
			return i
		}
	}
	return -1
}

func getPriceSlice(data []DailyData, startDate string, endDate string) []DailyData {
	startIdx, endIdx := findDateIndex(data, startDate), findDateIndex(data, endDate)
	if startIdx != -1 && endIdx != -1 && startIdx <= endIdx {
		return data[startIdx : endIdx+1]
	}
	return nil
}
