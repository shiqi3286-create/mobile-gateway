// Package stats 提供网关数据统计模块。
//
// 职责：
//   - 聚合统计：请求总数、成功率、平均延迟、流量（上行/下行）
//   - Token 统计：输入/输出 token、费用预估、按平台（OpenAI/Anthropic/Gemini）拆分
//   - 趋势数据：1h / 24h / 7d 三种粒度的请求量时间序列
//   - 排行榜：TOP API（请求数、成功率、平均耗时、累计流量）
//   - 错误率分布：正常 / 警告 / 错误 三类占比
//
// 设计说明：
//   - 所有计数在内存中原子累加（高并发安全），并周期性快照到磁盘（每日一份），
//     网关重启后可通过快照恢复"今日/本月"累计值。
//   - 数据源：路由转发模块（proxy）、聚合编排模块（aggregate）、协议转换模块（protocol）
//     在处理完每个请求后会调用本模块的 RecordXxx 方法上报一条记录。
//
// 字段命名与前端 UI 完全一致（见 E:\API聚合\web\desktop\mobile-gateway-admin.html 与
// E:\API聚合\web\mobile-preview\gateway-prototype.html），例如 today.total / input / output /
// by_provider / real_time.requests 等，前端无需改名即可直接渲染。
package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------- 对外上报使用的结构 ----------

// Usage 单次请求的统计信息，由 proxy / aggregate 上报。
// 说明：
//   - Path 是实际命中的路径（聚合时填聚合路径，如 /api/user/order）
//   - Provider 是平台标识：openai / anthropic / gemini / custom
//   - Status 为 0 表示网关内部错误（未得到上游响应）
//   - TrafficUp / TrafficDown 单位：字节
type Usage struct {
	Path        string // 请求路径（命中规则）
	Method      string // 请求方法 GET/POST/...
	Provider    string // 平台标识
	Status      int    // HTTP 状态码
	LatencyMs   int64  // 总耗时（毫秒）
	TokenIn     int64  // 输入 token 数
	TokenOut    int64  // 输出 token 数
	Cost        float64 // 本次预估费用（美元）
	TrafficUp   int64  // 上行字节数
	TrafficDown int64  // 下行字节数
	IsError     bool   // 是否计入"错误"（5xx 或内部错误）
	IsWarn      bool   // 是否计入"警告"（4xx）
	Time        time.Time // 发生时间（默认取当前时间）
}

// ---------- 返回给前端的数据结构 ----------

// Stats 是 GET /api/stats 的 data 字段。
// 所有 JSON 标签即前端字段名，与 UI 一一对应。
type Stats struct {
	Gateway    Gateway     `json:"gateway"`     // 网关状态卡
	Mode       string      `json:"mode"`        // hotspot | lan | ble
	PhoneIP    string      `json:"phone_ip"`    // 手机 IP
	ListenPort int         `json:"listen_port"` // 监听端口
	HotspotIP  string      `json:"hotspot_ip"`  // 电脑热点 IP
	Devices    int         `json:"devices"`     // 连接设备数
	CellularOff bool       `json:"cellular_off"`// 移动数据是否已关闭
	WifiSignal int         `json:"wifi_signal"` // Wi-Fi 信号 dBm
	WifiSignalText string  `json:"wifi_signal_text"` // 强/中/弱
	Token      TokenStats  `json:"token"`       // Token 消耗统计 + 费用预估
	RealTime   RealTime    `json:"real_time"`   // 实时统计
	Trend      Trend       `json:"trend"`       // 请求量趋势
	RecentLogs []LogEntry  `json:"recent_logs"` // 最近日志（仪表盘）
	AggStats   []AggStat   `json:"agg_stats"`   // 各聚合 API 耗时/成功率
	ErrorDist  ErrorDist   `json:"error_dist"`  // 错误率分布（统计页）
	Traffic    Traffic     `json:"traffic"`     // 流量统计（统计页）
	AggLatency []AggLatency `json:"agg_latency"`// 聚合链路耗时（统计页）
	TopAPIs    []TopAPI    `json:"top_apis"`    // TOP API 排行榜（统计页）
}

// Gateway 网关状态卡（仪表盘第 2 排第 1 张卡）
type Gateway struct {
	Running   bool   `json:"running"`    // 是否运行中
	PID       int    `json:"pid"`        // 进程 PID
	Uptime    string `json:"uptime"`     // 人类可读运行时长，如 "06:23:41"
	UptimeSec int64  `json:"uptime_sec"` // 运行秒数
}

