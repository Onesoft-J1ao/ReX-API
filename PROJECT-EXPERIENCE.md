# ReX-API 项目经验汇总

> 本文档汇总 ReX-API（new-api fork）运维与开发中沉淀的实战经验。
> 每一节都是踩过坑之后写下的，**不是从文档抄的**。
> 最后更新：2026-11-13

---

## 一、接手与协作

### 1.1 交付标准

- **交付到「可上传的镜像包」为止** —— 交付代码不算完成，能 `scp` 到服务器的 `.tgz` 才是
- **要实跑证据，不要「应该能用」** —— 编译通过 ≠ 能跑；改配置后要看生产日志确认真实生效
- **少开 agent** —— 能自己查清的别派 agent，多个 agent 并行改同一批文件会冲突
- **上游身份不外露** —— 对外文案不提上游来源

### 1.2 沟通

- 中文优先，直接说结论，不加免责声明
- 完整可运行的代码，不留 TODO 和占位符
- 用标准安全术语，不做委婉替换

---

## 二、部署

### 2.1 镜像存放位置（重要）

**固定路径：`/opt/1panel/apps/ReX-API/rexapi_linearteam/`**

所有 `docker save | gzip` 出来的 `.tgz` 都放这里，不放 `/root/images` 或别处。

### 2.2 版本 tag 规则

- 镜像 tag 从 **v0.20.0 起使用语义化**（`rex-api:v0.27.0`）
- `VERSION` 文件是日期戳（`2026.10.2.0`），**只给 root 看，不对外暴露**

### 2.3 部署流程

```bash
# 1. 上传并双向校验
scp rex-api-vX.Y.Z.tgz rex-sw:/opt/1panel/apps/ReX-API/rexapi_linearteam/
ssh rex-sw 'sha256sum /opt/1panel/apps/ReX-API/rexapi_linearteam/rex-api-vX.Y.Z.tgz'

# 2. 取当前容器的完整配置（回滚依据）
ssh rex-sw 'docker inspect ReX-API --format "{{json .Config}}" > /tmp/rexapi-config.json'
ssh rex-sw 'chmod 600 /tmp/rexapi-config.json'   # 含 DSN，别留明文

# 3. 切换
docker load -i <包路径>
DSN=$(docker inspect ReX-API --format '{{range .Config.Env}}{{println .}}{{end}}' | grep '^SQL_DSN=' | cut -d= -f2-)
docker stop ReX-API
docker rename ReX-API "ReX-API-v<旧版本>-$(date +%H%M%S)"    # 旧容器保留回滚
docker update --restart=no $(docker ps -a --filter 'name=ReX-API-v<旧版本>' -q | head -1)
docker run -d --name ReX-API --restart always -p 3000:3000 \
  -v <volume-id>:/data --network 1panel-network \
  -e SQL_DSN="$DSN" rex-api:vX.Y.Z
```

**关键**：旧容器永远保留，不要 `docker rm`。

### 2.4 本机构建

本机（Windows）**没有 docker**，docker 在 WSL Ubuntu-24.04 里。

```bash
wsl -d Ubuntu-24.04 -- bash -lc 'cd /mnt/d/.../build-ctx && docker build -t rex-api:vX.Y.Z .'
```

构建上下文用 `git archive HEAD | tar -x` 生成，避免把 `.git` 和工作区垃圾带进去。

### 2.5 现用流程：GitHub Actions 构建 + GHCR 拉取 ⚠️

本机构建会 OOM，**一律推 GitHub Actions**：

```
push 到 deploy
  → workflow "CI"（前端 typecheck/test + 后端 build/test）
  → workflow "Publish Docker image (Multi-arch)"
  → 镜像 ghcr.io/onesoft-j1ao/rex-api:<short-sha>
  → 服务器 docker pull + 重建容器
```

部署脚本：`work/_deploy_ghcr6.py`（改文件里的 tag 即可）。
它 replay 当前容器的 spec（env / network / ports），旧容器 rename 保留供回滚：

```bash
docker rm -f ReX-API && docker rename ReX-API-prev-<tag>-bak ReX-API && docker start ReX-API
```

**注意**：
- GitHub Artifacts 配额会满 —— 镜像 tgz 默认不上传，需要时手动勾 `export_tgz`
- 轮询 CI 用 `gh run list --workflow CI --limit 1`，不要用 `gh run view --branch`（不接受）

### 2.6 CI 挂了先看失败 job，不要看全量日志

```bash
gh run view <run-id> -R Onesoft-J1ao/ReX-API --log-failed
```

如果失败点是 `toHaveFocus()` / `toHaveValue()` 这类 DOM 状态断言，
而同一测试上一轮是绿的，**先怀疑 flaky（异步未包 waitFor）**，而不是新代码引入的 bug。

---

## 三、计费配置（踩坑最多）

### 3.1 存储格式：反斜杠层数决定一切 ⚠️

`options` 表里的 JSON blob（`billing_setting.billing_expr`、`ModelRatio` 等）值内部含引号。

**JSON 只认 1 个反斜杠**：

```
正确: {"a":"tier(\"base\", p * 1.0)"}
错误: {"a":"tier(\\\\base\\\\", p * 1.0)"}   ← 4 个反斜杠
```

写错的后果不是报错，而是 **`json.Unmarshal` 静默失败**（`setting/config` 的 map 分支遇到解析失败直接 `continue`），
整个配置的表达式全部失效，模型静默回退到 `ModelRatio` 的 37.5 占位价。

**真实踩坑**：29 个 tiered 表达式从未生效，`gpt-6-astra` 看起来正常是因为它是 Go 内置值，不走数据库。

### 3.2 验证必须用 hex

```bash
docker exec ... mysql -uroot -N -e "SELECT HEX(value) FROM options WHERE \`key\`='...'" | binascii.unhexlify
```

`mysql -N` 输出会**再转义一层**，看到的字节数比实际存储多。判断「库里到底是什么」只能靠 hex。

### 3.3 写回用 hex

```python
hexed = binascii.hexlify(text.encode()).decode()
sql("UPDATE options SET value=CONVERT(0x%s USING utf8mb4) WHERE `key`='%s';" % (hexed, key))
```

**绝不要**用 `raw.replace("\\", "\\\\")` 拼 SQL —— 会把已有的正确转义翻倍。

### 3.4 INSERT 用 `X'...'`

`CONVERT(0x... USING utf8mb4)` 在 `INSERT ... VALUES` 里会报 `Unknown column '0x'`。
插入时用 MySQL 十六进制字面量 `X'<hex>'`，它没有转义规则。

### 3.5 危险的 json.dump

`json.dump` 重写 locale 文件会**还原 `footer.newapi` 这类刻意的 Unicode 转义**
（那是为规避关键词扫描写的，必须保留）。

同步 i18n 键要**逐行插入**，不要整体重写。

### 3.6 heredoc 会吃掉反斜杠

通过 `ssh 'bash -s' <<'SH'` 传 Python 脚本时，`chr(92)` 之类的转义处理会出问题。
**脚本用 `Write` 工具写成本地文件，再 `base64 -w0` 传到服务器执行**，全程零转义。

---

