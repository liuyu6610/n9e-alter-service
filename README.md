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
- 记录首次出现/最后出现/缺失次数、**每个通知通道**的最后成功时间（webhook / 机器人），以及升级通知进度
- snapshot 定期落盘（建议集群挂 PVC）

1) Workers

- notify：按 route 聚合并发送（支持 silence、repeat interval、escalation）。多通道时按通道记账：成功的通道不会在 repeat 窗口内重发，失败的通道会继续重试。
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

热开启（规则发布把 `push.enabled` 从 false 改为 true）会立刻拉起 ingest worker。若 worker 尚未运行（例如进程启动时未调用 `Start`），`POST /api/v1/events/ingest` **拒绝入队**（HTTP 503 / `push ingest has no workers`），避免事件进入没有消费者的队列。

### 状态与 Redis（state）

- `state.snapshot_file`：快照文件（建议集群挂 PVC）
- `state.snapshot_interval_seconds`：快照间隔
- `state.retain_recovered_seconds`：保留 recovered 的时间
- `state.recover_miss_count`：缺失多少轮 pull 才判定恢复

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

## HTTP API 与规则版本库

### 鉴权

- 全局：`/api/*` 需要非空的 `api_token`（`X-Token` 或 `Authorization: Bearer`）。`api_token` 为空时返回 401，而不是关闭鉴权。
- `POST /api/v1/events/ingest` 使用独立的 `push.token`（同样不允许空 token 公开访问）。`/healthz`、`/readyz` 与静态资源不鉴权。

### 规则版本库 API（核心）

- `GET /api/v1/rules/current`：当前生效规则（敏感字段会脱敏）
- `POST /api/v1/rules/publish`：发布一份新规则（原子写入 + 生成 version/hash + audit）。请求体里若仍是 GET 脱敏占位符，会保留当前真实密钥，不会写回 `current.json`。
- `POST /api/v1/rules/rollback`：回滚到历史 version（会写 audit）
- `GET /api/v1/rules/versions`：版本列表
- `GET /api/v1/rules/version?version=...`：获取某个版本
- `GET /api/v1/rules/audits?limit=...`：审计列表

发布/回滚会触发 runtime 热更新（Cfg/Engine/Redis client 等），无需重启。

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

### 1) 获取当前规则

```bash
curl -s http://127.0.0.1:8080/api/v1/rules/current
```

PowerShell：

```powershell
Invoke-RestMethod -Method Get -Uri http://127.0.0.1:8080/api/v1/rules/current
```

### 2) 发布规则（publish）

说明：请求体包含 `rules`（完整 RuleSet）、`message`（变更说明）、`actor`（操作者）。

```bash
curl -s -X POST http://127.0.0.1:8080/api/v1/rules/publish \
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
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8080/api/v1/rules/publish -ContentType 'application/json' -Body ($payload | ConvertTo-Json -Depth 20)
```

> 实战建议：优先在 Web UI 的 Settings 页面修改并发布，它会生成完整 RuleSet（避免手写字段遗漏）。

### 3) 查看版本列表与审计

```bash
curl -s http://127.0.0.1:8080/api/v1/rules/versions
curl -s 'http://127.0.0.1:8080/api/v1/rules/audits?limit=50'
```

### 4) 回滚（rollback）

```bash
curl -s -X POST http://127.0.0.1:8080/api/v1/rules/rollback \
  -H 'Content-Type: application/json' \
  -d '{"version":"20260102-xxxxxx","message":"rollback","actor":"sre"}'
```

PowerShell：

```powershell
$payload = @{ version = '20260102-xxxxxx'; message = 'rollback'; actor = 'sre' }
Invoke-RestMethod -Method Post -Uri http://127.0.0.1:8080/api/v1/rules/rollback -ContentType 'application/json' -Body ($payload | ConvertTo-Json)
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
  - 多通道时看 snapshot 里的 `channel_notified`：某个通道成功不会再把其它失败通道标成已通知

- **只收到 escalation 不收到常规 webhook**：
  - 旧版本逻辑会要求至少一个 robot；当前版本已支持 webhook-only route

- **中文乱码**：
  - webhook 发送已使用 `application/json; charset=utf-8`

- **push 返回 401**：
  - 检查 `push.token` 与请求头 `X-Token` 是否一致

- **push 热开启后事件入队但不消费**：
  - 规则发布启用 push 会拉起 worker；若仍无 worker，ingest 返回 503 而不是默默堆积

- **pull skipped**：
  - 未配置 `n9e.base_url` 属正常（本地仅 push 时可忽略）

## 前端开发/构建

见 `web/README.md`。
