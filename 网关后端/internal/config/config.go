// Package config 提供网关配置管理模块。
//
// 职责：
//   - 维护全局网关设置（监听端口、HTTPS、mDNS、API Key、IP 白名单、蓝牙、热点模式等）
//   - 路由规则 CRUD（对应 PC 端"路由管理"表单字段）
//   - 聚合接口 CRUD（对应 PC 端"API 聚合"表单字段）
//   - 配置导入 / 导出 / 重置（PC 端设置页按钮）
//
// 字段命名与前端 UI 完全一致（见 E:\API聚合\win界面设计\mobile-gateway-admin.html）：
//   - route.path / target / proto / model / force / method / timeout / retry / qps / keys / prompt / cache / cache_ttl
//   - agg.name / path / method / mode / steps[].{name,method,url,extract,jsonpath} / mapping
//
// 数据持久化：全部配置序列化为 JSON 写入 data/gateway.config.json，热更新后自动落盘。
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ---------- 顶层配置 ----------

// Config 网关全部配置（对应设置页 + 路由 + 聚合）
type Config struct {
	// 基础设置（PC 设置页"基础设置"卡 + 手机设置页"网络/安全"卡）
	ListenPort  int      `json:"listen_port"`  // 监听端口，默认 8080
	EnableHTTPS bool     `json:"enable_https"` // 启用 HTTPS
	EnableMDNS  bool     `json:"enable_mdns"`  // 启用 mDNS
	APIKey      string   `json:"api_key"`      // API Key
	IPWhitelist []string `json:"ip_whitelist"` // IP 白名单（支持 CIDR）

	// 蓝牙设置
	EnableBLE  bool   `json:"enable_ble"`  // 启用蓝牙 BLE
	BLEService string `json:"ble_service"` // 服务 UUID
	BLEEncrypt string `json:"ble_encrypt"` // 加密方式：无加密 / Just Works / Passkey / LE Secure

	// 电脑热点模式
	AutoDetectHotspot     bool `json:"auto_detect_hotspot"`      // 自动检测电脑热点
	PreferHotspot         bool `json:"prefer_hotspot"`           // 优先使用热点接口
	CloseCellularReminder bool `json:"close_cellular_reminder"`  // 关闭移动数据提醒

	// 保活（手机端设置页）
	ForegroundService bool `json:"foreground_service"` // 前台服务
	BatteryWhitelist  bool `json:"battery_whitelist"`  // 电池白名单
	BootAutoStart     bool `json:"boot_auto_start"`    // 开机自启

	// 月预算（仪表盘费用预估进度条）
	MonthBudget float64 `json:"month_budget"` // 默认 200 美元

	Routes     []Route      `json:"routes"`      // 路由规则列表
	Aggregates []Aggregate  `json:"aggregates"`  // 聚合接口列表
}

// ---------- 路由规则 ----------

// Route 一条转发路由（对应 PC 端"添加路由"表单）
type Route struct {
	ID       string    `json:"id"`        // 唯一 ID（服务端生成，形如 r_xxxxxxxx）
	Path     string    `json:"path"`      // 路径规则，支持前缀 / 通配符，如 /user/*、/api/health
	Target   string    `json:"target"`    // 目标 URL，如 http://127.0.0.1:9001
	Proto    string    `json:"proto"`     // 协议类型：openai / anthropic / gemini / custom
	ProtoName string   `json:"proto_name"` // 平台预设名：OpenAI 兼容 / Anthropic / Google Gemini / 自定义（冗余展示用）
	Model    string    `json:"model"`     // 默认模型，如 gpt-4o-mini
	Force    bool      `json:"force"`     // 强制转为 OpenAI 格式
	Method   string    `json:"method"`    // 匹配方法：GET / POST / PUT / DELETE / ALL
	Timeout  int       `json:"timeout"`   // 超时（毫秒），默认 5000
	Retry    int       `json:"retry"`     // 重试次数，默认 1
	QPS      int       `json:"qps"`       // 限流 QPS，默认 50
	Enabled  bool      `json:"enabled"`   // 是否启用
	AdaptOK  bool      `json:"adapt_ok"`  // 协议适配是否成功（拉取模型验证结果）
	Keys     []APIKey  `json:"keys"`      // 多 API Key（权重轮询）
	Prompt   string    `json:"prompt"`    // 系统提示词注入
	PromptOn bool      `json:"prompt_on"` // 是否启用提示词注入
	Cache    bool      `json:"cache"`     // 响应缓存
	CacheTTL int       `json:"cache_ttl"` // 缓存 TTL（秒），默认 300
	CreatedAt int64   `json:"created_at"` // 创建时间（Unix）
	UpdatedAt int64   `json:"updated_at"` // 更新时间（Unix）
}

