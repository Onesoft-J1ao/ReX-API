# ReX API

> A self-hosted AI API gateway that unifies several upstream providers behind an OpenAI-compatible interface, with group multipliers, subscription plans, distributor pricing and upstream health mirroring.

This repository is a fork of [QuantumNous/new-api](https://github.com/QuantumNous/new-api) (AGPL-3.0). The upstream copyright notices, `NOTICE` and third-party license list are preserved.

## Site & community

| Entry | Address |
|---|---|
| Site | **https://ai.j1ao.vip** |
| Docs | https://docs.j1ao.vip |
| Model square | https://ai.j1ao.vip/pricing |
| QQ group | **1014805259** |



## What it does

| Area | Detail |
|---|---|
| Unified API | OpenAI / Anthropic / Gemini / xAI / DeepSeek upstreams behind `/v1/chat/completions`, `/v1/responses`, `/v1/messages` |
| Channels & groups | Multiple channels per group, weighted/priority routing, automatic retry and fallback |
| Pricing | Global model ratio + group multiplier + per-group model overrides (ratio, fixed price, cache price) |
| Billing | Per-token (with cache read/write tiers) or per-request; subscription plans, wallet, redemption codes |
| Distribution | Per-user group multipliers for downstream resellers, plus a dedicated group marker |
| Health mirror | Pull upstream `perf-metrics` summary/detail and map it onto local groups, per upstream group |
| Observability | Usage logs, audit log, content review, group status page, model square |

## Differences from upstream

- Frontend rewritten (React + TanStack Router + Tailwind v4 + shadcn), mobile card lists, and an `oklch()` → hex fallback for old WebViews (Chrome < 80)
- Billing: group multipliers, per-group model overrides, subscription plans, distributor rates
- Health: upstream mirror first, local probes as fallback, scoped per upstream group
- Operations: wallet / redemption / referrals, content review, modular announcement and footer

## Quick start

```bash
cp .env.example .env
docker compose up -d
```

The service listens on `3000`.

### Key environment variables

| Variable | Purpose | Notes |
|---|---|---|
| `SQL_DSN` | MySQL connection string | required |
| `REDIS_CONN_STRING` | Redis connection string | strongly recommended: sessions, rate limits, cache |
| `SESSION_SECRET` | session signing secret | **must be fixed**, ≥ 32 chars; otherwise a random one is generated per boot and every restart logs users out |
| `SESSION_COOKIE_SECURE` | send cookies over HTTPS only | pair with the URL below |
| `SESSION_COOKIE_TRUSTED_URL` | allowed origin | e.g. `https://ai.example.com` |
| `TRUSTED_PROXIES` | trusted proxy CIDRs | needed for real client IPs behind a reverse proxy |

> Pitfall: setting `SESSION_COOKIE_SECURE=true` without `SESSION_COOKIE_TRUSTED_URL` makes the process exit on startup and restart-loop.

## Development

```bash
go build ./...
go run main.go

cd web
npm install
npm run dev
npm run build
```

## Build & release

- CI runs on GitHub Actions (`.github/workflows/`)
- Images are built and pushed to GHCR, tagged with the short commit SHA
- Production: pull the new image and recreate the container; sessions survive restarts thanks to `SESSION_SECRET` + Redis

## License

AGPL-3.0. Original copyright belongs to QuantumNous and contributors — see `LICENSE`, `NOTICE` and `THIRD-PARTY-LICENSES.md`. Modifications in this repository are released under the same license, with a visible link back to the upstream project.
