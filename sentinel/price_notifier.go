// sentinel/price_notifier.go
// 实时价格监控通知系统 - 使用 Go 并发特性

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/go-resty/resty/v2"
)

// StockConfig 定义单个股票的监控配置
type StockConfig struct {
	Ticker    string
	StopLoss  float64 // 止损价
	AlertDrop float64 // 跌幅预警阈值 (默认 5%)
}

// PriceData 存储从 API 获取的价格数据
type PriceData struct {
	Ticker        string
	CurrentPrice  float64
	PreviousClose float64
	ChangePercent float64
	Timestamp     time.Time
	Error         error
}

// AlertMessage 定义告警消息
type AlertMessage struct {
	Level        string // "WARNING", "CRITICAL"
	Ticker       string
	Message      string
	CurrentPrice float64
	Threshold    float64
	Timestamp    time.Time
}

// PriceMonitor 价格监控器
type PriceMonitor struct {
	client    *resty.Client
	configs   []StockConfig
	alertChan chan AlertMessage
	stopChan  chan struct{}
	wg        sync.WaitGroup
}

// NewPriceMonitor 创建新的监控器实例
func NewPriceMonitor(configs []StockConfig) *PriceMonitor {
	return &PriceMonitor{
		client:    resty.New().SetTimeout(10 * time.Second),
		configs:   configs,
		alertChan: make(chan AlertMessage, 100),
		stopChan:  make(chan struct{}),
	}
}

// fetchYahooFinance 从 Yahoo Finance API 获取价格数据
func (pm *PriceMonitor) fetchYahooFinance(ticker string) (*PriceData, error) {
	// 使用 Yahoo Finance API (通过快速查询接口)
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=2d", ticker)

	type YahooResponse struct {
		Chart struct {
			Result []struct {
				Meta struct {
					RegularMarketPrice         float64 `json:"regularMarketPrice"`
					PreviousClose              float64 `json:"previousClose"`
					RegularMarketChange        float64 `json:"regularMarketChange"`
					RegularMarketChangePercent float64 `json:"regularMarketChangePercent"`
				} `json:"meta"`
			} `json:"result"`
			Error interface{} `json:"error"`
		} `json:"chart"`
	}

	var result YahooResponse
	resp, err := pm.client.R().
		SetHeader("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36").
		SetResult(&result).
		Get(url)

	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}

	if resp.IsError() {
		return nil, fmt.Errorf("HTTP 错误: %d", resp.StatusCode())
	}

	if len(result.Chart.Result) == 0 {
		return nil, fmt.Errorf("无数据返回")
	}

	meta := result.Chart.Result[0].Meta
	return &PriceData{
		Ticker:        ticker,
		CurrentPrice:  meta.RegularMarketPrice,
		PreviousClose: meta.PreviousClose,
		ChangePercent: meta.RegularMarketChangePercent,
		Timestamp:     time.Now(),
	}, nil
}

// monitorStock 监控单个股票 (在单独的 goroutine 中运行)
func (pm *PriceMonitor) monitorStock(ctx context.Context, config StockConfig) {
	defer pm.wg.Done()

	// 设置默认跌幅预警阈值为 5%
	alertDrop := config.AlertDrop
	if alertDrop == 0 {
		alertDrop = 5.0
	}

	ticker := config.Ticker
	log.Printf("[%s] 启动监控 - 止损价: $%.2f, 跌幅预警: %.1f%%",
		ticker, config.StopLoss, alertDrop)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[%s] 监控停止", ticker)
			return
		case <-pm.stopChan:
			log.Printf("[%s] 监控停止", ticker)
			return
		default:
		}

		// 获取价格数据
		data, err := pm.fetchYahooFinance(ticker)
		if err != nil {
			log.Printf("[%s] 获取数据失败: %v", ticker, err)
			time.Sleep(30 * time.Second) // 出错后等待 30 秒
			continue
		}

		// 检查跌幅预警 (相对于昨日收盘价)
		dropPercent := -data.ChangePercent // 转为正值表示跌幅
		if dropPercent > alertDrop {
			alert := AlertMessage{
				Level:        "WARNING",
				Ticker:       ticker,
				Message:      fmt.Sprintf("大跌预警！相对于昨日收盘价下跌 %.2f%%", dropPercent),
				CurrentPrice: data.CurrentPrice,
				Threshold:    alertDrop,
				Timestamp:    time.Now(),
			}
			pm.alertChan <- alert
		}

		// 检查止损价
		if config.StopLoss > 0 && data.CurrentPrice <= config.StopLoss {
			alert := AlertMessage{
				Level:  "CRITICAL",
				Ticker: ticker,
				Message: fmt.Sprintf("止损触发！当前价格 $%.2f 低于止损价 $%.2f",
					data.CurrentPrice, config.StopLoss),
				CurrentPrice: data.CurrentPrice,
				Threshold:    config.StopLoss,
				Timestamp:    time.Now(),
			}
			pm.alertChan <- alert
		}

		// 打印监控状态
		log.Printf("[%s] 当前: $%.2f | 昨收: $%.2f | 涨跌: %.2f%%",
			ticker, data.CurrentPrice, data.PreviousClose, data.ChangePercent)

		// 每 10 秒检查一次
		time.Sleep(10 * time.Second)
	}
}

