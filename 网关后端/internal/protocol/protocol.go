// Package protocol 提供 AI 协议转换模块（OpenAI / Anthropic / Gemini 格式互转）。
//
// 设计目标：以 OpenAI 结构为网关内部统一格式，进出都做一次映射：
//
//	请求方向：
//	  客户端(OpenAI 格式) ──┐
//	                       ├─→ 内部 OpenAI 统一结构 ──→ 按上游协议转换 ──→ 上游
//	  客户端(原生协议)   ──┘
//	响应方向：
//	  上游(原生协议) ──→ 统一 OpenAI 结构 ──→ 客户端（force=true 时转 OpenAI，否则透传）
//
// 本文件为占位实现（第一阶段仅定义结构与转换骨架），
// 第二阶段接入 proxy 模块时补充完整转换逻辑。
package protocol

// Protocol 协议类型枚举
type Protocol string

const (
	OpenAI   Protocol = "openai"
	Anthropic Protocol = "anthropic"
	Gemini   Protocol = "gemini"
	Custom   Protocol = "custom"
)

// ---------- 统一内部结构（OpenAI 兼容） ----------

// ChatMessage 统一消息
type ChatMessage struct {
	Role    string `json:"role"`              // system / user / assistant
	Content string `json:"content"`           // 文本内容
	// 结构化内容（Claude content blocks / Gemini parts）展开后的数组
	ContentParts []ContentPart `json:"content_parts,omitempty"`
}

// ContentPart 结构化内容块（Claude 与 Gemini 使用）
type ContentPart struct {
	Type string `json:"type"` // text / image / tool_use / tool_result
	Text string `json:"text,omitempty"`
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// ChatRequest 统一 Chat 请求（OpenAI 结构）
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	System      string        `json:"system,omitempty"` // 注入的系统提示词
}

// ---------- 占位转换接口 ----------

// Converter 协议转换器接口（第二阶段实现）
type Converter interface {
	// ToInternal 将上游原生请求转成统一 OpenAI 结构
	ToInternal(body []byte) (*ChatRequest, error)
	// FromInternal 将统一 OpenAI 结构转成上游原生请求
	FromInternal(req *ChatRequest) ([]byte, error)
	// ResponseToOpenAI 将上游响应转成 OpenAI choices[] 结构（force=true 时调用）
	ResponseToOpenAI(body []byte, stream bool) ([]byte, error)
	// ModelList 拉取模型列表并统一为 OpenAI 格式
	ModelList(baseURL, apiKey string) ([]string, error)
}

// NewConverter 根据协议创建转换器（占位：第二阶段实现各协议）。
func NewConverter(p Protocol) Converter {
	switch p {
	case Anthropic:
		return &anthropicConverter{}
	case Gemini:
		return &geminiConverter{}
	default:
		return &openAIConverter{}
	}
}

// openAIConverter OpenAI 原样透传（不需要转换）
type openAIConverter struct{}

func (c *openAIConverter) ToInternal(body []byte) (*ChatRequest, error) {
	return &ChatRequest{}, nil // TODO
}
func (c *openAIConverter) FromInternal(req *ChatRequest) ([]byte, error) { return nil, nil } // TODO
func (c *openAIConverter) ResponseToOpenAI(body []byte, stream bool) ([]byte, error) {
	return body, nil // 已是 OpenAI 格式，直接透传
}
func (c *openAIConverter) ModelList(baseURL, apiKey string) ([]string, error) {
	return nil, nil // TODO
}

// anthropicConverter Claude 格式（第二阶段实现）
type anthropicConverter struct{}

func (c *anthropicConverter) ToInternal(body []byte) (*ChatRequest, error) {
	return &ChatRequest{}, nil // TODO
}
func (c *anthropicConverter) FromInternal(req *ChatRequest) ([]byte, error) { return nil, nil } // TODO
func (c *anthropicConverter) ResponseToOpenAI(body []byte, stream bool) ([]byte, error) {
	return nil, nil // TODO
}
func (c *anthropicConverter) ModelList(baseURL, apiKey string) ([]string, error) {
	return nil, nil // TODO
}

// geminiConverter Gemini 格式（第二阶段实现）
type geminiConverter struct{}

func (c *geminiConverter) ToInternal(body []byte) (*ChatRequest, error) {
	return &ChatRequest{}, nil // TODO
}
func (c *geminiConverter) FromInternal(req *ChatRequest) ([]byte, error) { return nil, nil } // TODO
func (c *geminiConverter) ResponseToOpenAI(body []byte, stream bool) ([]byte, error) {
	return nil, nil // TODO
}
func (c *geminiConverter) ModelList(baseURL, apiKey string) ([]string, error) {
	return nil, nil // TODO
}
