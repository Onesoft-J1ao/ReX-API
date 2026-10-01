#!/usr/bin/env python3
"""
有状态的活 IP 池 —— 粘性复用 + 死了才换

和旧的 acquire_proxy() 的区别
------------------------------
旧逻辑：每次要代理都**现取一批 IP、全部测速、挑最快的、用完就扔**。
        结果 4 个 worker 各取各的、重复取重复测，而且手上那个明明还很快，
        下一次请求又不要了 —— 纯浪费。

新逻辑：
    1. 测活    取到的 IP 先验证能不能上目标站点，通过才算"活"
    2. 一直用  **粘性**：验证通过后一直用它，不轮换、不按时间踢
              （池子里有些 IP 是真的快，不能因为"用久了"就丢掉）
    3. 死了才换 连续失败到阈值才踢掉，从池里补一个活的进来
              单次抖动不算死 —— 网络偶尔抽一下不能把快 IP 误杀

两种用法
--------
单线程 / 想让调用方自己管粘性::

    from proxy_pool import LiveProxyPool
    pool = LiveProxyPool(probe_url="https://api.ngc.nvidia.com/")
    ip = pool.pick()                 # 拿一个活的
    pool.report_fail(ip)             # 它死了 -> 踢掉
    ip = pool.pick()                 # 自动补一个新的

多 worker（每个 worker 粘一个，互不干扰）::

    proxy = pool.sticky()            # 每个 worker 一个句柄
    requests.get(url, proxies=proxy.get())
    proxy.ok()                       # 成功 -> 失败计数清零
    proxy.fail()                     # 失败 -> 够了就自动换新的，get() 拿到新 IP

落盘
----
活的 IP 和已确认死掉的 IP 都写进 JSON，下次启动直接复用：
    - 活的：跳过测速，直接开工（冷启动省十几秒）
    - 死的：不再浪费时间重测

文件按 `probe_url` 区分 —— 同一个 IP 对 ngc 快、对 hcaptcha 可能是坏的，
所以不同站点的池子不能混用。
"""

from __future__ import annotations

import json
import logging
import os
import random
import re
import threading
import time
from concurrent.futures import ThreadPoolExecutor, as_completed
from dataclasses import dataclass, field
from typing import Optional

import requests

log = logging.getLogger(__name__)

# ─── 池子接口 ─────────────────────────────────────────────

POOL_API              = "http://139.224.75.133:5555/random"
POOL_TIMEOUT          = 10     # 取 IP 的超时
POOL_API_CONCURRENCY  = 40     # 同时打向池子接口的请求上限（避免打爆连接池）
POOL_FETCH_WORKERS    = 10     # 并发取 IP 的线程数

# ─── 测活 ─────────────────────────────────────────────────

PROBE_TIMEOUT         = 6      # 单个 IP 测活超时
PROBE_MAX_LATENCY     = 6.0    # 往返超过这个秒数算不合格（慢 IP 跑重定向链必挂）
PROBE_WORKERS         = 10     # 并发测活线程数
PROBE_ROUNDS          = 4      # 合格候选不足时最多再取几轮

# ─── 判定"死" ─────────────────────────────────────────────
# 单次网络抖动不算死：有些快 IP 偶尔抽一下，立刻丢掉太亏。
# 连续失败到阈值才踢。
FAIL_THRESHOLD        = 2      # 连续失败几次算死

# ─── 落盘 ─────────────────────────────────────────────────

STATE_FILE_DEFAULT    = "proxy_pool_state.json"
DEAD_MAX_KEEP         = 3000   # 死亡名单最多记多少条，防止文件无限膨胀

_IP_RE = re.compile(r"\d{1,3}(?:\.\d{1,3}){3}:\d{1,5}")

_UA = {
    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
                  "AppleWebKit/537.36 (KHTML, like Gecko) "
                  "Chrome/150.0.0.0 Safari/537.36"
}

# 池子接口一律直连：不能走代理去取代理
_direct = requests.Session()
_direct.trust_env = False
_adapter = requests.adapters.HTTPAdapter(
    pool_connections=64, pool_maxsize=64, pool_block=True)
