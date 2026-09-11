package httpapi

// models.go —— 拉取模型列表接口（对应 PC 端"测试并拉取模型"按钮 + 手机端 pullModels()）。
//
// 协议差异：
//   - openai:  GET {target}/v1/models  → OpenAI 原生返回，直接透传
//   - gemini:  GET {target}/v1beta/models → 转换为 OpenAI 格式（models[] 数组 + id 字段）
//   - anthropic: 不支持自动拉取 → 返回内置模型列表
//   - custom:  尝试请求 {target}/v1/models；失败则返回 502 与错误描述（前端显示"拉取失败"）

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// modelItem OpenAI 格式的模型条目
type modelItem struct {
	ID     string `json:"id"`
	Object string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// builtinModels 各平台内置模型字典（与前端 PROTO_MODELS / PROTOCOLS 保持一致）
var builtinModels = map[string][]string{
	"openai":    {"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "gpt-3.5-turbo"},
	"anthropic": {"claude-3-5-sonnet-20241022", "claude-3-5-haiku-20241022", "claude-3-opus-20240229"},
	"gemini":    {"gemini-1.5-pro", "gemini-1.5-flash", "gemini-2.0-flash"},
}

// handleProviderModels GET /api/providers/{proto}/models?target=...&api_key=...
// 注意：api_key 走查询参数仅用于联调；正式版应从前端 Header 传递并落库到路由 keys。
func (h *Handler) handleProviderModels(w http.ResponseWriter, r *http.Request) {
	if !ensureMethod(w, r, http.MethodGet) {
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/api/providers/")
	p = strings.TrimSuffix(p, "/models")
	proto := strings.ToLower(strings.Trim(p, "/"))
	target := r.URL.Query().Get("target")
	apiKey := r.URL.Query().Get("api_key")

	switch proto {
	case "anthropic":
		// 不支持自动拉取 → 内置列表
		ok(w, builtinModels[proto])
		return
	case "custom":
		// 尝试拉取，失败返回 502
		if target == "" {
			fail(w, http.StatusBadRequest, 400, "自定义平台需提供 target 地址")
			return
		}
		if err := pullOpenAIModels(w, target, apiKey); err != nil {
			fail(w, http.StatusBadGateway, 502, "拉取失败: "+err.Error())
		}
		return
	default: // openai / gemini
		if target == "" {
			// 无目标地址 → 内置列表
			ok(w, builtinModels[proto])
			return
		}
		if err := pullOpenAIModels(w, target, apiKey); err != nil {
			fail(w, http.StatusBadGateway, 502, "拉取失败: "+err.Error())
		}
	}
}

// pullOpenAIModels 请求上游 /v1/models（或 /v1beta/models），统一为 OpenAI 格式返回。
func pullOpenAIModels(w http.ResponseWriter, base, apiKey string) error {
	url := strings.TrimRight(base, "/") + "/v1/models"
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &upstreamError{status: resp.StatusCode}
	}
	var raw struct {
		Models []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return err
	}
	items := make([]modelItem, 0, len(raw.Models))
	for _, m := range raw.Models {
		items = append(items, modelItem{ID: m.ID, Object: "model", OwnedBy: "upstream"})
	}
	ok(w, items)
	return nil
}

type upstreamError struct{ status int }

func (e *upstreamError) Error() string {
	return "上游返回 " + http.StatusText(e.status) + " (" + itoaStatus(e.status) + ")"
}

func itoaStatus(s int) string {
	return map[int]string{400: "400", 401: "401", 403: "403", 404: "404", 429: "429", 500: "500", 502: "502", 503: "503"}[s]
}