// TokenStats Token 消耗统计 + 费用预估（仪表盘顶部两张卡 + 手机首页横滑卡）
type TokenStats struct {
	Today        TodayTokens   `json:"today"`
	ByProvider   []ProviderPct `json:"by_provider"`     // 平台 Token 占比（进度条）
	ByProviderCost []ProviderCost `json:"by_provider_cost"` // 平台费用拆分
}

// TodayTokens "今日" Token 与费用
type TodayTokens struct {
	Total        int64   `json:"total"`         // 总消耗 tokens
	Input        int64   `json:"input"`         // 输入 tokens
	Output       int64   `json:"output"`        // 输出 tokens
	YesterdayDeltaPct float64 `json:"yesterday_delta_pct"` // 较昨日变化百分比
	Cost         float64 `json:"cost"`          // 今日费用（美元）
	MonthTotal   float64 `json:"month_total"`   // 本月累计费用
	MonthBudget  float64 `json:"month_budget"`  // 月预算
	BudgetUsedPct float64 `json:"budget_used_pct"` // 预算已用百分比
}

// ProviderPct 平台 Token 占比
type ProviderPct struct {
	Provider string  `json:"provider"` // OpenAI / Anthropic / Gemini
	Pct      float64 `json:"pct"`      // 百分比（如 62）
}

// ProviderCost 平台费用
type ProviderCost struct {
	Provider string  `json:"provider"`
	Cost     float64 `json:"cost"`
}

// RealTime 实时统计 · 今日（仪表盘中部 + 手机首页三卡）
type RealTime struct {
	Requests     int64   `json:"requests"`      // 请求数
	SuccessRate  float64 `json:"success_rate"`  // 成功率（百分比）
	AvgLatencyMs float64 `json:"avg_latency_ms"`// 平均延迟（毫秒）
	TrafficMB    float64 `json:"traffic_mb"`    // 流量（MB）
}

// Trend 请求量趋势（仪表盘 SVG + 手机 spark + 统计页大图）
type Trend struct {
	Range  string        `json:"range"`  // 1h | 24h | 7d
	Points []TrendPoint  `json:"points"` // 时间序列
}

// TrendPoint 一个时间点
type TrendPoint struct {
	Ts    string `json:"ts"`    // 标签，如 "10:00" / "09-11"
	Count int64  `json:"count"` // 该时段请求数
}

// LogEntry 最近日志行（仪表盘"最近日志"）
type LogEntry struct {
	Time    string `json:"time"`    // "10:00:00"
	Level   string `json:"level"`   // INFO | WARN | ERROR
	Channel string `json:"channel"` // http | agg | net | hotspot | ble
	Path    string `json:"path"`    // 请求路径
	Status  int    `json:"status"`  // 状态码（可为 0）
	Ms      int64  `json:"ms"`      // 耗时
	Message string `json:"message"` // 消息
}

// AggStat 聚合 API 统计（仪表盘聚合卡片 + 列表）
type AggStat struct {
	Path   string  `json:"path"`   // 聚合路径
	Steps  int     `json:"steps"`  // 步骤数
	Mode   string  `json:"mode"`   // parallel | serial
	Ms     int64   `json:"ms"`     // 最近耗时
	Rate   float64 `json:"rate"`   // 成功率
}

// ErrorDist 错误率分布（统计页）
type ErrorDist struct {
	Ok    float64 `json:"ok"`    // 正常 %
	Warn  float64 `json:"warn"`  // 警告 %
	Error float64 `json:"error"` // 错误 %
}

// Traffic 流量统计（统计页）
type Traffic struct {
	UpMB       float64 `json:"up_mb"`       // 上行 MB
	DownMB     float64 `json:"down_mb"`     // 下行 MB
	UpDeltaPct float64 `json:"up_delta_pct"` // 上行环比 %
	DownDeltaPct float64 `json:"down_delta_pct"` // 下行环比 %
}

// AggLatency 聚合链路耗时（统计页）
type AggLatency struct {
	Path string `json:"path"`
	Ms   int64  `json:"ms"`
}

