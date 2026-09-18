# n9e-alter-service

Go 版 N9E 告警处理服务：

- 从 N9E **拉取（pull）**当前告警（或通过 **push** 注入事件）
- 通过可配置的 **路由（routes）/去重（dedup）/处理器（processors）** 做归并与字段变换
- 落入本地 **状态机（active/recovered）**，并定期 **snapshot 落盘**
- 按 route 配置做 **通知（DingTalk / Webhook）**、**Silence（抑制）**、**Escalation（升级）**、**日报（cron）**
- 通过规则仓库实现 **规则发布/审计/版本回滚**，并在运行时热更新（避免重启）

该项目目标是让你可以在集群（Kubernetes）里长期运行，并具备：

- **可维护**：规则版本化、审计、回滚
- **可靠**：热更新、限流/重复间隔、状态落盘
- **安全**：API 鉴权、敏感字段脱敏
- **可观测**：接口访问日志、基础 metrics

> 说明：本仓库提供了完整的本地联调工具（无需 N9E 环境即可 push 造事件验证 webhook/escalation）。

## 目录结构

- `main.go`：服务入口，初始化 runtime / rulesrepo / ingest / notifier 等
- `internal/config`：配置结构体、默认值、`config.json` 加载与 env 覆盖
- `internal/engine`：事件输入构建、routes 匹配、processors 执行、dedup key 生成
- `internal/state`：状态机（active/recovered）、snapshot、升级/通知标记
- `internal/ingest`：push ingest 队列与 worker
- `internal/workers`：通知（notify）与日报（daily report）任务
- `internal/rulesrepo`：规则版本库（publish/audit/rollback/current）
- `internal/runtime`：运行时快照（Cfg+Engine）原子热切换
- `internal/redismgr`：Redis client 热切换（用于热 bucket）
- `internal/server`：HTTP API + Web UI 静态托管
- `web/`：Vue3 + Vite 前端工程，构建产物输出到 `web/dist`

## 架构与数据流（建议先读）

### 数据流

1) 输入事件来源

- **Pull**：周期性从 N9E 拉取当前告警列表
- **Push**：外部直接调用 `POST /api/v1/events/ingest` 注入事件（本地联调强烈推荐）

1) Engine 处理

- match route（根据 group/rule/severity/tags 等）
- processors（drop / relabel / update）
- dedup key / service_hash 生成

1) State 状态机

- active / recovered
- 记录首次出现/最后出现/缺失次数、最后通知时间、升级通知进度
- snapshot 定期落盘（建议集群挂 PVC）
- **recovered 只由成功的 Pull 推进**（连续 miss 达到 `recover_miss_count`）。Push ingest 不会因为批次里缺了某条就恢复它，详见「恢复语义」。

1) Workers

- notify：按 route 聚合并发送（支持 silence、repeat interval、escalation）
- daily report：cron 触发生成日报

### 关键概念

- **Route**：一套匹配 + 去重 + 通知策略的组合。路由名建议全局唯一。
- **Processor**：对事件字段的过滤/改写链。
- **Silence**：按 route + tags/tag_regex 的抑制规则（可设置过期时间）。
- **Escalation**：告警持续 active 一段时间后触发升级通知（可按 step 配置 repeat）。
- **RuleSet**：一份完整规则集（n9e/pull/push/state/silences/routes/robots/bindings/dingtalk）。
- **规则版本库**：publish 生成新 version + audit；rollback 切回历史 version。

## 运行（本地）

### 1) 准备配置

复制 `config.example.json` 为 `config.json`，并补齐最少字段：

- `n9e.base_url`
- `n9e.user_token`（或 `n9e.authorization`）

### 2) （可选）构建前端静态文件

```bash
cd web
npm install
npm run build
```

### 3) 启动

```bash
go run . -config config.json
```

打开：`http://127.0.0.1:8080/`

### 本地联调：Push 造事件 + Webhook 接收（无需 N9E 环境）

项目内置了一套本地联调用的最小配置与脚本：`tools/local-test/`。

1) 启动本地 Webhook 接收器（新开一个 PowerShell 窗口）：

```powershell
./tools/local-test/webhook-receiver.ps1
```

