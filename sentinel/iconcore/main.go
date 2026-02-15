// main.go
package main

import (
	"encoding/json"
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
	assets := []string{"AMD", "SLV", "USO", "GLD", "IWY"}
	dxy := "DX-Y.NYB"

	// 获取数据
	dxyReturns := getReturns(dxy)
	plotData := PlotData{
		Assets: assets,
		Corrs:  make(map[string][]float64),
	}

	report := fmt.Sprintf("--- Beacon 资产审计报告 [%s] ---\n\n", time.Now().Format("2006-01-02"))
	report += "【美元引力场审计】\n"

	for _, symbol := range assets {
		assetReturns := getReturns(symbol)

		// 对齐数据长度
		n := len(dxyReturns)
		if len(assetReturns) < n {
			n = len(assetReturns)
		}

		// 计算相关性
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

	// 生成图表并发送邮件
	generateChart(plotData)
	sendEmail("Beacon 审计: 宏观资产风险分析", report)
}

func getReturns(symbol string) []float64 {
	currentTime := now()
	endDt := datetime.New(&currentTime)
	startTime := currentTime.AddDate(0, -6, 0)
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
