package httpapi

// aggregates.go —— 聚合接口 CRUD 接口实现（对应 PC 端"API 聚合"与"新建聚合"表单）。
//
// 表单字段（modal-aggregate）→ JSON 字段映射：
//   - 聚合路径       → path
//   - 方法           → method
//   - 编排步骤       → steps[]（addStep() 生成：URL + GET/POST + 输出提取模式 + JSONPath）
//   - 响应字段映射   → mapping（如 "user: {$.steps[0].data}\norders: {$.steps[1].data.list}"）
//   - 步骤执行方式   → mode（并行/串行）
//
// 手机端（gateway-prototype.html showModal('newAgg') + showAggDetail()）：
//   聚合名称 → name；步骤可拖拽排序 → 提交时按 DOM 顺序发 steps[]；
//   "输出提取"下拉 → step.extract（通用文本提取/自定义 JSONPath）+ step.jsonpath。

import (
	"encoding/json"
	"net/http"

	"mobile-gateway/internal/config"
)

// handleAggregates GET/POST /api/aggregates
func (h *Handler) handleAggregates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ok(w, h.Config.ListAggregates())
	case http.MethodPost:
		var body map[string]interface{}
		if err := readJSON(r, &body); err != nil {
			fail(w, http.StatusBadRequest, 400, "请求体解析失败: "+err.Error())
			return
		}
		agg, err := h.Config.AddAggregate(aggFromMap(body))
		if err != nil {
			fail(w, http.StatusBadRequest, 400, err.Error())
			return
		}
		ok(w, agg)
	default:
		fail(w, http.StatusMethodNotAllowed, 405, "不支持的方法")
	}
}

// handleAggregateByID PUT/DELETE /api/aggregates/{id} 和 PATCH /api/aggregates/{id}/enabled
func (h *Handler) handleAggregateByID(w http.ResponseWriter, r *http.Request) {
	id, rest := parseID(r.URL.Path, "/api/aggregates/")
	if id == "" {
		fail(w, http.StatusBadRequest, 400, "缺少聚合 ID")
		return
	}
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
		if err := h.Config.SetAggregateEnabled(id, body.Enabled); err != nil {
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
		agg, err := h.Config.UpdateAggregate(id, aggFromMap(body))
		if err != nil {
			fail(w, http.StatusNotFound, 404, err.Error())
			return
		}
		ok(w, agg)
	case http.MethodDelete:
		if err := h.Config.DeleteAggregate(id); err != nil {
			fail(w, http.StatusNotFound, 404, err.Error())
			return
		}
		ok(w, nil)
	default:
		fail(w, http.StatusMethodNotAllowed, 405, "不支持的方法")
	}
}

// aggFromMap 将前端 JSON 对象转为 config.Aggregate（两级序列化，字段名对齐）。
func aggFromMap(m map[string]interface{}) *config.Aggregate {
	data, _ := json.Marshal(m)
	var a config.Aggregate
	_ = json.Unmarshal(data, &a)
	return &a
}
