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
	"time"

	_ "github.com/mattn/go-sqlite3"
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
	CurrentPrice      float64 `json:"current_price"`
	Volume            float64 `json:"volume"`
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
}

type AuditStatus struct {
	Timestamp        time.Time          `json:"timestamp"`
	Assets           []AssetStatus      `json:"assets"`
	VixDxyCorr       float64            `json:"vix_dxy_corr"`
	VixWarning       bool               `json:"vix_warning"`
	SilentPeriod     bool               `json:"silent_period"`
	LastAlertTime    time.Time          `json:"last_alert_time"`
	CorrAcceleration map[string]float64 `json:"corr_acceleration"`
}

var (
	globalStatus AuditStatus
	lastCorrMap  map[string]float64
	db           *sql.DB
)

var dashboardHTML = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>IronCore 实时审计仪表盘</title>
    <meta http-equiv="refresh" content="30">
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 20px; background: #1a1a2e; color: #eee; }
        h1 { color: #00d4ff; }
        .status-bar { padding: 15px; margin: 10px 0; border-radius: 8px; }
        .normal { background: #0f3460; }
        .warning { background: #53354a; }
        .critical { background: #903749; animation: pulse 1s infinite; }
        @keyframes pulse { 0% { opacity: 1; } 50% { opacity: 0.7; } 100% { opacity: 1; } }
        table { width: 100%; border-collapse: collapse; margin-top: 20px; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #333; }
        th { background: #16213e; color: #00d4ff; }
        .alert { color: #ff6b6b; font-weight: bold; }
        .safe { color: #51cf66; }
        .section { margin-top: 30px; }
        .timestamp { color: #888; font-size: 0.9em; }
    </style>
</head>
<body>
    <h1>⚡ IronCore 实时资产异动审计</h1>
    <div style="text-align: right; margin-bottom: 10px;">
        <a href="/logout" style="color: #888; text-decoration: none;">[退出登录]</a>
    </div>
    <div class="status-bar {{if .SilentPeriod}}warning{{else}}normal{{end}}">
        <strong>状态:</strong> {{if .SilentPeriod}}🔇 静默期 (开盘前30分钟){{else}}🟢 监控中{{end}} | 
        <strong>VIX-DXY相关:</strong> {{printf "%.4f" .VixDxyCorr}} {{if .VixWarning}}<span class="alert">⚠️ 共振预警</span>{{end}} |
        <span class="timestamp">更新: {{.Timestamp.Format "2006-01-02 15:04:05"}}</span>
    </div>

    <div class="section">
        <h2>📊 全球宏观标的</h2>
        <table>
            <tr><th>标的</th><th>最新价</th><th>收益率</th><th>6月相关</th><th>30日相关</th><th>3-Sigma</th><th>状态</th></tr>
            {{range .Assets}}
            {{if ne .CorrelationStatus "china"}}
            <tr>
                <td><strong>{{.Symbol}}</strong><br><small>{{.Name}}</small></td>
                <td>{{printf "%.2f" .CurrentPrice}}</td>
                <td>{{printf "%.2f" .LatestReturn}}%</td>
                <td>{{printf "%.4f" .Corr6m}}</td>
                <td>{{printf "%.4f" .Corr30d}}</td>
                <td>μ={{printf "%.4f" .Mean}}, σ={{printf "%.4f" .Sigma}}</td>
                <td>{{if .IsCritical}}<span class="alert">🚨 {{.AlertMessage}}</span>{{else}}<span class="safe">🟢 正常</span>{{end}}</td>
            </tr>
            {{end}}
            {{end}}
        </table>
    </div>

    <div class="section">
        <h2>🇨🇳 中国电力枢纽标的</h2>
        <table>
            <tr><th>标的</th><th>最新价</th><th>收益率</th><th>vs DXY</th><th>vs 沪深300</th><th>大盘关联</th><th>状态</th></tr>
            {{range .Assets}}
            {{if eq .CorrelationStatus "china"}}
            <tr>
                <td><strong>{{.Symbol}}</strong><br><small>{{.Name}}</small></td>
                <td>{{printf "%.2f" .CurrentPrice}}</td>
                <td>{{printf "%.2f" .LatestReturn}}%</td>
                <td>{{printf "%.4f" .Corr30d}}</td>
                <td>{{printf "%.4f" .HS300Corr}}</td>
                <td>{{.MarketStatus}}</td>
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
	flag.Parse()

	if *versionFlag {
		fmt.Println("IronCore version:", version)
		os.Exit(0)
	}

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

	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Printf("数据库连接失败: %v", err)
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

	addr := ":" + httpPort
	log.Printf("🚀 IronCore 服务启动: http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func runAuditLoop(baseTime time.Time) {
	ticker := time.NewTicker(10 * time.Minute)
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

	assets := []string{"SRVR", "SLV", "USO", "GLD", "IWY"}
	chinaPowerAssets := []string{"600406.SS", "002028.SZ", "002270.SZ", "688676.SS", "159326.SZ"}
	hs300 := "000300.SS"
	dxy := "DX-Y.NYB"

	dxyReturns, dxyDates, _ := getReturnsWithRetry(dxy, endTime)
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

	vixReturns, vixDates, _ := getReturnsWithRetry("^VIX", endTime)
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

	for _, symbol := range assets {
		status := calculateAssetStatus(symbol, dxyMap, endTime, "global")
		assetStatuses = append(assetStatuses, status)
	}

	for _, symbol := range chinaPowerAssets {
		status := calculateAssetStatus(symbol, dxyMap, endTime, "china")
		if len(hs300Map) > 0 {
			status.HS300Corr = calculateHS300Corr(symbol, hs300Map, endTime)
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

	globalStatus = AuditStatus{
		Timestamp:        time.Now(),
		Assets:           assetStatuses,
		VixDxyCorr:       vixDxyCorr,
		VixWarning:       vixWarning,
		SilentPeriod:     silentPeriod,
		CorrAcceleration: acceleration,
	}

	checkAndSendAlert(vixWarning, assetStatuses)

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

func calculateAssetStatus(symbol string, dxyMap map[string]float64, endTime time.Time, assetType string) AssetStatus {
	if strings.HasSuffix(symbol, ".SS") || strings.HasSuffix(symbol, ".SZ") {
		assetType = "china"
	} else {
		assetType = "global"
	}
	status := AssetStatus{
		Symbol:            symbol,
		CorrelationStatus: assetType,
	}

	nameMap := map[string]string{
		"SRVR":      "全球数据中心REIT",
		"SLV":       "白银ETF",
		"USO":       "原油ETF",
		"GLD":       "黄金ETF",
		"IWY":       "纳斯达克科技ETF",
		"600406.SS": "国电南瑞",
		"002028.SZ": "思源电气",
		"002270.SZ": "华明装备",
		"688676.SS": "金盘科技",
		"159326.SZ": "电网设备ETF",
	}
	status.Name = nameMap[symbol]

	returns, dates, _ := getReturnsWithRetry(symbol, endTime)
	if returns == nil || len(returns) == 0 {
		return status
	}

	if len(returns) > 0 {
		status.CurrentPrice = 100 * (1 + returns[len(returns)-1])
		status.LatestReturn = returns[len(returns)-1] * 100
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
				if math.Abs(zScore) > 3.0 && !isSilentPeriod() {
					status.IsCritical = true
					status.AlertMessage = fmt.Sprintf("3-Sigma异动! z=%.2f", zScore)
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

func isSilentPeriod() bool {
	now := time.Now()
	loc, _ := time.LoadLocation("Asia/Shanghai")
	beijingNow := now.In(loc)

	hour := beijingNow.Hour()
	minute := beijingNow.Minute()

	if hour == 9 && minute < 30 {
		return true
	}
	return false
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
