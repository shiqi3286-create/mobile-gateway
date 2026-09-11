package httpapi

// test.go —— 测试接口（路由测试 + 聚合测试 SSE）。
//
// 前端交互：
//   - PC 端"测试接口"弹窗（modal-test）：点击后发 POST /api/test/route，
//     服务端返回请求/响应样例 + Token 统计 + 协议转换结果（对应测试弹窗三列）。
//   - PC 端"测试聚合"（modal-aggregate 内 testAggregate()）：发 POST /api/test/aggregate，
//     服务端按 并行/串行 执行各步骤并返回每步结果。
//   - 手机端"测试聚合"（openTestPage）：SSE 流式接收聚合文本 + 步骤状态 + Token，
//     前端实现打字机效果（对应 twText/twSteps/twIn/twOut/twTotal）。
//
// SSE 事件格式（手机端打字机）：
//   event: step     data: {"index":0,"state":"done","ms":118}
//   event: token    data: {"in":1024,"out":356,"cost":0.0087}
//   event: text     data: {"chunk":"用户「张三」近 30 天…"}   （每帧一段文本）
//   event: done     data: {"ok":true,"total_ms":460}

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// ---------- 路由测试（非流式） ----------

// testRouteResult 对应 PC 端 modal-test 三列数据
type testRouteResult struct {
	Endpoint string `json:"endpoint"` // 如 POST /example/*
	Proto    string `json:"proto"`    // 平台名 + 模型
	TotalMs  int64  `json:"total_ms"` // 总耗时
	Pass     bool   `json:"pass"`     // 是否通过
	Request  string `json:"request"`  // curl 样例文本
	Response string `json:"response"` // 响应样例文本
	Tokens   int64  `json:"tokens"`   // 合计 tokens
	TokenIn  int64  `json:"token_in"`
	TokenOut int64  `json:"token_out"`
	PromptTokens int64 `json:"prompt_tokens"` // 注入提示词 token
	Cost     float64 `json:"cost"`      // 预估费用
	CacheHit bool    `json:"cache_hit"` // 是否命中缓存
	Converted bool   `json:"converted"` // 是否已转 OpenAI 格式
}

// handleTestRoute POST /api/test/route
func (h *Handler) handleTestRoute(w http.ResponseWriter, r *http.Request) {
	if !ensureMethod(w, r, http.MethodPost) {
		return
	}
	var req struct {
		RouteID string `json:"route_id"`
		Path    string `json:"path"`   // 路由路径（用于展示）
		Method  string `json:"method"` // 测试方法
		Model   string `json:"model"`  // 测试模型
		Body    json.RawMessage `json:"body"` // 测试请求体（可选）
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, 400, "请求体解析失败: "+err.Error())
		return
	}

	// 若配置了 proxy，则真正转发测试；否则返回示例数据（前端联调阶段可用）
	if h.Proxy != nil {
		method := req.Method
		if method == "" {
			method = "POST"
		}
		path := req.Path
		if path == "" {
			path = "/example/*"
		}
		var body []byte
		if len(req.Body) > 0 {
			body = req.Body
		}
		status, _, respBody, ms, err := h.Proxy.TestRoute(path, method, body)
		res := testRouteResult{
			Endpoint:  method + " " + path,
			Proto:     "OpenAI 兼容",
			TotalMs:   ms,
			Pass:      err == nil && status >= 200 && status < 400,
			Request:   fmt.Sprintf("%s %s\nHost: 192.168.137.23:8080\nAuthorization: Bearer sk-***\n\n%s", method, path, string(body)),
			Response:  fmt.Sprintf("%d %s · %dms\n%s", status, http.StatusText(status), ms, string(respBody)),
			Converted: true,
		}
		if err != nil {
			res.Response = fmt.Sprintf("上游错误: %v", err)
		}
		ok(w, res)
		return
	}

	// 联调兜底：返回示例数据（与 UI 静态样例一致）
	res := testRouteResult{
		Endpoint:  "POST /example/*",
		Proto:     "Anthropic · claude-3-5-sonnet",
		TotalMs:   148,
		Pass:      true,
		Request:   "POST /v1/messages\nAuthorization: Bearer sk-***\nanthropic-version: 2023-06-01\n\n{ \"model\": \"claude-3-5-sonnet\",\n  \"max_tokens\": 1024,\n  \"system\": \"由网关注入的系统提示词…\",\n  \"messages\": [{\"role\":\"user\",\"content\":\"查询订单详情\"}] }",
		Response:  "{ \"id\": \"msg_01Hx…\",\n  \"type\": \"message\",\n  \"content\": [{\"type\":\"text\",\"text\":\"订单 #A-1024 已发货…\"}],\n  \"usage\": { \"input_tokens\": 384, \"output_tokens\": 96 } }\n\n→ 已转换为 OpenAI choices[] 结构",
		Tokens:     480,
		TokenIn:    384,
		TokenOut:   96,
		PromptTokens: 60,
		Cost:       0.0047,
		CacheHit:   false,
		Converted:  true,
	}
	ok(w, res)
}

