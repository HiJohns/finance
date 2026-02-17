// main.go
package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/smtp"
	"os"
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
	Assets      []string             `json:"assets"`
	ChinaAssets []string             `json:"china_assets"`
	Corrs6m     map[string][]float64 `json:"corrs6m"`
	Corrs30     map[string][]float64 `json:"corrs30"`
	ChinaCorr6m map[string][]float64 `json:"china_corr6m"`
	ChinaCorr30 map[string][]float64 `json:"china_corr30"`
	ChinaCorrHS map[string][]float64 `json:"china_corr_hs"`
	VixDxyCorr  float64              `json:"vix_dxy_corr"`
}

func main() {
	dateFlag := flag.String("date", "", "审计结束日期 (格式: YYYY-MM-DD)")
	modeFlag := flag.String("mode", "prod", "运行模式: prod(生产) 或 test(测试)")
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
	chinaPowerAssets := []string{"600406.SS", "002028.SZ", "002270.SZ", "688676.SS", "159326.SZ"}
	hs300 := "000300.SS"
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

	hs300Returns, hs300Dates, _ := getReturnsWithRetry(hs300, endTime)
	hs300Map := make(map[string]float64)
	if hs300Returns != nil && hs300Dates != nil {
		for i, date := range hs300Dates {
			hs300Map[date] = hs300Returns[i]
		}
		log.Printf("[沪深300] 获取到 %d 条数据", len(hs300Returns))
	}

	vixReturns, vixDates, _ := getReturnsWithRetry("^VIX", endTime)
	var vixDxyCorr float64
	vixWarning := ""
	if vixReturns != nil && vixDates != nil && dxyReturns != nil && dxyDates != nil {
		vixMap := make(map[string]float64)
		for i, date := range vixDates {
			vixMap[date] = vixReturns[i]
		}
		var alignedVix, alignedDxy []float64
		for i, date := range dxyDates {
			if v, ok := vixMap[date]; ok {
				if !math.IsNaN(v) && !math.IsNaN(dxyReturns[i]) && !math.IsInf(v, 0) && !math.IsInf(dxyReturns[i], 0) {
					alignedVix = append(alignedVix, v)
					alignedDxy = append(alignedDxy, dxyReturns[i])
				}
			}
		}
		if len(alignedVix) >= 30 {
			last30Vix := alignedVix[len(alignedVix)-30:]
			last30Dxy := alignedDxy[len(alignedDxy)-30:]
			vixDxyCorr = stat.Correlation(last30Vix, last30Dxy, nil)
		}
		log.Printf("[VIX-DXY] 30日相关性: %.4f", vixDxyCorr)
		dxyTrend := dxyReturns[len(dxyReturns)-1]
		if vixDxyCorr > 0.5 && dxyTrend > 0 {
			vixWarning = "警告：VIX 与 DXY 出现正向共振，市场进入非理性抽血模式。\n"
			log.Printf("[VIX-DXY] 🚨 流动性黑洞预警!")
		}
	}

	plotData := PlotData{
		Assets:      assets,
		ChinaAssets: chinaPowerAssets,
		Corrs6m:     make(map[string][]float64),
		Corrs30:     make(map[string][]float64),
		ChinaCorr6m: make(map[string][]float64),
		ChinaCorr30: make(map[string][]float64),
		ChinaCorrHS: make(map[string][]float64),
		VixDxyCorr:  vixDxyCorr,
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
			plotData.Corrs6m[symbol] = []float64{0}
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
			plotData.Corrs6m[symbol] = []float64{0}
			report += fmt.Sprintf("%-5s vs DXY: N/A (样本不足)\n", symbol)
			continue
		}

		if hasZeroVariance(validAsset) || hasZeroVariance(validDXY) {
			log.Printf("警告: %s 数据方差为0，无法计算相关性", symbol)
			plotData.Corrs6m[symbol] = []float64{0}
			report += fmt.Sprintf("%-5s vs DXY: N/A (方差为0)\n", symbol)
			continue
		}

		corr6m := stat.Correlation(validAsset, validDXY, nil)
		if math.IsNaN(corr6m) {
			log.Printf("[%s] 6个月相关性计算结果为 NaN", symbol)
			plotData.Corrs6m[symbol] = []float64{0}
			report += fmt.Sprintf("%-5s | 6mo: N/A | 30d: N/A | 状态: N/A\n", symbol)
			continue
		}

		var corr30d float64
		var corr30dStr string
		var status string
		if len(validAsset) >= 30 {
			shortAsset := validAsset[len(validAsset)-30:]
			shortDXY := validDXY[len(validDXY)-30:]
			corr30d = stat.Correlation(shortAsset, shortDXY, nil)
			if math.IsNaN(corr30d) {
				corr30dStr = "N/A"
			} else {
				corr30dStr = fmt.Sprintf("%.4f", corr30d)
				plotData.Corrs30[symbol] = []float64{corr30d}
				if corr30d < corr6m-0.2 || corr30d < -0.7 {
					status = "🚨 引力场收缩"
				} else if corr30d < -0.3 {
					status = "🟡 漂移"
				} else {
					status = "🟢 正常"
				}
			}
		} else {
			corr30dStr = "N/A"
			status = "🟢 正常"
		}

		log.Printf("[%s] 6mo: %.4f, 30d: %s, status: %s", symbol, corr6m, corr30dStr, status)
		plotData.Corrs6m[symbol] = []float64{corr6m}

		report += fmt.Sprintf("%-5s | 6mo: %.4f | 30d: %s | 状态: %s\n", symbol, corr6m, corr30dStr, status)
	}

	report += "\n【中国电力枢纽标的】(vs DXY & 沪深300)\n"
	report += "标的说明: 600406.SS=国电南瑞, 002028.SZ=思源电气, 002270.SZ=华明装备, 688676.SS=金盘科技, 159326.SZ=电网设备ETF\n\n"

	for _, symbol := range chinaPowerAssets {
		assetReturns, assetDates, _ := getReturnsWithRetry(symbol, endTime)

		if len(assetReturns) == 0 || len(dxyMap) == 0 {
			log.Printf("警告: %s 数据为空，跳过", symbol)
			plotData.ChinaCorr6m[symbol] = []float64{0}
			plotData.ChinaCorrHS[symbol] = []float64{0}
			report += fmt.Sprintf("%-10s vs DXY: N/A (数据不足)\n", symbol)
			continue
		}

		var validAssetDXY, validDXYArr []float64
		for i, date := range assetDates {
			if _, ok := dxyMap[date]; ok {
				ar := assetReturns[i]
				dr := dxyMap[date]
				if !math.IsNaN(ar) && !math.IsNaN(dr) && !math.IsInf(ar, 0) && !math.IsInf(dr, 0) {
					validAssetDXY = append(validAssetDXY, ar)
					validDXYArr = append(validDXYArr, dr)
				}
			}
		}

		log.Printf("[%s] 对齐DXY后有效样本: %d", symbol, len(validAssetDXY))

		if len(validAssetDXY) < 20 {
			log.Printf("警告: %s 对齐后样本量不足 (%d < 20)，跳过", symbol, len(validAssetDXY))
			plotData.ChinaCorr6m[symbol] = []float64{0}
			plotData.ChinaCorrHS[symbol] = []float64{0}
			report += fmt.Sprintf("%-10s vs DXY: N/A (样本不足)\n", symbol)
			continue
		}

		if hasZeroVariance(validAssetDXY) || hasZeroVariance(validDXYArr) {
			log.Printf("警告: %s 数据方差为0，无法计算相关性", symbol)
			plotData.ChinaCorr6m[symbol] = []float64{0}
			plotData.ChinaCorrHS[symbol] = []float64{0}
			report += fmt.Sprintf("%-10s vs DXY: N/A (方差为0)\n", symbol)
			continue
		}

		corr6m := stat.Correlation(validAssetDXY, validDXYArr, nil)
		if math.IsNaN(corr6m) {
			log.Printf("[%s] 6个月相关性计算结果为 NaN", symbol)
			plotData.ChinaCorr6m[symbol] = []float64{0}
			plotData.ChinaCorrHS[symbol] = []float64{0}
			report += fmt.Sprintf("%-10s | 6mo: N/A | 30d: N/A | 状态: N/A\n", symbol)
			continue
		}

		var corr30d float64
		var corr30dStr string
		if len(validAssetDXY) >= 30 {
			shortAsset := validAssetDXY[len(validAssetDXY)-30:]
			shortDXY := validDXYArr[len(validDXYArr)-30:]
			corr30d = stat.Correlation(shortAsset, shortDXY, nil)
			if math.IsNaN(corr30d) {
				corr30dStr = "N/A"
			} else {
				corr30dStr = fmt.Sprintf("%.4f", corr30d)
				plotData.ChinaCorr30[symbol] = []float64{corr30d}
			}
		} else {
			corr30dStr = "N/A"
		}

		log.Printf("[%s] 6mo vs DXY: %.4f, 30d: %s", symbol, corr6m, corr30dStr)
		plotData.ChinaCorr6m[symbol] = []float64{corr6m}

		var hsStatus string
		var hsCorr float64
		if len(hs300Map) > 0 && len(assetReturns) > 0 {
			var validAssetHS, validHSArr []float64
			for i, date := range assetDates {
				if hsVal, ok := hs300Map[date]; ok {
					ar := assetReturns[i]
					if !math.IsNaN(ar) && !math.IsNaN(hsVal) && !math.IsInf(ar, 0) && !math.IsInf(hsVal, 0) {
						validAssetHS = append(validAssetHS, ar)
						validHSArr = append(validHSArr, hsVal)
					}
				}
			}

			if len(validAssetHS) >= 20 {
				hsCorr = stat.Correlation(validAssetHS, validHSArr, nil)
				if math.IsNaN(hsCorr) {
					hsStatus = "N/A"
				} else if hsCorr > 0.6 {
					hsStatus = "跟随大盘内卷"
				} else if hsCorr < 0.3 {
					hsStatus = "独立走强"
				} else {
					hsStatus = "弱跟随"
				}
				plotData.ChinaCorrHS[symbol] = []float64{hsCorr}
			} else {
				hsStatus = "数据不足"
			}
		} else {
			hsStatus = "沪深300数据缺失"
		}

		report += fmt.Sprintf("%-10s | 6mo: %.4f | 30d: %s | 沪深300相关性: %.4f (%s)\n", symbol, corr6m, corr30dStr, hsCorr, hsStatus)
	}

	if vixWarning != "" {
		report = vixWarning + "\n" + report
	}

	subjectPrefix := ""
	if vixWarning != "" {
		subjectPrefix = "[🔴流动性黑洞预警] "
	}

	vixStatus := "Normal"
	if vixDxyCorr > 0.5 {
		vixStatus = "⚠️ RESONANCE"
	} else if vixDxyCorr > 0.3 {
		vixStatus = "Caution"
	}
	report += fmt.Sprintf("\n【VIX-DXY Resonance】当前相关性: %.4f (%s)\n", vixDxyCorr, vixStatus)

	subject := fmt.Sprintf("%sBeacon 审计: 宏观资产风险分析 [审计基准日: %s]", subjectPrefix, reportDate)

	generateChart(plotData)

	if *modeFlag == "test" {
		fmt.Println("\n" + strings.Repeat("=", 60))
		fmt.Println("📧 [TEST MODE] 邮件预览")
		fmt.Println(strings.Repeat("=", 60))
		fmt.Println("Subject:", subject)
		fmt.Println(strings.Repeat("-", 60))
		fmt.Println(report)
		fmt.Println(strings.Repeat("=", 60))
		fmt.Println("✅ 图表已生成 (audit_chart.png)")
		fmt.Println("⏭️ 跳过邮件发送 (test mode)")
	} else {
		sendEmail(subject, report)
	}
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
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("JSON 序列化失败: %v", err)
		return
	}
	cmd := exec.Command("python3", "plotter.py")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		log.Printf("创建 stdin pipe 失败: %v", err)
		return
	}
	go func() {
		defer stdin.Close()
		stdin.Write(jsonData)
	}()
	if err := cmd.Run(); err != nil {
		log.Printf("执行 plotter.py 失败: %v", err)
	}
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

	imgData, err := os.ReadFile("audit_chart.png")
	if err != nil {
		log.Printf("警告: audit_chart.png 不存在，发送纯文本邮件: %v", err)
		msg := []byte("From: " + from + "\r\n" +
			"To: " + receiver + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"Content-Type: text/plain; charset=\"utf-8\"\r\n" +
			"\r\n" +
			body)
		err = smtp.SendMail("smtp.qq.com:587", auth, smtpUser, []string{receiver}, msg)
		if err != nil {
			log.Printf("邮件发送失败: %v", err)
		}
		return
	}

	encoded := base64.StdEncoding.EncodeToString(imgData)
	boundary := "----IronCoreBoundary" + fmt.Sprintf("%d", time.Now().Unix())

	msg := []byte("From: " + from + "\r\n" +
		"To: " + receiver + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: multipart/mixed; boundary=" + boundary + "\r\n" +
		"\r\n" +
		"--" + boundary + "\r\n" +
		"Content-Type: text/html; charset=\"utf-8\"\r\n" +
		"\r\n" +
		"<html><body>" + body + "<br><br><img src=\"cid:chart\"></body></html>\r\n" +
		"--" + boundary + "\r\n" +
		"Content-Type: image/png; name=\"audit_chart.png\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"Content-ID: <chart>\r\n" +
		"Content-Disposition: inline; filename=\"audit_chart.png\"\r\n" +
		"\r\n" +
		encoded + "\r\n" +
		"--" + boundary + "--\r\n")

	err = smtp.SendMail("smtp.qq.com:587", auth, smtpUser, []string{receiver}, msg)
	if err != nil {
		log.Printf("邮件发送失败: %v", err)
	}
}
