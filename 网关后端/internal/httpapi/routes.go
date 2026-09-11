package httpapi

// routes.go —— 路由 CRUD 接口实现（对应 PC 端"路由管理"表格与"添加路由"表单）。
//
// 表单字段（modal-route）→ JSON 字段映射：
//   - route-path    → path      （路径规则，如 /user/*）
//   - route-url     → target    （目标地址，如 http://127.0.0.1:9001）
//   - platform-chips/route-proto → proto + proto_name（平台预设：OpenAI 兼容/Anthropic/Google Gemini/自定义）
//   - route-model   → model     （默认模型）
//   - route-force   → force     （强制转为 OpenAI 格式）
//   - route-method  → method    （GET/POST/PUT/DELETE/匹配全部）
//   - route-timeout → timeout   （超时毫秒）
//   - route-retry   → retry     （重试次数）
//   - route-qps     → qps       （限流 QPS）
//   - apikey-rows   → keys      （多 API Key + 权重）
//   - route-prompt-sw/route-prompt → prompt_on + prompt（系统提示词注入）
//   - route-cache/route-cache-ttl → cache + cache_ttl（响应缓存）
//
// 手机端"路由表单"字段映射见 gateway-prototype.html 中 showRouteForm()：
//   rfPath→path, rfTarget→target, rfProto→proto, rfForce→force, rfModel→model,
//   rfKeysBtn→keys（底部抽屉管理）, advBody 内提示词→prompt, 上下文缓存→cache

import (
	"encoding/json"
	"net/http"

	"mobile-gateway/internal/config"
)

// handleRoutes GET/POST /api/routes
func (h *Handler) handleRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ok(w, h.Config.ListRoutes())
	case http.MethodPost:
		// 新建路由：body 为 Route 的 JSON（未包含 id/created_at，由服务端生成）
		var body map[string]interface{}
		if err := readJSON(r, &body); err != nil {
			fail(w, http.StatusBadRequest, 400, "请求体解析失败: "+err.Error())
			return
		}
		// 转成 config.Route 结构（通过再序列化，字段名与前端一致）
		route, err := h.Config.AddRoute(routeFromMap(body))
		if err != nil {
			fail(w, http.StatusBadRequest, 400, err.Error())
			return
		}
		ok(w, route)
	default:
		fail(w, http.StatusMethodNotAllowed, 405, "不支持的方法")
	}
}

// handleRouteByID PUT/DELETE /api/routes/{id} 和 PATCH /api/routes/{id}/enabled
func (h *Handler) handleRouteByID(w http.ResponseWriter, r *http.Request) {
	id, rest := parseID(r.URL.Path, "/api/routes/")
	if id == "" {
		fail(w, http.StatusBadRequest, 400, "缺少路由 ID")
		return
	}
	// 启用/禁用子路径
	if rest == "enabled" {
		if !ensureMethod(w, r, http.MethodPatch) {
			return
		}
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := readJSON(r, &body); err != nil {
			fail(w, http.StatusBadRequest, 400, "请求体解析失败")
			return
		}
		if err := h.Config.SetRouteEnabled(id, body.Enabled); err != nil {
			fail(w, http.StatusNotFound, 404, err.Error())
			return
		}
		ok(w, nil)
		return
	}

	switch r.Method {
	case http.MethodPut:
		var body map[string]interface{}
		if err := readJSON(r, &body); err != nil {
			fail(w, http.StatusBadRequest, 400, "请求体解析失败: "+err.Error())
			return
		}
		route, err := h.Config.UpdateRoute(id, routeFromMap(body))
		if err != nil {
			fail(w, http.StatusNotFound, 404, err.Error())
			return
		}
		ok(w, route)
	case http.MethodDelete:
		if err := h.Config.DeleteRoute(id); err != nil {
			fail(w, http.StatusNotFound, 404, err.Error())
			return
		}
		ok(w, nil)
	default:
		fail(w, http.StatusMethodNotAllowed, 405, "不支持的方法")
	}
}

// routeFromMap 将前端提交的 JSON 对象（map[string]interface{}）转换为 config.Route 结构体。
// 通过两级 JSON 序列化实现字段名对齐，避免手写映射遗漏。
func routeFromMap(m map[string]interface{}) config.Route {
	data, _ := json.Marshal(m)
	var r config.Route
	_ = json.Unmarshal(data, &r)
	return r
}
