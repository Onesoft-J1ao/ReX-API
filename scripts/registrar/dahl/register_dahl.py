#!/usr/bin/env python3
"""
Dahl Inference 账号注册脚本

全程 HTTP，无邮箱、无验证码、无浏览器。流程：
  1. GET  /v1/auth/check-username?username=X   检查用户名可用
  2. POST /v1/auth/signup                      注册 → fingerprint(32位) + api_key + key_id
  3. POST /v1/auth/signin                      用 fingerprint 换 session cookie
  4. POST /v1/account/allocate                 把账户池额度划给这个 key
  5. GET  /v1/account/keys                     回查确认

**两个必须知道的坑：**

  a) signup 返回的 api_key 是 **0 额度 / status=exhausted**，直接调用会 402。
     100M 新手额度进的是**账户池**，不在 key 上，必须先 allocate 才有用。
     本脚本默认把整池划给这一个 key。

  b) fingerprint 是**唯一的登录凭证**（相当于密码），只在 signup 时返回一次，
     官方文档明确写了不可找回、无邮箱找回。所以必须和 api_key 一起保存。

输出格式：`用户名|fingerprint|APIKey`

用法：
    python register_dahl.py
    python register_dahl.py --allocate 50000000    # 只划 5000 万
    python register_dahl.py --no-proxy             # 直连，快很多（省掉取 IP + 测速）
    python register_dahl.py --no-allocate          # 只要账号，不做额度划转
"""

import os
import re
import sys
import time
import json
import random
import string
import logging
import threading
from typing import Optional

import requests

# 共享的活 IP 池在上一层目录（几个项目共用一份实现）
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from proxy_pool import LiveProxyPool, StickyProxy  # noqa: E402

# ─── 日志 ─────────────────────────────────────────────────
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger(__name__)

# ─── Dahl 端点 ────────────────────────────────────────────
DAHL_BASE      = "https://inference.dahl.global"
SIGNUP_PATH    = "/v1/auth/signup"
SIGNIN_PATH    = "/v1/auth/signin"
CHECK_USER_PATH = "/v1/auth/check-username"
KEYS_PATH      = "/v1/account/keys"
ALLOCATE_PATH  = "/v1/account/allocate"

DEFAULT_UA = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
    "AppleWebKit/537.36 (KHTML, like Gecko) "
    "Chrome/150.0.0.0 Safari/537.36"
)

# 新手池总额（文档写的 100M），allocate 默认全划
WELCOME_TOKENS = 100_000_000

# ─── IP 代理池 ────────────────────────────────────────────
# dahl 实测不限流，但默认仍然走代理池：不烧本机 IP 信誉、并发能摊到多个出口。
# 想直连就加 --no-proxy（速度快很多，因为省掉取 IP + 测速）。
PROXIES: Optional[dict] = None            # 填了就固定用这个，优先级最高
# PROXIES = {"http": "http://127.0.0.1:7890", "https": "http://127.0.0.1:7890"}

USE_PROXY_POOL     = True
PROXY_VALIDATE_URL = DAHL_BASE + "/"      # 探针直接打目标站
PROXY_MAX_LATENCY  = 8.0                  # 往返超过这个秒数丢弃
REQUEST_TIMEOUT    = 25                   # 业务请求超时：不设的话坏代理会一直挂着
                                          # （实测要等 60 秒才报错，换 IP 重试就没意义了）

# 传输层故障：换 IP 重试。HTTP 错误码不算（链路是通的，是服务端在回应）
_NET_ERRORS = (
    requests.exceptions.SSLError,
    requests.exceptions.ProxyError,
    requests.exceptions.ConnectionError,
    requests.exceptions.Timeout,
    requests.exceptions.ChunkedEncodingError,
)

# ─── 落盘 ─────────────────────────────────────────────────
KEYS_FILE_DEFAULT = "dahl_keys.txt"   # 每行: 用户名|fingerprint|APIKey