## 四、价格同步与哨兵

### 4.1 上游价格来源

`https://aiapi.mjiutang.top` 等上游站的 `/api/pricing` 或 `/v1/models` 可读，
但**很多上游不公开价格接口**（如 `aiapi.mjiutang.top` 返回空）—— 需要问用户要。

### 4.2 ratio 换算公式

站里 `quotaPerUnit = 500000`（$1 = 50万 tokens），所以：

```
ratio 1 = $2 / 1M tokens
$0.067/1M（上游价 ¥） → × 7.2 汇率 → $0.482 → ratio = 0.241
加 20% 毛利 → ratio = 0.289
```

### 4.3 哨兵三个设计陷阱 ⚠️

**陷阱 1：比较基准必须包含毛利**

改价公式是 `写入 = 上游 × (1 + margin)`，但比较时用的是**上游原始值**。
所以健康的价格（已含 20% 加价）必然显示 `-16.67%`（`1/1.2 - 1`），
apply 一次就把毛利抹掉，下次运行又报，**来回震荡**。

修法：比较用 `expected = theirs × (1 + margin)`。

**陷阱 2：`0` 不等于「未配置」**

`localPrices()` 把未配置的维度存成 `0`，上游不返回该字段（`nil`）。
`!haveOurs || !haveTheirs` 分支一律报 100% —— 一份报告 213 行里 140 行是这种噪音。

修法：`percentChange` 任一侧为 0 都返回 0；单边字段只在**有值那侧非 0** 时才报。

**陷阱 3：上游用 37.5 表示「未定价」**

上游对未定价模型返回 `model_ratio: 37.5, completion_ratio: 1` 而非省略。
直接采用会把免费模型定成 37.5（前端显示 $75/1M）。

注意 37.5 也是合法发布价（`gpt-4.5-preview`），必须**两个条件同时满足**才跳过。

**改价后必须验证**：apply 后重新跑 check，报告应变空（或只剩真实差异）。
若仍满，说明比较基准和写入公式不一致。

### 4.4 成本公式：与缓存率无关 ⚠️

```
quota = 表达式输出 / 1e6 × QuotaPerUnit(500000) × groupRatio
上游成本 = total_amount × (上游倍率 / 我方倍率) / QuotaPerUnit
```

**关键结论：成本只由 total_amount 决定，与用户缓存率完全无关。**
缓存率只影响用户能跑多少 token，不影响我们付多少。

做套餐/额度时只需问一件事：`total_amount × (上游/我方) / 500000 < 售价`？

实例（ds套餐，我方 0.04 / 上游 0.03）：

| total_amount | 显示 ◗ | 上游成本 | 卖 ¥2 毛利 |
|---|---|---|---|
| 500,000 | 1.00 | ¥0.75 | ¥1.25 (62%) |
| 1,000,000 | 2.00 | ¥1.50 | ¥0.50 (25%) |
| **1,333,333** | **2.67** | **¥2.00** | **¥0（保本上限）** |
| 2,000,000 | 4.00 | ¥3.00 | **−¥1.00（亏）** |

**保本上限 = 售价 / (上游倍率/我方倍率) × QuotaPerUnit**。超过必亏。

### 4.5 系数反算会留下长小数 ⚠️

上游的 `billing_expr` 系数是**整数**（`p * 2 + c * 10 + cr * 0.2`）。
如果先算出「最终单价」再除以倍率反推系数，会得到 `2.555555556` 这种值，
前端价格显示成 `$0.6388889`，非常难看。

**正确做法**：系数直接写「上游整数 × 调整倍数」，用 3 位以内小数。

排查：`grep -oE '\d+\.\d{7,}'` 扫 `billing_expr`，任何 >6 位小数都是反算残留。

反算残留的另一个副作用：**各字段倍数不一致**。
实测 CodexPro企业稳定 的 in/out/cc 都是 ×1.25，但 cr 是 ×1.32/1.16/1.65/1.09
—— 说明缓存价当初是单独算的，且算错了。

### 4.6 分组配置的四张表必须对齐 ⚠️

分组要在前端可见、可定价，需要**四处同时存在**：

| 表 | 作用 | 缺了会怎样 |
|---|---|---|
| `group_ratio_setting.group_ratio` | **运行时真源** | 无法定价（按 1 算或 0） |
| `GroupRatio` | 旧 key，后台界面读写 | 界面显示与实际不符 |
| `UserUsableGroups` | 用户可见性 | 用户看不到该分组 |
| `abilities` + `channels` | 有渠道才能服务 | 被 `HideModelsWithoutEnabledChannel` 过滤 |

**踩坑记录**：`distributor` 只写了 `GroupRatio` 和 `UserUsableGroups`，
漏了 `group_ratio_setting.group_ratio`，结果 DB 里 `GroupRatio[Grok]=0.17`
但线上 API 返回 0.1 —— 因为运行时读的是后者。

**两个 key 必须同步写**。验证方法：

```python
json.loads(raw("GroupRatio")) == json.loads(raw("group_ratio_setting.group_ratio"))
```

### 4.7 死配置会卡住保存操作 ⚠️

渠道被禁用后，分组在 `group_pricing_source` / `group_model_*` 里仍有条目。
这些条目**不出现在编辑界面**（列表只渲染有渠道的分组），但保存时会整份提交，
后端逐条校验 → 报 `X has no enabled channel and cannot be priced`，
而用户根本看不到 X，也就无法删除 → **死锁**。

修法（已落地 `1f9970c`）：
- 后端：`SaveDistributor` 遇到无渠道分组 **跳过**而不是拒绝
- 前端：回填草稿时只填 `props.groups` 里有的（有渠道的）

定期清理：

```python
served = 有启用渠道的分组集合
for key in ("group_pricing_source", "group_model_ratio", "group_model_price",
            "group_model_cache_ratio", "group_model_completion_ratio"):
    for g in list(load(key)):
        if g not in served: del load(key)[g]
```

### 4.8 价格配置体检清单

改完价格或新增分组后，跑这 7 项：

```
[ ] 1. billing_expr 里没有 >6 位小数
[ ] 2. 有 expr 的模型 mode 必须是 tiered_expr；mode=tiered_expr 必须有 expr
[ ] 3. GroupRatio 与 group_ratio_setting.group_ratio 完全一致
[ ] 4. 每个有渠道的分组都有倍率
[ ] 5. group_pricing_source 里没有无渠道分组
[ ] 6. group_model_* 四张表里没有无渠道分组
[ ] 7. UserUsableGroups 里每个分组都有倍率
```

对应脚本见 `work/_audit_final.py`（输出 OK/!! 逐项）。

### 4.9 上游「拿货价」与「公开价」不同

上游站对 distributor 身份会返回**更低的 group_ratio**。
实测 furry：匿名 `/api/pricing` 显示 `Grok = 0.17`，
但 distributor 视角是 `0.15` —— **0.17 是他们的零售价，0.15 才是我们的拿货价**。

**判断方法**：拿到的报价低于站上公开价时，先确认自己是不是 distributor，
否则会把「上游零售价」当成本算毛利，实际毛利被高估。

### 4.10 价格显示 `-` 的真正条件

