# 个人移动聚合网关 · Windows 网关

以 **Windows 电脑**为网关服务器，聚合多个本地端口、第三方 API 与 AI 模型服务到统一端口
和统一管理 API。手机为**瘦客户端**：连接电脑热点后，用浏览器访问电脑网关即可，无需安装 App。

## 目录结构

```
E:\API聚合\
├─ .github\workflows\build.yml   GitHub Actions 云端编译（仅 Windows）
├─ web\
│  ├─ desktop\mobile-gateway-admin.html   电脑端管理界面（正式 UI，唯一管理入口）
│  └─ mobile-preview\gateway-prototype.html 手机端浏览器样式预览（非网关，瘦客户端展示用）
├─ docs\聚合开发.txt               开发进度与决策记录
└─ gateway\                        Go 后端源码（本模块）
   ├─ main.go                     入口：装配 + 启动 HTTP 服务
   ├─ go.mod                      Go module（mobile-gateway）
   ├─ scripts\                    本地辅助脚本（静态校验，非编译）
   └─ internal\
      ├─ stats\      统计模块：请求数/成功率/耗时/token/费用/趋势/排行榜
      ├─ config\     配置管理：设置 + 路由 CRUD + 聚合 CRUD + 导入导出
      ├─ httpapi\    HTTP 管理接口层（/api/*，统一响应包）
      ├─ logs\       日志：内存环形缓冲 + 查询/清空/导出
      ├─ protocol\   AI 协议转换（OpenAI/Claude/Gemini 转换骨架）
      └─ seed\       演示数据（联调阶段让仪表盘有数据可看）
```

## 为什么不在本地装 Go？

本项目设计为**不依赖本地 Go 工具链**，编译统一交给 **GitHub Actions 云端**完成。
你在本地只做「写代码 → 推送到 GitHub → Actions 自动编译 → 下载 exe」。

已完成文档化迁移（提交记录中保留惊喜）：
- 放弃 Android APK 方案（`android-app/` 已从工作树移除）。
- 仅保留 Windows amd64 单目标产物（`聚合网关.exe`）。

## 已实现的接口（第一阶段：仪表盘）

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/stats?range=1h\|24h\|7d` | 仪表盘 + 统计页全部数据（token/费用/实时/趋势/日志/排行） |
| GET | `/api/health` | 健康检查 |
| GET/PUT/POST | `/api/config` | 读取 / 保存 / 导入配置 |
| GET/POST | `/api/routes` | 路由列表 / 新建路由 |
| PUT/DELETE/PATCH | `/api/routes/{id}[/enabled]` | 修改 / 删除 / 启停路由 |
| GET/POST | `/api/aggregates` | 聚合列表 / 新建聚合 |
| PUT/DELETE/PATCH | `/api/aggregates/{id}[/enabled]` | 修改 / 删除 / 启停聚合 |
| GET/DELETE | `/api/logs` | 日志分页查询 / 清空 |
| POST | `/api/test/route` | 路由测试（测试弹窗） |
| POST | `/api/test/aggregate?stream=1` | 聚合测试（SSE 流式） |
| GET | `/api/providers/{proto}/models` | 拉取模型列表 |

统一响应：`{ "code": 0, "message": "ok", "data": { ... } }`

## 本地运行（可选，需自行装 Go）

```bash
cd gateway
go build -o 聚合网关.exe .
聚合网关.exe -port 8080 -demo      # -demo 注入演示数据
聚合网关.exe -demo=false           # 关闭演示数据
```

启动后：
- 管理界面（当前阶段 `-web` 为空时不托管页面）：仅提供 API。
- 本机访问：`http://127.0.0.1:8080/admin`
- 手机访问（连接电脑热点后）：`http://<电脑局域网IP>:8080/admin`

> 当前阶段 `-web` 参数为空，不托管前端 HTML；后续把 `web/desktop/mobile-gateway-admin.html`
> 复制为静态目录即可（或在启动时传入 `-web web/desktop`）。