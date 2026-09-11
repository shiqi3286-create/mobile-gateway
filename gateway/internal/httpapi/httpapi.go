// Package httpapi 提供管理界面 HTTP 接口层。
//
// 路由约定：
//   - /api/stats   GET    仪表盘 + 统计页数据（第一闭环）
//   - /api/config  GET    读取全部配置（设置页表单回填）
//   - /api/config  PUT    保存设置（设置页"保存"按钮）
//   - /api/config  POST   导入配置
//   - /api/config/reset POST 重置为默认
//   - /api/config/export GET 导出配置 JSON 文件
//   - /api/routes  GET    路由列表
//   - /api/routes  POST   新建路由
//   - /api/routes/{id}    PUT 修改 / DELETE 删除
//   - /api/routes/{id}/enabled  PATCH 启用/禁用
//   - /api/aggregates GET/POST + /{id} PUT/DELETE + /{id}/enabled PATCH
//   - /api/logs    GET    日志列表（分页 / 级别 / 通道 / 关键字）
//   - /api/logs    DELETE 清空日志
//   - /api/logs/export GET 导出日志文本
//   - /api/test/route  POST 路由测试（非流式）
//   - /api/test/aggregate POST 聚合测试（SSE 流式）
//   - /api/providers/{proto}/models GET 拉取模型列表
//
// 统一响应格式：{"code":0,"message":"ok","data":{...}}
// 错误时 code 非 0，message 为人类可读中文描述。
package httpapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"mobile-gateway/internal/config"
	"mobile-gateway/internal/logs"
	"mobile-gateway/internal/stats"
)

// ---------- 统一响应 ----------

// Resp 统一响应体
type Resp struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ok 输出成功响应
func ok(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, Resp{Code: 0, Message: "ok", Data: data})
}

// fail 输出失败响应（code 为业务错误码，httpStatus 为 HTTP 状态码）
func fail(w http.ResponseWriter, httpStatus, code int, msg string) {
	writeJSON(w, httpStatus, Resp{Code: code, Message: msg})
}

// writeJSON 写入 JSON 响应（统一 UTF-8 + Content-Type）
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[httpapi] 写响应失败: %v", err)
	}
}

// readJSON 读取请求体 JSON 到 v。
func readJSON(r *http.Request, v interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// ---------- 路由 ID 解析 ----------

// parseID 从形如 /api/routes/r_abc/xxx 的路径中解析第一段资源 ID。
func parseID(path, prefix string) (id string, rest string) {
	p := strings.TrimPrefix(path, prefix)
	p = strings.Trim(p, "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i], p[i+1:]
	}
	return p, ""
}

// ---------- 处理器注册 ----------

// Handler 管理 API 处理器集合。
//
// 说明：这里直接持有各模块的具体类型（而非接口），
// 因为接口方法签名与实际实现极易出现细微不一致
// （例如 Snapshot 返回 *stats.Stats 与 interface{} 不等价），
// 直接引用可让编译器在编译期完成校验，也避免运行期类型断言 panic。
type Handler struct {
	Stats  *stats.Collector // 统计快照
	Config *config.Manager  // 配置 / 路由 / 聚合
	Logs   *logs.Store      // 日志
	Proxy  ProxyProvider    // 路由转发（proxy.Engine，用于测试；第二阶段接入）
	Agg    AggProvider      // 聚合编排（aggregate.Engine；第二阶段接入）
}

// ProxyProvider 路由转发接口（供测试弹窗调用）
type ProxyProvider interface {
	TestRoute(path string, method string, body []byte) (status int, headers map[string]string, respBody []byte, latencyMs int64, err error)
}

// AggProvider 聚合编排接口（供测试聚合调用）
type AggProvider interface {
	TestAggregate(aggID string, body []byte) (int, interface{}, int64, error)
}

// Register 把所有管理 API 挂到 mux 上。
// adminAPI 为 API 前缀（默认 /api），webRoot 为静态页面根目录（可选，为空则只提供 API）。
func (h *Handler) Register(mux *http.ServeMux, adminAPI, webRoot string) {
	if adminAPI == "" {
		adminAPI = "/api"
	}
	// 统一 /api 前缀的 API 入口
	mux.HandleFunc(adminAPI+"/stats", h.handleStats)
	mux.HandleFunc(adminAPI+"/config", h.handleConfig)
	mux.HandleFunc(adminAPI+"/config/reset", h.handleConfigReset)
	mux.HandleFunc(adminAPI+"/config/export", h.handleConfigExport)
	mux.HandleFunc(adminAPI+"/routes", h.handleRoutes)
	mux.HandleFunc(adminAPI+"/routes/", h.handleRouteByID)
	mux.HandleFunc(adminAPI+"/aggregates", h.handleAggregates)
	mux.HandleFunc(adminAPI+"/aggregates/", h.handleAggregateByID)
	mux.HandleFunc(adminAPI+"/logs", h.handleLogs)
	mux.HandleFunc(adminAPI+"/logs/export", h.handleLogsExport)
	mux.HandleFunc(adminAPI+"/test/route", h.handleTestRoute)
	mux.HandleFunc(adminAPI+"/test/aggregate", h.handleTestAggregate)
	mux.HandleFunc(adminAPI+"/providers/", h.handleProviderModels)

		// 静态页面（管理界面 HTML）。/admin 是 PC 浏览器和 Android WebView 的统一入口。
		if webRoot != "" {
			fs := http.FileServer(http.Dir(webRoot))
			serveIndex := func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/" || r.URL.Path == "/admin" || r.URL.Path == "/admin/" {
					http.ServeFile(w, r, filepath.Join(webRoot, "index.html"))
					return
				}
				fs.ServeHTTP(w, r)
			}
			mux.HandleFunc("/admin", serveIndex)
			mux.HandleFunc("/", serveIndex)
		}
}