默认监听：`http://127.0.0.1:18080/`，收到的 payload 会落盘到：`tools/local-test/out/`。

1) 启动服务（新开一个 PowerShell 窗口）：

```powershell
./tools/local-test/start-service.ps1
```

该脚本会使用：`tools/local-test/config.local.json`（已启用 `push.enabled=true` 与示例 webhook/escalations）。

1) Push 造事件（第三个 PowerShell 窗口）：

```powershell
./tools/local-test/push-event.ps1
```

（可选）一键验收（自动等待 /webhook 与 /escalation 落盘，并检查 active 告警字段更新）：

```powershell
./tools/local-test/verify.ps1
```

正常情况下：

- `POST /api/v1/events/ingest` 返回 `{"ok":true,"accepted":1,...}`
- `webhook-receiver.ps1` 会打印并保存 webhook 请求体

说明：

- `POST /api/v1/events/ingest` 使用 `push.token` 鉴权（见 `config.local.json`）。`push.token` 为空时拒绝请求，不会公开开放。
- 你也可以在 Web UI 的 Settings 页面修改 routes/silences 并发布，以验证热更新与回滚。

### 常用 API

- `GET /healthz`：存活探针
- `GET /readyz`：就绪探针
- `GET /api/v1/status`：运行状态摘要
- `GET /api/v1/metrics`：服务指标（JSON）
- `POST /api/v1/pull/run`：立即触发一次 pull
- `POST /api/v1/events/ingest`：push 注入事件（本地联调用）
- `POST /api/v1/routes/preview`：预览 routes 匹配结果
- `GET /api/v1/routes`：查看当前 routes
- `GET /api/v1/alerts?status=active|recovered&route=&offset=&limit=`：告警列表
- `GET /api/v1/alerts/get?hash=`：告警详情

## 配置说明（config.json / RuleSet）

### 全局字段

- `addr`：监听地址（默认 `:8080`）
- `web_dir`：前端静态文件目录（默认 `web/dist`，容器内推荐 `/app/web/dist`）
- `data_dir`：数据目录（规则版本库、快照等）（容器内推荐 `/app/data`）
- `api_token`：API 鉴权 token（对 `/api/*` 生效；空值不会关闭鉴权，未配置时 `/api/*` 返回 401。ingest 使用 `push.token`）

### N9E 拉取相关（n9e / pull）

- `n9e.base_url`：N9E 地址，例如 `https://nightingale.example.com`
- `n9e.api_path`：默认 `/api/n9e/alert-cur-events/list`
- `n9e.user_token`：N9E 用户 token（敏感字段）
- `n9e.authorization`：备用（例如 Bearer token）
- `n9e.timeout_seconds`：HTTP 超时
- `n9e.verify_tls`：是否校验证书

- `pull.interval_seconds`：拉取间隔
- 其他 pull 字段用于筛选/分页（见 `config.example.json`）

### Push 注入（push）

- `push.enabled`：是否启用 push ingest
- `push.token`：push 鉴权 token（敏感字段）
- `push.queue_size` / `push.worker_count`：吞吐相关
- `push.enqueue_timeout_milli`：入队超时

### 状态与 Redis（state）

- `state.snapshot_file`：快照文件（建议集群挂 PVC）
- `state.snapshot_interval_seconds`：快照间隔
- `state.retain_recovered_seconds`：保留 recovered 的时间（默认 86400；`<=0` 时按 86400 处理）
- `state.recover_miss_count`：连续多少轮 **Pull** 未再看到该告警才判定恢复（见下文「恢复语义」；`<=0` 时按 1 处理）

Redis 仅用于高并发去重/聚合热 bucket（不用于持久化）：

- `state.redis.enabled`：启用后会使用 redis 存储 bucket
- `state.redis.addr/password/db/key_prefix`：连接信息（password 为敏感字段）
- `state.redis.hot_ttl_seconds`：热 bucket TTL

### Silences（silences）

数组，每个元素：

- `name`：名称
- `enabled`：是否启用
- `route_name`：可选，指定路由
- `tags`：精确匹配（key/value）
- `tag_regex`：正则匹配（key/pattern）
- `expires_at_unix`：过期时间（unix seconds）

### Routes（routes）