模型卡片显示 `-` 只有一个原因：**它既没有 `ModelRatio` / `ModelPrice`，也不是 `tiered_expr`**。
分组倍率为 0 并不会让卡片显示 0——它只是把「已有价格」乘成 0；模型本身没有比例时前端算不出数字。

实测（2026-11-13，只读排查）：

| 分组 | 模型数 | 无价 | 例子 |
|---|---:|---:|---|
| free1 | 54 | 18 | `selfdeploy/d3`、`MiniMax-M2.7-free`、`glm-5.3-flash-free`、`kolors` |
| free2 | 10 | 3 | `grok-imagine-video`、`grok-4.20-multi-agent-0309`、`grok-composer-2.5-fast` |

修复方式二选一：
- 给模型写 `ModelRatio=0`（免费组下显示 `0 / 1M`，比 `-` 直观）；
- 或按次计费写 `ModelPrice`。
两者都不写，卡片就会一直显示 `-`，且**探针会报「价格未配置」直接失败**。

### 4.11 健康镜像 vs 自己探测：镜像优先，探针兜底

| 链路 | 数据源 | 落点 | 5 分钟刷新 |
|---|---|---|---|
| 镜像 | 上游 `/api/perf-metrics/summary` + `?model=&hours=` | `/api/perf-metrics/summary`（模型广场/详情） | 是 |
| 探针 | 本站真实请求（`model_health` / `group_health`） | `/api/status-check`（`/status` 页面） | 是（默认 5 分钟） |

**2026-11-13 起，`/status` 也读镜像**（`72755eb`）：`GetStatusCheck` 先按 `(本地分组, 模型)` 查上游 detail，
命中就用上游的可用率 + 上游自己的 24h 绿条；没有镜像源的分组才回落到探针 + 本站流量。

- 镜像行带 `source=mirror:<source>`（furry / sheapi / mcapple）。
- 一个分组只探 1 个模型，镜像没接上时同组其余模型全空 —— 这正是「同一模型在不同分组数字一模一样」的成因。
- 排查：`/api/status-check` 该行有没有 `source`；`perf_metrics_setting.upstream_health_groups` 有没有该分组映射。
- `/api/perf-metrics/summary` 仍是模型广场那条链路（171 个模型里 124 个来自镜像）。

**上游对 UA 的拦截容易被误判**：用 Python `urllib` 默认 UA 请求 furry / sheapi 的
`/api/perf-metrics/summary` 会 403，但换成 `Go-http-client/1.1` / `curl` / 浏览器 UA 全部 200。
Go 客户端默认就发 `Go-http-client/1.1`，所以镜像本身没被拦——排查时别拿 Python 默认 UA 当证据。

### 4.12 探针长期全红的排查顺序

先查 `model_health` 近 24h 的 `ok` 比例和 `error_msg`：

```sql
SELECT `group`, model_name, COUNT(*) c, SUM(ok) okc, LEFT(MAX(error_msg),80)
FROM model_health
WHERE probe_ts > UNIX_TIMESTAMP()-86400
GROUP BY `group`, model_name ORDER BY c DESC;
```

实测的四类原因：

| 现象 | error_msg | 原因 |
|---|---|---|
| 全 503 | `No available channel for model gpt-5.5` | 探针选了该分组**不服务**的模型（渠道测试模型残留） |
| 全 403 HTML | `<!DOCTYPE html>` | 上游站点拦截（Cloudflare/防盗链） |
| 全 403 文本 | `The API key group has been deleted` | 上游 key 的分组已被删 |
| 报价格错误 | `模型 xxx 的价格未配置` | 模型无价（见 4.10），探针在计价阶段就失败 |

另外：分组渠道关掉后 `model_health` 会停止写入（观察最后一条 `probe_ts` 即可判断该组是否还在服务）。


### 4.13 新增上游渠道标准流程（2026-11-13 实操）

以 `Deepseek限时特惠`（furry 上游 0.050 → 我方 0.06，无峰谷）为例。

**① 确认拿货价与目标分组**

```bash
curl -s -H "Authorization: Bearer sk-..." https://<上游>/api/user/groups
```

返回每个分组的 `ratio` / `desc` / `default_model`。`desc` 写「无峰谷」的就是不分峰谷的分组。
不要拿匿名 `/api/pricing` 的 group_ratio 当成本 —— 那是零售价（见 4.9）。

**② 确认模型与基础倍率**

```bash
curl -s -H "Authorization: Bearer sk-..." https://<上游>/api/pricing
```

在 `data[]` 里筛 `enable_groups` 包含该分组的模型，取 `model_ratio` / `completion_ratio`（`quota_type=0` 按 token）。

**③ 卖价公式**

```
我方单价($/1M) = model_ratio × 我方分组倍率 × 2
```

例：`0.5 × 0.06 × 2 = $0.06/1M` 输入；输出 ×4 = `$0.24/1M`。
上游成本 `0.5 × 0.05 × 2 = $0.05 / $0.20` → 毛利 16.7%。

**④ 「无峰谷」= 按 ratio，不写表达式**

模型已有全局 `tiered_expr`（峰谷）时，只在**本分组**覆盖，别动全局：

```
billing_setting.group_pricing_source[G][M]          = "ratio"
billing_setting.group_model_ratio[G][M]             = 0.5   # 上游基础倍率
billing_setting.group_model_completion_ratio[G][M]  = 4
```

**⑤ 必须同时写入的位置（缺一即残）**

| 位置 | 内容 |
|---|---|
| `channels` | 渠道行（type=1、status=1、group、models、base_url、key） |
| `abilities` | `(group, model, channel_id, enabled=1)`，否则模型广场直接过滤掉（见 5.5） |
| `group_ratio_setting.group_ratio` | 运行时真源倍率 |
| `GroupRatio` | 后台界面倍率，必须与上一条一致（见 4.6） |
| `UserUsableGroups` | 分组简介 / 用户可见性 |
| `ModelRatio` / `CompletionRatio` | 模型基础倍率；缺了卡片显示 `-`（见 4.10） |
| `status_check_setting.groups` | 加进探测列表（裸逗号串，不带外层引号） |
| `perf_metrics_setting.upstream_health_groups.<source>` | 本地分组 → 上游分组名，健康绿条靠它镜像 |

**⑥ 插渠道用 hex 字面量**

```sql
INSERT INTO channels (type, `key`, status, name, created_time, priority, auto_ban,
                      `group`, models, base_url, balance_updated_time, used_quota)
VALUES (1, CONVERT(X'<hex>' USING utf8mb4), 1, CONVERT(X'<hex>' USING utf8mb4),
        UNIX_TIMESTAMP(), 0, 1, CONVERT(X'<hex>' USING utf8mb4),
        CONVERT(X'<hex>' USING utf8mb4), CONVERT(X'<hex>' USING utf8mb4), 0, 0);
```

中文一律 hex，不拼字符串（见 3.3 / 3.4）。

**`channels.models` 是逗号分隔字符串，不是 JSON 数组 ⚠️**
写成 `["m"]` 不会报错，但重启后 `InitChannelCache` 会把它当成一个模型名再 append 一次，
分组里就多出一个名叫 `["m"]` 的模型（价格页、模型广场都会出现）。
正确写法：`m1,m2`。判断方法：

