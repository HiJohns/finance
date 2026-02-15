// main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/smtp"
	"os/exec"
	"strings"
	"time"

	"github.com/piquette/finance-go"
	"github.com/piquette/finance-go/chart"
	"github.com/piquette/finance-go/datetime"
	"gonum.org/v1/gonum/stat"
)

func init() {
	customClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	customClient.Transport = &http.Transport{
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 5,
	}

	original := http.DefaultTransport
	customClient.Transport = &customTransport{original}

	finance.SetHTTPClient(customClient)
}

type customTransport struct {
	http.RoundTripper
}

func (ct *customTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")
	return ct.RoundTripper.RoundTrip(req)
}

var now = time.Now

// 🔒 安全注入位：编译时使用 -ldflags "-X main.smtpPass=..." 注入
var (
	smtpUser string
	smtpPass string
	receiver string
)

type PlotData struct {
	Assets []string             `json:"assets"`
	Corrs  map[string][]float64 `json:"corrs"`
}

func main() {
	dateFlag := flag.String("date", "", "审计结束日期 (格式: YYYY-MM-DD)")
	flag.Parse()

	var endTime time.Time
	if *dateFlag != "" {
		var err error
		endTime, err = time.Parse("2006-01-02", *dateFlag)
		if err != nil {
			log.Printf("日期解析失败，使用当前时间: %v", err)
			endTime = time.Now()
		}
	} else {
		endTime = time.Now()
	}

	assets := []string{"AMD", "SLV", "USO", "GLD", "IWY"}
	dxy := "DX-Y.NYB"

	dxyReturns, dxyDates, dxySource := getReturnsWithRetry(dxy, endTime)
	if dxyReturns == nil {
		log.Printf("[DXY] 尝试备选: UUP")
		dxyReturns, dxyDates, dxySource = getReturnsWithRetry("UUP", endTime)
		if dxyReturns == nil {
			log.Printf("[DXY] 尝试备选: EURUSD=X")
			eurReturns, eurDates, _ := getReturnsWithRetry("EURUSD=X", endTime)
			if eurReturns != nil {
				dxyReturns = make([]float64, len(eurReturns))
				dxyDates = eurDates
				for i, r := range eurReturns {
					if r != 0 {
						dxyReturns[i] = -r
					}
				}
				dxySource = "EURUSD=X (反转)"
			} else {
				dxySource = ""
			}
		}
	}

	dxyMap := make(map[string]float64)
	if dxyReturns != nil && dxyDates != nil {
		for i, date := range dxyDates {
			dxyMap[date] = dxyReturns[i]
		}
	}

	plotData := PlotData{
		Assets: assets,
		Corrs:  make(map[string][]float64),
	}

	reportDate := endTime.Format("2006-01-02")
	report := fmt.Sprintf("--- Beacon 资产审计报告 [%s] ---\n\n", reportDate)
	report += "【美元引力场审计】\n"

	if dxySource == "" {
		report += "[CRITICAL] 数据源连接被封锁，请检查服务器 IP 或更换代理。\n"
	} else {
		report += fmt.Sprintf("美元指数基准: %s\n\n", dxySource)
	}

	for _, symbol := range assets {
		assetReturns, assetDates, _ := getReturnsWithRetry(symbol, endTime)

		if len(assetReturns) == 0 || len(dxyMap) == 0 {
			log.Printf("警告: %s 数据为空，跳过", symbol)
			plotData.Corrs[symbol] = []float64{0}
			report += fmt.Sprintf("%-5s vs DXY: N/A (数据不足)\n", symbol)
			continue
		}

		var validAsset, validDXY []float64
		for i, date := range assetDates {
			if _, ok := dxyMap[date]; ok {
				ar := assetReturns[i]
				dr := dxyMap[date]
				if !math.IsNaN(ar) && !math.IsNaN(dr) && !math.IsInf(ar, 0) && !math.IsInf(dr, 0) {
					validAsset = append(validAsset, ar)
					validDXY = append(validDXY, dr)
				}
			}
		}

		log.Printf("[%s] 对齐后有效样本: %d", symbol, len(validAsset))

		if len(validAsset) < 20 {
			log.Printf("警告: %s 对齐后样本量不足 (%d < 20)，跳过", symbol, len(validAsset))
			plotData.Corrs[symbol] = []float64{0}
			report += fmt.Sprintf("%-5s vs DXY: N/A (样本不足)\n", symbol)
			continue
		}

		if hasZeroVariance(validAsset) || hasZeroVariance(validDXY) {
			log.Printf("警告: %s 数据方差为0，无法计算相关性", symbol)
			plotData.Corrs[symbol] = []float64{0}
			report += fmt.Sprintf("%-5s vs DXY: N/A (方差为0)\n", symbol)
			continue
		}

		correlation := stat.Correlation(validAsset, validDXY, nil)
		if math.IsNaN(correlation) {
			log.Printf("[%s] 相关性计算结果为 NaN", symbol)
			plotData.Corrs[symbol] = []float64{0}
			report += fmt.Sprintf("%-5s vs DXY: N/A (计算NaN)\n", symbol)
			continue
		}
		log.Printf("[%s] 相关性: %.4f", symbol, correlation)
		plotData.Corrs[symbol] = []float64{correlation}

		status := "🟢 独立"
		if correlation < -0.7 {
			status = "🚨 极强负相关"
		} else if correlation < -0.5 {
			status = "⚠️ 警惕相关"
		}
		report += fmt.Sprintf("%-5s vs DXY: %.4f (%s)\n", symbol, correlation, status)
	}

	generateChart(plotData)
	sendEmail(fmt.Sprintf("Beacon 审计: 宏观资产风险分析 [审计基准日: %s]", reportDate), report)
}