// printRedAlert 在控制台打印大大的红色警告
func printRedAlert(alert AlertMessage) {
	// 使用 fatih/color 库打印彩色输出
	red := color.New(color.FgRed, color.Bold, color.BgBlack)
	white := color.New(color.FgWhite, color.Bold, color.BgRed)

	fmt.Println()
	fmt.Println(strings.Repeat("=", 80))

	if alert.Level == "CRITICAL" {
		white.Println(" 🚨🚨🚨 CRITICAL ALERT 🚨🚨🚨 ")
	} else {
		red.Println(" ⚠️⚠️⚠ WARNING ⚠️⚠️⚠ ")
	}

	fmt.Println(strings.Repeat("=", 80))
	red.Printf(" 标的: %s\n", alert.Ticker)
	red.Printf(" 时间: %s\n", alert.Timestamp.Format("2006-01-02 15:04:05"))
	red.Printf(" 当前价格: $%.2f\n", alert.CurrentPrice)
	red.Printf(" 告警信息: %s\n", alert.Message)
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()
}

// sendSystemNotification 发送系统通知
func sendSystemNotification(alert AlertMessage) {
	title := fmt.Sprintf("[%s] %s Alert", alert.Ticker, alert.Level)
	message := fmt.Sprintf("Price: $%.2f - %s", alert.CurrentPrice, alert.Message)

	switch runtime.GOOS {
	case "darwin":
		// macOS 使用 osascript
		script := fmt.Sprintf(`display notification "%s" with title "%s" sound name "Basso"`,
			message, title)
		cmd := exec.Command("osascript", "-e", script)
		if err := cmd.Run(); err != nil {
			log.Printf("发送 macOS 通知失败: %v", err)
		}
	case "linux":
		// Linux 使用 notify-send
		cmd := exec.Command("notify-send", "-u", "critical", title, message)
		if err := cmd.Run(); err != nil {
			log.Printf("发送 Linux 通知失败: %v", err)
		}
	default:
		log.Printf("不支持的操作系统: %s", runtime.GOOS)
	}
}

// processAlerts 处理告警消息 (在单独的 goroutine 中运行)
func (pm *PriceMonitor) processAlerts() {
	for alert := range pm.alertChan {
		// 打印红色警告
		printRedAlert(alert)

		// 如果是 CRITICAL 级别，发送系统通知
		if alert.Level == "CRITICAL" {
			sendSystemNotification(alert)
		}
	}
}

// Start 启动监控系统
func (pm *PriceMonitor) Start(ctx context.Context) {
	log.Println("🚀 启动实时价格监控系统...")
	log.Printf("📊 监控标的: %v", pm.configs)

	// 启动告警处理器 (goroutine)
	go pm.processAlerts()

	// 为每个股票启动一个监控 goroutine
	for _, config := range pm.configs {
		pm.wg.Add(1)
		go pm.monitorStock(ctx, config)
	}

	// 等待所有 goroutine 完成
	pm.wg.Wait()
	close(pm.alertChan)
}

// Stop 停止监控系统
func (pm *PriceMonitor) Stop() {
	close(pm.stopChan)
}

func main() {
	// 配置监控列表
	configs := []StockConfig{
		{
			Ticker:    "AMD",
			StopLoss:  110.0, // AMD 止损价 $110
			AlertDrop: 5.0,   // 5% 跌幅预警
		},
		{
			Ticker:    "USO",
			StopLoss:  75.0, // USO 止损价 $75
			AlertDrop: 5.0,
		},
		{
			Ticker:    "SLV",
			StopLoss:  28.0, // SLV 止损价 $28
			AlertDrop: 5.0,
		},
	}

	// 创建监控器
	monitor := NewPriceMonitor(configs)

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 优雅退出处理
	sigChan := make(chan os.Signal, 1)
	go func() {
		<-sigChan
		log.Println("\n🛑 接收到停止信号，正在关闭监控...")
		cancel()
		monitor.Stop()
	}()

	// 启动监控 (阻塞)
	monitor.Start(ctx)

	log.Println("✅ 监控系统已停止")
}
