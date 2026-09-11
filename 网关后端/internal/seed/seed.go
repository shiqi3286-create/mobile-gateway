// Package seed 提供演示数据（demo 数据）。
//
// 用途：
//   - 第一阶段（联调）：让 GET /api/stats 无需真实流量即有数据可看，
//     前端 UI 每个卡片/图表都能渲染出与设计稿一致的效果。
//   - 后续阶段：接入真实 proxy/aggregate 上报后，可通过
//     app.Config().DemoMode == false 关闭本包。
//
// 实现：在 stats.Collector 上回放一批模拟 Usage 记录（跨过去 7 天，
// 与趋势图 7d/24h/1h 对齐），并注入样例路由/聚合/日志到 config.Manager 与 logs.Store。
package seed

import (
	"math/rand"
	"time"

	"mobile-gateway/internal/config"
	"mobile-gateway/internal/logs"
	"mobile-gateway/internal/stats"
)

// Options 演示数据选项
type Options struct {
	Enabled bool // 是否启用（默认 true）
}

// Seed 注入演示数据。
//   - c：stats 收集器（会回放模拟请求记录）
//   - cm：配置管理器（写入示例路由/聚合）
//   - ls：日志存储（写入示例日志）
func Seed(c *stats.Collector, cm *config.Manager, ls *logs.Store) {
	// 1) 回放 7 天模拟请求（覆盖今日/本月 Token、成功率、流量、趋势、TOP API、错误率分布）
	replayStats(c)

	// 2) 注入示例路由（与 PC 端设计稿表格一致）
	seedRoutes(cm)

	// 3) 注入示例聚合（与 PC 端设计稿卡片一致）
	seedAggregates(cm)

	// 4) 注入示例日志（与 PC/手机端日志列表一致）
	seedLogs(ls)
}

// replayStats 按小时回放 7 天模拟流量。
// 生成规律：工作日高峰、周末平缓；今日请求数约 1234、成功率 99.2%、
// Token 输入/输出符合 2:1、费用按平台价格估算。
func replayStats(c *stats.Collector) {
	rng := rand.New(rand.NewSource(42))
	now := time.Now()

	// 平台价格（每 1K token 美元）：输入/输出
	price := map[string][2]float64{
		"openai":    {2.5, 10.0}, // gpt-4o 类
		"anthropic": {3.0, 15.0}, // claude 类
		"gemini":    {0.5, 1.5},  // gemini 类
	}
	paths := []string{"/api/user/order", "/user/*", "/order/*", "/api/health", "/api/checkout", "/api/user/friends"}
	providers := []string{"openai", "anthropic", "gemini"}
	methods := []string{"GET", "POST"}

	// 每天 24 小时，模拟 7 天
	for day := 6; day >= 0; day-- {
		base := now.AddDate(0, 0, -day)
		for h := 0; h < 24; h++ {
			// 请求密度：白天高、夜晚低；工作日高于周末
			density := 1.0
			if h >= 8 && h <= 22 {
				density = 1.0 + 0.8*float64(rng.Intn(10))/10
			} else {
				density = 0.2
			}
			if base.Weekday() == time.Saturday || base.Weekday() == time.Sunday {
				density *= 0.6
			}
			count := int(density * (30 + rng.Intn(40)))
			for i := 0; i < count; i++ {
				path := paths[rng.Intn(len(paths))]
				prov := providers[rng.Intn(len(providers))]
				method := methods[rng.Intn(2)]
				ts := base.Add(time.Duration(h)*time.Hour + time.Duration(rng.Intn(3600))*time.Second)
				if ts.After(now) {
					continue
				}
				// 状态码分布：99% 2xx，0.5% 4xx，0.5% 5xx
				status := 200
				r := rng.Float64()
				if r > 0.995 {
					status = 500
				} else if r > 0.99 {
					status = 404
				}
				tokenIn := int64(300 + rng.Intn(1200))
				tokenOut := int64(100 + rng.Intn(400))
				// 流量：近似 body 字节
				up := int64(200 + rng.Intn(3000))
				down := int64(500 + rng.Intn(6000))
				p := price[prov]
				cost := float64(tokenIn)/1000*p[0] + float64(tokenOut)/1000*p[1]
				c.Record(stats.Usage{
					Path:        path,
					Method:      method,
					Provider:    prov,
					Status:      status,
					LatencyMs:   int64(8 + rng.Intn(140)),
					TokenIn:     tokenIn,
					TokenOut:    tokenOut,
					Cost:        cost,
					TrafficUp:   up,
					TrafficDown: down,
					IsError:     status >= 500,
					IsWarn:      status >= 400 && status < 500,
					Time:        ts,
				})
			}
		}
	}

	// 注入聚合路径统计（agg_stats / agg_latency）
	aggStats := []struct {
		path  string
		steps int
		mode  string
	}{
		{"/api/user/order", 2, "parallel"},
		{"/api/checkout", 3, "serial"},
		{"/api/user/friends", 2, "parallel"},
	}
	for _, a := range aggStats {
		for i := 0; i < 300; i++ {
			ts := now.Add(-time.Duration(rng.Intn(86400)) * time.Second)
			c.RecordAggStat(a.path, a.steps, a.mode, int64(15+rng.Intn(140)), rng.Float64() > 0.02)
			_ = ts
		}
	}
}