// APIKey 一个 API Key（含权重，用于多 Key 轮询）
type APIKey struct {
	Key    string `json:"key"`    // Key 明文（仅本机存储；导出时脱敏/加密）
	Weight int    `json:"weight"` // 权重（0-100），权重越高命中越多
	Name   string `json:"name"`   // 备注名（手机端 UI 有）
}

// ---------- 聚合接口 ----------

// Aggregate 一个聚合接口（对应 PC 端"新建聚合"表单 + 手机端聚合详情）
type Aggregate struct {
	ID       string   `json:"id"`        // 唯一 ID（形如 a_xxxxxxxx）
	Name     string   `json:"name"`      // 聚合名称（手机端显示）
	Path     string   `json:"path"`      // 聚合路径，如 /api/user/order
	Method   string   `json:"method"`    // GET / POST
	Mode     string   `json:"mode"`      // parallel（并行）/ serial（串行）
	Steps    []Step   `json:"steps"`     // 编排步骤
	Mapping  string   `json:"mapping"`   // 响应字段映射（多行文本）
	Enabled  bool     `json:"enabled"`   // 是否启用
	Stream   bool     `json:"stream"`    // 是否流式输出 SSE
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// Step 一个编排步骤
type Step struct {
	Name     string `json:"name"`     // 步骤名（手机端显示，如"调用用户服务"）
	Method   string `json:"method"`   // GET / POST
	URL      string `json:"url"`      // 上游 URL（可含路径变量 :id）
	Headers  map[string]string `json:"headers,omitempty"` // 透传自定义头
	Extract  string `json:"extract"`  // 输出提取模式：auto（通用文本提取）/ jsonpath（自定义 JSONPath）
	JSONPath string `json:"jsonpath"` // 自定义 JSONPath，如 $.choices[0].message.content
}

// ---------- 存储 ----------

// Manager 配置管理器（线程安全）
type Manager struct {
	mu     sync.RWMutex
	cfg    *Config
	file   string // 配置文件路径
}

// DefaultConfig 返回带默认值的配置（网关首次启动 / 重置时使用）。
func DefaultConfig() *Config {
	return &Config{
		ListenPort:            8080,
		EnableHTTPS:           false,
		EnableMDNS:            true,
		APIKey:                "gw_live_0000000000000000", // 占位，首次启动应提示用户修改
		IPWhitelist:           []string{"192.168.137.1", "192.168.137.0/24"},
		EnableBLE:             false,
		BLEService:            "0000ffe0-0000-1000-8000-00805f9b34fb",
		BLEEncrypt:            "Just Works",
		AutoDetectHotspot:     true,
		PreferHotspot:         true,
		CloseCellularReminder: true,
		ForegroundService:     true,
		BatteryWhitelist:      true,
		BootAutoStart:         false,
		MonthBudget:           200.0,
		Routes:                []Route{},
		Aggregates:            []Aggregate{},
	}
}

// NewManager 加载（或创建）配置文件。
// 若 dataDir 下不存在配置文件，则写入默认配置。
func NewManager(dataDir string) (*Manager, error) {
	m := &Manager{cfg: DefaultConfig()}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %w", err)
	}
	m.file = filepath.Join(dataDir, "gateway.config.json")
	if _, err := os.Stat(m.file); os.IsNotExist(err) {
		// 首次启动：写入默认配置
		if err := m.save(); err != nil {
			return nil, err
		}
		return m, nil
	}
	data, err := os.ReadFile(m.file)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &m.cfg); err != nil {
		// 配置损坏时备份并回退默认
		_ = os.Rename(m.file, m.file+".bak")
		m.cfg = DefaultConfig()
		_ = m.save()
	}
	if m.cfg.Routes == nil {
		m.cfg.Routes = []Route{}
	}
	if m.cfg.Aggregates == nil {
		m.cfg.Aggregates = []Aggregate{}
	}
	return m, nil
}