routes 是核心配置，每条 route 包含：

- `match`：匹配条件
- `dedup`：去重策略
- `processors`：处理器链（可选）
- `notify`：通知策略（支持 webhook + escalation）
- `daily_report`：日报策略

`notify.webhook`：

- `enabled` / `url` / `timeout_seconds` / `headers`
- 仅配置 webhook、没有机器人也会发送（webhook-only route 支持）

`notify.escalations`：数组，每个元素：

- `after_seconds`：告警 active 持续多久后触发
- `repeat_interval_seconds`：升级重复间隔
- `robot_ids`：升级通知要发送的机器人列表（可为空）
- `webhook`：升级 webhook（可与常规 webhook 不同）

### DingTalk / Robots / Bindings

- `dingtalk`：全局默认钉钉配置
- `robots`：机器人列表（支持 fallback）
- `bindings`：绑定规则（按 group/rule/tags/tag_regex 等匹配机器人）

## 鉴权（api_token / push.token）

空 token **不会**关闭鉴权。未配置或为空时，对应接口返回 **401**。探针与静态 UI 除外。

| 范围 | Token | 请求头 | 空值行为 | 配置 / 环境变量 |
| --- | --- | --- | --- | --- |
| 除 ingest 外的 `/api/*` | `api_token` | `X-Token` 或 `Authorization: Bearer` | **401**（不是“关闭鉴权”） | `api_token` / `API_TOKEN` |
| `POST /api/v1/events/ingest` | `push.token` | 同上 | **401**（即使 `api_token` 有值也不放行） | `push.token` / `PUSH_TOKEN` |
| `/healthz`、`/readyz`、Web UI 静态文件 | 无 | — | 不鉴权 | — |

要点：

- **两个 token 互相独立**：ingest 只认 `push.token`，其它 API 只认 `api_token`。用 `api_token` 调 ingest、或用 `push.token` 调 `/api/v1/status` 都会 401。
- **Web UI Settings 页的 “API Token”** 只写入浏览器 `localStorage`，给前端请求带头；**不会**改服务端 `api_token`。服务端 token 来自 `config.json` / env，不在 RuleSet 里，publish 无法热更新它。
- `push.token` **在** RuleSet 里，可通过 Settings 发布热更新。发布时若 GET 回来的是脱敏占位符，会按下一节还原，不会把 `***` 写成真实 token。
- 请求头以外的位置（query、body）**不**作为凭据。Access log 只记 path，不记 query。
- `X-User-Token` 只用于本服务访问 N9E（`n9e.user_token`），与本服务 HTTP API 鉴权无关。

## 规则发布与敏感字段还原

`GET /api/v1/rules/current` 与 `GET /api/v1/rules/version` 返回的 RuleSet **会脱敏**。Settings 页再把这份 JSON `POST /api/v1/rules/publish` 时，服务端会在写入 `data/rules/current.json` 之前，把占位符还原成当前运行时密钥。

脱敏规则：

- 长度 `<= 6`：变成 `***`
- 更长：保留首 2 位与末 2 位，中间为 `***`（例如 `n9e-user-token-real` → `n9***al`）

还原规则（`POST /api/v1/rules/publish`）：

- 字段值是 `***`，或等于当前值的 `maskSecret` 结果 → **保留磁盘/运行时上的真实值**
- 字段值是新的非占位符字符串 → **按新值覆盖**
- 字段值是空字符串 → **按空值写入**（视为显式清空，不会还原）

会还原的字段包括：`n9e.user_token` / `n9e.authorization`、`push.token`、`state.redis.password`、`dingtalk.webhook` / `dingtalk.secret`、route/escalation 的 webhook `url` 与 `headers`、robots 的 `webhook` / `secret`。按 route `name`、robot `id`、escalation 下标对齐。

`api_token` 不在 RuleSet 中，GET/publish 都不触及它。

规则文件（`current.json` 与 versions）按 owner-only（`0600`）写入，避免把还原后的真实密钥暴露给同机其它用户。

## 恢复语义（仅 Pull 判定 recovered）

状态机里的 **recovered 只由 Pull 路径产生**（`Store.ApplyPull`）。Push ingest（`Store.ApplyIngest`）只会把事件标成 **active**，**不会**因为某次 push 批次里没带上该告警就把它恢复。

