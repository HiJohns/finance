// main.go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"math"
	"net/http"
	"net/smtp"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/piquette/finance-go"
	"github.com/piquette/finance-go/chart"
	"github.com/piquette/finance-go/datetime"
	"gonum.org/v1/gonum/stat"

	yaml "gopkg.in/yaml.v3"
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

// Config represents the dynamic configuration
type Config struct {
	mu sync.RWMutex

	Assets struct {
		GlobalMacro    []AssetConfig `yaml:"global_macro"`
		ChinaPowerGrid []AssetConfig `yaml:"china_power_grid"`
		Benchmarks     []AssetConfig `yaml:"benchmarks"`
	} `yaml:"assets"`

	SentinelKeywords struct {
		Geopolitical []string `yaml:"geopolitical"`
		SupplyChain  []string `yaml:"supply_chain"`
		Macro        []string `yaml:"macro"`
	} `yaml:"sentinel_keywords"`

	Thresholds struct {
		SigmaLimit              float64 `yaml:"sigma_limit"`
		ImpactScoreHigh         float64 `yaml:"impact_score_high"`
		ImpactScoreCritical     float64 `yaml:"impact_score_critical"`
		AuctionVolumeMultiplier float64 `yaml:"auction_volume_multiplier"`
		CorrelationDivergence   float64 `yaml:"correlation_divergence"`
	} `yaml:"thresholds"`

	API struct {
		NewsProvider         string `yaml:"news_provider"`
		LLMProvider          string `yaml:"llm_provider"`
		ConfigReloadEndpoint string `yaml:"config_reload_endpoint"`
	} `yaml:"api"`

	Runtime struct {
		AuditIntervalMinutes int    `yaml:"audit_interval_minutes"`
		AuctionTime          string `yaml:"auction_time"`
		Timezone             string `yaml:"timezone"`
	} `yaml:"runtime"`
}

// AssetConfig represents a single asset configuration
type AssetConfig struct {
	Symbol                string   `yaml:"symbol"`
	Name                  string   `yaml:"name"`
	Source                string   `yaml:"source"`
	Tags                  []string `yaml:"tags,omitempty"`
	IsBenchmark           bool     `yaml:"is_benchmark,omitempty"`
	IsSentimentIndicator  bool     `yaml:"is_sentiment_indicator,omitempty"`
	SensitivityMultiplier float64  `yaml:"sensitivity_multiplier,omitempty"`
	AuctionMonitor        bool     `yaml:"auction_monitor,omitempty"`
}

var (
	smtpUser      string
	smtpPass      string
	receiver      string
	dbPath        string
	httpPort      string
	AdminUser     string = "admin"
	AdminPass     string = "admin123"
	SessionSecret string = "test-secret-key"
	version       string
)

var (
	globalConfig *Config
	configPath   string = "config.yaml"
)

// GetAssets returns all configured assets safely
func (c *Config) GetAssets() ([]AssetConfig, []AssetConfig, []AssetConfig) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Assets.GlobalMacro, c.Assets.ChinaPowerGrid, c.Assets.Benchmarks
}

// GetAssetBySymbol returns asset config by symbol
func (c *Config) GetAssetBySymbol(symbol string) (AssetConfig, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	allAssets := append(c.Assets.GlobalMacro, c.Assets.ChinaPowerGrid...)
	allAssets = append(allAssets, c.Assets.Benchmarks...)

	for _, asset := range allAssets {
		if asset.Symbol == symbol {
			return asset, true
		}
	}
	return AssetConfig{}, false
}

// GetSentinelKeywords returns all sentinel keywords
func (c *Config) GetSentinelKeywords() ([]string, []string, []string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.SentinelKeywords.Geopolitical, c.SentinelKeywords.SupplyChain, c.SentinelKeywords.Macro
}

// GetThresholds returns threshold values
func (c *Config) GetThresholds() (float64, float64, float64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Thresholds.SigmaLimit, c.Thresholds.ImpactScoreHigh, c.Thresholds.ImpactScoreCritical
}

// ReloadConfig reloads configuration from file
func (c *Config) ReloadConfig() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	newConfig := &Config{}
	if err := yaml.Unmarshal(data, newConfig); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	*c = *newConfig
	log.Println("✅ Configuration reloaded successfully")
	return nil
}

// LoadConfig loads configuration from file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := &Config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Set defaults if not specified
	if config.Thresholds.SigmaLimit == 0 {
		config.Thresholds.SigmaLimit = 3.0
	}
	if config.Thresholds.ImpactScoreHigh == 0 {
		config.Thresholds.ImpactScoreHigh = 0.8
	}
	if config.Thresholds.ImpactScoreCritical == 0 {
		config.Thresholds.ImpactScoreCritical = 0.9
	}
	if config.Thresholds.AuctionVolumeMultiplier == 0 {
		config.Thresholds.AuctionVolumeMultiplier = 2.0
	}
	if config.Runtime.AuditIntervalMinutes == 0 {
		config.Runtime.AuditIntervalMinutes = 10
	}
	if config.Runtime.Timezone == "" {
		config.Runtime.Timezone = "Asia/Shanghai"
	}

	return config, nil
}

// GetAllAssetSymbols returns all asset symbols as string slices
func (c *Config) GetAllAssetSymbols() ([]string, []string) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	globalSymbols := make([]string, len(c.Assets.GlobalMacro))
	for i, asset := range c.Assets.GlobalMacro {
		globalSymbols[i] = asset.Symbol
	}

	chinaSymbols := make([]string, len(c.Assets.ChinaPowerGrid))
	for i, asset := range c.Assets.ChinaPowerGrid {
		chinaSymbols[i] = asset.Symbol
	}

	return globalSymbols, chinaSymbols
}

// GetBenchmarkSymbol returns the benchmark symbol (HS300)
func (c *Config) GetBenchmarkSymbol() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, asset := range c.Assets.Benchmarks {
		if asset.Symbol == "000300.SS" {
			return asset.Symbol
		}
	}
	return "000300.SS" // fallback
}

// GetSentimentIndicator returns VIX symbol
func (c *Config) GetSentimentIndicator() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for _, asset := range c.Assets.GlobalMacro {
		if asset.IsSentimentIndicator {
			return asset.Symbol
		}
	}
	return "^VIX" // fallback
}

