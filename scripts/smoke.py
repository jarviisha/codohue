#!/usr/bin/env python3
"""Operational smoke test for Codohue.

Hits the RUNNING api (2001) + admin (2002) services, seeds the bundled demo
dataset, triggers a batch run, then asserts that the whole pipeline produces
sane output end-to-end:

    ingest -> compute (sparse + dense) -> qdrant -> recommend -> trending

This is a black-box check against a live stack — distinct from `make test-e2e`,
which spins up its own throwaway binaries. Run it any time to answer the
question "is the deployed stack actually healthy?".

Usage:
    make smoke                       # infra + app must already be up
    python3 scripts/smoke.py

Config (env vars, with defaults):
    API_URL                http://localhost:2001
    ADMIN_URL              http://localhost:2002
    CODOHUE_ADMIN_API_KEY  read from env, else parsed from ./.env
    KEEP_DEMO              set to 1 to skip demo-data cleanup at the end

Requires: python3 (stdlib only — no jq, no curl, no pip installs).
"""

from __future__ import annotations

import http.cookiejar
import json
import os
import sys
import time
import urllib.error
import urllib.request

DEMO_NS = "demo"
DEMO_SUBJECT = "u_ava"


def _dotenv() -> dict:
    """Best-effort parse of ./.env into a dict (no external deps)."""
    out = {}
    if os.path.exists(".env"):
        with open(".env") as fh:
            for line in fh:
                line = line.strip()
                if not line or line.startswith("#") or "=" not in line:
                    continue
                k, v = line.split("=", 1)
                out[k.strip()] = v.strip().strip("'\"")
    return out


_ENV = _dotenv()


def _conf(name: str, default: str = "") -> str:
    return os.environ.get(name) or _ENV.get(name, default)


# API_URL / ADMIN_URL win if set; otherwise build from the (env/.env) ports so
# the test follows whatever ports the stack actually runs on.
API_URL = (os.environ.get("API_URL") or f"http://localhost:{_conf('CODOHUE_API_PORT', '2001')}").rstrip("/")
ADMIN_URL = (os.environ.get("ADMIN_URL") or f"http://localhost:{_conf('CODOHUE_ADMIN_PORT', '2002')}").rstrip("/")

# --- output helpers ---------------------------------------------------------

_tty = sys.stdout.isatty()
GREEN = "\033[32m" if _tty else ""
RED = "\033[31m" if _tty else ""
YELLOW = "\033[33m" if _tty else ""
DIM = "\033[2m" if _tty else ""
BOLD = "\033[1m" if _tty else ""
RESET = "\033[0m" if _tty else ""

_counts = {"pass": 0, "fail": 0, "warn": 0}


def ok(msg: str) -> None:
    _counts["pass"] += 1
    print(f"  {GREEN}PASS{RESET} {msg}")


def bad(msg: str) -> None:
    _counts["fail"] += 1
    print(f"  {RED}FAIL{RESET} {msg}")


def warn(msg: str) -> None:
    _counts["warn"] += 1
    print(f"  {YELLOW}WARN{RESET} {msg}")


def section(msg: str) -> None:
    print(f"\n{BOLD}>{RESET} {msg}")


def note(msg: str) -> None:
    print(f"    {DIM}{msg}{RESET}")


def die(msg: str) -> None:
    print(f"{RED}error:{RESET} {msg}", file=sys.stderr)
    sys.exit(2)


# --- http -------------------------------------------------------------------

_cookies = http.cookiejar.CookieJar()
_opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(_cookies))


def request(method, url, body=None, token=None, timeout=300):
    """Return (status_code, parsed_json_or_none, raw_text). Network errors -> (0, None, msg)."""
    headers = {"Accept": "application/json"}
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        resp = _opener.open(req, timeout=timeout)
        raw = resp.read().decode("utf-8", "replace")
        code = resp.getcode()
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        code = e.code
    except (urllib.error.URLError, OSError) as e:
        return 0, None, str(e)
    try:
        parsed = json.loads(raw) if raw.strip() else None
    except json.JSONDecodeError:
        parsed = None
    return code, parsed, raw


# --- config -----------------------------------------------------------------


def resolve_admin_key() -> str:
    key = _conf("CODOHUE_ADMIN_API_KEY")
    if not key:
        die("CODOHUE_ADMIN_API_KEY not set (export it or add to ./.env)")
    return key


# --- test steps -------------------------------------------------------------