| 行为 | Pull（N9E 当前告警列表 / `POST /api/v1/pull/run`） | Push（`POST /api/v1/events/ingest`） |
| --- | --- | --- |
| 看到事件 | 置为 active，`miss_count=0` | 置为 active，`miss_count=0` |
| 本轮没看到已有 active | `miss_count++`；达到 `state.recover_miss_count` → **recovered** | **忽略**，保持 active |
| 失败 / 未执行 | `n9e.base_url` 为空会 skip pull；N9E 请求失败不调用 `ApplyPull`，**不会**误恢复 | 入队失败不影响已有状态 |
| 成功但列表为空 | 视为所有 active 都 missing，会按 miss count **恢复** | 空 `[]` 不改变已有告警 |
| recovered 后再出现 | 重新 active（清除 recovered/通知/升级进度） | 同样重新 active |
| 清理 | recovered 超过 `retain_recovered_seconds` 后从 snapshot 删除 | 不清理 |

运维含义：

- **纯 push 联调**（`n9e.base_url` 为空）：pull 被 skip，告警会一直 active，直到你配置了 N9E 并跑成功的 pull，或进程丢状态。不要指望“停止 push”会发出恢复通知。
- **pull + push 混用**：Pull 是恢复的权威来源。只通过 push 注入、从未出现在 N9E 当前告警列表里的记录，下一轮成功 pull 会按 miss 规则恢复。
- 失败的 pull（超时、N9E 5xx）**不会**把全量告警打成 recovered；只有 **成功** 拉到的列表（包括空列表）才会推进 miss/recover。
- `recover_miss_count` 用来抗单次漏拉。默认配置/示例里常见 `1` 或 `2`；代码里 `<=0` 按 `1` 处理。

## HTTP API 与规则版本库

### 规则版本库 API（核心）

- `GET /api/v1/rules/current`：当前生效规则（敏感字段会脱敏）
- `POST /api/v1/rules/publish`：发布一份新规则（还原脱敏占位符 + 原子写入 + version/hash + audit）
- `POST /api/v1/rules/rollback`：回滚到历史 version（会写 audit）
- `GET /api/v1/rules/versions`：版本列表
- `GET /api/v1/rules/version?version=...`：获取某个版本（同样脱敏）
- `GET /api/v1/rules/audits?limit=...`：审计列表

发布/回滚会触发 runtime 热更新（Cfg/Engine/Redis client 等），无需重启。JSON 请求体上限 8MiB。

## 可观测性（Observability）

- Access log：只记录 `/api/` 与异常（>=400）
- `GET /api/v1/metrics`：返回 JSON 形式的统计快照

建议集群侧补齐：

- Pod liveness：`/healthz`
- Pod readiness：`/readyz`
- 日志采集：stdout 结构化字段（method/path/status/dur_ms 等）

## Docker

仓库内置 `Dockerfile`（多阶段构建，包含前端产物），默认：

- 以非 root 用户运行（uid=10001）
- 静态二进制（CGO=0）
- 默认使用 `/app/config.example.json` 作为配置（生产请挂载自己的 `config.json`）

### 构建

```bash
docker build -t n9e-alter-service:local .
```

### 运行（推荐挂载 config 与 data）

```bash
docker run --rm -p 8080:8080 \
  -v $(pwd)/config.json:/app/config.json:ro \
  -v $(pwd)/data:/app/data \
  n9e-alter-service:local -config /app/config.json
```

## Kubernetes 部署（ClusterIP + Ingress）

本仓库已提供一套可直接套用的清单（含 `securityContext` 最佳实践、PDB、Ingress、HPA 可选）：

- `deploy/k8s/`

推荐直接按 `deploy/k8s/README.md` 的顺序 apply。

### K8s 最佳实践要点

- **建议先保持 `replicas=1`**：本地 snapshot/state 不是为多副本一致性设计。
- **只读根文件系统**：`readOnlyRootFilesystem: true` 时需要挂载 `emptyDir` 到 `/tmp`（示例已包含）。
- **数据持久化**：建议 PVC 挂载到 `/app/data`（规则版本库 + state snapshot）。
- **健康检查**：liveness `/healthz`、readiness `/readyz`。