// TopAPI TOP API 排行榜（统计页表格行）
type TopAPI struct {
	Path       string  `json:"path"`        // 接口
	Requests   int64   `json:"requests"`    // 请求数
	SuccessRate float64 `json:"success_rate"` // 成功率
	AvgMs      float64 `json:"avg_ms"`      // 平均耗时
	TrafficMB  float64 `json:"traffic_mb"`  // 累计流量
}

// ---------- 内部状态 ----------

// dayAccum 单日累计（内存 + 磁盘快照）
type dayAccum struct {
	Requests    int64
	Success     int64 // 2xx 或 3xx
	Warn        int64 // 4xx
	Err         int64 // 5xx 或内部错误
	LatencySum  int64 // 毫秒累加
	TokenIn     int64
	TokenOut    int64
	Cost        float64
	TrafficUp   int64
	TrafficDown int64
	// 按平台拆分
	ByProviderToken map[string]int64   // provider -> token 合计
	ByProviderCost  map[string]float64 // provider -> 费用
	// 按路径拆分（排行榜 / 聚合耗时）
	ByPath map[string]*pathAccum
}

// pathAccum 单路径累计
type pathAccum struct {
	Requests   int64
	Success    int64
	LatencySum int64
	Traffic    int64
	IsAgg      bool // 聚合路径
	StepCount  int  // 聚合步骤数
	Mode       string
}

// hourlyBucket 趋势用的环形桶（保留 7 天 * 24 小时）
type hourlyBucket struct {
	Hour  int64 // 时间戳（整点，Unix）
	Count int64 // 请求数
}

// Collector 统计收集器（线程安全）
type Collector struct {
	mu        sync.RWMutex
	start     time.Time // 本次进程启动时间（算 uptime）
	pid       int
	today     dayAccum
	monthCost float64 // 本月累计费用（含历史快照）
	buckets   []hourlyBucket // 环形桶，按整点索引
	snapshotDir string  // 快照目录（如 /data/data/<pkg>/files/stats）
	lastReset time.Time
}

// ---------- 构造 ----------

// NewCollector 创建统计收集器。
// pid 为当前进程 PID；snapshotDir 为快照目录（不存在会自动创建）。
func NewCollector(pid int, snapshotDir string) *Collector {
	c := &Collector{
		start:       time.Now(),
		pid:         pid,
		monthCost:   0,
		buckets:     make([]hourlyBucket, 0, 24*7),
		snapshotDir: snapshotDir,
		lastReset:   time.Now().Truncate(time.Hour),
	}
	c.today.ByProviderToken = map[string]int64{}
	c.today.ByProviderCost = map[string]float64{}
	c.today.ByPath = map[string]*pathAccum{}
	c.loadSnapshot() // 启动时尝试恢复快照
	return c
}

// ---------- 上报入口 ----------

