// Package main 个人移动聚合网关 - Windows 服务端入口。
//
// 启动方式（开发期，在 gateway 源码目录下）：
//
//	cd gateway
//	go build -o 聚合网关.exe .
//	聚合网关.exe -port 8080 -demo
//
// 访问：
//   管理界面（本机）：       http://127.0.0.1:8080/admin
//   健康检查：              http://127.0.0.1:8080/api/health
//   手机（连接电脑热点后）： http://<电脑局域网IP>:8080/admin
//
// 手机为瘦客户端：通过浏览器访问电脑网关，无需安装任何 App。
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
	webFlag := flag.String("web", "", "管理界面静态目录（为空则不托管页面）")
	flag.Parse()

	// ---------- 目录初始化 ----------
	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("[main] 创建数据目录失败: %v", err)
	}
	snapshotDir := filepath.Join(*dataDir, "stats")
	webRoot := *webFlag

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
		Stats:  col,
		Config: cm,
		Logs:   ls,
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
	log.Printf("[main] 监听地址: http://0.0.0.0:%d  (本机: http://127.0.0.1:%d/admin)", *port, *port)
	log.Printf("[main] 局域网访问（手机/其他设备，连接电脑热点后）: http://<电脑局域网IP>:%d/admin", *port)

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.Serve(ln); err != nil {
		log.Fatalf("[main] HTTP 服务异常退出: %v", err)
	}
}
