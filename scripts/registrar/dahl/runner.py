#!/usr/bin/env python3
"""ReX-API registrar runner for Dahl Inference.

The gateway executes this script and reads one JSON account per stdout line.
Everything else — progress, warnings, tracebacks — goes to stderr, so stdout
stays a pure record stream the host can parse line by line:

    {"username": "...", "fingerprint": "...", "api_key": "dahl_..."}

The account flow itself lives in register_dahl.py; this file only adds the
machine-readable output, the worker pool, and a stop condition, so the gateway
never has to parse a text log or a result file.

Usage:
    python3 runner.py --count 20 --workers 8 --no-proxy
"""

import argparse
import json
import logging
import signal
import sys
import threading
import time

from register_dahl import DahlRegistration, WELCOME_TOKENS

# Logging must not touch stdout: the host parses every stdout line as an account.
logging.basicConfig(
    level=logging.INFO,
    stream=sys.stderr,
    format="%(asctime)s [%(levelname)s] [%(threadName)s] %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger("runner")

stop_event = threading.Event()
_count_lock = threading.Lock()
_reserved = 0   # slots claimed but not yet emitted
_done = 0       # records already emitted


def claim_slot(target: int) -> bool:
    """Reserve one of the target slots.

    A slot is reserved before the account is registered, so the number of
    in-flight registrations plus the number already emitted can never exceed the
    requested count. That keeps the host's batch cap exact instead of letting
    the last workers overshoot it.
    """
    global _reserved
    with _count_lock:
        if _done + _reserved >= target:
            return False
        _reserved += 1
        return True


def release_slot() -> None:
    global _reserved
    with _count_lock:
        _reserved -= 1


def complete_slot(target: int) -> int:
    global _reserved, _done
    with _count_lock:
        _reserved -= 1
        _done += 1
        done = _done
        if done >= target:
            stop_event.set()
        return done


def emit(account: dict) -> None:
    """Write one account record and flush immediately.

    Flushing matters: the gateway persists each account as the line arrives, so
    a run that is cancelled part-way still keeps what it already registered.
    """
    sys.stdout.write(json.dumps(account) + "\n")
    sys.stdout.flush()


def record_success(reg: DahlRegistration, target: int) -> int:
    """Emit one registered account and return the running success count."""
    emit({
        "username": reg.username,
        "fingerprint": reg.fingerprint,
        "api_key": reg.api_key,
    })
    return complete_slot(target)


def worker(worker_id: int, target: int, allocate: int, use_proxy: bool) -> None:
    ok_n = fail_n = 0
    while not stop_event.is_set():
        if not claim_slot(target):
            # Every slot is either emitted or in flight. A peer may still fail
            # and release one, so wait for that instead of exiting and leaving
            # the batch short.
            time.sleep(0.2)
            continue
        reg = DahlRegistration(allocate=allocate, use_proxy=use_proxy)
        try:
            key = reg.run()
        except Exception as exc:  # one failed account must not end the run
            fail_n += 1
            release_slot()
            log.warning("注册失败: %s — %s", reg.username, exc)
            time.sleep(1)
            continue
        if not key:
            fail_n += 1
            release_slot()
            log.warning("注册完成但没拿到 Key: %s", reg.username)
            continue
        ok_n += 1
        done = record_success(reg, target)
        log.info("注册成功 #%d/%d: %s", done, target, key[:20] + "...")
    log.info("worker %d 退出 (成功:%d 失败:%d)", worker_id, ok_n, fail_n)


def main() -> int:
    parser = argparse.ArgumentParser(description="Dahl Inference 注册 runner（JSON 行输出）")
    parser.add_argument("--count", type=int, required=True, help="成功注册多少个后停止")
    parser.add_argument("--workers", type=int, default=8, help="并发数（默认 8）")
    parser.add_argument("--allocate", type=int, default=WELCOME_TOKENS,
                        help="每个 key 划多少额度（默认整池 %d）" % WELCOME_TOKENS)
    parser.add_argument("--no-proxy", action="store_true",
                        help="直连，不走 IP 池")
    parser.add_argument("--json", action="store_true",
                        help="JSON 行输出（默认且唯一的输出模式，保留该开关以兼容调用方）")
    parser.add_argument("--debug", action="store_true")
    args = parser.parse_args()

    if args.debug:
        logging.getLogger().setLevel(logging.DEBUG)

    if args.count <= 0:
        log.error("--count 必须为正数")
        return 2

    workers = max(1, min(args.workers, 32))
    log.info("目标 %d 个，%d 并发，每 key 划 %s 额度，%s",
             args.count, workers, f"{args.allocate:,}",
             "直连" if args.no_proxy else "走 IP 池")

    def on_sigterm(_signum, _frame):
        log.info("收到终止信号，正在退出...")
        stop_event.set()

    signal.signal(signal.SIGTERM, on_sigterm)
    signal.signal(signal.SIGINT, on_sigterm)

    threads = []
    for i in range(workers):
        thread = threading.Thread(target=worker, args=(i, args.count, args.allocate, not args.no_proxy),
                                  name=f"worker-{i}", daemon=True)
        thread.start()
        threads.append(thread)

    for thread in threads:
        thread.join()

    if _done < args.count:
        log.error("只注册了 %d/%d 个", _done, args.count)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
