# Web (Vue3 + Vite + Element Plus)

## 本地开发

1) 先启动后端（默认 8080）：

```bash
go run . -config ../config.example.json
```

2) 启动前端 dev server（会代理后端 `/api` / `/healthz` / `/readyz`）：

```bash
npm install
npm run dev
```

打开：`http://127.0.0.1:5173/`

## 构建产物

```bash
npm run build
```

产物输出到：`web/dist/`，后端默认 `web_dir=web/dist` 会直接托管该目录。
