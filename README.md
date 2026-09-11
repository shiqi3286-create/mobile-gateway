# 个人移动聚合网关 · 后端

Android 个人移动聚合网关的 Go 服务端（第一阶段：仪表盘 `GET /api/stats` 闭环）。

## 目录结构

```
E:\API聚合\
├─ .github\workflows\build.yml   GitHub Actions 云端编译（无需本地安装 Go）
├─ win界面设计\                    PC 端管理界面（UI 设计，前端原型）
├─ 手机界面设计\                   手机端界面（UI 设计，前端原型）
└─ 网关后端\                       Go 后端源码（本模块）
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

本仓库提供两个本地辅助脚本（用 Node.js 运行，node 已内置）：
```bash
cd 网关后端
node scripts/verify_go.js        # 静态检查：模块名/导入路径/括号平衡（不编译）
```

## 已实现的接口（第一闭环：仪表盘）

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
cd 网关后端
go build -o gateway.exe .
gateway.exe -port 8080 -demo      # -demo 注入演示数据
gateway.exe -demo=false           # 关闭演示数据
```
启动后：
- 管理界面（暂未托管页面）：仅提供 API。浏览器访问 API 接口可看 JSON。
- 仪表盘数据：`http://127.0.0.1:8080/api/stats?range=24h`

> 当前阶段 `-web` 参数为空，不托管前端 HTML；后续把 `win界面设计` 复制为静态目录即可。