// Record 记录一次完整请求的统计（由 proxy/aggregate 在请求结束后调用）。
func (c *Collector) Record(u Usage) {
	if u.Time.IsZero() {
		u.Time = time.Now()
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	d := &c.today
	d.Requests++
	d.LatencySum += u.LatencyMs
	d.TokenIn += u.TokenIn
	d.TokenOut += u.TokenOut
	d.Cost += u.Cost
	d.TrafficUp += u.TrafficUp
	d.TrafficDown += u.TrafficDown

	switch {
	case u.IsError || (u.Status >= 500 && u.Status != 0):
		d.Err++
	case u.IsWarn || (u.Status >= 400 && u.Status < 500):
		d.Warn++
	default:
		d.Success++
	}

	// 平台拆分
	prov := u.Provider
	if prov == "" {
		prov = "custom"
	}
	d.ByProviderToken[prov] += u.TokenIn + u.TokenOut
	d.ByProviderCost[prov] += u.Cost

	// 路径拆分（排行榜 / 聚合耗时）
	p := u.Path
	if p == "" {
		p = "/"
	}
	pa, ok := d.ByPath[p]
	if !ok {
		pa = &pathAccum{}
		d.ByPath[p] = pa
	}
	pa.Requests++
	if u.Status >= 200 && u.Status < 400 {
		pa.Success++
	}
	pa.LatencySum += u.LatencyMs
	pa.Traffic += u.TrafficUp + u.TrafficDown

	// 趋势桶（整点）
	hour := u.Time.Truncate(time.Hour).Unix()
	c.upsertBucket(hour)
}

// RecordAggStat 记录聚合接口的统计（含步骤数/模式），供仪表盘 agg_stats 与统计页 agg_latency 使用。
func (c *Collector) RecordAggStat(path string, steps int, mode string, latencyMs int64, success bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := &c.today
	pa, ok := d.ByPath[path]
	if !ok {
		pa = &pathAccum{}
		d.ByPath[path] = pa
	}
	pa.IsAgg = true
	pa.StepCount = steps
	pa.Mode = mode
	pa.Requests++
	pa.LatencySum += latencyMs
	if success {
		pa.Success++
	}
}

// RecordGatewayEvent 记录网关自身事件（连接/断开/热点切换等），写入日志通道，不计入统计。
// 此处仅占位：实际日志记录在 logs 包实现。
func (c *Collector) RecordGatewayEvent(level, channel, msg string) {}

// upsertBucket 向趋势环形桶写入/更新一个整点计数。
// 调用方必须已持有锁。
func (c *Collector) upsertBucket(hour int64) {
	for i := range c.buckets {
		if c.buckets[i].Hour == hour {
			c.buckets[i].Count++
			return
		}
	}
	// 清理 7 天前的数据
	cutoff := time.Now().AddDate(0, 0, -7).Truncate(time.Hour).Unix()
	fresh := c.buckets[:0]
	for _, b := range c.buckets {
		if b.Hour >= cutoff {
			fresh = append(fresh, b)
		}
	}
	fresh = append(fresh, hourlyBucket{Hour: hour, Count: 1})
	c.buckets = fresh
}

// ---------- 查询 ----------

// Snapshot 生成当前统计快照（供 GET /api/stats 使用）。
// range 取值：1h / 24h / 7d，决定趋势点的粒度与个数。
func (c *Collector) Snapshot(rangeStr string) *Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	// 今日累计（从今日零点累计，若跨天则按今天的桶重建，见 resetIfNeeded）
	c.maybeResetDayLocked(now)

	d := &c.today
	s := &Stats{}
	s.Gateway.Running = true
	s.Gateway.PID = c.pid
	up := now.Sub(c.start)
	s.Gateway.UptimeSec = int64(up.Seconds())
	s.Gateway.Uptime = formatUptime(s.Gateway.UptimeSec)

	// Token 统计
	totalToken := d.TokenIn + d.TokenOut
	td := TodayTokens{
		Total:            totalToken,
		Input:            d.TokenIn,
		Output:           d.TokenOut,
		YesterdayDeltaPct: c.computeYesterdayDeltaLocked(),
		Cost:             round2(d.Cost),
		MonthTotal:       round2(c.monthCost + d.Cost),
		MonthBudget:      200.0, // 默认月预算，可由设置页覆盖
	}
	if td.MonthBudget > 0 {
		td.BudgetUsedPct = round2(td.MonthTotal / td.MonthBudget * 100)
	}
	s.Token.Today = td
	s.Token.ByProvider = c.providerPctLocked(d)
	s.Token.ByProviderCost = c.providerCostLocked(d)

	// 实时统计
	reqs := d.Requests
	successRate := 100.0
	avgMs := 0.0
	if reqs > 0 {
		successRate = round2(float64(d.Success) / float64(reqs) * 100)
		avgMs = round1(float64(d.LatencySum) / float64(reqs))
	}
	s.RealTime = RealTime{
		Requests:     reqs,
		SuccessRate:  successRate,
		AvgLatencyMs: avgMs,
		TrafficMB:    round2(float64(d.TrafficUp+d.TrafficDown) / 1024 / 1024),
	}

	// 趋势
	s.Trend = c.trendLocked(rangeStr, now)

	// 最近日志（由 logs 包注入，此处返回空列表占位）
	s.RecentLogs = []LogEntry{}

	// 聚合统计
	s.AggStats = c.aggStatsLocked(d)

	// 错误率分布
	total := d.Success + d.Warn + d.Err
	if total == 0 {
		s.ErrorDist = ErrorDist{Ok: 100, Warn: 0, Error: 0}
	} else {
		s.ErrorDist = ErrorDist{
			Ok:    round2(float64(d.Success) / float64(total) * 100),
			Warn:  round2(float64(d.Warn) / float64(total) * 100),
			Error: round2(float64(d.Err) / float64(total) * 100),
		}
	}

	// 流量统计（上行/下行 + 环比占位）
	s.Traffic = Traffic{
		UpMB:        round2(float64(d.TrafficUp) / 1024 / 1024),
		DownMB:      round2(float64(d.TrafficDown) / 1024 / 1024),
		UpDeltaPct:  0,
		DownDeltaPct: 0,
	}

	// 聚合链路耗时 + TOP API
	s.AggLatency = c.aggLatencyLocked(d)
	s.TopAPIs = c.topAPIsLocked(d, 10)

	return s
}