```sql
SELECT id, HEX(models) FROM channels WHERE id=<id>;   -- 看到 5B226D225D 就是错的
```

**缓存价别忘**：卡片上的缓存价来自 `CacheRatio[model]`，分组再按
`billing_setting.group_model_cache_ratio[G][M]` 覆盖。只写 `ModelRatio` 时缓存价显示 `-`。
上游 `/api/pricing` 的 `cache_ratio` 字段直接照抄即可（1017 = 0.02，即输入价的 2%）。

**⑦ 重启 + 四条验收**

```bash
docker restart ReX-API
```

1. 分组出现在 `/api/status-check`；
2. 模型行有 `source=mirror:<source>`，`series` 接近 24 小时；
3. 价格 = ③ 的算式（模型广场卡片不为 `-`）；
4. 用新 key 直连一次 `POST /v1/chat/completions` 返回 200。

**⑧ 备份**

脚本先 dump `channels options abilities` 到 `/root/billing-backup/<name>-<时间>.sql.gz`，出事直接回灌。
现成脚本：`work/_add_limited_ds.py`（默认 dry run，`--apply` 才写）。

### 4.14 分组改名清单 ⚠️

改名要同步这些位置，漏一个就是「分组显示但无价」或「老 token 失效」：

```
channels.group / abilities.group / tokens.group / users.group
group_health.group / model_health.group / perf_metrics.group
status_check_probe_states.group / tasks.group
GroupRatio / group_ratio_setting.group_ratio
UserUsableGroups / group_ratio_setting.group_group_ratio[<身份>]
billing_setting.group_model_{ratio,completion_ratio,price,cache_ratio}
billing_setting.group_pricing_source
perf_metrics_setting.upstream_health_groups.<source>
status_check_setting.groups（裸逗号串）
model_square_group_descriptions / console_setting.group_access_rules / LandingModels.serviceRoutes[].group
```

`logs.group` **不改** —— 那是历史账单，改了等于篡改过去的分组归属。

脚本：`work/_rename_group.py`（默认 dry run，`--apply` 才写）。

### 4.15 MaiBot 模型配置（宿主机 systemd 服务）

MaiBot 不是容器，是 `maibot-kurisu.service`，配置在 `/opt/maibot-kurisu/app/config/`：

| 文件 | 作用 |
|---|---|
| `bot_config.toml` | 机器人本体与 `[webui]`（端口 8041、`allowed_ips`；登录 token 在 `data/webui.json`） |
| `model_config.toml` | `[[api_providers]]` + `[[models]]` + `[model_task_config.*]` 任务模型 |

改模型三步：备份 → 改 `model_config.toml` → `systemctl restart maibot-kurisu`。

⚠️ **上游 key 是分组绑定的**：`/api/pricing` 会列出几百个模型，但实际能用哪些由 key 所属分组决定。
不在该分组里的模型会直接返回 `分组 X 下模型 Y 无可用渠道 (distributor)`。
上 key 前先用目标模型发一次 `POST /v1/chat/completions` 验证，别照价格页的模型列表配。

⚠️ **模型名要和上游完全一致**：furry 的 `deepseek-v4.1-flash` 在特惠分组里叫
`deepseek-v4.1-flash-1017`，写错就是 `model_not_found`。

改完验证：`systemctl is-active maibot-kurisu`、`curl -o /dev/null -w '%{http_code}' http://127.0.0.1:8041/`（200），
再看 `journalctl -u maibot-kurisu --since '3 minutes ago'` 有没有模型报错。
`model_config.toml.bak-<时间>` 就地留备份。

### 4.16 群内发码插件 `onesoft.redeem-grant`

群主 / 管理员在群里发 `发放@某人 0.01元`，机器人用 root 的访问令牌建一张等额兑换码，
通过**本群临时会话**私聊发给对方，群里只回执一句。