_direct.mount("http://", _adapter)
_direct.mount("https://", _adapter)
_pool_api_sem = threading.Semaphore(POOL_API_CONCURRENCY)


def _proxy_dict(ip: str) -> dict:
    return {"http": f"http://{ip}", "https": f"http://{ip}"}


def _is_valid_ip(text: str) -> bool:
    return bool(_IP_RE.fullmatch(text.strip()))


# ─── 单条记录 ─────────────────────────────────────────────

@dataclass
class ProxyInfo:
    ip: str
    latency: float = 99.0
    consecutive_fails: int = 0      # 连续失败次数，成功即清零
    ok_count: int = 0               # 累计成功请求数
    added_at: float = field(default_factory=time.time)

    def to_json(self) -> dict:
        return {"ip": self.ip, "latency": self.latency, "ok_count": self.ok_count}

    @classmethod
    def from_json(cls, d: dict) -> "ProxyInfo":
        return cls(ip=d["ip"], latency=float(d.get("latency", 99.0)),
                   ok_count=int(d.get("ok_count", 0)))


# ─── 池 ───────────────────────────────────────────────────

class LiveProxyPool:
    """有状态的活 IP 池。线程安全。

    probe_urls 决定"活"的定义 —— 必须打**真正决定成败的域名**：
    池子里有大量 IP 能上站点首页却过不了它的 API / 验证码域名，
    用首页当探针会选出一堆"看着能用、实际不行"的代理。

    可以传多个域名（列表）：**全部通过**才算活，延迟取最慢的那个。
    比如 opencode 打码页要同时从 js.hcaptcha.com 和 newassets.hcaptcha.com
    拉资源，只通一个还是渲染不出验证码框。
    """

    def __init__(self, probe_url,
                 state_file: str = STATE_FILE_DEFAULT,
                 fail_threshold: int = FAIL_THRESHOLD,
                 max_latency: float = PROBE_MAX_LATENCY,
                 min_live: int = 2):
        urls = [probe_url] if isinstance(probe_url, str) else list(probe_url)
        urls = [u for u in urls if u]
        if not urls:
            raise ValueError("probe_url 不能为空 —— 没有探针就不知道 IP 算不算活")
        self.probe_urls = urls
        self.probe_url = urls[0]        # 兼容单探针的旧调用
        self.state_file = state_file
        self.fail_threshold = fail_threshold
        self.max_latency = max_latency
        self.min_live = min_live        # 池里低于这个数就去补货

        self._lock = threading.RLock()
        self._live: dict[str, ProxyInfo] = {}
        self._dead: set[str] = set()
        self._refilling = False
        self._load()

    @property
    def _state_key(self) -> str:
        """状态文件按探针集合区分 —— 同一 IP 对 ngc 快、对 hcaptcha 可能是坏的"""
        return "|".join(self.probe_urls)

    # ── 落盘 ──

    def _load(self) -> None:
        """读上次留下的活/死名单。probe_url 不匹配就整份作废"""
        if not self.state_file or not os.path.exists(self.state_file):
            return
        try:
            with open(self.state_file, encoding="utf-8") as f:
                d = json.load(f)
        except Exception as e:
            log.debug("代理池状态文件读不了(%s)，当空的用", e)
            return

        if d.get("probe_url") != self._state_key:
            log.debug("状态文件是别的探针(%s)留的，作废", d.get("probe_url"))
            return

        self._dead = set(d.get("dead") or [])
        for item in d.get("live") or []:
            try:
                info = ProxyInfo.from_json(item)
            except Exception:
                continue
            # 死亡名单优先：上次踢掉的别又捡回来
            if info.ip in self._dead:
                continue
            self._live[info.ip] = info
        if self._live:
            log.info("    代理池: 复用上次留下的 %d 个活 IP（跳过测速）", len(self._live))

    def save(self) -> None:
        if not self.state_file:
            return
        with self._lock:
            snapshot = {
                "probe_url": self._state_key,
                "saved_at": int(time.time()),
                "live": [i.to_json() for i in
                         sorted(self._live.values(), key=lambda x: x.latency)],
                # 只留最近的若干条，别让文件无限涨
                "dead": sorted(self._dead)[-DEAD_MAX_KEEP:],
            }
        try:
            tmp = self.state_file + ".tmp"
            with open(tmp, "w", encoding="utf-8") as f:
                json.dump(snapshot, f, indent=2)
            os.replace(tmp, self.state_file)
        except Exception as e:
            log.debug("代理池状态写盘失败: %s", e)

    # ── 取 IP + 测活 ──

    def _fetch_one(self) -> Optional[str]:
        try:
            with _pool_api_sem:
                r = _direct.get(POOL_API, timeout=POOL_TIMEOUT)
            if r.status_code != 200:
                return None
            ip = r.text.strip()
            return ip if _is_valid_ip(ip) else None
        except Exception:
            return None

    def _fetch_many(self, n: int) -> list:
        """并发取 n 个不重复 IP"""
        out: list = []
        ex = ThreadPoolExecutor(max_workers=min(POOL_FETCH_WORKERS, max(1, n)))
        try:
            for fut in as_completed([ex.submit(self._fetch_one) for _ in range(n)]):
                ip = fut.result()
                if ip and ip not in out:
                    out.append(ip)
        finally:
            ex.shutdown(wait=False, cancel_futures=True)
        return out

    def probe(self, ip: str) -> Optional[float]:
        """测活：所有探针域名全通返回**最慢**的往返秒数，任一不通返回 None

        取最慢而不是最快的：只要有一个域名慢，整个流程就会卡在那里。
        """
        pd = _proxy_dict(ip)
        worst = 0.0
        for url in self.probe_urls:
            t0 = time.time()
            try:
                r = requests.get(url, proxies=pd, timeout=PROBE_TIMEOUT,
                                 headers=_UA, allow_redirects=False)
            except Exception:
                return None
            # 4xx 也算活：链路通了，只是站点拒了这个请求
            if not (200 <= r.status_code < 500):
                return None
            worst = max(worst, time.time() - t0)
        return worst

    def refill(self, want: int = PROBE_ROUNDS) -> int:
        """从池子取新的 IP 并测活，补进 _live。返回补进几个"""
        with self._lock:
            if self._refilling:
                return 0
            self._refilling = True
        try:
            added = 0
            for rnd in range(1, PROBE_ROUNDS + 1):
                with self._lock:
                    skip = set(self._live) | self._dead
                n = max(10, want * 5)
                ips = [ip for ip in self._fetch_many(n) if ip not in skip]
                if not ips:
                    break

                with ThreadPoolExecutor(max_workers=PROBE_WORKERS) as ex:
                    futs = {ex.submit(self.probe, ip): ip for ip in ips}
                    for fut in as_completed(futs):
                        ip = futs[fut]
                        lat = fut.result()
                        if lat is None or lat > self.max_latency:
                            with self._lock:
                                self._dead.add(ip)     # 慢 IP 也进死亡名单，别再重测
                            continue
                        with self._lock:
                            self._live[ip] = ProxyInfo(ip=ip, latency=lat)
                        added += 1
                        log.info("    测活通过: %s (%.2fs)", ip, lat)

                with self._lock:
                    if len(self._live) >= self.min_live:
                        break
                if rnd < PROBE_ROUNDS:
                    log.debug("    活 IP 只有 %d 个，再取一轮", len(self._live))
            return added
        finally:
            with self._lock:
                self._refilling = False

    # ── 分配 ──

    def pick(self, exclude: Optional[set] = None) -> Optional[ProxyInfo]:
        """拿一个活 IP。优先挑**最快**的（快 IP 值得反复用）。

        exclude 用于避开同进程里别的 worker 正粘着的 IP。
        """
        with self._lock:
            cands = [i for i in self._live.values()
                     if not exclude or i.ip not in exclude]
            if cands:
                return min(cands, key=lambda x: x.latency)

        self.refill()
        with self._lock:
            cands = [i for i in self._live.values()
                     if not exclude or i.ip not in exclude]
            return min(cands, key=lambda x: x.latency) if cands else None

    def get(self, exclude: Optional[set] = None) -> Optional[dict]:
        """便捷方法，直接返回 requests 能用的 proxies dict"""
        info = self.pick(exclude)
        return _proxy_dict(info.ip) if info else None

    # ── 反馈 ──

    def is_live(self, ip: str) -> bool:
        with self._lock:
            return ip in self._live

    def report_ok(self, ip: str) -> None:
        """这个 IP 刚成功了一次：失败计数清零（单次抖动不该累计成死）"""
        with self._lock:
            info = self._live.get(ip)
            if info:
                info.consecutive_fails = 0
                info.ok_count += 1

    def report_fail(self, ip: str) -> bool:
        """这个 IP 刚失败一次。连续失败到阈值就踢掉，返回是否踢了"""
        with self._lock:
            info = self._live.get(ip)
            if not info:
                return False
            info.consecutive_fails += 1
            if info.consecutive_fails < self.fail_threshold:
                log.debug("    %s 失败 %d/%d，先留着（可能只是抖动）",
                          ip, info.consecutive_fails, self.fail_threshold)
                return False
            del self._live[ip]
            self._dead.add(ip)
        log.warning("    IP 已死，踢掉: %s（连续失败 %d 次）", ip, self.fail_threshold)
        self.save()
        return True

    def sticky(self, exclude: Optional[set] = None) -> "StickyProxy":
        """给一个 worker 用的粘性句柄"""
        return StickyProxy(self, exclude)

    # ── 统计 ──

    def stats(self) -> dict:
        with self._lock:
            return {
                "live": len(self._live),
                "dead": len(self._dead),
                "fastest": min((i.latency for i in self._live.values()),
                               default=None),
                "total_ok": sum(i.ok_count for i in self._live.values()),
            }


