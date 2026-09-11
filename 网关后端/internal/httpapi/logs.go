package httpapi

// logs.go —— 日志查询 / 清空 / 导出接口（对应 PC 端"日志"页 + 手机端"日志"页）。
//
// PC 端 UI 交互：
//   - 顶部 chips 按级别过滤（INFO/WARN/ERROR，可多选）→ 服务端支持 level 逗号分隔
//   - 搜索框关键字过滤（filterLogs）→ keyword
//   - 暂停/继续（客户端行为，无接口）
//   - 清空按钮 → DELETE /api/logs
//   - 导出按钮 → GET /api/logs/export（text/plain 附件 log.txt）
//   - 分页（每页 10 条）
//
// 手机端 UI 交互：
//   - 下拉选：全部级别 / INFO / WARN / ERROR → level
//   - 下拉选：全部通道 / 局域网 / 蓝牙 → channel（lan/bt，服务端映射）
//   - 清空按钮 → DELETE /api/logs；导出按钮 → GET /api/logs/export
//   - 展开日志行显示 detail.req / detail.resp → 列表项需返回请求/响应摘要

import (
	"net/http"
	"strconv"
	"strings"
)

// handleLogs GET/DELETE /api/logs
func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		level := q.Get("level")       // INFO/WARN/ERROR 或逗号分隔多选
		channel := q.Get("channel")   // lan / bt / http / agg / net / hotspot / ble
		keyword := q.Get("keyword")   // 路径/关键字
		page, _ := strconv.Atoi(q.Get("page"))
		size, _ := strconv.Atoi(q.Get("size"))
		if page <= 0 {
			page = 1
		}
		if size <= 0 || size > 100 {
			size = 10
		}
		data := h.Logs.Query(normalizeLevels(level), channel, keyword, page, size)
		ok(w, data)
	case http.MethodDelete:
		if err := h.Logs.Clear(); err != nil {
			fail(w, http.StatusInternalServerError, 500, err.Error())
			return
		}
		ok(w, nil)
	default:
		fail(w, http.StatusMethodNotAllowed, 405, "不支持的方法")
	}
}

// handleLogsExport GET /api/logs/export
func (h *Handler) handleLogsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		fail(w, http.StatusMethodNotAllowed, 405, "仅支持 GET")
		return
	}
	data := h.Logs.ExportText()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="log.txt"`)
	w.Write(data)
}

// normalizeLevels 将 "INFO,WARN" 规范化为 []string{"INFO","WARN"}。
func normalizeLevels(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToUpper(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
