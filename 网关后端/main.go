// Package main 个人移动聚合网关 - 服务端入口。
//
// 启动方式（开发期，在网关后端源码目录下）：
//
//	cd /d E:\API聚合\网关后端
//	go mod tidy && go build -o gateway.exe .
//	gateway.exe -port 8080 -demo
//
// 访问：
//   管理界面（本机 PC 调试）： http://127.0.0.1:8080/admin
//   健康检查：                http://127.0.0.1:8080/api/health
//
// 手机部署：
//   编译 GOOS=android（后续阶段接入，此处先在 PC 上联调 API 与前端）。
package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"mobile-gateway/internal/config"
	"mobile-gateway/internal/httpapi"
	"mobile-gateway/internal/logs"
	"mobile-gateway/internal/seed"
	"mobile-gateway/internal/stats"
)

// version 版本号（与 UI 关于页一致）
const version = "v0.9.1"

func main() {
	// ---------- 命令行参数 ----------
	port := flag.Int("port", 8080, "监听端口")
	demo := flag.Bool("demo", true, "启用演示数据（联调阶段默认开启）")
	dataDir := flag.String("data", "./data", "配置与快照数据目录")
	flag.Parse()

	// ---------- 目录初始化 ----------
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("[main] 创建数据目录失败: %v", err)
	}
	snapshotDir := filepath.Join(*dataDir, "stats")
	webRoot := *flag.String("web", "", "管理界面静态目录（为空则不托管页面）")

	// ---------- 核心模块 ----------
	// 配置管理
	cm, err := config.NewManager(*dataDir)
	if err != nil {
		log.Fatalf("[main] 初始化配置失败: %v", err)
	}
	// 日志存储（内存保留 5000 条）
	ls := logs.NewStore(5000)
	// 统计收集器（今日数据 + 快照）
	col := stats.NewCollector(os.Getpid(), snapshotDir)
	go col.RunSnapshotLoop(nil) // TODO: 传入可关闭的 stop channel

	// 演示数据（联调阶段）
	if *demo {
		seed.Seed(col, cm, ls)
		log.Printf("[main] 已注入演示数据（demo 模式），正式使用时请加 -demo=false")
	}

	// ---------- API 层 ----------
	h := &httpapi.Handler{
		Stats: col,
		Config: configAdapter{cm: cm},
		Logs:  logAdapter{ls: ls},
		// Proxy / Agg 在路由转发与聚合编排模块完成后接入
	}

	mux := http.NewServeMux()
	h.Register(mux, "/api", webRoot)

	// 附加系统接口：健康检查 + 管理界面快捷入口
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"code":0,"message":"ok","data":{"version":"` + version + `","running":true}}`))
	})

	// ---------- 监听 0.0.0.0 ----------
	addr := "0.0.0.0:" + strconv.Itoa(*port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[main] 监听 %s 失败: %v", addr, err)
	}
	log.Printf("[main] 个人移动聚合网关 %s 已启动", version)
	log.Printf("[main] 监听地址: http://0.0.0.0:%d  (局域网访问: http://<手机IP>:%d)", *port, *port)
	log.Printf("[main] 管理界面: http://127.0.0.1:%d/admin", *port)
	log.Printf("[main] 仪表盘数据: http://127.0.0.1:%d/api/stats?range=24h", *port)

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.Serve(ln); err != nil {
		log.Fatalf("[main] HTTP 服务异常退出: %v", err)
	}
}

// ---------- 适配器：将具体模块适配为 httpapi 的接口 ----------

// configAdapter 适配 config.Manager → httpapi.ConfigProvider
type configAdapter struct{ cm *config.Manager }

func (a configAdapter) Get() interface{}                        { return a.cm.Get() }
func (a configAdapter) Update(c interface{}) error               { return a.cm.Update(c.(*config.Config)) }
func (a configAdapter) ImportJSON(data []byte) error             { return a.cm.ImportJSON(data) }
func (a configAdapter) ExportJSON() ([]byte, error)              { return a.cm.ExportJSON() }
func (a configAdapter) Reset() error                             { return a.cm.Reset() }
func (a configAdapter) ListRoutes() interface{}                  { return a.cm.ListRoutes() }
func (a configAdapter) AddRoute(r *config.Route) (interface{}, error) {
	return a.cm.AddRoute(*r)
}
func (a configAdapter) UpdateRoute(id string, r *config.Route) (interface{}, error) {
	return a.cm.UpdateRoute(id, *r)
}
func (a configAdapter) DeleteRoute(id string) error              { return a.cm.DeleteRoute(id) }
func (a configAdapter) SetRouteEnabled(id string, e bool) error  { return a.cm.SetRouteEnabled(id, e) }
func (a configAdapter) ListAggregates() interface{}              { return a.cm.ListAggregates() }
func (a configAdapter) AddAggregate(x *config.Aggregate) (interface{}, error) {
	return a.cm.AddAggregate(*x)
}
func (a configAdapter) UpdateAggregate(id string, x *config.Aggregate) (interface{}, error) {
	return a.cm.UpdateAggregate(id, *x)
}
func (a configAdapter) DeleteAggregate(id string) error          { return a.cm.DeleteAggregate(id) }
func (a configAdapter) SetAggregateEnabled(id string, e bool) error {
	return a.cm.SetAggregateEnabled(id, e)
}

// logAdapter 适配 logs.Store → httpapi.LogsProvider
type logAdapter struct{ ls *logs.Store }

func (a logAdapter) Query(levels []string, channel, keyword string, page, size int) interface{} {
	return a.ls.Query(levels, channel, keyword, page, size)
}
func (a logAdapter) Clear() error              { return a.ls.Clear() }
func (a logAdapter) ExportText() []byte        { return a.ls.ExportText() }
