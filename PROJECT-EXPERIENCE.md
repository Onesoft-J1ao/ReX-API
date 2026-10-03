# ReX-API 项目经验汇总

> 本文档汇总 ReX-API（new-api fork）运维与开发中沉淀的实战经验。
> 每一节都是踩过坑之后写下的，**不是从文档抄的**。
> 最后更新：2026-10-03

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
| 计费配置不生效 | 反斜杠层数、`json.Unmarshal` 静默失败 |
| 验证数据库内容 | `SELECT HEX(value)` |
| 传脚本 | `base64 -w0`，不要 heredoc |
| 注册 IP 全是服务器 | Tunnel origin 指向公网 IP |
| 表头错位 | `tableLayout: 'fixed'` + 每列都有 `size` |
| 文字溢出 | 徽章 `w-fit` 吃掉 `max-w-full` |
| 毛利被抹掉 | 哨兵比较基准未含 margin |
| 并行分支合并 | 必扫静默重复符号 |
| 磁盘满 | `go clean -cache` / 清 `bun-cache` / 重启 docker |