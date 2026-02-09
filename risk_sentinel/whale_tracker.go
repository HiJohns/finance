// risk_sentinel/whale_tracker.go

package main

import (
	"fmt"
	"log"

	"github.com/piquette/finance-go/chart"
	"github.com/piquette/finance-go/datetime"
	"github.com/shopspring/decimal"
)

// VPDResult stores the Volume-Price Divergence detection result.
type VPDResult struct {
	Ticker        string
	VolumeSpike   bool
	PriceStagnant bool
	Alert         bool
}

// detectVPD detects Volume-Price Divergence for a given ticker.
// It returns true if there is a volume spike but price stagnation, indicating potential whale retreat.
func detectVPD(ticker string) VPDResult {
	// Set chart parameters to get daily data.
	params := &chart.Params{
		Symbol:   ticker,
		Interval: datetime.OneDay,
	}

	// Get chart data.
	iter := chart.Get(params)

	var volumes []int
	var prices []decimal.Decimal

	// Iterate over the chart data and collect volume and closing price.
	for iter.Next() {
		bar := iter.Bar()
		volumes = append(volumes, bar.Volume)
		prices = append(prices, bar.Close)
	}

	if err := iter.Err(); err != nil {
		log.Printf("Error iterating chart data for %s: %v", ticker, err)
		return VPDResult{Ticker: ticker}
	}

	// Need at least 6 days of data to compare.
	if len(volumes) < 6 {
		log.Printf("Not enough data for %s to detect VPD", ticker)
		return VPDResult{Ticker: ticker}
	}

	// Calculate average volume of the last 5 days (excluding the most recent day).
	var avgVolume int
	for i := len(volumes) - 6; i < len(volumes)-1; i++ {
		avgVolume += volumes[i]
	}
	avgVolume /= 5

	// Get the most recent volume and price.
	recentVolume := volumes[len(volumes)-1]
	recentPrice := prices[len(prices)-1]
	previousPrice := prices[len(prices)-2]

	// Check for volume spike (> 150% of average).
	volumeSpike := float64(recentVolume) > float64(avgVolume)*1.5

	// Check for price stagnation (< 2% change).
	priceDiff := recentPrice.Sub(previousPrice)
	priceChange, _ := priceDiff.Div(previousPrice).Float64()
	priceStagnant := abs(priceChange) < 0.02

	// Alert if volume spike and price stagnation.
	alert := volumeSpike && priceStagnant

	if alert {
		fmt.Printf("ALERT: Volume-Price Divergence detected for %s! Volume spike: %v, Price stagnant: %v\n", ticker, volumeSpike, priceStagnant)
	}

	return VPDResult{
		Ticker:        ticker,
		VolumeSpike:   volumeSpike,
		PriceStagnant: priceStagnant,
		Alert:         alert,
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