| 项 | 值 |
|---|---|
| 服务器路径 | `/opt/maibot-kurisu/app/plugins/onesoft_redeem-grant/` |
| 本地源码 | `D:\Code\MaiBot\plugins\onesoft_redeem-grant\` |
| 兑换比例 | `quota_per_yuan = 500000`（1 元 = $1 额度） |
| 审计日志 | `/opt/maibot-kurisu/app/data/redeem_grant_audit.jsonl`（含明文兑换码） |
| 鉴权 | `users.access_token`（root，32 位，明文列，`Authorization: Bearer`） |

四个必须记住的坑（都是实测踩出来的）：

1. **`ON_MESSAGE` 事件是死代码**：`src/chat/message_receive/bot.py:872` 那行是注释掉的，
   `@EventHandler(EventType.ON_MESSAGE)` 永远不会触发。要用
   `@HookHandler(hook="chat.receive.after_process", mode=HookMode.BLOCKING)`，
   返回 `{"action": "abort"}` 阻止消息继续进 LLM。
2. **`reportSelfMessage: false`**（NapCat `onebot11_*.json`）：机器人账号自己发的消息不会回传给
   MaiBot。用机器人号自测永远收不到——必须用另一个 QQ 发。
3. **手打 `@昵称` 不产生 at 段**：QQ 里必须从弹出成员列表点选，否则 NapCat 只给一个 text 段，
   插件拿不到 QQ 号。插件已加兜底：按群昵称 `get_group_member_list` 反查，唯一命中才发。
4. **NapCat 长跑后 NTQQ 消息内核会假死**：`get_status` 仍报 `online/good`，但收发全部失败，
   `retcode 1200 / EventChecker Failed: NodeIKernelMsgService/sendMsg`。
   处理：`docker restart napcat-kurisu`（可能要重新扫码，二维码在容器内 `/app/napcat/cache/qrcode.png`）。

验收标准：群里出现「已处理：兑换码将通过本群临时会话发送给 X（QQ号）」，
被 @ 的人收到临时会话，`redemptions` 表新增一行且 `quota = 金额 × 500000`。

### 4.17 未定价模型体检（含脚本）

卡片显示 `-` 的判定口径（缺一即未定价）：

```
ModelRatio[m]
或 ModelPrice[m]
或 billing_mode[m] == "tiered_expr" 且 billing_expr[m] 存在
或 group_model_ratio[g][m] / group_model_price[g][m]
或 group_pricing_source[g][m] 明确 pin 了 ratio / price / tiered_expr
```

**只看 `ModelRatio` 会误判**：按次计费（`ModelPrice`）、表达式计费、分组级 pin 都算有价。

脚本：`scripts/check_unpriced_models.py`

```bash
python scripts/check_unpriced_models.py          # 人类可读
python scripts/check_unpriced_models.py --json   # 机器可读
# 退出码 0=全部有价 1=有未定价 2=查询失败
```

首次跑出 14 个，处理方式分三类：

| 情况 | 做法 | 例子 |
|---|---|---|
| 上游按 token 计费 | 照抄上游 `model_ratio`，并补 `CompletionRatio` / `CacheRatio` | `gemini-4-hydrogen` → ratio 0.75 / completion 5 / cache 0.05 |
| 上游按次计费 | 写 `ModelPrice` | `nvidia/nemotron-3.5-bolt-e4b` → 0.0001 |
| 免费分组 / 无价上游 | 写 `ModelRatio = 0` | `nvidia/nemotron-3.5-embed-1b`、`naiwa/gemini-4-argon` |

### 4.18 邀请返利与套餐分组限制

**返利表** `invite_commissions`（`model/invite_commission.go`）：

- 规则：被邀请人前 10 笔、单笔实付 > 15 元、法币充值 → 邀请人 7.5%；
- 下单时在同一事务里登记（`top_up_id` 唯一，回调重放不重复）；
- `available_at = now + 86400`，`InviteCommissionLoop()` 每 5 分钟扫到期记录，
  先 `UPDATE ... WHERE status='pending'` 抢占再入账，多实例也只给一次；
- 排除：`IsRedemptionPurchase()`、`monero` / `nowpayments` / 余额抵扣；
- 前端接口 `GET /api/user/aff/commissions`，钱包页「推荐计划」下方表格。

**套餐只能用于某个分组**：用 `subscription_plans` 的三个字段，不需要改代码：

```
wallet_only_groups_enabled = 1
wallet_only_groups_mode    = 'whitelist'
wallet_only_groups         = 'ds套餐'
```

判定函数 `SubscriptionPlan.IsWalletOnlyForGroup`：白名单模式下**只有列出的分组**
能消耗套餐额度，其它分组一律走钱包。黑名单模式相反。

### 4.19 内容审查抓不到 AI 回复（时序坑）

`ContentReviewCapture` 中间件把响应体 tee 到包装 writer，但
`c.Set("content_review_captured_body", ...)` 是在 `c.Next()` **返回后**才执行；
而 `captureReviewContent` 在 Relay 内部调用，此刻 context 里还没有 body，
于是 `response_text` 永远为空。

修法：中间件导出 `CapturedResponseBody(c)`，直接从包装 writer 读；
`UpsertLogContent` 同时改成「只覆盖非空字段」，避免写回复时把请求文本清空。

自检 SQL：

```sql
SELECT id, model_name, LENGTH(request_text), LENGTH(response_text)
FROM log_contents ORDER BY id DESC LIMIT 5;
```

### 4.20 充值页的四个旋钮

| 旋钮 | 存储 | 语义 |
|---|---|---|
| `Price` | options | 充值金额倍率（1.0 = 面值；1.035 = 加价 3.5%），也用于人民币显示换算 |
| `fee_rate` | Epay 网关 `pay_methods[]` | **百分数**，7 表示 +7%；只在 Epay 网关里生效 |
| `amount_discount` | `payment_setting.*` | `{"10":0.98}` 按充值面额打折；清空 `{}` 即关闭全部折扣 |
| `min_topup` | 每个支付方式 | 方法级最小金额，前端取 `max(全局, 方法级)` |

两个易踩点：

1. `/api/user/topup/info` **只读 EpayGateways[].pay_methods**，顶层 `PayMethods` 不参与渲染——
   加「进群充值」这类公告入口必须写进网关。
2. 支付按钮原来对方法名 `truncate`，长名字会显示不全；已改为换行显示。

### 4.21 本机 C 盘与编译缓存

C 盘常年只剩几百 MB 时，本地 `go build` / `tsgo` 会直接 OOM（报
`runtime: cannot allocate memory` 或栈溢出），**不要据此判断代码有问题**，以 CI 为准。

可安全清理的缓存清单：

```
go clean -cache && go clean -modcache
%USERPROFILE%\.nuget\packages
%USERPROFILE%\.cargo\registry\cache  +  .cargo\git
%LOCALAPPDATA%\pip\cache
%LOCALAPPDATA%\electron-builder\Cache
%LOCALAPPDATA%\uv
%LOCALAPPDATA%\Temp
%USERPROFILE%\.cache\opencode
```

大块空间（需要管理员/用户决策）：

| 项 | 大小 |
|---|---:|
| `C:\pagefile.sys`（挪到 D 盘） | 9.5 GB |
| `C:\hiberfil.sys`（`powercfg -h off`） | 6.3 GB |
| `C:\ProgramData\Huorong`（火绒隔离区） | 12.9 GB |
| `C:\Windows\WinSxS`（DISM StartComponentCleanup） | 9.0 GB |

不要删 `%USERPROFILE%\.cache\codex-runtimes`：正在跑的 CLI 运行时就在里面。

### 4.22 NapCat 掉登录：风控 + 巡检告警

**先分清两种「掉登录」**：

| 现象 | 证据 | 处理 |
|---|---|---|
| QQ 侧风控踢下线 | 日志出现 `serverErrorCode:168`「账号近期存在安全风险…」 | 手机 QQ 登录该号完成安全验证；别多设备同时登 |
| 进程被资源干掉 | `docker inspect` 里 `restarts>0`，或 dmesg 有 cgroup OOM | 提高容器内存上限，见下 |

实测这台机器 2026-11 的掉线是**第一种**；服务器本身没问题（磁盘 65%、无近期 OOM、
容器 `restarts=0`）。NapCat 原来限制 768 MB，已调到：

```bash
docker update --memory 1280m --memory-swap 1536m napcat-kurisu
```

**巡检 + 邮件告警**（脚本 `/root/napcat_health.py`，cron 每 5 分钟）：

```bash
*/5 * * * * /opt/maibot-kurisu/app/.venv/bin/python /root/napcat_health.py >/dev/null 2>&1
```

- 通过 OneBot WS `get_status` 判断在线；
- 只在**状态变化**时写 `/var/log/napcat-health.log`，并给 `1987800833@qq.com` 发信；
- 首次运行只记录状态、不发信，避免部署时误报；
- SMTP 复用站点配置（`smtp.resend.com:587` STARTTLS，发件人
  `noreply@auth0.j1ao.vip`），凭据写在 `/root/napcat_health.conf`（600）。

排查顺序：

```bash
tail -20 /var/log/napcat-health.log
docker logs --since 30m napcat-kurisu | grep -iE 'Login Error|二维码|下线'
docker inspect napcat-kurisu --format '{{.State.Status}} restarts={{.RestartCount}}'
```

注意：**每重启一次 NapCat 都可能触发一次设备验证**，非必要不要重启；
换 QQ 号或长期被 168 拦，优先在手机端养号/开设备锁。

---

## 五、源码陷阱

### 5.1 平行分叉不可盲合

10 个分支独立演进同一批文件，git 合并会产出**静默重复**：
同一字段、同一常量、同一 case 分支各写一遍，**编译能过但行为错**。

**每轮合并后必扫**：

```bash
go build ./...                    # 重复 import / 重复 case
python -c "import re,io; ..."     # 同一 key 出现多次
```

本项目已清理的静默重复：`SystemTaskFilters`、`PriceSentinelSection`、
`getHostProtocolEndpoints`、`FAMILY_ORDER`、`RegisterDailyLimit`（常量+表单项+case 三处）、
`OAuthRegisterDailyLimitError`、`syncCapDifficultyForOption`、`RelayDailyLimitError`、路由组重复。

### 5.2 iota 哨兵会撞值

```go
ChannelTypeTypeSafe = 67
ChannelTypeOpenCode = 68
ChannelTypeDummy        // ← 靠 iota 自动变 68，和 OpenCode 撞了！
```

加类型必须在 Dummy 之前，或给 Dummy **显式赋值**（`= ChannelTypeOpenCode + 1`）。

### 5.3 配置热加载 60 秒

`SyncOptions` 每 60 秒从数据库重读配置（`SYNC_FREQUENCY`，默认 60）。
改完配置等一轮即可生效，**不用重启容器**。

> 例外：`consoleSetting` 等部分是启动时加载的内存配置，改 DB 后仍需
> `docker restart ReX-API`；通过后台界面保存会自动热更新。

### 5.4 缓存命中率分母：不能把 cache 加回 prompt ⚠️

日志里的 `prompt_tokens` **已经是完整 prompt**：
Anthropic 的「未缓存余量」和 OpenAI 的 `prompt_cache_hit_tokens`
在写库前已被归一化成一个数。

旧代码 `分母 = prompt + cache_read + cache_write` 把缓存读算了两遍，
结果每条 DeepSeek 请求都显示 ~50%，不管实际命中多少。

**正确公式**：`命中率 = cache_read / (prompt_tokens + cache_write)`

验证数据（真实日志）：

| prompt | cache_read | 旧算法 | 新算法 |
|---|---|---|---|
| 23,384 | 23,168 | 49.77% | **99.08%** |
| 700,909 | 672,104 | 48.95% | **95.89%** |

交叉验证：Claude 三条 `897/700`、`601/469`、`1473/1149` 新算法都是 ~78.0%，
高度一致 —— 如果 `prompt_tokens` 真是「未缓存余量」，这三条会分别只有
197/132/324，不可能这么稳定。

已落地：`88585d2`，公式抽到 `getCacheHitPercent()` 并带 5 个单测。

### 5.5 无渠道分组会被从模型广场删掉

`controller/pricing.go` 的 `HideModelsWithoutEnabledChannel` 会删掉
所有「没有启用渠道」的分组。

身份标识类分组（如 `distributor`）**按设计就没有渠道**，所以必被删。

修法（已落地 `bc90227`）：`pruneUnservedGroups` 豁免「用户自己的分组」：

```go
for name := range usableGroup {
    if name == ownGroup { continue }   // 身份分组不删
    if _, ok := served[name]; !ok { delete(usableGroup, name) }
}
```

注意：未登录访客的 `ownGroup` 为空，所以匿名访问仍然看不到该分组（正确）。

### 5.6 前端测试的异步聚焦必须包在 waitFor 里

`ModelMappingEditor` 的聚焦链路是：
`useEffect(draftRequest) → requestAnimationFrame → setRows → useEffect(rows) → focus()`

测试若只 `waitFor` 了「行数」，然后同步断言 `toHaveFocus()`，
在 CI 机器上必然 race（本地快就过、CI 慢就挂）。

**任何跨 rAF / 跨 commit 的 DOM 状态断言都要包 `waitFor`。**

### 5.7 固定列网格不适合长短不一的分组名

分组筛选器原来用 `grid-cols-2` + canvas 测量决定哪些 chip 跨列：
短名（`Grok`、`NIM`）被拉成大方块，长名（`CodexPro企业稳定`）还是截断，
还多出 105 行测量 hook。

改成 `flex flex-wrap` + chip 自适应宽度后，短名紧凑、长名完整显示，
代码也少了 92 行。

**经验：标签宽度差异大时，用流式布局而不是固定列网格。**

---

## 六、网络与 IP

### 6.1 Cloudflare Tunnel 必须指向环回 ⚠️

`ai.j1ao.vip` 走 Cloudflare Tunnel。如果 origin 配成
`http://<服务器公网IP>:3000`，流量会**绕公网一圈回到本机**，
Cloudflare 加的 `CF-Connecting-IP` 头在绕行中丢失。

