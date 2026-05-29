# QNAP 监控面板

独立的 QNAP NAS 监控工具，提供长期历史数据和温度告警，弥补 QTS 自带资源监控只有实时数据的不足。

## 功能

- **指标**：CPU 使用率、内存使用率、系统温度、卷总量/已用量/使用率
- **采集**：默认 10 秒一次，可在 Web 界面调整
- **存储**：SQLite 单文件，默认保留 30 天，每小时自动清理过期数据
- **历史曲线**：1h / 6h / 24h / 7d / 30d 五档，自动按时间范围聚合（避免一次返回 10 万个点）
- **告警**：温度超过阈值（默认 55°C）→ 页面红色横幅 + 一次性弹窗 + 企业微信 Webhook 推送；恢复时同样通知一次。状态机去重，不会刷屏
- **配置**：所有连接信息在 Web 界面动态修改，密码 AES-256-GCM 加密落盘

## 技术栈

| 端 | 技术 |
|---|---|
| 后端 | Go 1.22 / chi / modernc.org/sqlite（纯 Go，无 CGO）|
| 前端 | React 18 / TypeScript / Vite / TanStack Query / Recharts / Tailwind |
| 部署 | 单 Docker 镜像（前端 dist 编译时 embed 进 Go 二进制）|

## 快速开始

### Docker（推荐）

```bash
docker compose up -d --build
```

访问 `http://localhost:8080`，第一次打开会提示去 **设置** 页面填写：
1. QNAP URL（含协议+端口，如 `http://192.168.1.10:8080`）
2. 用户名 / 密码
3. （可选）企业微信 Webhook URL

点 **测试连接** 验证凭据，**保存** 后回仪表盘等 10 秒看到第一条数据。

### 本地开发

后端：
```bash
cd backend
go run ./cmd/server -addr :8080 -data ./data
```

前端（另开终端）：
```bash
cd frontend
npm install
npm run dev    # http://localhost:5173, 自动代理 /api 到 :8080
```

## 数据 / 配置位置

容器卷 `./data/`，包含：
- `monitor.db` — SQLite 数据库（指标、告警、配置）
- `key.bin` — 32 字节 AES 密钥，首次启动自动生成，**请勿删除**（否则保存的 QNAP 密码无法解密）

## QNAP 接口说明

通过 QTS 的内部 CGI 接口（无需开启 SSH/SNMP）：

- `POST /cgi-bin/authLogin.cgi`（密码 base64 编码）→ 拿 session id
- `GET /cgi-bin/management/manaRequest.cgi?subfunc=sysinfo&hd=no&multicpu=1&sid=...` → CPU/内存/温度
- `GET /cgi-bin/management/chartReq.cgi?chart_func=disk_usage&disk_select=all&include=all&sid=...` → 卷信息

session 内存缓存，遇到 404 或 `authPassed=0` 自动重新登录一次。

## API

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/status/current` | 最近一次采集 + 告警状态 |
| GET | `/api/metrics?from=&to=&bucket=raw\|1m\|5m\|30m\|1h` | 历史数据 |
| GET / PUT | `/api/config` | 配置读写（密码字段不返回明文）|
| POST | `/api/config/test` | 用提交的凭据试登（不持久化）|
| GET | `/api/alerts?limit=50` | 告警历史 |
| POST | `/api/alerts/:id/ack` | 标记已读 |

## 数据量估算

10s/次 × 86400/10 ≈ 8640 行/天，30 天约 26 万行，单 SQLite 文件 < 50MB，完全没问题。
若调整为 1s 采集，30 天约 260 万行——仍可接受，但 `metrics` 表查询建议依赖 `idx_metrics_ts` 索引。

## 安全

- QNAP 密码 AES-256-GCM 加密存 `qnap_password_enc` 字段，密钥单独存放
- API 返回 `ConfigView` 不含密码，前端只显示 `passwordSet: true/false`
- **当前没有面板自身的访问鉴权**——务必只暴露在内网或加一层反向代理鉴权（如 nginx basic auth、Authelia 等）

## 测试

```bash
cd backend
go test ./...
```

覆盖：
- QNAP 客户端：XML 解析、404 后自动重登
- 告警状态机：连续上穿/恢复只各触发一次
- 存储：按时间范围聚合（温度取 MAX、其他取 AVG）、过期清理