def main() -> int:
    admin_key = resolve_admin_key()

    print(f"{BOLD}Codohue smoke test{RESET}")
    note(f"api={API_URL}  admin={ADMIN_URL}  namespace={DEMO_NS}")

    # 1. data-plane liveness + health
    section(f"1. Data-plane health ({API_URL})")
    code, _, _ = request("GET", f"{API_URL}/ping")
    ok("GET /ping -> 200") if code == 200 else bad(f"GET /ping -> {code} (is cmd/api running?)")

    code, body, _ = request("GET", f"{API_URL}/healthz")
    if code == 200 and isinstance(body, dict):
        for dep in ("postgres", "redis", "qdrant"):
            st = body.get(dep, "missing")
            ok(f"healthz.{dep} = ok") if st == "ok" else bad(f"healthz.{dep} = {st}")
    else:
        bad(f"GET /healthz -> {code}")

    # 2. admin session
    section(f"2. Admin session ({ADMIN_URL})")
    code, body, _ = request("POST", f"{ADMIN_URL}/api/v1/auth/sessions", {"api_key": admin_key})
    if code == 201:
        ok("POST /api/v1/auth/sessions -> 201")
    else:
        bad(f"POST /api/v1/auth/sessions -> {code} (check CODOHUE_ADMIN_API_KEY / is cmd/admin running?)")
        print(f"\n{RED}Cannot continue without an admin session.{RESET}")
        return 1

    code, _, _ = request("GET", f"{ADMIN_URL}/api/admin/v1/health")
    ok("GET /api/admin/v1/health -> 200") if code == 200 else bad(f"GET /api/admin/v1/health -> {code}")

    # 3. seed demo data
    section("3. Seed demo dataset")
    code, body, _ = request("POST", f"{ADMIN_URL}/api/admin/v1/demo-data")
    if code in (200, 202) and isinstance(body, dict):
        events = body.get("events_created", 0)
        items = body.get("catalog_items_created", 0)
        if events > 0:
            ok(f"demo-data seeded ({events} events, {items} catalog items)")
        else:
            warn(f"demo-data returned {code} but events_created={events} (already seeded?)")
    else:
        bad(f"POST /api/admin/v1/demo-data -> {code}")

    # 4. trigger batch run (compute phases)
    section("4. Batch recompute (sparse + dense + trending)")
    code, body, _ = request("POST", f"{ADMIN_URL}/api/admin/v1/namespaces/{DEMO_NS}/batch-runs")
    if code not in (200, 202):
        bad(f"POST batch-runs -> {code}")
    else:
        body = body or {}
        run_id = body.get("id", "?")
        status = body.get("status", "unknown")
        tries = 0
        while status == "running" and tries < 30:
            time.sleep(1)
            tries += 1
            _, lst, _ = request(
                "GET", f"{ADMIN_URL}/api/admin/v1/namespaces/{DEMO_NS}/batch-runs?limit=1"
            )
            items = (lst or {}).get("items") or [{}]
            status = items[0].get("status", "unknown")
        ok(f"batch run #{run_id} -> succeeded") if status == "succeeded" else bad(
            f"batch run #{run_id} -> {status}"
        )

    # 5. qdrant collections populated
    section("5. Qdrant vectors built")
    code, body, _ = request("GET", f"{ADMIN_URL}/api/admin/v1/namespaces/{DEMO_NS}/qdrant")
    if code == 200 and isinstance(body, dict):
        subj = body.get("subjects", {}).get("points_count", 0)
        obj = body.get("objects", {}).get("points_count", 0)
        objd = body.get("objects_dense", {}).get("points_count", 0)
        ok(f"sparse subjects = {subj} points") if subj > 0 else bad(
            f"sparse subjects = {subj} (expected > 0)"
        )
        ok(f"sparse objects = {obj} points") if obj > 0 else bad(f"sparse objects = {obj} (expected > 0)")
        note(f"dense objects = {objd} points")
    else:
        bad(f"GET .../qdrant -> {code}")

    # 6. recommendations (the actual product)
    section(f"6. Recommendations for {DEMO_SUBJECT}")
    code, body, _ = request(
        "GET",
        f"{API_URL}/v1/namespaces/{DEMO_NS}/subjects/{DEMO_SUBJECT}/recommendations?limit=10",
        token=admin_key,
    )
    if code == 200 and isinstance(body, dict):
        items = body.get("items") or []
        src = body.get("source", "?")
        if items:
            ok(f"{len(items)} recommendations returned (source={src})")
            top = [{"object_id": i.get("object_id"), "score": round(i.get("score", 0), 4)} for i in items[:3]]
            note(f"top: {json.dumps(top)}")
        else:
            bad(f"0 recommendations returned (source={src})")
    else:
        bad(f"GET recommendations -> {code}")

    # 7. rankings
    section(f"7. Rank candidates for {DEMO_SUBJECT}")
    candidates = ["item_wireless_mouse", "item_desk_lamp", "item_monitor_arm", "item_usb_c_hub"]
    code, body, _ = request(
        "POST",
        f"{API_URL}/v1/namespaces/{DEMO_NS}/rankings",
        {"subject_id": DEMO_SUBJECT, "candidates": candidates},
        token=admin_key,
    )
    if code == 200 and isinstance(body, dict):
        items = body.get("items") or []
        src = body.get("source", "?")
        ok(f"{len(items)} candidates ranked (source={src})") if items else bad("0 candidates ranked")
    else:
        bad(f"POST rankings -> {code}")

    # 8. trending
    section("8. Trending")
    code, body, _ = request(
        "GET",
        f"{API_URL}/v1/namespaces/{DEMO_NS}/trending?limit=10&window_hours=24",
        token=admin_key,
    )
    if code == 200 and isinstance(body, dict):
        items = body.get("items") or []
        if items:
            ok(f"{len(items)} trending items returned")
        else:
            warn("trending returned 0 items (redis ZSET empty — ok if trending phase was skipped)")
    else:
        bad(f"GET trending -> {code}")

    # summary
    print(f"\n{DIM}──────────────────────────────────────────{RESET}")
    parts = [f"{GREEN}{_counts['pass']} passed{RESET}"]
    if _counts["warn"]:
        parts.append(f"{YELLOW}{_counts['warn']} warnings{RESET}")
    if _counts["fail"]:
        parts.append(f"{RED}{_counts['fail']} failed{RESET}")
    print(f"{BOLD}Summary:{RESET} " + ", ".join(parts))

    if _counts["fail"]:
        print(f"{RED}Smoke test FAILED.{RESET}")
        return 1
    print(f"{GREEN}Stack is healthy end-to-end.{RESET}")
    return 0


if __name__ == "__main__":
    rc = 1
    try:
        rc = main()
    finally:
        if not os.environ.get("KEEP_DEMO"):
            request("DELETE", f"{ADMIN_URL}/api/admin/v1/demo-data")
    sys.exit(rc)