**后果**：所有注册 IP、审计 IP 全部变成服务器自己的 IP。

**正确配置**（Cloudflare 控制台 → Tunnels → Public Hostname）：

```
Service: http://127.0.0.1:3000
```

### 6.2 代码侧兜底

`middleware.ResolveClientIP()` 读 `CF-Connecting-IP`，
但**只在直接对端是可信代理时**才采信，防止调用方伪造自己的地址。

### 6.3 历史数据不可恢复

2026-09-28 之前注册的 19 个用户（id 131-149）真实注册 IP **永久丢失**。
查过 `users` / `logs` / `user_sessions` / `audit_logs` 四张表，全部只有服务器 IP
（那段时间真实 IP 从未到达过任何表）。用户选择**保持历史数据不动**。

---

## 七、前端 UI 陷阱

### 7.1 `tableLayout: 'fixed'` 才会用 colgroup 宽度

默认的 `auto` 布局下浏览器按内容重新分配列宽，**colgroup 里声明的 size 被忽略**，
表头就会和内容错位。

日志表格已改用 `fixed`，**每个列必须都有 `size`**
（没有的在 fixed 下退回内容 sizing，仍然会错位）。

### 7.2 徽章的 `w-fit` 会吃掉 `max-w-full`

`StatusBadge` 基础类里有 `inline-flex w-fit max-w-full`。
`w-fit` 让徽章按内容收缩，**直接抵消** `max-w-full`，
导致 `truncate` 拿不到确定宽度，长文本溢出到相邻列。

修复：在调用方 `className` 里用 `w-full` 覆盖（`twMerge` 会正确解析 `w-fit` → `w-full`）。

### 7.3 单元格外层不能用 `w-fit`

```tsx
<div className='flex w-fit flex-col'>   // ← 内容撑开，宽度失控
<div className='flex w-full min-w-0 flex-col'>  // ✓
```

---

## 八、本机工具链

### 8.1 磁盘经常满

- **C 盘**：`go-build` 缓存 4.6 GB 可清（`go clean -cache`，自动重建）
- **D 盘**：`bun-cache` 4.1 GB、`tmp-rex` 里的镜像包副本（服务器有）
- 构建 Go 前用 `TMP`/`TEMP` 把临时目录挪到 D 盘
- 磁盘满会导致 **docker buildkit 报 `read-only file system`**，
  需要 `sudo systemctl restart docker` 恢复

### 8.2 编码

- Windows 终端是 **GBK**，Python 输出中文会 `UnicodeEncodeError`
- 传脚本一律 **base64**，不用 heredoc
- SSH 看中文显示 `????` 是终端问题，库里是正常 UTF-8（用 hex 验证）

### 8.3 类型检查

```bash
cd web && ./node_modules/.bin/tsgo -b    # 不要用 bun 包装，内存不够
```

---

## 九、常用排查命令

```bash
# 渠道/分组配置（避开转义）
docker exec -e MYSQL_PWD=$PW MySQL_Onesoft mysql -uroot -N -e \
  "SELECT HEX(value) FROM options WHERE \`key\`='GroupRatio';" rex_db | \
  while read h; do python3 -c "import binascii,json;print(json.loads(binascii.unhexlify('$h').decode()))"; done

# 计费是否真的按表达式走（唯一可靠方式）
docker logs ReX-API 2>&1 | grep "record consume log" | tail -3
# 看 billing_mode / matched_tier / expr_b64

# 客户端 IP 是否真实
docker logs ReX-API 2>&1 | grep "GET /api/status" | tail -2

# 配置热加载
docker logs ReX-API 2>&1 | grep "syncing options" | tail -2
```

