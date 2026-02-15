// main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
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

	dxyReturns, dxySource := getReturnsWithRetry(dxy, endTime)
	if dxyReturns == nil {
		log.Printf("[DXY] 尝试备选: UUP")
		dxyReturns, dxySource = getReturnsWithRetry("UUP", endTime)
		if dxyReturns == nil {
			log.Printf("[DXY] 尝试备选: EURUSD=X")
			eurReturns, _ := getReturnsWithRetry("EURUSD=X", endTime)
			if eurReturns != nil {
				dxyReturns = make([]float64, len(eurReturns))
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
		assetReturns, _ := getReturnsWithRetry(symbol, endTime)

		if len(assetReturns) == 0 || len(dxyReturns) == 0 {
			log.Printf("警告: %s 数据为空，跳过", symbol)
			plotData.Corrs[symbol] = []float64{0}
			report += fmt.Sprintf("%-5s vs DXY: N/A (数据不足)\n", symbol)
			continue
		}

		n := len(dxyReturns)
		if len(assetReturns) < n {
			n = len(assetReturns)
		}

		correlation := stat.Correlation(assetReturns[:n], dxyReturns[:n], nil)
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

func getReturnsWithRetry(symbol string, endTime time.Time) ([]float64, string) {
	delays := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

	for i, delay := range delays {
		returns, err := getReturnsWithError(symbol, endTime)
		if err != nil {
			if strings.Contains(err.Error(), "remote-error") || strings.Contains(err.Error(), "429") {
				log.Printf("[%s] 第 %d 次重试遇到错误: %v, 等待 %.0fs", symbol, i+1, err, delay.Seconds())
				time.Sleep(delay)
				continue
			}
		}
		if returns != nil {
			return returns, symbol
		}
		if i < len(delays)-1 {
			log.Printf("[%s] 数据为空，第 %d 次重试...", symbol, i+1)
			time.Sleep(delay)
		}
	}

	log.Printf("[%s] 所有重试均失败", symbol)
	return nil, ""
}

func getReturnsWithError(symbol string, endTime time.Time) ([]float64, error) {
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
	}
	if err := iter.Err(); err != nil {
		log.Printf("[%s] 迭代器错误: %v", symbol, err)
		return nil, fmt.Errorf("remote-error: %v", err)
	}
	if len(prices) < 2 {
		log.Printf("[%s] 数据不足 (%d 条)，尝试 OneMin...", symbol, len(prices))
		p.Interval = datetime.OneMin
		iter = chart.Get(p)
		prices = nil
		for iter.Next() {
			bar := iter.Bar()
			f, _ := bar.Close.Float64()
			prices = append(prices, f)
		}
		if err := iter.Err(); err != nil {
			log.Printf("[%s] OneMin 迭代器错误: %v", symbol, err)
		}
		log.Printf("[%s] OneMin 数据条数: %d", symbol, len(prices))
		if len(prices) < 2 {
			return nil, nil
		}
	}
	returns := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		returns[i-1] = (prices[i] - prices[i-1]) / prices[i-1]
	}
	return returns, nil
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