// maybeResetDayLocked 若已跨天，将"今日"累计滚动到新的一天。
// 调用方必须已持有读锁（内部会升级为写锁）。
func (c *Collector) maybeResetDayLocked(now time.Time) {
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	if c.lastReset.Before(todayStart) {
		c.mu.RUnlock()
		c.mu.Lock()
		if c.lastReset.Before(todayStart) {
			// 把昨日累计并入本月费用（简化：按日快照持久化在 saveSnapshot 中完成）
			c.monthCost += c.today.Cost
			c.today = dayAccum{
				ByProviderToken: map[string]int64{},
				ByProviderCost:  map[string]float64{},
				ByPath:          map[string]*pathAccum{},
			}
			c.lastReset = todayStart
		}
		c.mu.Unlock()
		c.mu.RLock()
	}
}

// providerPctLocked 计算平台 Token 占比（按百分比，保留 1 位小数）
func (c *Collector) providerPctLocked(d *dayAccum) []ProviderPct {
	total := int64(0)
	for _, v := range d.ByProviderToken {
		total += v
	}
	order := []string{"openai", "anthropic", "gemini"}
	names := map[string]string{
		"openai": "OpenAI", "anthropic": "Anthropic", "gemini": "Gemini",
	}
	out := make([]ProviderPct, 0, len(order))
	for _, k := range order {
		v := d.ByProviderToken[k]
		if v == 0 {
			continue
		}
		pct := 0.0
		if total > 0 {
			pct = round1(float64(v) / float64(total) * 100)
		}
		out = append(out, ProviderPct{Provider: names[k], Pct: pct})
	}
	return out
}

// providerCostLocked 平台费用拆分
func (c *Collector) providerCostLocked(d *dayAccum) []ProviderCost {
	order := []string{"openai", "anthropic", "gemini"}
	names := map[string]string{
		"openai": "OpenAI", "anthropic": "Anthropic", "gemini": "Gemini",
	}
	out := make([]ProviderCost, 0, len(order))
	for _, k := range order {
		v := d.ByProviderCost[k]
		if v == 0 {
			continue
		}
		out = append(out, ProviderCost{Provider: names[k], Cost: round2(v)})
	}
	return out
}

// trendLocked 生成趋势点。
// range=1h → 每分钟 1 点，最近 60 点；24h → 每小时 1 点，最近 24 点；7d → 每天 1 点，最近 7 点。
func (c *Collector) trendLocked(rangeStr string, now time.Time) Trend {
	tr := Trend{Range: rangeStr}
	perHour := make(map[int64]int64)
	for _, b := range c.buckets {
		perHour[b.Hour] = b.Count
	}
	switch rangeStr {
	case "1h":
		// 最近 60 分钟，每分钟一点
		start := now.Truncate(time.Hour)
		for i := 59; i >= 0; i-- {
			ts := start.Add(-time.Duration(i) * time.Minute)
			hour := ts.Truncate(time.Hour).Unix()
			// 按分钟拆分（简化：用小时计数近似）
			cnt := perHour[hour] / 60
			tr.Points = append(tr.Points, TrendPoint{Ts: ts.Format("15:04"), Count: cnt})
		}
	case "24h":
		start := now.Truncate(time.Hour)
		for i := 23; i >= 0; i-- {
			ts := start.Add(-time.Duration(i) * time.Hour)
			tr.Points = append(tr.Points, TrendPoint{Ts: ts.Format("15:04"), Count: perHour[ts.Truncate(time.Hour).Unix()]})
		}
	default: // 7d
		start := now.Truncate(24 * time.Hour)
		for i := 6; i >= 0; i-- {
			ts := start.AddDate(0, 0, -i)
			cnt := int64(0)
			for h := 0; h < 24; h++ {
				hour := time.Date(ts.Year(), ts.Month(), ts.Day(), h, 0, 0, 0, time.Local)
				cnt += perHour[hour.Unix()]
			}
			tr.Points = append(tr.Points, TrendPoint{Ts: ts.Format("01-02"), Count: cnt})
		}
	}
	return tr
}

