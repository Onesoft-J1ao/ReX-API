#!/usr/bin/env python3
# NapCat 在线巡检 + 邮件告警（状态变化时才发信）
import asyncio
import json
import os
import smtplib
import ssl
import time
from email.message import EmailMessage
from email.utils import formatdate

import websockets

CONF = "/root/napcat_health.conf"
STATE = "/root/.napcat_last_state"
LOG = "/var/log/napcat-health.log"


def load_conf():
    conf = {}
    with open(CONF, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            k, v = line.split("=", 1)
            conf[k.strip()] = v.strip()
    return conf


async def check(url):
    async with websockets.connect(url, open_timeout=5, close_timeout=3) as ws:
        await ws.send(json.dumps({"action": "get_status", "echo": "h"}))
        while True:
            data = json.loads(await asyncio.wait_for(ws.recv(), timeout=8))
            if data.get("echo") == "h":
                return bool((data.get("data") or {}).get("online"))


def send_mail(conf, subject, body):
    msg = EmailMessage()
    msg["Subject"] = subject
    msg["From"] = conf["from"]
    msg["To"] = conf["to"]
    msg["Date"] = formatdate(localtime=True)
    msg.set_content(body)

    ctx = ssl.create_default_context()
    with smtplib.SMTP(conf["host"], int(conf.get("port", "587")), timeout=20) as client:
        client.ehlo()
        client.starttls(context=ctx)
        client.ehlo()
        client.login(conf["user"], conf["pass"])
        client.send_message(msg)


def main():
    conf = load_conf()
    url = "ws://127.0.0.1:3001?access_token=" + conf["token"]

    try:
        online = asyncio.run(check(url))
        state = "online" if online else "offline"
    except Exception as exc:
        state = "unreachable:%s" % type(exc).__name__

    last = ""
    if os.path.exists(STATE):
        last = open(STATE, encoding="utf-8").read().strip()
    if state == last:
        return

    now = time.strftime("%Y-%m-%d %H:%M:%S")
    line = "%s %s -> %s\n" % (now, last or "<init>", state)
    with open(LOG, "a", encoding="utf-8") as fh:
        fh.write(line)
    open(STATE, "w", encoding="utf-8").write(state)

    # 首次记录不发信，只在真正的状态切换时告警
    if not last:
        return

    if state == "online":
        subject = "[NapCat] 已恢复在线"
        body = "机器人 QQ 3176977550 已重新在线。\n时间：%s\n" % now
    else:
        subject = "[NapCat] 掉线告警：%s" % state
        body = (
            "机器人 QQ 3176977550 掉线。\n时间：%s\n状态：%s\n\n"
            "排查：\n"
            "  docker logs --since 30m napcat-kurisu | tail -30\n"
            "  tail -20 /var/log/napcat-health.log\n"
            "常见原因：QQ 风控 serverErrorCode=168，需要手机 QQ 完成安全验证。\n" % (now, state)
        )

    try:
        send_mail(conf, subject, body)
    except Exception as exc:
        with open(LOG, "a", encoding="utf-8") as fh:
            fh.write("%s mail failed: %s\n" % (now, exc))


if __name__ == "__main__":
    main()