---

## 十、快速索引

| 主题 | 关键词 |
|---|---|
| 镜像放哪 | `/opt/1panel/apps/ReX-API/rexapi_linearteam/` |
| 构建/部署 | GitHub Actions + GHCR，`work/_deploy_ghcr6.py` |
| CI 红了 | `gh run view <id> --log-failed`；先疑 flaky |
| 计费配置不生效 | 反斜杠层数、`json.Unmarshal` 静默失败 |
| 验证数据库内容 | `SELECT HEX(value)` |
| 传脚本 | `base64 -w0`，不要 heredoc |
| 注册 IP 全是服务器 | Tunnel origin 指向公网 IP |
| 表头错位 | `tableLayout: 'fixed'` + 每列都有 `size` |
| 文字溢出 | 徽章 `w-fit` 吃掉 `max-w-full` |
| 毛利被抹掉 | 哨兵比较基准未含 margin |
| 并行分支合并 | 必扫静默重复符号 |
| 磁盘满 | `go clean -cache` / 清 `bun-cache` / 重启 docker |
| **价格显示长小数** | 系数是反算残留，改回「上游整数 × 倍数」 |
| **改了倍率不生效** | 只写了 `GroupRatio`，漏 `group_ratio_setting.group_ratio` |
| **保存分销商报错** | 无渠道分组的死配置，清理 `group_model_*` |
| **套餐卖一份亏一份** | 成本只看 `total_amount`，与缓存率无关 |
| **缓存率总是 50%** | 分母重复加了 cache_read |
| **分组看不到** | 无渠道分组被 `HideModelsWithoutEnabledChannel` 删了 |
| **成本算高了** | 把上游零售价当拿货价（忘了 distributor 折扣） |
| **价格体检** | `work/_audit_final.py` 七项逐项 OK/!! |
| **卡片价格显示 `-`** | 模型缺 ModelRatio/ModelPrice，不是分组问题 |
| **上游有数据但 /status 空** | 先查 `upstream_health_groups` 映射，再看 `/api/status-check` 该行有无 `source` |
| **镜像 403** | 上游挡 Python 默认 UA，Go/curl/浏览器 UA 正常 |
| **探针长期全红** | 查 `model_health.error_msg`：503 选错模型 / 403 被拦 / 价格未配置 |
| **新增上游渠道** | 见 4.13：channels+abilities+两张倍率表+UserUsableGroups+探测列表+镜像映射 |
| **分组改名** | 见 4.14：9 张表 + 6 组配置，`logs.group` 不动 |
| **MaiBot 换模型** | 见 4.15：key 有分组限制，先单模型试通再改 `model_config.toml` |
| **卡片价格显示 `-`（未定价）** | `scripts/check_unpriced_models.py`，见 4.17 |
| **套餐额度串组** | 见 4.18：`wallet_only_groups` whitelist |
| **内容审查没回复** | 见 4.19：捕获时序 + `CapturedResponseBody` |
| **充值手续费不对** | 见 4.20：`fee_rate` 是百分数且只认网关配置 |
| **NapCat 掉登录** | 见 4.22：`serverErrorCode:168` 是风控；巡检日志 + 邮件告警 |
| **群内发码没反应** | 见 4.16：ON_MESSAGE 是死代码 / 自消息不回传 / 手打 @ 无 at 段 |
## 4.23 上游分组改名会让镜像静默失效（2026-11-15）

**现象**：几个 Codex 分组的小绿条突然全变成自探测数据，卡片数值和上游对不上；`/api/status-check` 里这些分组 `models[].source` 不再是 `mirror:*`。

**根因**：上游把 perf-metrics 里的分组名整体改了（例：`plus_0720` → `chatgpt-plus【特惠】`、`codex-不降智` → `chatgpt-plus【稳定】`、`codex-不降智-高缓` → `chatgpt-pro【稳定】`、`cc-max` → `claude-max【企业】`）。`UpstreamMirrorRow` 先按模型名在 summary 里命中，再要求 detail 里存在同名 group，名字对不上就返回 false，调用方无声退回自探测——不报错、不告警。

**排查三步**（都在生产机跑，只读）：

```bash
# 1. 上游现在到底有哪些分组名：遍历 summary 的每个模型取 detail，并集
curl -s 'https://<上游>/api/perf-metrics/summary?hours=24' | python3 -c 'import sys,json;print([m["model_name"] for m in json.load(sys.stdin)["data"]["models"]])'
curl -s 'https://<上游>/api/perf-metrics?model=<模型>&hours=24' | python3 -c 'import sys,json;print([g["group"] for g in json.load(sys.stdin)["data"]["groups"]])'

# 2. 本站在线的「本地分组 → base_url」对照（abilities join channels，去重）
#    确认这个分组到底挂在哪家上游，再决定是改映射还是它根本不该镜像

# 3. 本地 /api/status-check 看目标分组的 models[].source 是否 mirror:<source>
```

**改映射**：`options.perf_metrics_setting.upstream_health_groups`（JSON，source→本地组→上游组）。改库不会热加载，`docker restart ReX-API` 后镜像缓存重建（detail 循环 15 分钟一轮，重启即跑一次）。

**命名对不上的判断依据**：用本站 `group_ratio_setting.group_ratio` 的售价从低到高，和上游分组档次（特惠 < 稳定 < 企业）对齐，别按字面猜——猜错就是两个分组显示同一份数据。

**注意**：`?` 只是 mysql 客户端终端编码问题，库里 JSON 是好的；要核对就 `SELECT HEX(value)` 再本地解码，别被 `?` 误导去"修编码"。

## 4.24 自建网关的营业时间

网关（`EpayGateways[].pay_methods` 所在的那个对象）支持 `schedule_enabled` / `schedule_start` / `schedule_end` / `schedule_timezone`：

- 窗口外 `GetAvailableEpayGateways()` 不返回该网关 → 充值页看不到这个支付方式；
- `RequestEpay` 二次校验，直接拒绝并回「该支付方式仅在 08:00-21:00 开放」；
- **回调/对账不受窗口限制**，20:59 下单 21:02 到账照样入账；
- 时间按 `Asia/Shanghai` 判断（容器是 UTC，代码里做了 +8 兜底）；
- 后台「Epay 网关」编辑框有开关和起止时间，保存时不会把字段吞掉。
## 4.25 Playground 包装成 API（2026-11-15）

**背景**：build.nvidia.com 的 playground 现在把 hCaptcha 关在 sandbox iframe 里，顶层 window.hcaptcha 恒为 undefined（注入 api.js 也没用），老的「页面铸 token + 直连 buildapi」方案彻底失效。

**可用方案**：不碰 token，直接驱动页面，读它自己的响应。