def append_key(path: str, username: str, fingerprint: str, key: str) -> None:
    """账号信息追加写入。fingerprint 是唯一登录凭证，必须一起存"""
    with open(path, "a", encoding="utf-8") as f:
        f.write(f"{username}|{fingerprint}|{key}\n")
    log.info("    已追加写入 %s", path)


# ─── 代理池（可选）────────────────────────────────────────
# 取 IP / 测活 / 粘性复用 / 死亡踢除都在 ../proxy_pool.py 里。
# 粘性策略：测活通过的 IP 一直用，只有连续失败才踢掉换新的。
_proxy_pool: Optional[LiveProxyPool] = None
_proxy_pool_lock = threading.Lock()


def get_proxy_pool() -> LiveProxyPool:
    """拿全局活 IP 池（懒加载，只建一次）"""
    global _proxy_pool
    if _proxy_pool is None:
        with _proxy_pool_lock:
            if _proxy_pool is None:
                _proxy_pool = LiveProxyPool(
                    # 探针直接打 dahl 本体：有些 IP 上别的站没问题，到这里就是坏的
                    probe_url=PROXY_VALIDATE_URL,
                    state_file=os.path.join(
                        os.path.dirname(os.path.abspath(__file__)),
                        "proxy_pool_state.json"),
                    max_latency=PROXY_MAX_LATENCY,
                    min_live=3,
                )
    return _proxy_pool


def acquire_proxy(*_args, **_kwargs) -> Optional[dict]:
    """兼容旧接口：从活 IP 池拿一个可用代理

    实现已换成粘性池（见 ../proxy_pool.py）—— 验过的 IP 一直用，
    连续失败才踢掉换新的，不再每次现取 20 个 IP 全部测速。
    """
    return get_proxy_pool().get()


# ─── 账号生成 ─────────────────────────────────────────────

def generate_username() -> str:
    """dahl 用户名规则：3–20 字符，字母/数字/下划线"""
    return "u_" + "".join(random.choices(string.ascii_lowercase + string.digits, k=10))


# ─── 注册核心类 ───────────────────────────────────────────