# ─── 粘性句柄 ─────────────────────────────────────────────

class StickyProxy:
    """一个 worker 粘着一个活 IP，用到死为止。

    用法::

        p = pool.sticky()
        requests.get(url, proxies=p.get())
        p.ok()     # 成功
        p.fail()   # 失败 -> 够了就换，下一次 p.get() 是新 IP
    """

    def __init__(self, pool: LiveProxyPool, exclude: Optional[set] = None):
        self._pool = pool
        self._exclude = exclude or set()
        self._info: Optional[ProxyInfo] = None
        self._lock = threading.Lock()

    def get(self) -> Optional[dict]:
        """当前粘着的代理 dict；没粘上或已死就自动补一个"""
        with self._lock:
            if self._info is None:
                self._info = self._pool.pick(exclude=self._exclude)
                if self._info:
                    log.info("    粘住代理: %s (%.2fs)", self._info.ip, self._info.latency)
            elif not self._pool.is_live(self._info.ip):
                # 被别人踢了（比如多个 worker 共用一个 IP 时）
                self._info = None
                return self.get()
            return _proxy_dict(self._info.ip) if self._info else None

    @property
    def ip(self) -> str:
        return self._info.ip if self._info else ""

    def ok(self) -> None:
        with self._lock:
            if self._info:
                self._pool.report_ok(self._info.ip)

    def rotate(self) -> bool:
        """换一个活 IP，但**不算当前这个失败**

        用于"策略性换 IP"：比如某个步骤失败了想换个出口重试，
        但那个 IP 本身没坏（请求根本没到传输层就失败了）。
        这种换法不该把好 IP 计入失败，否则两个步骤失误就能误杀一个快 IP。
        """
        with self._lock:
            old = self._info.ip if self._info else ""
            # 老 IP 留在池子里给别的 worker 用，只是自己不再粘它
            avoid = self._exclude | ({old} if old else set())
            self._info = self._pool.pick(exclude=avoid)
            if self._info:
                log.info("    换用代理: %s (%.2fs)", self._info.ip, self._info.latency)
                return True
        return False

    def fail(self) -> bool:
        """报一次失败。返回 True 表示这个 IP 已被踢、下面 get() 会换新的"""
        with self._lock:
            if not self._info:
                return False
            evicted = self._pool.report_fail(self._info.ip)
            if evicted:
                self._info = None
            return evicted