// Get 返回配置副本（前端只读展示）。
func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := *m.cfg
	cp.Routes = append([]Route(nil), m.cfg.Routes...)
	cp.Aggregates = append([]Aggregate(nil), m.cfg.Aggregates...)
	cp.IPWhitelist = append([]string(nil), m.cfg.IPWhitelist...)
	return &cp
}

// Update 全量更新设置（保存按钮）。
func (m *Manager) Update(c *Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg.ListenPort = c.ListenPort
	m.cfg.EnableHTTPS = c.EnableHTTPS
	m.cfg.EnableMDNS = c.EnableMDNS
	m.cfg.APIKey = c.APIKey
	m.cfg.IPWhitelist = c.IPWhitelist
	m.cfg.EnableBLE = c.EnableBLE
	m.cfg.BLEService = c.BLEService
	m.cfg.BLEEncrypt = c.BLEEncrypt
	m.cfg.AutoDetectHotspot = c.AutoDetectHotspot
	m.cfg.PreferHotspot = c.PreferHotspot
	m.cfg.CloseCellularReminder = c.CloseCellularReminder
	m.cfg.ForegroundService = c.ForegroundService
	m.cfg.BatteryWhitelist = c.BatteryWhitelist
	m.cfg.BootAutoStart = c.BootAutoStart
	if c.MonthBudget > 0 {
		m.cfg.MonthBudget = c.MonthBudget
	}
	return m.save()
}

// save 序列化并落盘（调用方需持有锁）。
func (m *Manager) save() error {
	data, err := json.MarshalIndent(m.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.file)
}

// ---------- 路由 CRUD ----------

// ListRoutes 返回全部路由。
func (m *Manager) ListRoutes() []Route {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Route(nil), m.cfg.Routes...)
}

// AddRoute 新增路由。
func (m *Manager) AddRoute(r Route) (Route, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(r.Path) == "" {
		return r, fmt.Errorf("路径规则不能为空")
	}
	if strings.TrimSpace(r.Target) == "" {
		return r, fmt.Errorf("目标地址不能为空")
	}
	r.ID = newID("r")
	r.Enabled = true
	r.AdaptOK = r.Proto != "custom" || r.Force // 自定义协议且未强制转换视为适配失败
	r.Timeout = defaultInt(r.Timeout, 5000)
	r.Retry = defaultInt(r.Retry, 1)
	r.QPS = defaultInt(r.QPS, 50)
	r.CacheTTL = defaultInt(r.CacheTTL, 300)
	r.CreatedAt = nowUnix()
	r.UpdatedAt = r.CreatedAt
	m.cfg.Routes = append(m.cfg.Routes, r)
	return r, m.save()
}

// UpdateRoute 修改路由（按 ID 查找）。
func (m *Manager) UpdateRoute(id string, r Route) (Route, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.cfg.Routes {
		if m.cfg.Routes[i].ID == id {
			r.ID = id
			r.CreatedAt = m.cfg.Routes[i].CreatedAt
			r.UpdatedAt = nowUnix()
			m.cfg.Routes[i] = r
			return r, m.save()
		}
	}
	return r, fmt.Errorf("路由 %s 不存在", id)
}

// DeleteRoute 删除路由。
func (m *Manager) DeleteRoute(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.cfg.Routes {
		if m.cfg.Routes[i].ID == id {
			m.cfg.Routes = append(m.cfg.Routes[:i], m.cfg.Routes[i+1:]...)
			return m.save()
		}
	}
	return fmt.Errorf("路由 %s 不存在", id)
}