func getReturnsWithRetry(symbol string, endTime time.Time) ([]float64, []string, string) {
	delays := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

	for i, delay := range delays {
		returns, dates, err := getReturnsWithError(symbol, endTime)
		if err != nil {
			if strings.Contains(err.Error(), "remote-error") || strings.Contains(err.Error(), "429") {
				log.Printf("[%s] 第 %d 次重试遇到错误: %v, 等待 %.0fs", symbol, i+1, err, delay.Seconds())
				time.Sleep(delay)
				continue
			}
		}
		if returns != nil {
			return returns, dates, symbol
		}
		if i < len(delays)-1 {
			log.Printf("[%s] 数据为空，第 %d 次重试...", symbol, i+1)
			time.Sleep(delay)
		}
	}

	log.Printf("[%s] 所有重试均失败", symbol)
	return nil, nil, ""
}

func getReturnsWithError(symbol string, endTime time.Time) ([]float64, []string, error) {
	endTimeWithDay := endTime.Add(24 * time.Hour)
	startTime := endTime.AddDate(0, -6, 0)
	startDt := datetime.New(&startTime)
	endDt := datetime.New(&endTimeWithDay)

	log.Printf("[%s] 请求时间窗口: Start=%d, End=%d", symbol, startTime.Unix(), endTimeWithDay.Unix())

	p := &chart.Params{
		Symbol:   symbol,
		Start:    startDt,
		End:      endDt,
		Interval: datetime.OneDay,
	}
	iter := chart.Get(p)
	var prices []float64
	var timestamps []int64
	var firstTime int64
	for iter.Next() {
		bar := iter.Bar()
		if firstTime == 0 {
			firstTime = int64(bar.Timestamp)
			close, _ := bar.Close.Float64()
			log.Printf("[%s] 首条数据: Time=%d, Close=%.4f", symbol, firstTime, close)
		}
		f, _ := bar.Close.Float64()
		prices = append(prices, f)
		timestamps = append(timestamps, int64(bar.Timestamp))
	}
	if err := iter.Err(); err != nil {
		log.Printf("[%s] 迭代器错误: %v", symbol, err)
		return nil, nil, fmt.Errorf("remote-error: %v", err)
	}
	if len(prices) < 2 {
		log.Printf("[%s] 数据不足 (%d 条)，尝试 OneMin...", symbol, len(prices))
		p.Interval = datetime.OneMin
		iter = chart.Get(p)
		prices = nil
		timestamps = nil
		for iter.Next() {
			bar := iter.Bar()
			f, _ := bar.Close.Float64()
			prices = append(prices, f)
			timestamps = append(timestamps, int64(bar.Timestamp))
		}
		if err := iter.Err(); err != nil {
			log.Printf("[%s] OneMin 迭代器错误: %v", symbol, err)
		}
		log.Printf("[%s] OneMin 数据条数: %d", symbol, len(prices))
		if len(prices) < 2 {
			return nil, nil, nil
		}
	}
	dates := make([]string, len(prices)-1)
	returns := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		tm := time.Unix(timestamps[i], 0)
		dates[i-1] = tm.Format("2006-01-02")
		returns[i-1] = (prices[i] - prices[i-1]) / prices[i-1]
	}
	return returns, dates, nil
}

func generateChart(data PlotData) {
	jsonData, _ := json.Marshal(data)
	cmd := exec.Command("python3", "plotter.py")
	stdin, _ := cmd.StdinPipe()
	go func() {
		defer stdin.Close()
		stdin.Write(jsonData)
	}()
	cmd.Run()
}

func hasZeroVariance(data []float64) bool {
	if len(data) < 2 {
		return true
	}
	first := data[0]
	for _, v := range data[1:] {
		if v != first {
			return false
		}
	}
	return true
}

func sendEmail(subject, body string) {
	if smtpUser == "" || smtpPass == "" {
		log.Println("❌ 错误：SMTP 凭证未注入。请检查编译脚本。")
		return
	}
	auth := smtp.PlainAuth("", smtpUser, smtpPass, "smtp.qq.com")
	from := "IronCore <" + smtpUser + ">"
	msg := []byte("From: " + from + "\r\n" +
		"To: " + receiver + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=\"utf-8\"\r\n" +
		"\r\n" +
		body)
	err := smtp.SendMail("smtp.qq.com:587", auth, smtpUser, []string{receiver}, msg)
	if err != nil {
		log.Printf("邮件发送失败: %v", err)
	}

}