// ---------- 聚合测试（SSE 流式） ----------

// handleTestAggregate POST /api/test/aggregate?stream=1
// 当 stream=1 时以 SSE 逐帧推送（供手机端打字机 / PC 端进度展示）。
func (h *Handler) handleTestAggregate(w http.ResponseWriter, r *http.Request) {
	if !ensureMethod(w, r, http.MethodPost) {
		return
	}
	stream, _ := strconv.Atoi(r.URL.Query().Get("stream"))
	if stream == 1 {
		h.streamAggregate(w, r)
		return
	}
	// 非流式：返回聚合测试汇总（PC 端 testAggregate() 使用）
	var req struct {
		AggID string `json:"agg_id"`
		Path  string `json:"path"`
		Body  json.RawMessage `json:"body"`
	}
	if err := readJSON(r, &req); err != nil {
		fail(w, http.StatusBadRequest, 400, "请求体解析失败: "+err.Error())
		return
	}
	// 示例汇总结果
	ok(w, map[string]interface{}{
		"ok":       true,
		"total_ms": 42,
		"steps": []map[string]interface{}{
			{"index": 1, "url": "http://127.0.0.1:9001", "proto": "OpenAI 兼容", "status": 200, "ms": 22, "ok": true},
			{"index": 2, "url": "http://127.0.0.1:9002", "proto": "Anthropic", "status": 200, "ms": 20, "ok": true},
		},
	})
}

// streamAggregate 以 SSE 推送聚合测试流。
func (h *Handler) streamAggregate(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		fail(w, http.StatusInternalServerError, 500, "当前连接不支持流式输出")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// 有聚合引擎时走真实执行；否则播放示例流
	if h.Agg != nil {
		var req struct {
			AggID string `json:"agg_id"`
			Body  json.RawMessage `json:"body"`
		}
		_ = readJSON(r, &req)
		// 真实执行：聚合引擎会把每步的文本增量写入 respBody 并调用 Flush
		// 这里先占位，聚合模块联调时替换。
	}
	// 示例打字机流（与手机端 TW_TEXT 逻辑对齐）
	send := func(event string, data interface{}) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	}
	send("step", map[string]interface{}{"index": 0, "state": "run"})
	send("token", map[string]interface{}{"in": 1024, "out": 0, "cost": 0.003072})
	send("text", map[string]interface{}{"chunk": "【聚合结果 · gpt-4o-mini + claude-sonnet-4】\n\n"})
	send("step", map[string]interface{}{"index": 0, "state": "done", "ms": 118})
	send("text", map[string]interface{}{"chunk": "用户「张三」近 30 天共产生 12 笔订单，累计消费 ¥3,486。"})
	send("step", map[string]interface{}{"index": 1, "state": "run"})
	send("token", map[string]interface{}{"in": 1024, "out": 178, "cost": 0.0057})
	send("text", map[string]interface{}{"chunk": "\n建议：推送智能家居品类优惠券，预计转化率提升约 12%。"})
	send("step", map[string]interface{}{"index": 1, "state": "done", "ms": 342})
	send("token", map[string]interface{}{"in": 1024, "out": 356, "cost": 0.0084})
	send("done", map[string]interface{}{"ok": true, "total_ms": 460})
}

// bufio 保持导入（后续真实 SSE 逐行读取上游流时使用）
var _ = bufio.NewReader
