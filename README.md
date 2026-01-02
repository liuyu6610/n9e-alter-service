# n9e-alter-service

Go 版 N9E 告警处理服务：主动拉取 N9E 当前告警 -> 本地状态机（active/recovered）-> snapshot 落盘 -> 可配置路由聚合/留观/节流/恢复通知（钉钉）-> cron 日报。

## 功能概览

- 主动拉取：按 `pull.interval_seconds` 拉取 `/api/n9e/alert-cur-events/list`
- 聚合去重：按 route 配置生成 `service_hash`（优先 N9E `event.hash`，缺失时使用 fallback 字段组合）
- 状态机：active/recovered；`recover_miss_count=1`（下一次拉取消失即恢复）
- 落盘：`state.snapshot_file`（默认 `data/state.json`）
- 通知：留观 `observe_seconds` + 重复间隔 `repeat_interval_seconds` + 恢复通知 `send_recovered`
- 日报：每 route 配置 cron（5 段）发送；发送后默认 `clear_mode=reset_notified`（仅清理今日发送标记）
- Web UI：Vue3 + Element Plus（工程化构建产物由后端托管）

## 目录结构

- `main.go`：服务入口
- `internal/engine`：拉取/路由匹配/生成 service_hash
- `internal/state`：状态机与 snapshot
- `internal/workers`：通知与日报后台任务
- `web/`：Vue3 + Vite 前端工程，构建产物输出到 `web/dist`

## 运行（本地）

1) 准备配置：复制 `config.example.json` 为 `config.json`，并补齐以下最少字段：

- `n9e.base_url`
- `n9e.user_token`（或 `n9e.authorization`）

1) （可选）构建前端静态文件（用于访问 Web UI；若只用 API 可跳过）：

```bash
cd web
npm install
npm run build
```

1) 启动：

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

- `POST /api/v1/events/ingest` 的鉴权由 `push.token` 控制（见 `config.local.json`）；该 endpoint 不受全局 `api_token` 影响，便于本地联调。
- 你也可以在 Web UI 的 Settings 页面修改 routes/silences 并发布，以验证热更新与回滚。

### 常用 API

- `GET /healthz`
- `GET /readyz`
- `GET /api/v1/status`
- `POST /api/v1/pull/run`
- `GET /api/v1/routes`
- `GET /api/v1/alerts?status=active|recovered&route=&offset=&limit=`
- `GET /api/v1/alerts/get?hash=`

## 前端开发/构建

见 `web/README.md`。

## Docker

### 构建镜像

在项目根目录：

```bash
docker build -t n9e-alter-service:local .
```

### 运行容器

推荐挂载配置与数据目录：

```bash
docker run --rm -p 8080:8080 \
  -v $(pwd)/config.json:/app/config.json:ro \
  -v $(pwd)/data:/app/data \
  n9e-alter-service:local
```

默认启动命令等同于：

```bash
/app/n9e-alter-service -config /app/config.json
```

你也可以不挂载配置文件，改用环境变量（见 `internal/config/config.go` 中支持的 env 列表）。

## Kubernetes 部署示例

本服务支持两种配置注入方式：

- **ConfigMap 挂载配置文件**：通过 `-config /app/config/config.json` 读取。
- **环境变量覆盖**：`SERVICE_ADDR/WEB_DIR/DATA_DIR/N9E_*` 等环境变量会覆盖配置文件中的对应字段。

建议：

- `n9e.user_token` / `n9e.authorization` 属于敏感信息，生产上建议使用 **Secret** 注入（示例中为简化使用 ConfigMap）。

### 1) ConfigMap（config.json）

将你自己的 `config.json` 内容写入 ConfigMap：

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: n9e-alter-service-config
data:
  config.json: |
    {
      "addr": ":8080",
      "web_dir": "/app/web/dist",
      "data_dir": "/app/data",
      "n9e": {
        "base_url": "https://nightingale.example.com",
        "api_path": "/api/n9e/alert-cur-events/list",
        "user_token": "",
        "authorization": "",
        "timeout_seconds": 10,
        "verify_tls": true
      },
      "pull": { "interval_seconds": 30, "page_limit": 200, "max_pages": 2000, "my_groups": false, "hours": 0, "stime": 0, "etime": 0, "query": "", "severity": "", "prods": "", "rule_prods": "", "cate": "", "rid": 0, "event_ids": "" },
      "state": { "snapshot_file": "/app/data/state.json", "snapshot_interval_seconds": 30, "retain_recovered_seconds": 86400, "recover_miss_count": 1 },
      "dingtalk": { "webhook": "", "secret": "", "keyword": "" },
      "routes": []
    }
```

### 2) （可选）PVC（持久化 snapshot）

如果你希望 `state.json` 持久化，建议挂载 PVC 到 `/app/data`：

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: n9e-alter-service-data
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 2Gi
```

### 3) Deployment + Service

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: n9e-alter-service
spec:
  replicas: 1
  selector:
    matchLabels:
      app: n9e-alter-service
  template:
    metadata:
      labels:
        app: n9e-alter-service
    spec:
      containers:
        - name: app
          image: n9e-alter-service:local
          imagePullPolicy: IfNotPresent
          args: ["-config", "/app/config/config.json"]
          ports:
            - name: http
              containerPort: 8080
          env:
            - name: SERVICE_ADDR
              value: ":8080"
            - name: WEB_DIR
              value: "/app/web/dist"
            - name: DATA_DIR
              value: "/app/data"
            # 可选：使用 env 覆盖 N9E 地址/鉴权等
            # - name: N9E_BASE_URL
            #   value: "https://nightingale.example.com"
            # - name: N9E_USER_TOKEN
            #   valueFrom:
            #     secretKeyRef:
            #       name: n9e-alter-service-secret
            #       key: user_token
          volumeMounts:
            - name: config
              mountPath: /app/config
              readOnly: true
            - name: data
              mountPath: /app/data
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 10
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /readyz
              port: http
            initialDelaySeconds: 5
            periodSeconds: 5
      volumes:
        - name: config
          configMap:
            name: n9e-alter-service-config
        - name: data
          persistentVolumeClaim:
            claimName: n9e-alter-service-data
            # 不需要持久化可改为：emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: n9e-alter-service
spec:
  selector:
    app: n9e-alter-service
  ports:
    - name: http
      port: 80
      targetPort: http
  type: ClusterIP
```
