// main.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/smtp"
	"os/exec"
	"time"

	"github.com/piquette/finance-go/chart"
	"github.com/piquette/finance-go/datetime"
	"gonum.org/v1/gonum/stat"
)

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

	dxyReturns := getReturns(dxy, endTime)
	plotData := PlotData{
		Assets: assets,
		Corrs:  make(map[string][]float64),
	}

	reportDate := endTime.Format("2006-01-02")
	report := fmt.Sprintf("--- Beacon 资产审计报告 [%s] ---\n\n", reportDate)
	report += "【美元引力场审计】\n"

	for _, symbol := range assets {
		assetReturns := getReturns(symbol, endTime)

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

func getReturns(symbol string, endTime time.Time) []float64 {
	endDt := datetime.New(&endTime)
	startTime := endTime.AddDate(0, -6, 0)
	startDt := datetime.New(&startTime)
	p := &chart.Params{
		Symbol:   symbol,
		Start:    startDt,
		End:      endDt,
		Interval: datetime.OneDay,
	}
	iter := chart.Get(p)
	var prices []float64
	for iter.Next() {
		f, _ := iter.Bar().Close.Float64()
		prices = append(prices, f)
	}
	if len(prices) < 2 {
		return nil
	}
	returns := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		returns[i-1] = (prices[i] - prices[i-1]) / prices[i-1]
	}
	return returns
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