// seedRoutes 写入与 PC 端设计稿一致的示例路由。
func seedRoutes(cm *config.Manager) {
	_, _ = cm.AddRoute(config.Route{
		Path:   "/user/*",
		Target: "http://127.0.0.1:9001",
		Proto:  "openai", ProtoName: "OpenAI 兼容",
		Model: "gpt-4o-mini", Force: true,
		Method: "ALL", Timeout: 5000, Retry: 1, QPS: 50, Enabled: true, AdaptOK: true,
		PromptOn: true,
		Prompt:   "你是一个由个人移动聚合网关调度的 AI 助手。\n遵循 JSON 输出约定，仅返回结构化结果。\n不要提及内部网关配置。",
		Cache: true, CacheTTL: 300,
		Keys: []config.APIKey{
			{Key: "sk-live-••••••••••a1B2C3", Weight: 60},
			{Key: "sk-live-••••••••••d4E5F6", Weight: 30},
			{Key: "sk-live-••••••••••g7H8I9", Weight: 10},
		},
	})
	_, _ = cm.AddRoute(config.Route{
		Path: "/order/*", Target: "http://127.0.0.1:9002",
		Proto: "anthropic", ProtoName: "Anthropic",
		Model: "claude-3-5-sonnet", Force: true,
		Method: "GET", Timeout: 3000, Retry: 1, QPS: 50, Enabled: true, AdaptOK: true,
		Cache: false, CacheTTL: 300,
		Keys: []config.APIKey{{Key: "sk-ant-••••••••••", Weight: 100}},
	})
	_, _ = cm.AddRoute(config.Route{
		Path: "/api/health", Target: "http://127.0.0.1:9000",
		Proto: "gemini", ProtoName: "Google Gemini",
		Model: "gemini-1.5-flash", Force: true,
		Method: "GET", Timeout: 1000, Retry: 0, QPS: 100, Enabled: true, AdaptOK: true,
		Keys: []config.APIKey{{Key: "AIza-••••••••••", Weight: 100}},
	})
	_, _ = cm.AddRoute(config.Route{
		Path: "/api/config", Target: "http://127.0.0.1:9003",
		Proto: "custom", ProtoName: "自定义",
		Model: "", Force: false,
		Method: "ALL", Timeout: 5000, Retry: 1, QPS: 50, Enabled: false, AdaptOK: false,
		Keys: []config.APIKey{{Key: "sk-custom-••••••", Weight: 100}},
	})
}

