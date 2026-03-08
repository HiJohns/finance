# Project Management

## [TODO]

- **任务**: 集成 Sentinel 哨兵模块与 AI 审计逻辑 (Sentinel Engine)
  - **描述**: 新增 sentinel.py 独立采集模块
  - **详细需求**:
    1. **新闻抓取**: 对接 NewsAPI 或 GNews，根据配置中的关键词（如：Hormuz, Ga2O3, Transformer Shortage）抓取全球主流媒体标题
    2. **AI 评分逻辑**: 对抓取的标题调用 LLM API，生成 0.0-1.0 的 ImpactScore
    3. **数据交互**: 将评分结果存入 ironcore.db 的新表 news_events，并与相关资产的 Ticker 建立关联
    4. **联动审计**: 修改 Go 引擎的 runAuditLoop，将 ImpactScore > 0.8 作为触发 3-Sigma 告警的加权因子
  - **依赖**: 需要先完成 Data Schema 任务

- **任务**: 重构静默期，新增"开盘竞价审计"模式 (Auction Monitor)
  - **描述**: 修改 isSilentPeriod 逻辑及采集频率
  - **详细需求**:
    1. **取消静默**: 移除 9:00-9:30 的完全静默，将其定义为 High_Frequency_Auction_Mode
    2. **竞价侦测**: 在 09:25 采集一次关键标的（如 159326.SZ）的集合竞价成交量
    3. **异常触发**: 如果 09:25 的 Volume > 过去 5 日均值的 2 倍，立即在 Web Dashboard 标记"🔥 换血资金进场"，并发送特级告警

- **任务**: 可视化仪表盘增强 (UI/UX Optimization)
  - **描述**: 升级 plotter.py 和 Web 界面
  - **详细需求**:
    1. **标注新闻事件**: 在相关性趋势图或价格线上，用小圆点标注 ImpactScore > 0.8 的新闻发生点，实现"图文合一"
    2. **实时状态面板**: 在 /dashboard 增加一个"地缘政治风险灯"，根据 Sentinel 的平均评分显示：Green (Calm), Yellow (Tension), Red (Crisis)
    3. **操作建议输出**: 根据 3-Sigma 异动 + 产业链共振 + AI 审计结果，在 API 返回值中生成一段人类可读的 TacticalAdvice（例如："检测到物理基建共振且地缘评分高，建议加仓 159326"）

## [WIP]

- **任务**: 数据库 Schema 升级与 Go 结构体对齐 (Data Schema)
  - **描述**: 更新存储与内存模型
  - **状态**: 待执行（用户离开，暂停状态）
  - **开始时间**: 2026-03-08
  - **详细需求**:
    1. **SQLite 更新**:
       - market_data 增加 turnover_rate（换手率）字段
       - 新建 news_events 表：timestamp, symbol, title, impact_score, sentiment, logic_summary
    2. **Go Struct 升级**: 在 AssetStatus 中增加 SentimentScore (float64) 和 LatestNews (string) 字段
    3. **持久化逻辑**: 确保 collector.py 在存入价格的同时，能通过 API 同步 sentinel.py 的最新审计结论
  - **审计建议**:
    - 执行前需要创建 ironcore.db.bak 备份
    - 使用"新建表+数据复制"策略进行 Schema 迁移

## [READY]

- **任务**: 重构 Iron Core 的资产观察列表 (Config Engine)
  - **状态**: 已完成
  - **完成时间**: 2026-03-08
  - **Patch**: `review_Config_Engine.patch`
  - **完成内容**:
    1. ✅ 创建 config.yaml 配置文件
    2. ✅ 重构 main.go 支持动态配置和热重载
    3. ✅ 重构 collector.py 从配置读取资产列表
    4. ✅ 支持分组管理：Global_Macro, China_Power_Grid, Benchmarks
    5. ✅ 实现热加载 API: POST /api/reload-config
    6. ✅ 动态阈值配置支持

## [执行状态摘要]

**当前状态**: 用户离开，任务暂停
**最后更新时间**: 2026-03-08
**已完成**: 1/5 任务
**待执行**: 4/5 任务

**依赖链**:
1. Config Engine ✅ (已完成)
2. Data Schema ⏸️ (暂停中，用户离开后恢复)
3. Sentinel Engine ⏳ (依赖 Data Schema)
4. Auction Monitor ⏳
5. UI/UX Optimization ⏳

**生成的 Patch 文件**:
- sentinel/ironcore/review_Config_Engine.patch

**待生成 Patch 文件**:
- review_Data_Schema.patch
- review_Sentinel_Engine.patch
- review_Auction_Monitor.patch
- review_UI_UX_Optimization.patch

**注意事项**:
- 用户返回后，请从 Data Schema 任务继续执行
- 需要先执行数据库备份脚本
- Sentinel Engine 任务依赖 Data Schema 完成

---
**会话恢复提示**: 用户返回后，输入 "proceed" 继续执行当前 WIP 任务