// aggStatsLocked 仪表盘聚合卡片数据
func (c *Collector) aggStatsLocked(d *dayAccum) []AggStat {
	out := make([]AggStat, 0, 8)
	for path, pa := range d.ByPath {
		if !pa.IsAgg {
			continue
		}
		rate := 0.0
		if pa.Requests > 0 {
			rate = round2(float64(pa.Success) / float64(pa.Requests) * 100)
		}
		out = append(out, AggStat{
			Path:  path,
			Steps: pa.StepCount,
			Mode:  pa.Mode,
			Ms:    c.avgMsLocked(pa),
			Rate:  rate,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ms < out[j].Ms })
	return out
}

// aggLatencyLocked 统计页聚合链路耗时
func (c *Collector) aggLatencyLocked(d *dayAccum) []AggLatency {
	out := make([]AggLatency, 0, 8)
	for path, pa := range d.ByPath {
		if !pa.IsAgg {
			continue
		}
		out = append(out, AggLatency{Path: path, Ms: c.avgMsLocked(pa)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ms > out[j].Ms })
	return out
}

// topAPIsLocked TOP API 排行榜
func (c *Collector) topAPIsLocked(d *dayAccum, n int) []TopAPI {
	out := make([]TopAPI, 0, n)
	for path, pa := range d.ByPath {
		rate := 0.0
		if pa.Requests > 0 {
			rate = round2(float64(pa.Success) / float64(pa.Requests) * 100)
		}
		out = append(out, TopAPI{
			Path:        path,
			Requests:    pa.Requests,
			SuccessRate: rate,
			AvgMs:       float64(c.avgMsLocked(pa)),
			TrafficMB:   round2(float64(pa.Traffic) / 1024 / 1024),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Requests > out[j].Requests })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// avgMsLocked 单路径平均耗时
func (c *Collector) avgMsLocked(pa *pathAccum) int64 {
	if pa.Requests == 0 {
		return 0
	}
	return pa.LatencySum / pa.Requests
}

// computeYesterdayDeltaLocked 较昨日变化百分比（占位实现：从快照读取昨日总数）。
// 简化处理：返回 0，由快照模块增强。
func (c *Collector) computeYesterdayDeltaLocked() float64 {
	return 0
}

// ---------- 快照持久化 ----------

// snapshotFile 今日快照文件名（按日期）
func (c *Collector) snapshotFile() string {
	name := "stats-" + time.Now().Format("2006-01-02") + ".json"
	return filepath.Join(c.snapshotDir, name)
}

// saveSnapshot 将"今日"累计写入磁盘（应每小时调用一次，由后台 goroutine 驱动）。
func (c *Collector) saveSnapshot() error {
	c.mu.RLock()
	d := c.today
	c.mu.RUnlock()

	if err := os.MkdirAll(c.snapshotDir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.snapshotFile(), data, 0o644)
}

// loadSnapshot 启动时恢复最近一次快照（仅当快照日期是今天）。
func (c *Collector) loadSnapshot() {
	data, err := os.ReadFile(c.snapshotFile())
	if err != nil {
		return
	}
	var d dayAccum
	if err := json.Unmarshal(data, &d); err != nil {
		return
	}
	if d.ByProviderToken == nil {
		d.ByProviderToken = map[string]int64{}
	}
	if d.ByProviderCost == nil {
		d.ByProviderCost = map[string]float64{}
	}
	if d.ByPath == nil {
		d.ByPath = map[string]*pathAccum{}
	}
	c.mu.Lock()
	c.today = d
	c.lastReset = time.Now().Truncate(24 * time.Hour)
	c.mu.Unlock()
}

// RunSnapshotLoop 后台快照循环：每小时保存一次今日累计。
func (c *Collector) RunSnapshotLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			_ = c.saveSnapshot()
		}
	}
}

// ---------- 工具函数 ----------

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }
func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

// formatUptime 将秒数格式化为 HH:MM:SS。
func formatUptime(sec int64) string {
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	return strings.Join([]string{
		pad(h), pad(m), pad(s),
	}, ":")
}

func pad(n int64) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
