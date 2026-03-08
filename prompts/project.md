[TODO]

[WIP]

[READY]

[DONE]

1. 解耦硬编码，建立动态配置中心 (Config Engine)
   - 重构 IronCore 的资产观察列表
   - 动态化：将 collector.py 和 main.go 中硬编码的 Ticker 列表（如 SRVR, 600406.SS 等）全部迁移到外部 config.yaml 或 assets.json 文件中
   - 分组管理：支持按分类配置，如 Global_Macro (Yahoo), China_Power_Grid (efinance), Sentinel_Keywords (News) 等
   - 热加载：Go 服务启动时读取该配置，并提供一个内部函数支持在不重启服务的情况下刷新观察名单
   - 状态：✅ 已完成 (配置系统已存在，已验证完整性)

2. 集成 Sentinel 哨兵模块与 AI 审计逻辑 (Sentinel Engine)
   - 新增 sentinel.py 独立采集模块
   - 新闻抓取：对接 NewsAPI 或 GNews，根据配置中的关键词（如：Hormuz, Ga2O3, Transformer Shortage）抓取全球主流媒体标题
   - AI 评分逻辑：对抓取的标题调用 LLM API，生成 0.0-1.0 的 ImpactScore
   - 数据交互：将评分结果存入 ironcore.db 的新表 news_events，并与相关资产的 Ticker 建立关联
   - 联动审计：修改 Go 引擎的 runAuditLoop，将 ImpactScore > 0.8 作为触发 3-Sigma 告警的加权因子
   - 状态：✅ 已完成

3. 修改 isSilentPeriod 逻辑及采集频率 (Auction Mode)
   - 取消静默：移除 9:00-9:30 的完全静默，将其定义为 High_Frequency_Auction_Mode
   - 竞价侦测：在 09:25 采集一次关键标的（如 159326.SZ）的集合竞价成交量
   - 异常触发：如果 09:25 的 Volume > 过去 5 日均值的 2 倍，立即在 Web Dashboard 标记"🔥 换血资金进场"，并发送特级告警
   - 状态：✅ 已完成

4. 数据库 Schema 升级与 Go 结构体对齐 (Data Schema)
   - SQLite 更新：
     - market_data 增加 turnover_rate（换手率）字段
     - 新建 news_events 表：timestamp, symbol, title, impact_score, sentiment, logic_summary
   - Go Struct 升级：在 AssetStatus 中增加 SentimentScore (float64) 和 LatestNews (string) 字段
   - 持久化逻辑：确保 collector.py 在存入价格的同时，能通过 API 同步 sentinel.py 的最新审计结论
   - 状态：✅ 已完成

5. 可视化仪表盘增强 (UI/UX Optimization)
   - 升级 plotter.py 和 Web 界面
   - 标注新闻事件：在相关性趋势图或价格线上，用小圆点标注 ImpactScore > 0.8 的新闻发生点，实现"图文合一"
   - 实时状态面板：在 /dashboard 增加一个"地缘政治风险灯"，根据 Sentinel 的平均评分显示：Green (Calm), Yellow (Tension), Red (Crisis)
   - 操作建议输出：根据 3-Sigma 异动 + 产业链共振 + AI 审计结果，在 API 返回值中生成一段人类可读的 TacticalAdvice（例如："检测到物理基建共振且地缘评分高，建议加仓 159326"）
   - 状态：✅ 已完成