// GetAuctionMonitorAssets returns assets marked for auction monitoring
func (c *Config) GetAuctionMonitorAssets() []AssetConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var auctionAssets []AssetConfig
	for _, asset := range c.Assets.ChinaPowerGrid {
		if asset.AuctionMonitor {
			auctionAssets = append(auctionAssets, asset)
		}
	}
	return auctionAssets
}

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

type AssetStatus struct {
	Symbol            string  `json:"symbol"`
	Name              string  `json:"name"`
	Tags              string  `json:"tags"`
	CurrentPrice      float64 `json:"current_price"`
	MarketPrice       float64 `json:"market_price"`
	Volume            float64 `json:"volume"`
	TurnoverRate      float64 `json:"turnover_rate"`
	LatestReturn      float64 `json:"latest_return"`
	Corr6m            float64 `json:"corr_6m"`
	Corr30d           float64 `json:"corr_30d"`
	Sigma             float64 `json:"sigma"`
	Mean              float64 `json:"mean"`
	IsCritical        bool    `json:"is_critical"`
	AlertMessage      string  `json:"alert_message"`
	HS300Corr         float64 `json:"hs300_corr"`
	CorrelationStatus string  `json:"correlation_status"`
	MarketStatus      string  `json:"market_status"`
	SentimentScore    float64 `json:"sentiment_score"`
	LatestNews        string  `json:"latest_news"`
	ImpactScore       float64 `json:"impact_score"`
	TacticalAdvice    string  `json:"tactical_advice"`
}

type ResonanceStatus struct {
	IsActive        bool    `json:"is_active"`
	SyncRate        float64 `json:"sync_rate"`
	Confidence      string  `json:"confidence"`
	TriggeredAssets string  `json:"triggered_assets"`
	Message         string  `json:"message"`
}

type AuditStatus struct {
	Timestamp           time.Time          `json:"timestamp"`
	Assets              []AssetStatus      `json:"assets"`
	VixDxyCorr          float64            `json:"vix_dxy_corr"`
	VixWarning          bool               `json:"vix_warning"`
	SilentPeriod        bool               `json:"silent_period"`
	LastAlertTime       time.Time          `json:"last_alert_time"`
	CorrAcceleration    map[string]float64 `json:"corr_acceleration"`
	Resonance           ResonanceStatus    `json:"resonance"`
	GeopoliticalRisk    string             `json:"geopolitical_risk"`
	CapitalAlert        bool               `json:"capital_alert"`
	CapitalAlertMessage string             `json:"capital_alert_message"`
	CapitalAlertTime    time.Time          `json:"capital_alert_time"`
	TacticalAdvice      string             `json:"tactical_advice"`
}

var (
	globalStatus  AuditStatus
	statusMutex   sync.RWMutex // Protects globalStatus access
	lastCorrMap   map[string]float64
	lastCorrMutex sync.RWMutex // Protects lastCorrMap access
	db            *sql.DB
)

func initDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create market_data table with turnover_rate
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS market_data (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			symbol TEXT NOT NULL,
			price REAL,
			volume REAL,
			turnover_rate REAL,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create market_data table: %w", err)
	}

	// Create index
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_symbol_timestamp 
		ON market_data(symbol, timestamp)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create index: %w", err)
	}

	// Create news_events table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS news_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp TEXT NOT NULL,
			symbol TEXT,
			title TEXT NOT NULL,
			impact_score REAL DEFAULT 0.0,
			sentiment TEXT,
			logic_summary TEXT,
			created_at TEXT DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create news_events table: %w", err)
	}

	// Create index for news_events
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_news_timestamp 
		ON news_events(timestamp)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create news index: %w", err)
	}

	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_news_symbol 
		ON news_events(symbol)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create news symbol index: %w", err)
	}

	return db, nil
}

func getLatestSentimentForSymbol(symbol string) (float64, string, error) {
	if db == nil {
		return 0, "", fmt.Errorf("database not initialized")
	}

	var score float64
	var title string
	err := db.QueryRow(`
		SELECT impact_score, title FROM news_events 
		WHERE symbol = ? OR symbol IS NULL 
		ORDER BY timestamp DESC LIMIT 1
	`, symbol).Scan(&score, &title)

	if err == sql.ErrNoRows {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", err
	}

	return score, title, nil
}

func getGeopoliticalRiskLevel() string {
	if db == nil {
		return "Green"
	}

	since := time.Now().Add(-6 * time.Hour).Format("2006-01-02 15:04:05")

	var avgScore float64
	err := db.QueryRow(`
		SELECT AVG(impact_score) FROM news_events 
		WHERE timestamp > ?
	`, since).Scan(&avgScore)

	if err != nil || avgScore == 0 {
		return "Green"
	}

	if avgScore >= 0.9 {
		return "Red"
	} else if avgScore >= 0.8 {
		return "Yellow"
	}
	return "Green"
}

func getRecentHighImpactNews() []map[string]interface{} {
	if db == nil {
		return nil
	}

	since := time.Now().Add(-24 * time.Hour).Format("2006-01-02 15:04:05")

	rows, err := db.Query(`
		SELECT title, impact_score, sentiment, timestamp 
		FROM news_events 
		WHERE timestamp > ? AND impact_score >= 0.8
		ORDER BY timestamp DESC LIMIT 10
	`, since)

	if err != nil {
		return nil
	}
	defer rows.Close()

	var events []map[string]interface{}
	for rows.Next() {
		var title, sentiment, timestamp string
		var score float64
		if err := rows.Scan(&title, &score, &sentiment, &timestamp); err == nil {
			events = append(events, map[string]interface{}{
				"title":     title,
				"score":     score,
				"sentiment": sentiment,
				"time":      timestamp,
			})
		}
	}

	return events
}