class DahlRegistration:
    def __init__(self, username: str = "", allocate: int = WELCOME_TOKENS,
                 use_proxy: bool = USE_PROXY_POOL):
        self.username = username or generate_username()
        self.allocate_amount = allocate
        self.use_proxy = use_proxy

        self.session = requests.Session()
        self.session.trust_env = False      # 忽略系统环境代理，只用池里挑的
        self.session.headers.update({
            "User-Agent": DEFAULT_UA,
            "Accept": "application/json",
            "Origin": DAHL_BASE,
            "Referer": DAHL_BASE + "/account",
        })
        self.proxy_ip = ""
        # 粘性句柄：这个账号粘住一个活 IP 用到死为止。
        # 池子是进程级共享的，多个 worker 不会各取各的、重复测速。
        self._sticky: Optional[StickyProxy] = None
        if PROXIES:
            self.session.proxies = PROXIES
            self.proxy_ip = PROXIES.get("https", "").replace("http://", "")
        elif use_proxy:
            self._sticky = get_proxy_pool().sticky()
            self._attach_proxy()

        self.fingerprint: str = ""
        self.api_key: str = ""
        self.key_id: int = 0
        self.pool_after: int = 0

    def _attach_proxy(self) -> bool:
        """把粘性句柄当前粘着的代理挂到 session 上（不报失败）"""
        if not self._sticky:
            return False
        p = self._sticky.get()
        if not p:
            return False
        self.session.proxies = p
        self.proxy_ip = p["https"].replace("http://", "")
        return True

    def refresh_proxy(self) -> bool:
        """手上这个 IP 死了：踢掉它，换一个新的活 IP

        首次获取走 _attach_proxy 才行 —— 这里会**记一次失败**，
        新拿到的 IP 不该一上来就背着失败额度。
        """
        if PROXIES or not self.use_proxy:
            return bool(PROXIES)
        if not self._sticky:
            self._sticky = get_proxy_pool().sticky()
        self._sticky.fail()          # 连续失败够了才踢，不够只计数
        return self._attach_proxy()

    def swap_proxy(self) -> bool:
        """策略性换 IP：换个出口重试，但**不给当前 IP 记失败**

        区分两种换法：
          refresh_proxy()  传输层挂了 -> 记失败，够了就踢
          swap_proxy()     想换个出口重试，IP 本身是好的 -> 只轮换
        否则外层重试会把传输层的失败重复计一遍，误杀好 IP。
        """
        if PROXIES or not self.use_proxy:
            return bool(PROXIES)
        if not self._sticky:
            return self.refresh_proxy()
        self._sticky.rotate()
        return self._attach_proxy()

    def _request(self, method: str, url: str, **kw):
        """带换 IP 重试的请求

        网络层挂了就报给活 IP 池：连续失败够了会被踢掉，这里自动换一个。
        成功的请求也回报（失败计数清零），免得零星抖动累计成"死"。
        """
        kw.setdefault("timeout", REQUEST_TIMEOUT)
        last: Optional[BaseException] = None
        for attempt in range(1, 4):
            try:
                r = self.session.request(method, url, **kw)
            except _NET_ERRORS as e:
                last = e
                log.warning("    网络中断 (%s)，换 IP 重试 %d/3 — %s",
                            type(e).__name__, attempt, url.split("/")[-1][:50])
                if self._sticky and self._sticky.fail():
                    self._attach_proxy()
                elif attempt >= 3:
                    break
                continue
            if self._sticky:
                self._sticky.ok()
            return r
        assert last is not None
        raise last

    def _post(self, path: str, body: dict, ok=(200, 201)) -> dict:
        r = self._request("POST", DAHL_BASE + path, json=body)
        if r.status_code not in ok:
            raise RuntimeError(f"{path} 返回 {r.status_code}: {r.text[:200]}")
        return r.json() if r.content else {}

    def _get(self, path: str, ok=(200,)) -> dict:
        r = self._request("GET", DAHL_BASE + path)
        if r.status_code not in ok:
            raise RuntimeError(f"{path} 返回 {r.status_code}: {r.text[:200]}")
        return r.json() if r.content else {}

    # ── 步骤 ──

    def step1_check_username(self) -> None:
        d = self._get(f"{CHECK_USER_PATH}?username={self.username}")
        if not d.get("available", False):
            raise ValueError(f"用户名不可用: {self.username} ({d})")
        log.debug("    用户名 %s 可用", self.username)

    def step2_signup(self) -> None:
        log.info("[1] 注册 %s ...", self.username)
        d = self._post(SIGNUP_PATH, {"username": self.username})
        self.fingerprint = d.get("fingerprint", "")
        key = d.get("api_key") or {}
        self.api_key = key.get("token", "")
        self.key_id = key.get("id", 0)
        if not self.fingerprint or not self.api_key:
            raise RuntimeError(f"signup 返回缺字段: {str(d)[:200]}")
        log.info("    注册成功  fingerprint=%s  key_id=%s  初始额度=%s",
                 self.fingerprint[:12] + "...", self.key_id,
                 key.get("available_tokens", 0))

    def step3_signin(self) -> None:
        """用 fingerprint 换 session cookie —— 账号类接口都要它"""
        log.info("[2] 用 fingerprint 登录拿 session ...")
        self._post(SIGNIN_PATH, {"fingerprint": self.fingerprint})
        log.info("    session OK (%d 个 cookie)", len(self.session.cookies))

    def step4_allocate(self) -> None:
        """把账户池的额度划给这个 key。不做这步 key 就是废的（402）"""
        if not self.allocate_amount:
            log.info("[3] 跳过额度划转（--no-allocate）")
            return
        log.info("[3] 从账户池划 %s 额度给 key %s ...",
                 f"{self.allocate_amount:,}", self.key_id)
        d = self._post(ALLOCATE_PATH,
                       {"key_id": self.key_id, "amount": self.allocate_amount})
        k = d.get("key") or {}
        self.pool_after = d.get("token_balance", 0)
        log.info("    key 现有 %s 额度（status=%s），池余 %s",
                 f"{k.get('available_tokens', 0):,}", k.get("status"),
                 f"{self.pool_after:,}")

    def step5_verify(self) -> bool:
        log.info("[4] 回查 key 状态 ...")
        d = self._get(KEYS_PATH)
        for k in d.get("keys", []):
            if k.get("id") == self.key_id:
                ok = k.get("available_tokens", 0) > 0 or not self.allocate_amount
                log.info("    校验: id=%s status=%s available=%s",
                         k.get("id"), k.get("status"),
                         f"{k.get('available_tokens', 0):,}")
                return ok
        log.warning("    没在 key 列表里找到 %s", self.key_id)
        return False

    def print_summary(self) -> None:
        print("\n" + "=" * 66)
        print(f"  Username    : {self.username}")
        print(f"  Fingerprint : {self.fingerprint}    ← 唯一登录凭证，务必保存")
        print(f"  API Key     : {self.api_key}")
        print(f"  Key 额度    : {self.allocate_amount:,}  (池余 {self.pool_after:,})")
        if self.proxy_ip:
            print(f"  Proxy       : {self.proxy_ip}")
        print("=" * 66)

    def run(self, max_attempts: int = 4) -> str:
        last_exc: Optional[BaseException] = None
        for attempt in range(1, max_attempts + 1):
            try:
                self.step1_check_username()
                self.step2_signup()
                self.step3_signin()
                self.step4_allocate()
                self.step5_verify()
                self.print_summary()
                return self.api_key
            except ValueError as e:
                # 用户名冲突：换个名字重试，不用换 IP
                log.warning("%s，换用户名重试", e)
                self.username = generate_username()
                continue
            except (requests.exceptions.RequestException, RuntimeError) as e:
                last_exc = e
                log.warning("中断 (%s)，换 IP 重试 %d/%d — %s",
                            type(e).__name__, attempt, max_attempts, str(e)[:140])
                if attempt < max_attempts:
                    # 用 swap 不用 refresh：传输层的失败 _request 里已经报过了，
                    # 这里再记一次就成了双重计数，两次业务重试就能杀掉一个快 IP
                    self.swap_proxy()
                    time.sleep(1)
                    continue
                break
        assert last_exc is not None
        raise last_exc


