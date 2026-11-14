#!/usr/bin/env python3
"""检测「未定价模型」：在启用渠道的分组里，既没有倍率也没有按次价、也没有
表达式、也没有分组级 pin 的模型。

判定口径（与定价页显示 `-` 一致）：
    ModelRatio[m] 存在
    或 ModelPrice[m] 存在
    或 billing_setting.billing_mode[m] == "tiered_expr"（且 billing_expr[m] 存在）
    或 billing_setting.group_model_ratio[g][m] / group_model_price[g][m] 存在
    或 billing_setting.group_pricing_source[g][m] 指定了 ratio/price/tiered_expr

用法（仓库根目录执行）：
    python scripts/check_unpriced_models.py
    python scripts/check_unpriced_models.py --json

退出码：0 = 全部有价；1 = 存在未定价模型；2 = 连接/查询失败。
依赖：work/mny.py（SSH 助手，含生产库凭据，已被 .gitignore 忽略）。
"""
import binascii
import collections
import json
import os
import sys
import tempfile

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, os.path.join(ROOT, "work"))
import mny  # noqa: E402

PASSWORD_FILE = "/root/migration/mysql_root_pw.txt"


def sql(client, query):
    handle, path = tempfile.mkstemp(suffix=".sql")
    os.close(handle)
    try:
        with open(path, "w", encoding="utf-8", newline="\n") as fh:
            fh.write(query)
        mny.put(client, path, "/root/_unpriced_q.sql")
    finally:
        try:
            os.remove(path)
        except OSError:
            pass
    return mny.run_capture(
        client,
        "docker exec -i MySQL_Onesoft mysql -uroot -p$(cat %s) -N -B --default-character-set=utf8mb4 "
        "rex_db < /root/_unpriced_q.sql 2>/dev/null" % PASSWORD_FILE,
        timeout=180,
    )


def load_json(client, key, default):
    out = sql(client, "SELECT HEX(value) FROM options WHERE `key`='%s';" % key).strip()
    if not out:
        return default
    try:
        return json.loads(binascii.unhexlify(out).decode("utf-8"))
    except Exception:
        return default


def main():
    as_json = "--json" in sys.argv
    client = mny.connect()
    try:
        rows = []
        for line in sql(client, "SELECT HEX(model), HEX(abilities.group) FROM abilities WHERE enabled=1;").splitlines():
            parts = line.split("\t")
            if len(parts) == 2:
                rows.append((bytes.fromhex(parts[0]).decode("utf-8"), bytes.fromhex(parts[1]).decode("utf-8")))

        mr = load_json(client, "ModelRatio", {})
        mp = load_json(client, "ModelPrice", {})
        bm = load_json(client, "billing_setting.billing_mode", {})
        ex = load_json(client, "billing_setting.billing_expr", {})
        gmr = load_json(client, "billing_setting.group_model_ratio", {})
        gmp = load_json(client, "billing_setting.group_model_price", {})
        gps = load_json(client, "billing_setting.group_pricing_source", {})
    finally:
        client.close()

    groups = collections.defaultdict(set)
    for model, group in rows:
        groups[group].add(model)

    result = collections.OrderedDict()
    for group in sorted(groups):
        missing = []
        for model in sorted(groups[group]):
            if gps.get(group, {}).get(model) in ("ratio", "price", "tiered_expr"):
                continue
            if model in mr or model in mp:
                continue
            if bm.get(model) == "tiered_expr" and ex.get(model):
                continue
            if model in gmr.get(group, {}) or model in gmp.get(group, {}):
                continue
            missing.append(model)
        if missing:
            result[group] = missing

    total = sum(len(v) for v in result.values())
    if as_json:
        print(json.dumps({"unpriced": result, "total": total}, ensure_ascii=False, indent=2))
    else:
        if not result:
            print("unpriced models: 0")
        for group, models in result.items():
            print("%-20s %2d: %s" % (group, len(models), ", ".join(models)))
        print("total:", total)
    return 0 if total == 0 else 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as exc:  # noqa: BLE001
        sys.stderr.write("check failed: %s\n" % exc)
        sys.exit(2)
