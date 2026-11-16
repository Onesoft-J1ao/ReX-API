# ReX API

> 自建的 AI API 聚合网关：把多家上游的模型统一成 OpenAI 兼容接口，附带分组倍率、订阅套餐、下游分发与上游健康度镜像。

本项目 fork 自 [QuantumNous/new-api](https://github.com/QuantumNous/new-api)（AGPL-3.0），在前端交互、计费模型和多上游运营能力上做了较大改造。上游的版权声明、`NOTICE` 与第三方许可清单全部保留。

## 站点与社区

| 入口 | 地址 |
|---|---|
| 站点 | **https://ai.j1ao.vip** |
| 使用文档 | https://docs.j1ao.vip |
| 模型广场 | https://ai.j1ao.vip/pricing |
| QQ 群 | **1014805259** |



## 能力概览

| 能力 | 说明 |
|---|---|
| 统一接入 | OpenAI / Anthropic / Gemini / xAI / DeepSeek 等上游，统一 `/v1/chat/completions`、`/v1/responses`、`/v1/messages` |
| 渠道与分组 | 一个分组可挂多条渠道，按权重/优先级路由，失败自动重试与降级 |
| 倍率体系 | 全局模型倍率 + 分组倍率 + 分组级模型覆盖（ratio / 固定价 / 缓存价） |
| 计费 | 按 Token（含缓存读写分档）或按次；支持订阅套餐、余额、兑换码 |
| 下游分发 | 给指定用户单独设定各分组倍率，并生成专属分组标识 |
| 健康度镜像 | 定期拉取上游 `perf-metrics` 的 summary/detail，按上游分组映射成本站分组的健康数据 |
| 可观测 | 使用日志、审计日志、内容审查、分组状态页、模型广场 |
| 其他 | 邀请返利、充值渠道、降智检测样例画廊（静态页） |

## 与上游的差异

- **前端重做**：React + TanStack Router + Tailwind v4 + shadcn；移动端卡片式列表；旧版 WebView（Chrome < 80）的 `oklch()` 颜色回退
- **计费**：分组倍率与分组级模型覆盖、订阅套餐、下游分发倍率
- **健康度**：上游镜像优先、探测兜底；按上游分组区分，避免多分组共用同一份数据
- **运营**：余额/兑换码/推广、内容审查、公告与页脚模块化

## 快速开始

```bash
cp .env.example .env      # 按需填写
docker compose up -d
```

默认监听 `3000`，首次启动后访问 `/` 完成初始化。

### 关键环境变量

| 变量 | 作用 | 备注 |
|---|---|---|
| `SQL_DSN` | MySQL 连接串 | 必填 |
| `REDIS_CONN_STRING` | Redis 连接串 | 强烈建议：会话、限流、缓存 |
| `SESSION_SECRET` | 会话签名密钥 | **必须固定**，长度 ≥ 32；不设则每次启动随机生成，重启后所有登录态失效 |
| `SESSION_COOKIE_SECURE` | 仅 HTTPS 下发 Cookie | 配合下面的 URL 使用 |
| `SESSION_COOKIE_TRUSTED_URL` | 允许的站点 origin | 如 `https://ai.example.com` |
| `TRUSTED_PROXIES` | 可信反代网段 | 反代后取真实 IP 用 |

> 踩过的坑：只设 `SESSION_COOKIE_SECURE=true` 而不设 `SESSION_COOKIE_TRUSTED_URL`，进程会直接启动失败并反复重启。

## 本地开发

```bash
# 后端
go build ./...
go run main.go

# 前端
cd web
npm install
npm run dev      # rsbuild dev server
npm run build
```

前端有 `npm run typecheck` / `npm run test` / `npm run lint`；提交前建议跑一遍。

## 构建与发布

- CI：GitHub Actions（`.github/workflows/ci.yml`）
- 镜像：`.github/workflows/*image*` 构建并推送到 GHCR，标签为提交短 SHA
- 生产部署：拉取新镜像后重建容器即可；会话由 `SESSION_SECRET` + Redis 保证跨重启存活

## 目录结构（节选）

```
controller/    HTTP 处理
relay/         上游协议转换与转发
service/       业务逻辑（会话、计费、健康度等）
model/         数据模型与持久化
setting/       运行时配置
web/           前端（React + rsbuild）
pkg/           通用包（perf_metrics 等）
docs/          开发文档
```

## 许可

AGPL-3.0。原始版权归 QuantumNous 与 contributors 所有，见 `LICENSE`、`NOTICE`、`THIRD-PARTY-LICENSES.md`。本仓库的修改同样以 AGPL-3.0 发布，并在界面保留上游项目链接。
