// Package logs 提供网关日志模块（内存环形缓冲 + 可选文件持久化）。
//
// 日志来源：
//   - proxy 转发：转发成功/失败、重试、超时
//   - aggregate：聚合完成、步骤状态
//   - protocol：协议转换结果
//   - 系统事件：网关启停、热点切换、移动数据关闭、BLE 广播
//
// 查询接口（对应 PC 端日志页 / 手机端日志页）：
//   - Query(level[], channel, keyword, page, size) 分页查询
//   - Clear() 清空
//   - ExportText() 导出为 log.txt 文本
//
// 级别：INFO / WARN / ERROR
// 通道：http / agg / net / hotspot / ble（PC 端）；lan / bt（手机端，映射关系见查询处）
package logs

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Entry 一条日志
type Entry struct {
	ID      int64     `json:"id"`
	Time    time.Time `json:"time"`
	TimeStr string    `json:"time_str"` // 格式化 "15:04:05"（PC 端列）
	Level   string    `json:"level"`    // INFO / WARN / ERROR
	Channel string    `json:"channel"`  // http / agg / net / hotspot / ble
	Method  string    `json:"method"`   // GET/POST/...
	Path    string    `json:"path"`     // 请求路径
	Status  int       `json:"status"`   // 状态码（系统事件为 0）
	Ms      int64     `json:"ms"`       // 耗时
	Message string    `json:"message"`  // 消息
	Req     string    `json:"req,omitempty"`   // 请求摘要（手机端展开详情）
	Resp    string    `json:"resp,omitempty"`  // 响应摘要（手机端展开详情）
}

// Store 内存日志存储（线程安全，环形保留最近 N 条）
type Store struct {
	mu      sync.RWMutex
	entries []Entry
	max     int
	nextID  int64
}

// NewStore 创建日志存储，maxEntries 为内存保留上限（默认 5000）。
func NewStore(maxEntries int) *Store {
	if maxEntries <= 0 {
		maxEntries = 5000
	}
	return &Store{entries: make([]Entry, 0, 1024), max: maxEntries}
}

// Add 追加一条日志（自动填 ID / 时间戳）。
func (s *Store) Add(level, channel, message string) {
	s.AddEntry(Entry{
		Level:   level,
		Channel: channel,
		Message: message,
	})
}

// AddEntry 追加完整日志条目。
func (s *Store) AddEntry(e Entry) {
	now := time.Now()
	e.ID = 0 // 由内部统一分配
	if e.Time.IsZero() {
		e.Time = now
	}
	e.TimeStr = e.Time.Format("15:04:05")
	s.mu.Lock()
	s.nextID++
	e.ID = s.nextID
	s.entries = append(s.entries, e)
	if len(s.entries) > s.max {
		// 丢弃最旧的一半，保持容量
		keep := s.max / 2
		copy(s.entries, s.entries[len(s.entries)-keep:])
		s.entries = s.entries[:keep]
	}
	s.mu.Unlock()
}

// Query 分页查询（按时间倒序）。
// levels 为空表示全部级别；channel 为空表示全部通道；keyword 匹配 path/message/method。
// 手机端通道映射：lan→http/agg/net/hotspot，bt→ble。
func (s *Store) Query(levels []string, channel, keyword string, page, size int) map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filtered := make([]Entry, 0, len(s.entries))
	for i := len(s.entries) - 1; i >= 0; i-- { // 倒序：最新在前
		e := s.entries[i]
		if len(levels) > 0 && !contains(levels, e.Level) {
			continue
		}
		if channel != "" && !channelMatch(e.Channel, channel) {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(e.Path+e.Message+e.Method), strings.ToLower(keyword)) {
			continue
		}
		filtered = append(filtered, e)
	}
	total := len(filtered)
	// 分页
	start := (page - 1) * size
	end := start + size
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	pageData := filtered[start:end]
	// 序列化副本（避免调用方改内部状态）
	out := make([]Entry, len(pageData))
	copy(out, pageData)
	return map[string]interface{}{
		"list":  out,
		"total": total,
		"page":  page,
		"size":  size,
	}
}

// Clear 清空全部日志。
func (s *Store) Clear() error {
	s.mu.Lock()
	s.entries = s.entries[:0]
	s.mu.Unlock()
	return nil
}

// ExportText 导出为纯文本（log.txt）。
func (s *Store) ExportText() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	lines := make([]string, 0, len(s.entries))
	for _, e := range s.entries {
		lines = append(lines, fmt.Sprintf("%s [%s] [%s] %s %s %d %dms %s",
			e.TimeStr, e.Level, e.Channel, e.Method, e.Path, e.Status, e.Ms, e.Message))
	}
	return []byte(strings.Join(lines, "\n"))
}

// Recent 返回最近 n 条（仪表盘"最近日志"）。
func (s *Store) Recent(n int) []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if n <= 0 || n > len(s.entries) {
		n = len(s.entries)
	}
	out := make([]Entry, 0, n)
	for i := len(s.entries) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, s.entries[i])
	}
	return out
}

// Stats 返回当前日志数量（供状态栏展示）。
func (s *Store) Stats() (total int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

// ---------- 内部工具 ----------

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// channelMatch 通道匹配（支持手机端 lan/bt 宽泛匹配）。
func channelMatch(actual, want string) bool {
	if actual == want {
		return true
	}
	switch want {
	case "lan":
		return actual == "http" || actual == "agg" || actual == "net" || actual == "hotspot"
	case "bt":
		return actual == "ble"
	}
	return false
}

// 保持 sort 引用（未来按 ms/level 排序时使用）
var _ = sort.Slice