// SetRouteEnabled 启用/禁用路由（表格切换开关）。
func (m *Manager) SetRouteEnabled(id string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.cfg.Routes {
		if m.cfg.Routes[i].ID == id {
			m.cfg.Routes[i].Enabled = enabled
			m.cfg.Routes[i].UpdatedAt = nowUnix()
			return m.save()
		}
	}
	return fmt.Errorf("路由 %s 不存在", id)
}

// ---------- 聚合 CRUD ----------

// ListAggregates 返回全部聚合接口。
func (m *Manager) ListAggregates() []Aggregate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]Aggregate(nil), m.cfg.Aggregates...)
}

// AddAggregate 新增聚合接口。
func (m *Manager) AddAggregate(a Aggregate) (Aggregate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(a.Path) == "" {
		return a, fmt.Errorf("聚合路径不能为空")
	}
	if len(a.Steps) == 0 {
		return a, fmt.Errorf("至少需要一个编排步骤")
	}
	a.ID = newID("a")
	a.Mode = defaultStr(a.Mode, "parallel")
	a.Enabled = true
	a.CreatedAt = nowUnix()
	a.UpdatedAt = a.CreatedAt
	m.cfg.Aggregates = append(m.cfg.Aggregates, a)
	return a, m.save()
}

// UpdateAggregate 修改聚合接口。
func (m *Manager) UpdateAggregate(id string, a Aggregate) (Aggregate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.cfg.Aggregates {
		if m.cfg.Aggregates[i].ID == id {
			a.ID = id
			a.CreatedAt = m.cfg.Aggregates[i].CreatedAt
			a.UpdatedAt = nowUnix()
			m.cfg.Aggregates[i] = a
			return a, m.save()
		}
	}
	return a, fmt.Errorf("聚合接口 %s 不存在", id)
}

// DeleteAggregate 删除聚合接口。
func (m *Manager) DeleteAggregate(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.cfg.Aggregates {
		if m.cfg.Aggregates[i].ID == id {
			m.cfg.Aggregates = append(m.cfg.Aggregates[:i], m.cfg.Aggregates[i+1:]...)
			return m.save()
		}
	}
	return fmt.Errorf("聚合接口 %s 不存在", id)
}

// SetAggregateEnabled 启用/禁用聚合接口。
func (m *Manager) SetAggregateEnabled(id string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.cfg.Aggregates {
		if m.cfg.Aggregates[i].ID == id {
			m.cfg.Aggregates[i].Enabled = enabled
			m.cfg.Aggregates[i].UpdatedAt = nowUnix()
			return m.save()
		}
	}
	return fmt.Errorf("聚合接口 %s 不存在", id)
}

// ---------- 导入 / 导出 / 重置 ----------

// ExportJSON 导出完整配置 JSON（导出按钮）。
func (m *Manager) ExportJSON() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return json.MarshalIndent(m.cfg, "", "  ")
}

// ImportJSON 导入完整配置（导入按钮）。导入成功后立即生效并落盘。
func (m *Manager) ImportJSON(data []byte) error {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return fmt.Errorf("配置解析失败: %w", err)
	}
	if c.ListenPort == 0 {
		c.ListenPort = 8080
	}
	m.mu.Lock()
	m.cfg = &c
	err := m.save()
	m.mu.Unlock()
	return err
}

// Reset 恢复默认配置（重置按钮）。
func (m *Manager) Reset() error {
	m.mu.Lock()
	m.cfg = DefaultConfig()
	err := m.save()
	m.mu.Unlock()
	return err
}

// ---------- 内部工具 ----------

var (
	idCounter int64
	// timeNow 便于测试替换；默认返回真实当前时间
	timeNow = func() time.Time { return time.Now() }
)

// newID 生成形如 "r_9f8a2b1c" 的唯一 ID。
func newID(prefix string) string {
	idCounter++
	return fmt.Sprintf("%s_%x", prefix, timeNow().UnixNano())
}

func nowUnix() int64 {
	return timeNow().Unix()
}

// defaultInt 当 v<=0 时返回 fallback。
func defaultInt(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}

// defaultStr 当 v 为空时返回 fallback。
func defaultStr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