// seedAggregates 写入与 PC 端设计稿一致的示例聚合。
func seedAggregates(cm *config.Manager) {
	_, _ = cm.AddAggregate(config.Aggregate{
		Name:   "用户订单聚合",
		Path:   "/api/user/order",
		Method: "GET",
		Mode:   "parallel",
		Mapping: "user: {$.steps[0].data}\norders: {$.steps[1].data.list}",
		Enabled: true,
		Steps: []config.Step{
			{Name: "调用用户服务", Method: "GET", URL: "http://127.0.0.1:9001/users/:id", Extract: "auto"},
			{Name: "调用订单服务", Method: "GET", URL: "http://127.0.0.1:9002/orders?uid=:id", Extract: "auto"},
		},
	})
	_, _ = cm.AddAggregate(config.Aggregate{
		Name:   "结算编排",
		Path:   "/api/checkout",
		Method: "POST",
		Mode:   "serial",
		Mapping: "order: {$.steps[2].data}",
		Enabled: true,
		Steps: []config.Step{
			{Name: "库存检查", Method: "POST", URL: "http://127.0.0.1:9004/stock/check", Extract: "auto"},
			{Name: "支付扣款", Method: "POST", URL: "http://127.0.0.1:9005/pay/charge", Extract: "auto"},
			{Name: "创建订单", Method: "POST", URL: "http://127.0.0.1:9001/order/create", Extract: "auto"},
		},
	})
	_, _ = cm.AddAggregate(config.Aggregate{
		Name:   "用户好友聚合",
		Path:   "/api/user/friends",
		Method: "GET",
		Mode:   "parallel",
		Mapping: "friends: {$.steps[1].data.list}",
		Enabled: false, // 已暂停（对应设计稿第三张卡 opacity-60）
		Steps: []config.Step{
			{Name: "获取用户", Method: "GET", URL: "http://127.0.0.1:9001/users/:id", Extract: "auto"},
			{Name: "获取好友", Method: "GET", URL: "http://127.0.0.1:9006/friends?uid=:id", Extract: "auto"},
		},
	})
}

// seedLogs 写入示例日志（与两端日志页样式一致）。
func seedLogs(ls *logs.Store) {
	now := time.Now()
	ls.AddEntry(logs.Entry{
		Time: now.Add(-30 * time.Second), Level: "INFO", Channel: "http",
		Method: "GET", Path: "/api/user/order", Status: 200, Ms: 45,
		Message: "聚合完成",
		Req:     "GET /api/user/order HTTP/1.1\nHost: 192.168.137.23:8080\nAuthorization: Bearer sk-gw-***",
		Resp:    "200 OK · 98B · 45ms\n{\"code\":0,\"data\":{\"order\":{}}}",
	})
	ls.AddEntry(logs.Entry{
		Time: now.Add(-90 * time.Second), Level: "WARN", Channel: "http",
		Method: "GET", Path: "/order/*", Status: 503, Ms: 120,
		Message: "上游重试 1 次后成功",
		Req:     "GET /order/2034/pay",
		Resp:    "503 · 上游超时 · 120ms",
	})
	ls.AddEntry(logs.Entry{
		Time: now.Add(-120 * time.Second), Level: "INFO", Channel: "agg",
		Method: "GET", Path: "/api/user/order", Status: 200, Ms: 22,
		Message: "并行步骤均成功",
	})
	ls.AddEntry(logs.Entry{
		Time: now.Add(-180 * time.Second), Level: "ERROR", Channel: "net",
		Method: "", Path: "—", Status: 0, Ms: 5000,
		Message: "连接手机网关超时，自动重连",
	})
	ls.AddEntry(logs.Entry{
		Time: now.Add(-240 * time.Second), Level: "INFO", Channel: "hotspot",
		Method: "", Path: "—", Status: 0, Ms: 0,
		Message: "移动数据已自动关闭",
	})
	ls.AddEntry(logs.Entry{
		Time: now.Add(-300 * time.Second), Level: "INFO", Channel: "ble",
		Method: "", Path: "—", Status: 0, Ms: 3,
		Message: "BLE 广播已对外可见",
	})
	
}