## 配置与敏感信息策略（ConfigMap vs Secret）

建议原则：

- **任何 token/密码/签名密钥都必须用 Secret**（避免泄露与审计合规问题）
- **结构化、可公开的配置用 ConfigMap**（例如 pull 间隔、routes 结构、webhook URL 等视安全要求而定）

### 必须 Secret 的字段（强烈建议）

- `api_token`（或 env `API_TOKEN`）
- `push.token`（或 env `PUSH_TOKEN`）
- `n9e.user_token`（或 env `N9E_USER_TOKEN`）
- `n9e.authorization`（或 env `N9E_AUTHORIZATION`）
- `state.redis.password`（或 env `STATE_REDIS_PASSWORD`）
- 钉钉机器人相关：`dingtalk.secret`、机器人 `secret`

### 可 ConfigMap 的字段（常规）

- `addr/web_dir/data_dir`
- `pull.*`（拉取间隔与筛选）
- `routes/silences/bindings/robots` 的结构配置

### Secret 模板

参考：`deploy/k8s/11-secret.yaml`。

## 运行参数与资源建议（SRE 视角）

### 1) Push（高并发注入）

- `push.worker_count`
  - **CPU 越高/事件处理越重**越需要增加。
  - 建议从 `2` 起步，逐步调到 `CPU 核数` 或 `2*CPU 核数`（以压测与 webhook 速率为准）。

- `push.queue_size`
  - 队列越大，越能抗突发，但也意味着内存占用可能上升。
  - 推荐起步：`20000`（示例值），如果 webhook 下游慢且突发多，可提高到 `100000`，同时要配合资源与丢弃策略。

- `push.enqueue_timeout_milli`
  - 用于控制上游调用的“背压”行为。
  - 生产建议设置为 `30000` 左右，上游自行重试或降级。

### 2) Pull（从 N9E 拉取）

- `pull.interval_seconds`
  - 拉取越频繁，状态越实时，但对 N9E 与本服务 CPU/网络压力越大。
  - 常规建议：`30s` ~ `60s`。

- `pull.page_limit` / `pull.max_pages`
  - 告警量大时这两个参数会直接影响单次拉取数据量与处理耗时。
  - 如果你告警规模较大（例如数万条），建议：
    - 降低 `page_limit`（例如 100~200）
    - 提高 `interval_seconds`（例如 60~120）
    - 或开启 Redis bucket 来降低重复处理成本

### 3) Redis（可选：用于热 bucket）

- 默认可以不开启 Redis（简化部署）。
- 当出现以下情况建议启用：
  - push/pull 事件量很大
  - 需要更强的去重/聚合性能
  - 需要跨重启保留更细的 bucket 行为（注意：Redis 也会丢失，需要你自己保证 Redis HA）

相关 env：

- `STATE_REDIS_ENABLED=true`
- `STATE_REDIS_ADDR`
- `STATE_REDIS_PASSWORD`

### 4) 资源建议（起步）

- requests：`cpu 100m` / `memory 256Mi`
- limits：`cpu 1` / `memory 1Gi`

当你启用大量 webhook、并发 push、或 routes/processors 很复杂时，需要把 CPU/内存上调并做压测。

## 发布/回滚 SOP（运维手册）

规则发布/回滚会触发 runtime 热更新，无需重启。

调用 `/api/*` 时带上 `api_token`（`X-Token` 或 `Authorization: Bearer`）。下面用环境变量 `API_TOKEN`。

### 1) 获取当前规则

```bash
curl -s -H "X-Token: $API_TOKEN" http://127.0.0.1:8080/api/v1/rules/current
```

PowerShell：

```powershell
$headers = @{ 'X-Token' = $env:API_TOKEN }
Invoke-RestMethod -Method Get -Uri http://127.0.0.1:8080/api/v1/rules/current -Headers $headers
```

### 2) 发布规则（publish）

说明：请求体包含 `rules`（完整 RuleSet）、`message`（变更说明）、`actor`（操作者）。