func generateTacticalAdvice(assets []AssetStatus, resonance ResonanceStatus, geoRisk string) string {
	// Find critical assets with high impact scores
	var criticalAssets []AssetStatus
	var highImpactAssets []AssetStatus
	var gridHardAssets []AssetStatus
	var aidcAssets []AssetStatus

	for _, a := range assets {
		if a.IsCritical {
			criticalAssets = append(criticalAssets, a)
		}
		if a.ImpactScore >= 0.8 {
			highImpactAssets = append(highImpactAssets, a)
		}
		if strings.Contains(a.Tags, "Grid-Hard") {
			gridHardAssets = append(gridHardAssets, a)
		}
		if strings.Contains(a.Tags, "AIDC-Leader") {
			aidcAssets = append(aidcAssets, a)
		}
	}

	// Generate advice based on conditions
	var adviceParts []string

	// Check for physical infrastructure resonance
	if resonance.IsActive && len(gridHardAssets) > 0 {
		adviceParts = append(adviceParts, "检测到物理基建板块共振信号")
	}

	// Check for high geopolitical risk with sector impact
	if geoRisk == "Red" && len(highImpactAssets) > 0 {
		adviceParts = append(adviceParts, "地缘政治风险处于危机级别")
	} else if geoRisk == "Yellow" {
		adviceParts = append(adviceParts, "地缘政治风险处于紧张状态")
	}

	// Check for 3-sigma anomalies
	if len(criticalAssets) > 0 {
		symbols := []string{}
		for _, a := range criticalAssets {
			symbols = append(symbols, a.Symbol)
		}
		adviceParts = append(adviceParts, fmt.Sprintf("%d 个标的触发3-Sigma异动", len(criticalAssets)))
	}

	// Generate final recommendation
	if len(adviceParts) == 0 {
		return "市场处于正常波动区间，建议维持现有仓位观察。"
	}

	advice := strings.Join(adviceParts, "，") + "。"

	// Add specific recommendation
	if resonance.IsActive && geoRisk == "Red" && len(gridHardAssets) > 0 {
		advice += " 建议关注电力枢纽标的配置机会，如："
		for _, a := range gridHardAssets[:min(3, len(gridHardAssets))] {
			advice += fmt.Sprintf(" %s", a.Symbol)
		}
		advice += "。"
	} else if len(criticalAssets) > 0 && geoRisk == "Green" {
		advice += " 异动与地缘因素无关，建议谨慎追高。"
	}

	return advice
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getLatestVolumeAndTurnover(symbol string) (float64, float64, error) {
	if db == nil {
		return 0, 0, fmt.Errorf("database not initialized")
	}
	var volume, turnover float64
	normalizedSymbol := symbol
	if strings.HasSuffix(symbol, ".SS") {
		normalizedSymbol = strings.TrimSuffix(symbol, ".SS")
	} else if strings.HasSuffix(symbol, ".SH") {
		normalizedSymbol = strings.TrimSuffix(symbol, ".SH")
	} else if strings.HasSuffix(symbol, ".SZ") {
		normalizedSymbol = strings.TrimSuffix(symbol, ".SZ")
	}
	err := db.QueryRow(
		"SELECT volume, turnover_rate FROM market_data WHERE symbol LIKE ? ORDER BY timestamp DESC LIMIT 1",
		normalizedSymbol+"%",
	).Scan(&volume, &turnover)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to query volume for %s: %w", symbol, err)
	}
	return volume, turnover, nil
}