# ─── 入口 ─────────────────────────────────────────────────

def main():
    import argparse

    parser = argparse.ArgumentParser(description="Dahl Inference 账号注册")
    parser.add_argument("--username", default="", help="指定用户名（留空随机）")
    parser.add_argument("-o", "--output", default=KEYS_FILE_DEFAULT, metavar="FILE",
                        help="输出文件（默认 %s）" % KEYS_FILE_DEFAULT)
    parser.add_argument("--allocate", type=int, default=WELCOME_TOKENS,
                        help="给 key 划多少额度（默认整池 %d）" % WELCOME_TOKENS)
    parser.add_argument("--no-allocate", action="store_true",
                        help="不划转额度（key 会是 0 额度、不可用）")
    parser.add_argument("--no-proxy", action="store_true",
                        help="不走 IP 池，直接连（快很多，但烧本机 IP）")
    parser.add_argument("--debug", action="store_true")
    args = parser.parse_args()

    if args.debug:
        logging.getLogger().setLevel(logging.DEBUG)

    reg = DahlRegistration(
        username=args.username,
        allocate=0 if args.no_allocate else args.allocate,
        use_proxy=not args.no_proxy,
    )
    key = reg.run()
    if key:
        append_key(args.output, reg.username, reg.fingerprint, key)


if __name__ == "__main__":
    main()
