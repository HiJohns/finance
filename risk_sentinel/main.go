// risk_sentinel/main.go

package main

import (
	"fmt"
	"log"
	"time"

	"github.com/piquette/finance-go/quote"
)

// Config represents the configuration for the price monitor.
type Config struct {
	Tickers             []string `json:"tickers"`
	HardStopLossPercent float64  `json:"hard_stop_loss_percent"`
	Ma200Threshold      float64  `json:"ma200_threshold"`
	PollIntervalSeconds int      `json:"poll_interval_seconds"`
}

func pollMarketData(ticker string) (float64, error) {
	q, err := quote.Get(ticker)
	if err != nil {
		return 0, fmt.Errorf("error fetching quote for %s: %w", ticker, err)
	}
	if q == nil {
		return 0, fmt.Errorf("empty quote for %s", ticker)
	}
	return q.RegularMarketPrice, nil
}

func calculateMA200(ticker string) float64 {
	// TODO: implement MA200 calculation
	return 100.0 // dummy value
}

func main() {
	fmt.Println("Starting Risk Sentinel...")

	// Configuration
	tickers := []string{"AMD", "UBS", "USO", "GLD", "SLV"}
	stopLossPercent := 0.15
	ma200Threshold := 0.95

	// Initialize prices
	initialPrices := make(map[string]float64)
	for _, ticker := range tickers {
		price, err := pollMarketData(ticker)
		if err != nil {
			log.Printf("Error fetching initial price for %s: %v", ticker, err)
			continue
		}
		initialPrices[ticker] = price
		fmt.Printf("Initial price for %s: %.2f\n", ticker, price)
	}

	// Main monitoring loop
	for {
		fmt.Println("\n--- Monitoring Cycle ---")

		// Check price monitoring for all tickers
		for _, ticker := range tickers {
			currentPrice, err := pollMarketData(ticker)
			if err != nil {
				log.Printf("Error fetching price for %s: %v", ticker, err)
				continue
			}

			initialPrice, ok := initialPrices[ticker]
			if !ok {
				continue
			}

			// Check hard stop-loss
			drawdown := (initialPrice - currentPrice) / initialPrice
			if drawdown > stopLossPercent {
				fmt.Printf("ALERT: Hard stop-loss triggered for %s! Drawdown: %.2f%%\n", ticker, drawdown*100)
			}

			// Check MA200 threshold
			ma200 := calculateMA200(ticker)
			if currentPrice < ma200*ma200Threshold {
				fmt.Printf("ALERT: Price below MA200 threshold for %s! Current: %.2f, MA200: %.2f\n", ticker, currentPrice, ma200)
			}
		}

		// Check whale anomaly detection for SLV and GLD
		fmt.Println("\n--- Whale Anomaly Detection ---")
		whaleTickers := []string{"SLV", "GLD"}
		for _, ticker := range whaleTickers {
			result := detectVPD(ticker)
			if result.Alert {
				fmt.Printf("WHALE ALERT for %s: Volume spike=%v, Price stagnant=%v\n",
					ticker, result.VolumeSpike, result.PriceStagnant)
			}
		}

		time.Sleep(5 * time.Second)
	}
}