```bash
curl -s -X POST http://127.0.0.1:8080/api/v1/rules/publish \
  -H "X-Token: $API_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"rules": {"n9e": {}, "pull": {}, "push": {}, "state": {}, "silences": [], "routes": [], "robots": [], "bindings": [], "dingtalk": {}}, "message": "change routes", "actor": "sre"}'
```

PowerShell：

```powershell
$payload = @{
  rules = @{
    n9e = @{}
    pull = @{}
    push = @{}
    state = @{}
    silences = @()
    routes = @()
    robots = @()
    bindings = @()
    dingtalk = @{}
  }
  message = 'change routes'
  actor = 'sre'
}
$headers = @{ 'X-Token' = $env:API_TOKEN }
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8080/api/v1/rules/publish -Headers $headers -ContentType 'application/json' -Body ($payload | ConvertTo-Json -Depth 20)
```

> 实战建议：优先在 Web UI 的 Settings 页面修改并发布，它会生成完整 RuleSet（避免手写字段遗漏）。

### 3) 查看版本列表与审计

```bash
curl -s -H "X-Token: $API_TOKEN" http://127.0.0.1:8080/api/v1/rules/versions
curl -s -H "X-Token: $API_TOKEN" 'http://127.0.0.1:8080/api/v1/rules/audits?limit=50'
```

### 4) 回滚（rollback）

```bash
curl -s -X POST http://127.0.0.1:8080/api/v1/rules/rollback \
  -H "X-Token: $API_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"version":"20260102-xxxxxx","message":"rollback","actor":"sre"}'
```

PowerShell：

```powershell
$payload = @{ version = '20260102-xxxxxx'; message = 'rollback'; actor = 'sre' }
$headers = @{ 'X-Token' = $env:API_TOKEN }
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8080/api/v1/rules/rollback -Headers $headers -ContentType 'application/json' -Body ($payload | ConvertTo-Json)
```

### 关键建议

- **Deployment replicas 建议先 1**（本地 snapshot/state 不是为多副本一致性设计）
- `data_dir` 建议挂 PVC（保存 state snapshot 与规则库）
- `api_token` / `push.token` / N9E token 等敏感字段用 Secret 注入

### 资源建议（起步）

- requests：`cpu 100m` / `memory 256Mi`
- limits：`cpu 1` / `memory 1Gi`

### 典型 YAML（示例，需按你的集群规范调整）

README 下半部分保留了 ConfigMap/PVC/Deployment/Service 示例，你可以直接改字段后落地。

## 故障排查（SRE 常用）

- **收不到 webhook**：
  - 检查 route `notify.enabled=true` + `notify.webhook.enabled=true` + `url` 非空
  - 检查 silence 是否命中
  - 检查 `repeat_interval_seconds` 是否导致未到发送窗口
  - 看服务日志：`notify webhook route=... err=...`

- **只收到 escalation 不收到常规 webhook**：
  - 旧版本逻辑会要求至少一个 robot；当前版本已支持 webhook-only route

- **中文乱码**：
  - webhook 发送已使用 `application/json; charset=utf-8`

- **push 返回 401**：
  - 检查 `push.enabled=true`、`push.token` 非空，且请求头 `X-Token` / `Authorization: Bearer` 与 `push.token` 一致（不要用 `api_token`）

- **其它 `/api/*` 返回 401**：
  - 空 `api_token` 现在是拒绝而不是放行。配置 `API_TOKEN` 或 `config.json` 的 `api_token`，请求带头。

- **Settings 发布后密钥变成 `***`**：
  - 当前版本会在 publish 时还原脱敏占位符。若仍被覆盖，确认请求体里不是把密钥改成了空字符串（空值会显式清空）。

- **push 注入的告警一直 active / 没有恢复通知**：
  - 恢复只走 pull。本地未配 `n9e.base_url` 时 pull 会被 skip。混用时，N9E 当前列表里没有的 hash 会在成功 pull 后按 `recover_miss_count` 恢复。

- **pull skipped**：
  - 未配置 `n9e.base_url` 属正常（本地仅 push 时可忽略）

## 测试与 CI

```bash
go test ./...
```

GitHub Actions（`.github/workflows/go-test.yml`）在 push / pull_request 上跑同一条命令。

## 前端开发/构建

见 `web/README.md`。
