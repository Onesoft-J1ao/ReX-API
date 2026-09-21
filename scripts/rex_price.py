#!/usr/bin/env python3
"""ReX-API model price control script.

Reads the live pricing state, diffs it against an upstream reference, and can
apply price changes back to the gateway. Written for a local agent to drive:
every command prints JSON on stdout and exits non-zero on failure, so it can be
called from a tool loop without parsing prose.

The gateway's own option endpoints are the write path. A backend code change
would need a new image; an option change takes effect immediately.

Usage:
    python rex_price.py show                      # current prices
    python rex_price.py compare --against furry   # our prices vs the reference site
    python rex_price.py apply --file changes.json # apply a change set
    python rex_price.py set --model glm-5.3 --ratio 0.48
    python rex_price.py set --model foo --price 0.1
    python rex_price.py catalog                   # models.dev metadata coverage

Credentials come from the environment so nothing secret lands in this file:
    REX_BASE_URL   default https://ai.j1ao.vip
    REX_TOKEN      dashboard access token (required for writes)
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

DEFAULT_BASE = "https://ai.j1ao.vip"
DEFAULT_REFERENCE = "https://ai.furry.vg"
TIMEOUT = 60

# Cloudflare answers a bare urllib User-Agent with error 1010 (browser signature
# banned), so every request identifies itself as an ordinary client.
USER_AGENT = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"
)


def base_url() -> str:
    return os.environ.get("REX_BASE_URL", DEFAULT_BASE).rstrip("/")


def token() -> str:
    value = os.environ.get("REX_TOKEN", "").strip()
    if not value:
        raise SystemExit("REX_TOKEN is not set; writes need a dashboard access token")
    return value


def request(url: str, *, method: str = "GET", body: dict | None = None, auth: bool = False) -> dict:
    data = None
    headers = {"Accept": "application/json", "User-Agent": USER_AGENT}
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    if auth:
        headers["Authorization"] = f"Bearer {token()}"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=TIMEOUT) as resp:
            raw = resp.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        raise SystemExit(f"HTTP {exc.code} from {url}: {raw[:500]}") from exc
    except urllib.error.URLError as exc:
        raise SystemExit(f"cannot reach {url}: {exc.reason}") from exc
    try:
        return json.loads(raw)
    except json.JSONDecodeError as exc:
        raise SystemExit(f"non-JSON response from {url}: {raw[:300]}") from exc


def fetch_pricing(base: str) -> dict:
    return request(f"{base}/api/pricing")


def index_models(payload: dict) -> dict[str, dict]:
    return {m["model_name"]: m for m in payload.get("data", []) if m.get("model_name")}


# --------------------------------------------------------------------------
# Price shape
# --------------------------------------------------------------------------

# The fields that together define what a model costs. `quota_type` selects the
# billing shape: 0 bills by token ratio, 1 bills a flat price per request.
PRICE_FIELDS = (
    "quota_type",
    "model_ratio",
    "completion_ratio",
    "cache_ratio",
    "create_cache_ratio",
    "image_ratio",
    "audio_ratio",
    "audio_completion_ratio",
    "model_price",
    "billing_mode",
    "billing_expr",
)


def price_snapshot(model: dict) -> dict:
    return {field: model.get(field) for field in PRICE_FIELDS}


# Numeric price fields where "absent" and "zero" both mean "no charge". The
# gateway omits a ratio it has no value for while a reference site may carry an
# explicit 0, and treating those as different flagged nearly every model as
# changed.
NUMERIC_PRICE_FIELDS = {
    "model_ratio",
    "completion_ratio",
    "cache_ratio",
    "create_cache_ratio",
    "image_ratio",
    "audio_ratio",
    "audio_completion_ratio",
    "model_price",
}


def normalize(field: str, value: object) -> object:
    if field in NUMERIC_PRICE_FIELDS and value is None:
        return 0
    return value


def significant(local: dict, upstream: dict, fields: tuple[str, ...] = PRICE_FIELDS) -> dict:
    """Return the fields that differ, ignoring float noise below 1e-9."""
    diffs = {}
    for field in fields:
        a, b = normalize(field, local.get(field)), normalize(field, upstream.get(field))
        if isinstance(a, (int, float)) and isinstance(b, (int, float)):
            if abs(float(a) - float(b)) < 1e-9:
                continue
        elif a == b:
            continue
        diffs[field] = {"local": local.get(field), "upstream": upstream.get(field)}
    return diffs


# --------------------------------------------------------------------------
# Commands
# --------------------------------------------------------------------------

def cmd_show(args: argparse.Namespace) -> int:
    models = index_models(fetch_pricing(base_url()))
    rows = []
    for name, model in sorted(models.items()):
        if args.model and args.model not in name:
            continue
        row = {"model_name": name}
        row.update(price_snapshot(model))
        row["enable_groups"] = model.get("enable_groups")
        rows.append(row)
    json.dump({"count": len(rows), "models": rows}, sys.stdout, ensure_ascii=False, indent=1)
    sys.stdout.write("\n")
    return 0


def cmd_compare(args: argparse.Namespace) -> int:
    ours = index_models(fetch_pricing(base_url()))
    theirs = index_models(fetch_pricing(args.against))
    shared = sorted(set(ours) & set(theirs))

    changed, ours_only, theirs_only = [], [], []
    for name in shared:
        diffs = significant(price_snapshot(ours[name]), price_snapshot(theirs[name]))
        if diffs:
            changed.append({"model_name": name, "differences": diffs})
    for name in sorted(set(ours) - set(theirs)):
        ours_only.append(name)
    for name in sorted(set(theirs) - set(ours)):
        theirs_only.append(name)

    json.dump(
        {
            "ours": base_url(),
            "reference": args.against,
            "shared": len(shared),
            "changed": changed,
            "ours_only": ours_only,
            "reference_only": theirs_only,
        },
        sys.stdout,
        ensure_ascii=False,
        indent=1,
    )
    sys.stdout.write("\n")
    return 0


def cmd_apply(args: argparse.Namespace) -> int:
    with open(args.file, encoding="utf-8") as handle:
        changes = json.load(handle)
    if not isinstance(changes, list):
        raise SystemExit("change file must be a JSON array of {model_name, fields}")

    applied, failed = [], []
    for change in changes:
        name = change.get("model_name")
        fields = change.get("fields") or {}
        if not name or not fields:
            failed.append({"model_name": name, "reason": "missing model_name or fields"})
            continue
        try:
            for key, value in fields.items():
                write_option(key, value)
            applied.append({"model_name": name, "fields": list(fields)})
        except SystemExit as exc:
            failed.append({"model_name": name, "reason": str(exc)})

    json.dump({"applied": applied, "failed": failed}, sys.stdout, ensure_ascii=False, indent=1)
    sys.stdout.write("\n")
    return 1 if failed else 0


def write_option(key: str, value: object) -> None:
    """Write one option. Price tables live under the ratio/pricing option keys."""
    payload = request(
        f"{base_url()}/api/option/",
        method="PUT",
        body={"key": key, "value": value},
        auth=True,
    )
    if not payload.get("success", False):
        raise SystemExit(f"option {key} rejected: {payload.get('message')}")


def cmd_set(args: argparse.Namespace) -> int:
    """Read the current price table, change one model, write the whole table back.

    The gateway stores every model's ratio in a single JSON option, so a partial
    update is not possible: the table is read, one entry is replaced, and the
    table is written back.
    """
    key = args.table
    payload = request(f"{base_url()}/api/option/", auth=True)
    data = payload.get("data") or {}
    raw = data.get(key)
    if raw is None:
        raise SystemExit(f"option {key} not present in the option payload")
    table = json.loads(raw) if isinstance(raw, str) else raw
    if not isinstance(table, dict):
        raise SystemExit(f"option {key} is not a JSON object")

    before = table.get(args.model)
    if args.ratio is not None:
        table[args.model] = args.ratio
    elif args.price is not None:
        table[args.model] = args.price
    else:
        raise SystemExit("pass --ratio or --price")

    write_option(key, json.dumps(table, ensure_ascii=False))
    json.dump(
        {"model_name": args.model, "table": key, "before": before, "after": table[args.model]},
        sys.stdout,
        ensure_ascii=False,
        indent=1,
    )
    sys.stdout.write("\n")
    return 0


def cmd_catalog(args: argparse.Namespace) -> int:
    """Report how much of the live catalog an external metadata source covers."""
    ours = index_models(fetch_pricing(base_url()))
    document = request(args.source)
    entries: dict[str, dict] = {}
    for provider in document.values():
        for model in (provider.get("models") or {}).values():
            model_id = model.get("id") or ""
            if model_id:
                entries[model_id] = model
                entries.setdefault(model_id.rsplit("/", 1)[-1], model)

    covered, missing = [], []
    for name in sorted(ours):
        hit = entries.get(name) or entries.get(name.rsplit("/", 1)[-1])
        if hit:
            covered.append(
                {
                    "model_name": name,
                    "context_length": (hit.get("limit") or {}).get("context"),
                    "max_output_tokens": (hit.get("limit") or {}).get("output"),
                    "modalities": hit.get("modalities"),
                    "release_date": hit.get("release_date"),
                }
            )
        else:
            missing.append(name)

    json.dump(
        {
            "source": args.source,
            "our_models": len(ours),
            "covered": len(covered),
            "missing": len(missing),
            "coverage_pct": round(len(covered) * 100 / max(len(ours), 1), 1),
            "missing_models": missing,
        },
        sys.stdout,
        ensure_ascii=False,
        indent=1,
    )
    sys.stdout.write("\n")
    return 0


def main() -> int:
    parser = argparse.ArgumentParser(description="ReX-API model price control")
    sub = parser.add_subparsers(dest="command", required=True)

    show = sub.add_parser("show", help="print current prices")
    show.add_argument("--model", help="substring filter")
    show.set_defaults(func=cmd_show)

    compare = sub.add_parser("compare", help="diff our prices against a reference site")
    compare.add_argument("--against", default=DEFAULT_REFERENCE)
    compare.set_defaults(func=cmd_compare)

    apply_cmd = sub.add_parser("apply", help="apply a change set from a JSON file")
    apply_cmd.add_argument("--file", required=True)
    apply_cmd.set_defaults(func=cmd_apply)

    set_cmd = sub.add_parser("set", help="set one model's ratio or per-request price")
    set_cmd.add_argument("--model", required=True)
    set_cmd.add_argument("--ratio", type=float)
    set_cmd.add_argument("--price", type=float)
    set_cmd.add_argument(
        "--table",
        default="ModelRatio",
        help="option key holding the table (ModelRatio or ModelPrice)",
    )
    set_cmd.set_defaults(func=cmd_set)

    catalog = sub.add_parser("catalog", help="report external catalog coverage")
    catalog.add_argument("--source", default="https://models.dev/api.json")
    catalog.set_defaults(func=cmd_catalog)

    args = parser.parse_args()
    return args.func(args)


if __name__ == "__main__":
    sys.exit(main())
