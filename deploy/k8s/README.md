# Kubernetes 部署（ClusterIP + Ingress）

本目录提供一套可直接套用的示例清单（偏生产默认）：

- `00-namespace.yaml`
- `10-configmap.yaml`：非敏感配置（建议仅保留结构，敏感字段用 Secret/env 覆盖）
- `11-secret.yaml`：敏感信息（API_TOKEN/PUSH_TOKEN/N9E token/Redis password）
- `20-pvc.yaml`：持久化 `/app/data`（包含 state snapshot 与规则版本库）
- `30-deployment.yaml`：含 `securityContext` 最佳实践（runAsNonRoot/readOnlyRootFilesystem/cap drop/seccomp）
- `40-service.yaml`：ClusterIP
- `50-ingress.yaml`：Ingress（示例按 nginx-ingress 写，按你集群实际调整 class/annotations/TLS）
- `60-pdb.yaml`：单副本场景保护（minAvailable=1）
- `70-hpa.yaml`：默认 maxReplicas=1（避免多副本状态一致性问题）；如要扩容请先评估状态模型

## 快速部署

```bash
kubectl apply -f deploy/k8s/00-namespace.yaml
kubectl apply -f deploy/k8s/10-configmap.yaml
kubectl apply -f deploy/k8s/11-secret.yaml
kubectl apply -f deploy/k8s/20-pvc.yaml
kubectl apply -f deploy/k8s/30-deployment.yaml
kubectl apply -f deploy/k8s/40-service.yaml
kubectl apply -f deploy/k8s/50-ingress.yaml
kubectl apply -f deploy/k8s/60-pdb.yaml
# HPA 可选
kubectl apply -f deploy/k8s/70-hpa.yaml
```

## TLS（无 cert-manager 场景）

如果你的集群没有 cert-manager，Ingress 的 `spec.tls[].secretName` 需要你手工创建。

1) 推荐命令方式创建（避免手写 base64）：

```bash
kubectl -n n9e-alter-service create secret tls n9e-alter-service-tls \
  --cert=/path/to/tls.crt \
  --key=/path/to/tls.key
```

1) 或者使用 YAML 模板：`deploy/k8s/52-tls-secret.yaml`（把 `tls.crt/tls.key` 填成 base64）。

注意：

- `deploy/k8s/50-ingress.yaml` 里的 `host` 与 `tls.hosts` 要和你的域名一致。
- `secretName` 要和你创建的 TLS secret 名称一致。

## 注意事项（重要）

- 建议先保持 `replicas=1`。
- 如果开启 `readOnlyRootFilesystem: true`，必须挂载 `emptyDir` 到 `/tmp`（Deployment 已包含）。
- N9E/Push/API token 等敏感字段必须走 Secret。