1. Camoufox(Firefox) 打开 playground
2. textarea.fill(prompt) 然后 press Enter
3. 页面自己铸 token、自己 POST buildapi（Playwright expect_response 捕获）
4. resp.text() 拿完整 SSE，解析 delta.content / reasoning_content / usage
5. 对客户端暴露 OpenAI 兼容接口（真流式做不到，按 24 字切片合成）

实现：D:\Code\NewApi\nim-proxy\playground_api.py（约 330 行，独立于老的 nim_proxy.py）

**实测**（本机，1 worker）：

```
GET  /health              {"status":"ok","workers_ready":1}
GET  /v1/models           kimi-k3.1-pro
POST /v1/chat/completions 非流式 25.7s -> content="hello world"，usage 120 tokens
POST 同上 stream:true     11.6s -> 标准 chat.completion.chunk + [DONE]
```

**渠道填法**：Base URL http://<host>:9766（不要带 /v1，new-api 自己拼），模型 kimi-k3.1-pro，密钥留空则随便填。

**坑**：
- textarea.click() 会被「Acknowledge & Continue」弹窗遮罩挡住（nv-modal-overlay 拦截 pointer events），只用 fill()，它走程序化聚焦。
- 页面内容在 shadow DOM（nv-* 组件）里，document.body.innerText 只拿得到 cookie 横幅，别指望 DOM 抓答案。
- 每个请求前 page.goto 重置页面，否则上一轮对话串进上下文；代价约 8-10s。
- --workers N = 并发数 = 同时开 N 个 Camoufox（每个 400-500 MB）。

**启动**：start-playground-api.bat（默认 2 workers，端口 9766）
## 4.26 Playground 代理的协议全兼容（2026-11-15）

客户端会按自己那套协议打 `/v1/*`，而 ReX 对 OpenAI 型渠道是**原样转发路径**的（不做 Responses→Chat 转换），所以代理必须自己认全下面这些入口，否则就是 404：

| 入口 | 处理 |
|---|---|
| `POST /v1/chat/completions` | 主路径 |
| `POST /v1/completions` | legacy，`prompt` → messages，回 `text_completion` |
| `POST /v1/messages` | Anthropic 信封（content 块 / tool_use / tool_result / thinking） |
| `POST /v1/messages/count_tokens` | 本地估算（字符数/4），不打上游、不计费 |
| `POST /v1/responses` | Responses API：`instructions`→system、`input`→messages、回 `output_text` |
| `POST /v1/responses/compact` | 同 `/v1/responses`（Codex 压缩上下文用） |

**三个必踩的坑**：

1. **路由判断必须剥查询串**：`self.path` 带着 `?beta=true`，`path.rstrip("/")` 永远匹配不上 `/v1/messages`。用 `urlsplit(self.path).path`。
2. **异常分支里的日志不能引用别的函数的局部变量**：`_forward` 里打 `clean_messages`（其实定义在 `do_POST`）→ 上游一报错就 `NameError`，请求线程死掉、socket 关闭，调用方看到的是 `do request failed` / EOF。日志一律包 `try/except`，只用本函数能拿到的变量。
3. **reasoning 兜底不能污染工具轮**：content 为空、有 `tool_calls` 时若把 reasoning 填进 content，客户端会把这一轮当成“最终回答”，只跑一轮就停。条件必须是 `not tool_calls and finish_reason != "tool_calls"`。

**NVCF 的校验口味**（错了就是 422/400）：

- 422：`stream_options.continuous_usage_stats`、`seed`、`max_tokens` 超模型上限、content 是数组
- 400：`role: tool/function`、空 content 消息、`reasoning_effort` 非 low/medium/high
- 兜底：422 用最小 payload 重试；400 把历史裁到最后 6 轮、每轮 8000 字重试
- `reasoning_effort` 默认值从 `max` 降到 `medium`，单轮从 20–60s 降到 7–11s（客户端超时多半是这个）

## 4.27 渠道协议覆盖审计（同一天实测）

| 渠道类型 | chat | messages | responses |
|---|---|---|---|
| type=1 OpenAI（furry / sheapi / mcapple / mjiutang） | ✅ | ✅（ReX 转 chat） | ✅（上游是 new-api） |
| type=58 NIM | ✅ | ❌ `404 model not found` | ✅ |
| 自建 playground 代理 | ✅ | ✅ | ✅ |

**结论**：type=1 三条全通；NIM（type 58）的 adaptor 没做 Claude→chat 转换，Claude 客户端落到 NIM 组会 404。要么路由避开，要么在 nim adaptor 里补转换。

另外 `relay-router.go` 里 `/v1/messages/count_tokens` 原来是被注释掉的（TODO），Claude Code 每轮前会调它，缺了就是 `404 Invalid URL`——已放开，复用现成的 `controller.CountClaudeTokens`。

## 4.28 NIM 死模型清理（channel 12，50/78）

`model_health` 里取每个模型**最新一次探测**，`error_code=404` 且 `error_msg` 含 `Function ... Not found` 的就是上游已下架的模型：

```sql
SELECT model_name FROM model_health mh
JOIN (SELECT model_name, MAX(probe_ts) ts FROM model_health WHERE `group`='NIM' GROUP BY model_name) l
  ON l.model_name=mh.model_name AND l.ts=mh.probe_ts
WHERE mh.`group`='NIM' AND mh.error_code=404;
```

处理：只改 `channels.id=12` 的 `models` 列 + `DELETE FROM abilities WHERE channel_id=12 AND model IN (...)`，**不要动同组的另一个渠道**（NIM 组还有 furry 的 channel 14，它供的模型是活的）。改完重启 ReX 刷 abilities 缓存。

结果：78 → 28，脚本 `work/_dead_models.py`（dry-run 默认，`--apply` 才写库），备份在 `/root/billing-backup/deadmodel-*.sql.gz`。

## 4.29 使用日志页的五个问题（同一天）

| 现象 | 根因 | 修法 |
|---|---|---|
| “总 Token”不是所选时间段 | 徽标用的是 `tpm`，后端只统计**最近 60 秒** | `Stat` 增加 `tokens`（按筛选条件 SUM prompt+completion），前端改用它 |
| 排除用户只支持逗号 | `strings.Split(raw, ",")` | 逗号换空格后 `strings.Fields`，逗号/空格/混合都行 |
| 排除用户不影响总数 | 统计接口没解析 `exclude_user_id` | 解析后加 `user_id NOT IN (...)`，列表和统计一致 |
| 按模型筛选必须输全名 | 后端对 `model_name` 是精确匹配（除非值里带 `%`） | 前端 `buildApiParams` 统一包 `%...%`，列表/统计/导出一起生效 |
| 悬浮层和背景糊在一起 | `popover`/`combobox` 只有 `ring-foreground/10`；liquid 主题的浮层选择器漏了 `combobox-content`/`context-menu`/`menubar`/`hover-card` | 组件加 `border + shadow-lg`，CSS 补全浮层 slot 列表 |

**测试联动**：改 `placeholder` 会让按文案取输入框的测试（`getByPlaceholderText`）失败。要么同步改测试，要么别动文案——这次选了后者，把提示改到 `title` 上。

**文案**：邀请返利的“法币充值”改成“人民币充值”（本站只有人民币充值）。