// ---------- 各处理器实现（具体逻辑分文件） ----------

// handleStats GET /api/stats?range=1h|24h|7d —— 仪表盘 + 统计页数据
func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		fail(w, http.StatusMethodNotAllowed, 405, "仅支持 GET")
		return
	}
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = "24h"
	}
	if h.Stats == nil {
		fail(w, http.StatusInternalServerError, 500, "统计模块未初始化")
		return
	}
	data := h.Stats.Snapshot(rng)
	ok(w, data)
}

// handleConfig GET/PUT/POST /api/config —— 配置读取 / 保存 / 导入
func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ok(w, h.Config.Get())
	case http.MethodPut:
		var cfg config.Config
		if err := readJSON(r, &cfg); err != nil {
			fail(w, http.StatusBadRequest, 400, "请求体解析失败: "+err.Error())
			return
		}
		if err := h.Config.Update(&cfg); err != nil {
			fail(w, http.StatusBadRequest, 400, err.Error())
			return
		}
		ok(w, h.Config.Get())
	case http.MethodPost:
		// 导入配置（body 为完整配置 JSON）
		data, err := readAll(r)
		if err != nil {
			fail(w, http.StatusBadRequest, 400, "读取请求体失败")
			return
		}
		if err := h.Config.ImportJSON(data); err != nil {
			fail(w, http.StatusBadRequest, 400, err.Error())
			return
		}
		ok(w, h.Config.Get())
	default:
		fail(w, http.StatusMethodNotAllowed, 405, "不支持的方法")
	}
}

// handleConfigReset POST /api/config/reset —— 恢复默认
func (h *Handler) handleConfigReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, http.StatusMethodNotAllowed, 405, "仅支持 POST")
		return
	}
	if err := h.Config.Reset(); err != nil {
		fail(w, http.StatusInternalServerError, 500, err.Error())
		return
	}
	ok(w, h.Config.Get())
}

// handleConfigExport GET /api/config/export —— 导出配置为 gateway.config.json
func (h *Handler) handleConfigExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		fail(w, http.StatusMethodNotAllowed, 405, "仅支持 GET")
		return
	}
	data, err := h.Config.ExportJSON()
	if err != nil {
		fail(w, http.StatusInternalServerError, 500, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="gateway.config.json"`)
	w.Write(data)
}

// readAll 读取请求体全部字节。
func readAll(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	var buf strings.Builder
	buf.Grow(1 << 20)
	b := make([]byte, 32*1024)
	for {
		n, err := r.Body.Read(b)
		buf.Write(b[:n])
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
	}
	return []byte(buf.String()), nil
}

// ensureMethod 简易方法检查（避免重复代码）
func ensureMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		fail(w, http.StatusMethodNotAllowed, 405, fmt.Sprintf("仅支持 %s", method))
		return false
	}
	return true
}