var dashboardHTML = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>IronCore 2.0 实时审计仪表盘</title>
    <meta http-equiv="refresh" content="30">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 20px; background: #1a1a2e; color: #eee; }
        h1 { color: #00d4ff; margin-bottom: 5px; }
        .header-bar { display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px; }
        .sync-btn { 
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%); 
            border: none; color: white; padding: 12px 24px; 
            border-radius: 8px; cursor: pointer; font-size: 14px;
            font-weight: bold; box-shadow: 0 4px 15px rgba(102, 126, 234, 0.4);
            transition: transform 0.2s, box-shadow 0.2s;
        }
        .sync-btn:hover { transform: translateY(-2px); box-shadow: 0 6px 20px rgba(102, 126, 234, 0.6); }
        .status-bar { padding: 15px; margin: 10px 0; border-radius: 8px; }
        .normal { background: #0f3460; }
        .warning { background: #53354a; }
        .critical { background: #903749; animation: pulse 1s infinite; }
        .resonance-active { background: linear-gradient(90deg, #11998e, #38ef7d); animation: pulse 1s infinite; }
        .capital-alert { background: linear-gradient(90deg, #ff416c, #ff4b2b); animation: pulse 0.5s infinite; }
        @keyframes pulse { 0% { opacity: 1; } 50% { opacity: 0.7; } 100% { opacity: 1; } }
        table { width: 100%; border-collapse: collapse; margin-top: 20px; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #333; }
        th { background: #16213e; color: #00d4ff; }
        tr.critical-row { animation: critical-flash 1s infinite; background: rgba(255, 107, 107, 0.15); }
        @keyframes critical-flash { 0%, 100% { background: rgba(255, 107, 107, 0.15); } 50% { background: rgba(255, 107, 107, 0.3); } }
        .alert { color: #ff6b6b; font-weight: bold; }
        .safe { color: #51cf66; }
        .cyan { color: #00ffff; font-weight: bold; }
        .section { margin-top: 30px; }
        .timestamp { color: #888; font-size: 0.9em; }
        .tag { display: inline-block; background: #2d3748; padding: 2px 8px; border-radius: 4px; font-size: 12px; margin-left: 5px; }
        .market-price { color: #f6e05e; font-weight: bold; }
        .risk-light { display: inline-block; width: 20px; height: 20px; border-radius: 50%; margin-right: 8px; vertical-align: middle; }
        .risk-green { background: #28a745; box-shadow: 0 0 10px #28a745; }
        .risk-yellow { background: #ffc107; box-shadow: 0 0 10px #ffc107; }
        .risk-red { background: #dc3545; box-shadow: 0 0 10px #dc3545; animation: blink 1s infinite; }
        @keyframes blink { 0%, 100% { opacity: 1; } 50% { opacity: 0.5; } }
        .tactical-advice { background: linear-gradient(135deg, #1e3c72, #2a5298); padding: 15px; border-radius: 8px; margin: 15px 0; border-left: 4px solid #00d4ff; }
        .advice-title { color: #00d4ff; font-weight: bold; margin-bottom: 8px; }
        .news-badge { display: inline-block; background: #ff4757; color: white; padding: 2px 6px; border-radius: 4px; font-size: 10px; margin-left: 5px; }
        .impact-high { background: #ff4757; }
    </style>
    <script>
        function syncNow() {
            fetch('/api/audit').then(r => r.json()).then(d => {
                alert('已触发实时同步!');
                location.reload();
            });
        }
        function getRiskClass(level) {
            if (level === 'Red') return 'risk-red';
            if (level === 'Yellow') return 'risk-yellow';
            return 'risk-green';
        }
    </script>
</head>
<body>
    <div class="header-bar">
        <h1>⚡ IronCore 2.0 实时资产异动审计</h1>
        <button class="sync-btn" onclick="syncNow()">🔄 立即同步实时引力场</button>
    </div>
    <div style="text-align: right; margin-bottom: 10px;">
        <a href="/logout" style="color: #888; text-decoration: none;">[退出登录]</a>
    </div>

    {{if .CapitalAlert}}
    <div class="status-bar capital-alert">
        <strong>🔥 换血资金进场!</strong> {{.CapitalAlertMessage}} |
        <span class="timestamp">检测时间: {{.CapitalAlertTime.Format "15:04:05"}}</span>
    </div>
    {{end}}
    
    {{if .Resonance.IsActive}}
    <div class="status-bar resonance-active">
        <strong>🔥 核心共振:</strong> {{.Resonance.Message}} | 
        <strong>触发标的:</strong> {{.Resonance.TriggeredAssets}} |
        <strong>同步率:</strong> {{printf "%.2f" .Resonance.SyncRate}} |
        <strong>入场置信度:</strong> {{.Resonance.Confidence}}
    </div>
    {{else}}
    <div class="status-bar {{if .SilentPeriod}}warning{{else}}normal{{end}}">
        <strong>状态:</strong> {{if .SilentPeriod}}🔇 静默期 (周末/闭市){{else}}🟢 监控中{{end}} | 
        <strong>VIX-DXY相关:</strong> {{printf "%.4f" .VixDxyCorr}} {{if .VixWarning}}<span class="alert">⚠️ 共振预警</span>{{end}} |
        <span class="timestamp">更新: {{.Timestamp.Format "2006-01-02 15:04:05"}}</span>
    </div>
    {{end}}

    <div class="status-bar normal">
        <strong>🌍 地缘政治风险:</strong>
        <span class="risk-light {{if eq .GeopoliticalRisk "Red"}}risk-red{{else if eq .GeopoliticalRisk "Yellow"}}risk-yellow{{else}}risk-green{{end}}"></span>
        <span style="font-weight: bold;">
            {{if eq .GeopoliticalRisk "Red"}}Red (Crisis){{else if eq .GeopoliticalRisk "Yellow"}}Yellow (Tension){{else}}Green (Calm){{end}}
        </span>
        <span style="margin-left: 20px; color: #888;">基于过去6小时新闻分析</span>
    </div>

    {{if .TacticalAdvice}}
    <div class="tactical-advice">
        <div class="advice-title">🎯 战术建议 (Tactical Advice)</div>
        <div>{{.TacticalAdvice}}</div>
    </div>
    {{end}}

    <div class="section">
        <h2>📊 全球宏观标的</h2>
        <table>
            <tr><th>标的</th><th>标签</th><th>最新价</th><th>Market Price</th><th>收益率</th><th>6月相关</th><th>30日相关</th><th>3-Sigma</th><th>新闻评分</th><th>状态</th></tr>
            {{range .Assets}}
            {{if ne .CorrelationStatus "china"}}
            <tr {{if .IsCritical}}class="critical-row"{{end}}>
                <td><strong>{{.Symbol}}</strong><br><small>{{.Name}}</small></td>
                <td>{{if .Tags}}<span class="tag">{{.Tags}}</span>{{end}}</td>
                <td>{{printf "%.2f" .CurrentPrice}}</td>
                <td><span class="market-price">{{printf "%.2f" .MarketPrice}}</span></td>
                <td>{{printf "%.2f" .LatestReturn}}%</td>
                <td {{if lt .Corr6m 0.1}}class="cyan"{{end}}>{{printf "%.4f" .Corr6m}}</td>
                <td {{if lt .Corr30d 0.1}}class="cyan"{{end}}>{{printf "%.4f" .Corr30d}}</td>
                <td>μ={{printf "%.4f" .Mean}}, σ={{printf "%.4f" .Sigma}}</td>
                <td>{{if ge .ImpactScore 0.8}}<span class="news-badge impact-high">{{printf "%.2f" .ImpactScore}}</span>{{else}}{{printf "%.2f" .ImpactScore}}{{end}}</td>
                <td>{{if .IsCritical}}<span class="alert">🚨 {{.AlertMessage}}</span>{{else}}<span class="safe">🟢 正常</span>{{end}}</td>
            </tr>
            {{end}}
            {{end}}
        </table>
    </div>

    <div class="section">
        <h2>🇨🇳 中国电力枢纽标的</h2>
        <table>
            <tr><th>标的</th><th>标签</th><th>最新价</th><th>Market Price</th><th>收益率</th><th>换手率</th><th>vs DXY</th><th>vs 沪深300</th><th>新闻评分</th><th>状态</th></tr>
            {{range .Assets}}
            {{if eq .CorrelationStatus "china"}}
            <tr {{if .IsCritical}}class="critical-row"{{end}}>
                <td><strong>{{.Symbol}}</strong><br><small>{{.Name}}</small></td>
                <td>{{if .Tags}}<span class="tag">{{.Tags}}</span>{{end}}</td>
                <td>{{printf "%.2f" .CurrentPrice}}</td>
                <td><span class="market-price">{{printf "%.2f" .MarketPrice}}</span></td>
                <td>{{printf "%.2f" .LatestReturn}}%</td>
                <td>{{printf "%.2f" .TurnoverRate}}%</td>
                <td {{if lt .Corr30d 0.1}}class="cyan"{{end}}>{{printf "%.4f" .Corr30d}}</td>
                <td {{if lt .HS300Corr 0.1}}class="cyan"{{end}}>{{printf "%.4f" .HS300Corr}}</td>
                <td>{{if ge .ImpactScore 0.8}}<span class="news-badge impact-high">{{printf "%.2f" .ImpactScore}}</span>{{else}}{{printf "%.2f" .ImpactScore}}{{end}}</td>
                <td>{{if .IsCritical}}<span class="alert">🚨 {{.AlertMessage}}</span>{{else}}<span class="safe">🟢 正常</span>{{end}}</td>
            </tr>
            {{end}}
            {{end}}
        </table>
    </div>
</body>
</html>`

var loginHTML = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>企业级分布式日志管理系统 v4.2</title>
    <link href="https://cdn.jsdelivr.net/npm/bootstrap@3.4.1/dist/css/bootstrap.min.css" rel="stylesheet">
    <style>
        body { background: #f5f5f5; padding-top: 80px; }
        .login-container { max-width: 400px; margin: 0 auto; background: #fff; padding: 30px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        .login-title { text-align: center; color: #333; margin-bottom: 30px; }
        .alert-info { background: #d9edf7; border-color: #bce8f1; color: #31708f; font-size: 12px; }
        .footer { text-align: center; margin-top: 20px; color: #999; font-size: 11px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="login-container">
            <h3 class="login-title">企业级分布式日志管理系统 v4.2</h3>
            <div class="alert alert-info">
                <strong>⚠️ 安全警示：</strong>本系统仅限授权人员访问，所有操作将自动记录 IP 地址及操作时间。
            </div>
            {{if .Error}}
            <div class="alert alert-danger">{{.Error}}</div>
            {{end}}
            <form method="POST" action="/auth">
                <div class="form-group">
                    <label>Operator ID</label>
                    <input type="text" name="username" class="form-control" placeholder="请输入操作员账号" required>
                </div>
                <div class="form-group">
                    <label>Access Key</label>
                    <input type="password" name="password" class="form-control" placeholder="请输入访问密钥" required>
                </div>
                <button type="submit" class="btn btn-primary btn-block">验证身份</button>
            </form>
            <div class="footer">
                © 2024 企业技术架构部 | 系统版本 v4.2.0 | 构建时间: 2024-01-15
            </div>
        </div>
    </div>
</body>
</html>`

func main() {
	versionFlag := flag.Bool("v", false, "显示版本")
	dateFlag := flag.String("date", "", "审计结束日期 (格式: YYYY-MM-DD)")
	_ = flag.String("mode", "prod", "运行模式: prod(生产) 或 test(测试)")
	flag.StringVar(&dbPath, "db", "ironcore.db", "SQLite数据库路径")
	flag.StringVar(&httpPort, "port", "9070", "HTTP服务端口")
	flag.StringVar(&configPath, "config", "config.yaml", "配置文件路径")
	flag.Parse()

	if *versionFlag {
		fmt.Println("IronCore version:", version)
		os.Exit(0)
	}

	// Load configuration
	var err error
	globalConfig, err = LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("✅ Configuration loaded from %s", configPath)

	var endTime time.Time
	if *dateFlag != "" {
		endTime, err = time.Parse("2006-01-02", *dateFlag)
		if err != nil {
			log.Printf("日期解析失败，使用当前时间: %v", err)
			endTime = time.Now()
		}
	} else {
		endTime = time.Now()
	}

	db, err = initDB(dbPath)
	if err != nil {
		log.Printf("数据库初始化失败: %v", err)
	} else {
		defer db.Close()
	}

	globalStatus = AuditStatus{
		Timestamp:        time.Now(),
		Assets:           []AssetStatus{},
		CorrAcceleration: make(map[string]float64),
	}
	lastCorrMap = make(map[string]float64)

	go runAuditLoop(endTime)

	http.HandleFunc("/login", handleLogin)
	http.HandleFunc("/auth", handleAuth)
	http.HandleFunc("/logout", handleLogout)
	http.HandleFunc("/", authMiddleware(handleDashboard))
	http.HandleFunc("/api/status", authMiddleware(handleAPIStatus))
	http.HandleFunc("/api/audit", authMiddleware(handleTriggerAudit))
	http.HandleFunc("/api/reload-config", authMiddleware(handleReloadConfig))

	addr := ":" + httpPort
	log.Printf("🚀 IronCore 服务启动: http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func runAuditLoop(baseTime time.Time) {
	interval := time.Duration(globalConfig.Runtime.AuditIntervalMinutes) * time.Minute
	if interval == 0 {
		interval = 10 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		endTime := baseTime.Add(time.Since(baseTime))
		if endTime.Before(time.Now().Add(-24 * time.Hour)) {
			endTime = time.Now()
		}
		performAudit(endTime)
		<-ticker.C
	}
}

func performAudit(endTime time.Time) {
	log.Println("🔄 执行审计...")

	// Check auction time and trigger alerts if needed
	if isAuctionSnapshotTime() {
		log.Println("📊 集合竞价时段，检查成交量...")
		auctionAssets := globalConfig.GetAuctionMonitorAssets()
		for _, asset := range auctionAssets {
			isAnomaly, volume, ratio := checkAuctionVolumeAlert(asset)
			if isAnomaly {
				triggerCapitalAlert(asset, volume, ratio)
			}
		}
	}

	// Get assets from config
	globalAssets, chinaAssets, benchmarks := globalConfig.GetAssets()

	// Find DXY benchmark
	var dxySymbol string
	for _, asset := range globalAssets {
		if asset.Symbol == "DX-Y.NYB" {
			dxySymbol = asset.Symbol
			break
		}
	}
	if dxySymbol == "" {
		dxySymbol = "DX-Y.NYB"
	}

	// Find HS300
	var hs300 string
	for _, asset := range benchmarks {
		if asset.Symbol == "000300.SS" {
			hs300 = asset.Symbol
			break
		}
	}
	if hs300 == "" {
		hs300 = "000300.SS"
	}

	// Get VIX symbol
	vixSymbol := globalConfig.GetSentimentIndicator()

	dxyReturns, dxyDates, _ := getReturnsWithRetry(dxySymbol, endTime)
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
	}

	vixReturns, vixDates, _ := getReturnsWithRetry(vixSymbol, endTime)
	vixDxyCorr := 0.0
	vixWarning := false
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
		if vixDxyCorr > 0.5 && dxyReturns[len(dxyReturns)-1] > 0 {
			vixWarning = true
			log.Printf("[VIX-DXY] 🚨 流动性黑洞预警!")
		}
	}

	silentPeriod := isSilentPeriod()

	var assetStatuses []AssetStatus

	// Process global assets
	for _, asset := range globalAssets {
		status := calculateAssetStatusWithConfig(asset, dxyMap, endTime, "global")
		assetStatuses = append(assetStatuses, status)
	}

	// Process China power grid assets
	for _, asset := range chinaAssets {
		status := calculateAssetStatusWithConfig(asset, dxyMap, endTime, "china")
		if len(hs300Map) > 0 {
			status.HS300Corr = calculateHS300Corr(asset.Symbol, hs300Map, endTime)
			if status.HS300Corr > 0.6 {
				status.MarketStatus = "跟随大盘内卷"
			} else if status.HS300Corr < 0.3 {
				status.MarketStatus = "独立走强"
			} else {
				status.MarketStatus = "弱跟随"
			}
		}
		assetStatuses = append(assetStatuses, status)
	}

	acceleration := calculateCorrAcceleration(assetStatuses)
	resonance := calculateResonance(assetStatuses)

	// Get geopolitical risk level and generate tactical advice
	geoRisk := getGeopoliticalRiskLevel()
	tacticalAdvice := generateTacticalAdvice(assetStatuses, resonance, geoRisk)

	globalStatus = AuditStatus{
		Timestamp:        time.Now(),
		Assets:           assetStatuses,
		VixDxyCorr:       vixDxyCorr,
		VixWarning:       vixWarning,
		SilentPeriod:     silentPeriod,
		CorrAcceleration: acceleration,
		Resonance:        resonance,
		GeopoliticalRisk: geoRisk,
		TacticalAdvice:   tacticalAdvice,
	}

	checkAndSendAlert(vixWarning, assetStatuses)

	// Build plot data
	plotAssets := make([]string, len(globalAssets))
	for i, a := range globalAssets {
		plotAssets[i] = a.Symbol
	}
	plotChinaAssets := make([]string, len(chinaAssets))
	for i, a := range chinaAssets {
		plotChinaAssets[i] = a.Symbol
	}

	plotData := PlotData{
		Assets:      plotAssets,
		ChinaAssets: plotChinaAssets,
		Corrs6m:     make(map[string][]float64),
		Corrs30:     make(map[string][]float64),
		ChinaCorr6m: make(map[string][]float64),
		ChinaCorr30: make(map[string][]float64),
		ChinaCorrHS: make(map[string][]float64),
		VixDxyCorr:  vixDxyCorr,
	}

	for _, a := range assetStatuses {
		if a.CorrelationStatus == "global" {
			plotData.Corrs6m[a.Symbol] = []float64{a.Corr6m}
			plotData.Corrs30[a.Symbol] = []float64{a.Corr30d}
		} else {
			plotData.ChinaCorr6m[a.Symbol] = []float64{a.Corr6m}
			plotData.ChinaCorr30[a.Symbol] = []float64{a.Corr30d}
			plotData.ChinaCorrHS[a.Symbol] = []float64{a.HS300Corr}
		}
	}

	generateChart(plotData)
	log.Println("✅ 审计完成")
}

func getLatestMarketPrice(symbol string) float64 {
	if db == nil {
		return 0
	}
	var price float64
	normalizedSymbol := symbol
	if strings.HasSuffix(symbol, ".SS") {
		normalizedSymbol = strings.TrimSuffix(symbol, ".SS")
	} else if strings.HasSuffix(symbol, ".SH") {
		normalizedSymbol = strings.TrimSuffix(symbol, ".SH")
	} else if strings.HasSuffix(symbol, ".SZ") {
		normalizedSymbol = strings.TrimSuffix(symbol, ".SZ")
	}
	err := db.QueryRow(
		"SELECT price FROM market_data WHERE symbol LIKE ? ORDER BY timestamp DESC LIMIT 1",
		normalizedSymbol+"%",
	).Scan(&price)
	if err != nil {
		return 0
	}
	return price
}

func calculateAssetStatusWithConfig(asset AssetConfig, dxyMap map[string]float64, endTime time.Time, assetType string) AssetStatus {
	if strings.HasSuffix(asset.Symbol, ".SS") || strings.HasSuffix(asset.Symbol, ".SZ") {
		assetType = "china"
	} else {
		assetType = "global"
	}

	status := AssetStatus{
		Symbol:            asset.Symbol,
		Name:              asset.Name,
		CorrelationStatus: assetType,
	}

	// Format tags
	if len(asset.Tags) > 0 {
		status.Tags = strings.Join(asset.Tags, " ")
	}

	returns, dates, _ := getReturnsWithRetry(asset.Symbol, endTime)
	if returns == nil || len(returns) == 0 {
		return status
	}

	if len(returns) > 0 {
		status.CurrentPrice = 100 * (1 + returns[len(returns)-1])
		status.LatestReturn = returns[len(returns)-1] * 100
		status.MarketPrice = getLatestMarketPrice(asset.Symbol)
		volume, turnover, err := getLatestVolumeAndTurnover(asset.Symbol)
		if err != nil {
			log.Printf("[WARN] Failed to get volume for %s: %v", asset.Symbol, err)
		} else {
			status.Volume = volume
			status.TurnoverRate = turnover
		}
	}

	// Get sentiment data from news_events
	impactScore, latestNews, err := getLatestSentimentForSymbol(asset.Symbol)
	if err != nil {
		log.Printf("[WARN] Failed to get sentiment for %s: %v", asset.Symbol, err)
	}
	status.ImpactScore = impactScore
	status.LatestNews = latestNews
	if impactScore >= 0.8 {
		status.SentimentScore = impactScore
	}

	var validAsset, validDXY []float64
	for i, date := range dates {
		if _, ok := dxyMap[date]; ok {
			ar := returns[i]
			dr := dxyMap[date]
			if !math.IsNaN(ar) && !math.IsNaN(dr) && !math.IsInf(ar, 0) && !math.IsInf(dr, 0) {
				validAsset = append(validAsset, ar)
				validDXY = append(validDXY, dr)
			}
		}
	}

	if len(validAsset) >= 20 {
		status.Corr6m = stat.Correlation(validAsset, validDXY, nil)
		if len(validAsset) >= 30 {
			status.Corr30d = stat.Correlation(validAsset[len(validAsset)-30:], validDXY[len(validDXY)-30:], nil)
		}
	}

	sigmaLimit, _, _ := globalConfig.GetThresholds()
	if sigmaLimit == 0 {
		sigmaLimit = 3.0
	}

	if len(returns) >= 30 {
		recentReturns := returns[len(returns)-30:]
		sum := 0.0
		for _, r := range recentReturns {
			sum += r
		}
		status.Mean = sum / float64(len(recentReturns))

		variance := 0.0
		for _, r := range recentReturns {
			diff := r - status.Mean
			variance += diff * diff
		}
		status.Sigma = math.Sqrt(variance / float64(len(recentReturns)))

		if len(returns) >= 2 {
			latestReturn := returns[len(returns)-1]
			if status.Sigma > 0 {
				zScore := (latestReturn - status.Mean) / status.Sigma
				// ImpactScore > 0.8 lowers the threshold for critical alerts
				effectiveLimit := sigmaLimit
				if status.ImpactScore >= 0.8 {
					effectiveLimit = sigmaLimit * 0.85 // 15% lower threshold when high impact news
					log.Printf("[%s] High impact news detected (%.2f), lowering sigma threshold to %.2f",
						asset.Symbol, status.ImpactScore, effectiveLimit)
				}
				if math.Abs(zScore) > effectiveLimit && !isSilentPeriod() {
					status.IsCritical = true
					status.AlertMessage = fmt.Sprintf("%.1f-Sigma异动! z=%.2f", effectiveLimit, zScore)
					if status.ImpactScore >= 0.8 {
						status.AlertMessage += fmt.Sprintf(" (新闻加权: %.1f)", status.ImpactScore)
					}
				}
			}
		}
	}

	return status
}

func calculateHS300Corr(symbol string, hs300Map map[string]float64, endTime time.Time) float64 {
	returns, dates, _ := getReturnsWithRetry(symbol, endTime)
	if returns == nil || len(returns) == 0 || len(hs300Map) == 0 {
		return 0
	}

	var validAsset, validHS []float64
	for i, date := range dates {
		if hsVal, ok := hs300Map[date]; ok {
			ar := returns[i]
			if !math.IsNaN(ar) && !math.IsNaN(hsVal) && !math.IsInf(ar, 0) && !math.IsInf(hsVal, 0) {
				validAsset = append(validAsset, ar)
				validHS = append(validHS, hsVal)
			}
		}
	}

	if len(validAsset) >= 20 {
		return stat.Correlation(validAsset, validHS, nil)
	}
	return 0
}

func calculateCorrAcceleration(assets []AssetStatus) map[string]float64 {
	acceleration := make(map[string]float64)
	for _, a := range assets {
		if lastCorr, ok := lastCorrMap[a.Symbol]; ok {
			delta := a.Corr30d - lastCorr
			acceleration[a.Symbol] = delta
		}
		lastCorrMap[a.Symbol] = a.Corr30d
	}
	return acceleration
}

func calculateResonance(assets []AssetStatus) ResonanceStatus {
	// Get Grid-Hard and AIDC assets from config
	gridHardAssets := []string{}
	aidcAssets := []string{}

	_, chinaAssets, _ := globalConfig.GetAssets()
	for _, asset := range chinaAssets {
		for _, tag := range asset.Tags {
			if tag == "Grid-Hard" {
				gridHardAssets = append(gridHardAssets, asset.Symbol)
			}
			if tag == "AIDC-Leader" {
				aidcAssets = append(aidcAssets, asset.Symbol)
			}
		}
	}

	var gridAssets, aidcAssetsStatus []AssetStatus
	for _, a := range assets {
		for _, g := range gridHardAssets {
			if a.Symbol == g {
				gridAssets = append(gridAssets, a)
			}
		}
		for _, a2 := range aidcAssets {
			if a.Symbol == a2 {
				aidcAssetsStatus = append(aidcAssetsStatus, a)
			}
		}
	}

	gridTriggered := 0
	aidcTriggered := 0
	for _, a := range gridAssets {
		if a.IsCritical {
			gridTriggered++
		}
	}
	for _, a := range aidcAssetsStatus {
		if a.IsCritical {
			aidcTriggered++
		}
	}

	syncRate := 0.0
	if len(gridAssets) > 0 && len(aidcAssetsStatus) > 0 {
		totalCorr := 0.0
		count := 0
		for _, g := range gridAssets {
			for _, a := range aidcAssetsStatus {
				totalCorr += math.Abs(g.Corr30d - a.Corr30d)
				count++
			}
		}
		if count > 0 {
			syncRate = 1.0 - (totalCorr / float64(count))
		}
	}

	resonance := ResonanceStatus{
		IsActive:        false,
		SyncRate:        syncRate,
		Confidence:      "低",
		TriggeredAssets: "",
		Message:         "无共振",
	}

	if (gridTriggered > 0 || aidcTriggered > 0) && syncRate < 0.5 {
		resonance.IsActive = true
		resonance.Confidence = "高"
		resonance.SyncRate = syncRate
		resonance.Message = "🔥 核心共振 全产业链造血中"

		triggered := []string{}
		if gridTriggered > 0 {
			triggered = append(triggered, "Grid-Hard")
		}
		if aidcTriggered > 0 {
			triggered = append(triggered, "AIDC-Leader")
		}
		resonance.TriggeredAssets = strings.Join(triggered, " + ")
	}

	return resonance
}

func loadLocationSafe() *time.Location {
	timezone := globalConfig.Runtime.Timezone
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		log.Printf("[WARN] Failed to load timezone %s: %v, falling back to Asia/Shanghai", timezone, err)
		loc, err = time.LoadLocation("Asia/Shanghai")
		if err != nil {
			log.Printf("[ERROR] Failed to load fallback timezone: %v, using UTC", err)
			loc = time.UTC
		}
	}
	return loc
}

func isSilentPeriod() bool {
	now := time.Now()
	loc := loadLocationSafe()
	beijingNow := now.In(loc)

	weekday := beijingNow.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return true
	}

	// Market closed hours (15:00 - next day 09:00)
	hour := beijingNow.Hour()
	if hour >= 15 || hour < 9 {
		return true
	}

	return false
}

func isAuctionTime() bool {
	now := time.Now()
	loc := loadLocationSafe()
	beijingNow := now.In(loc)

	hour := beijingNow.Hour()
	minute := beijingNow.Minute()

	// Pre-market auction: 09:15 - 09:25
	if hour == 9 && minute >= 15 && minute <= 25 {
		return true
	}
	return false
}

func isAuctionSnapshotTime() bool {
	now := time.Now()
	loc := loadLocationSafe()
	beijingNow := now.In(loc)

	hour := beijingNow.Hour()
	minute := beijingNow.Minute()

	// Snapshot at exactly 09:25
	if hour == 9 && minute == 25 {
		return true
	}
	return false
}

func getAverageVolume(symbol string, days int) (float64, error) {
	if db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	since := time.Now().AddDate(0, 0, -days).Format("2006-01-02")

	var avgVolume float64
	err := db.QueryRow(`
		SELECT AVG(volume) FROM market_data 
		WHERE symbol LIKE ? AND timestamp > ?
	`, symbol+"%", since).Scan(&avgVolume)

	if err != nil {
		return 0, err
	}

	return avgVolume, nil
}

func checkAuctionVolumeAlert(asset AssetConfig) (bool, float64, float64) {
	currentVolume, _, err := getLatestVolumeAndTurnover(asset.Symbol)
	if err != nil {
		log.Printf("[WARN] Failed to get volume for auction check %s: %v", asset.Symbol, err)
		return false, 0, 0
	}
	if currentVolume == 0 {
		return false, 0, 0
	}

	avgVolume, err := getAverageVolume(asset.Symbol, 5)
	if err != nil || avgVolume == 0 {
		return false, currentVolume, 0
	}

	multiplier := globalConfig.Thresholds.AuctionVolumeMultiplier
	if multiplier == 0 {
		multiplier = 2.0
	}

	isAnomaly := currentVolume > avgVolume*multiplier
	return isAnomaly, currentVolume, currentVolume / avgVolume
}

func triggerCapitalAlert(asset AssetConfig, volume float64, ratio float64) {
	message := fmt.Sprintf("🔥 换血资金进场! %s 集合竞价成交量 %.0f (%.1fx 5日均值)",
		asset.Symbol, volume, ratio)

	log.Println("[ALERT] " + message)

	// Update global status with mutex protection
	statusMutex.Lock()
	globalStatus.SilentPeriod = false
	globalStatus.CapitalAlert = true
	globalStatus.CapitalAlertMessage = message
	globalStatus.CapitalAlertTime = time.Now()
	statusMutex.Unlock()

	// Send email if configured
	if smtpUser != "" && smtpPass != "" {
		subject := "[特级告警] IronCore 检测到换血资金进场"
		body := fmt.Sprintf("--- IronCore 特级告警 ---\n\n%s\n\n时间: %s\n",
			message, time.Now().Format("2006-01-02 15:04:05"))
		sendEmail(subject, body)
	}
}

func checkAndSendAlert(vixWarning bool, assets []AssetStatus) {
	if isSilentPeriod() {
		log.Println("🔇 静默期，跳过报警")
		return
	}

	shouldAlert := vixWarning

	for _, a := range assets {
		if a.IsCritical {
			shouldAlert = true
			break
		}
	}

	if shouldAlert && smtpUser != "" && smtpPass != "" {
		sendAlertEmail(vixWarning, assets)
	}
}

func sendAlertEmail(vixWarning bool, assets []AssetStatus) {
	subject := "[紧急预警] IronCore 检测到市场异动"
	body := "--- IronCore 紧急预警 ---\n\n"

	if vixWarning {
		body += "🚨 VIX-DXY 强正相关共振！市场进入非理性抽血模式。\n\n"
	}

	body += "异动标的:\n"
	for _, a := range assets {
		if a.IsCritical {
			body += fmt.Sprintf("  %s: %s (最新收益: %.2f%%)\n", a.Symbol, a.AlertMessage, a.LatestReturn*100)
		}
	}

	body += fmt.Sprintf("\n时间: %s\n", time.Now().Format("2006-01-02 15:04:05"))

	sendEmail(subject, body)
	globalStatus.LastAlertTime = time.Now()
	log.Println("📧 预警邮件已发送")
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.New("dashboard").Parse(dashboardHTML)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tmpl.Execute(w, globalStatus)
}

func handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(globalStatus)
}

func handleTriggerAudit(w http.ResponseWriter, r *http.Request) {
	go performAudit(time.Now())
	w.Write([]byte(`{"status":"triggered"}`))
}

func handleReloadConfig(w http.ResponseWriter, r *http.Request) {
	if err := globalConfig.ReloadConfig(); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Configuration reloaded",
	})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.New("login").Parse(loginHTML)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	tmpl.Execute(w, map[string]string{})
}

func handleAuth(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	username := r.Form.Get("username")
	password := r.Form.Get("password")

	if username == AdminUser && password == AdminPass {
		signature := signCookie(username)
		cookie := &http.Cookie{
			Name:     "ironcore_session",
			Value:    username + "|" + signature,
			Path:     "/",
			HttpOnly: true,
			MaxAge:   86400 * 7,
		}
		http.SetCookie(w, cookie)
		http.Redirect(w, r, "/", http.StatusFound)
	} else {
		tmpl, _ := template.New("login").Parse(loginHTML)
		tmpl.Execute(w, map[string]string{"Error": "凭证无效，请重试"})
	}
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie := &http.Cookie{
		Name:   "ironcore_session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	}
	http.SetCookie(w, cookie)
	http.Redirect(w, r, "/login", http.StatusFound)
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" || r.URL.Path == "/auth" {
			next(w, r)
			return
		}

		cookie, err := r.Cookie("ironcore_session")
		if err != nil || cookie == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		parts := strings.Split(cookie.Value, "|")
		if len(parts) != 2 {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		expectedSig := signCookie(parts[0])
		if parts[1] != expectedSig {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		next(w, r)
	}
}

func signCookie(value string) string {
	h := hmac.New(sha256.New, []byte(SessionSecret))
	h.Write([]byte(value))
	return base64.URLEncoding.EncodeToString(h.Sum(nil))
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
