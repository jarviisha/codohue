#!/usr/bin/env python3
"""Offline recommendation-quality evaluation for Codohue.

Answers the question the dashboard cannot: *are the recommendations actually
good?* — not just "does the pipeline run".

How it works (classic temporal leave-last-N-out protocol):

  1. Generate STRUCTURED synthetic behaviour — users have a "taste" (a primary
     item cluster) and interact mostly within it, plus a little noise. Structure
     is deliberate: with purely random data every metric collapses to chance and
     can't tell a good recommender from a broken one.
  2. Per user, sort interactions by time and hold out the last N distinct items
     as the TEST set; the rest is TRAIN.
  3. Wipe + recreate an eval namespace (pure-CF config: alpha=1, dense disabled),
     ingest ONLY the train events, trigger a batch run.
  4. Ask Codohue for recommendations per user and score them against the
     held-out test items: Precision@K, Recall@K, NDCG@K, MAP@K, HitRate@K,
     Coverage.
  5. Compute the same metrics for two BASELINES computed locally from the train
     data — global popularity and random. CF must beat them, otherwise either
     the recommender or the metric is meaningless.

REAL DATA: pass --from-namespace NS to evaluate genuine recorded behaviour
instead of synthetic data. Its events are read via the admin API, temporal-
split, and replayed into a SEPARATE eval namespace (the source is never
touched). Config (action_weights, alpha, dense_strategy, ...) is inherited from
the source namespace unless overridden by flags. This is the only number that
reflects real-world quality; the synthetic mode validates the mechanism.

This is a black-box harness over the live HTTP stack (like scripts/smoke.py):
no internal imports, python3 stdlib only — no jq, curl, numpy, or pip installs.

Usage:
    make eval                                   # synthetic, validates the engine + metric
    python3 scripts/eval.py --users 200 --clusters 10 --k 10 --seed 7
    python3 scripts/eval.py --from-namespace prod   # REAL quality on replayed prod events
    python3 scripts/eval.py --from-namespace prod --alpha 0.7 --dense-strategy item2vec
    KEEP_EVAL=1 python3 scripts/eval.py         # keep the eval namespace for inspection

Config (CLI flags override env vars override .env):
    API_URL / ADMIN_URL                base URLs (default: built from .env ports)
    CODOHUE_ADMIN_API_KEY              admin key (env or .env)
    KEEP_EVAL                          set to 1 to skip namespace teardown
"""

from __future__ import annotations

import argparse
import http.cookiejar
import json
import math
import os
import random
import sys
import time
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor
from datetime import datetime, timedelta, timezone
from typing import NoReturn

EVAL_NS = "eval"
ACTIONS = ["VIEW", "LIKE", "COMMENT", "SHARE"]  # positive-signal actions only
ACTION_WEIGHTS = {"VIEW": 1.0, "LIKE": 3.0, "COMMENT": 5.0, "SHARE": 8.0}
# Sampling weights for how often each action occurs (VIEW common, SHARE rare).
ACTION_FREQ = [0.62, 0.22, 0.10, 0.06]

# --- output helpers ---------------------------------------------------------

_tty = sys.stdout.isatty()
GREEN = "\033[32m" if _tty else ""
RED = "\033[31m" if _tty else ""
YELLOW = "\033[33m" if _tty else ""
CYAN = "\033[36m" if _tty else ""
DIM = "\033[2m" if _tty else ""
BOLD = "\033[1m" if _tty else ""
RESET = "\033[0m" if _tty else ""


def section(msg: str) -> None:
    print(f"\n{BOLD}>{RESET} {msg}")


def note(msg: str) -> None:
    print(f"    {DIM}{msg}{RESET}")


def die(msg: str) -> NoReturn:
    print(f"{RED}error:{RESET} {msg}", file=sys.stderr)
    sys.exit(2)


# --- config (.env) ----------------------------------------------------------


def _dotenv() -> dict:
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


# --- http -------------------------------------------------------------------

_cookies = http.cookiejar.CookieJar()
_admin_opener = urllib.request.build_opener(
    urllib.request.HTTPCookieProcessor(_cookies)
)


def _do(method, url, body=None, token=None, opener=None, timeout=600):
    """Return (status, parsed_json_or_none, raw). Network error -> (0, None, msg)."""
    headers = {"Accept": "application/json"}
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        # opener=None uses urlopen (no shared state) — safe for threaded calls.
        resp = (
            opener.open(req, timeout=timeout)
            if opener
            else urllib.request.urlopen(req, timeout=timeout)
        )
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


def admin(method, path, body=None):
    return _do(method, f"{ADMIN_URL}{path}", body=body, opener=_admin_opener)


def api(method, path, body=None, token=None):
    return _do(method, f"{API_URL}{path}", body=body, token=token)


# --- synthetic data ---------------------------------------------------------


def generate(rng, n_users, n_items, n_clusters, min_inter, max_inter, noise):
    """Build structured interaction logs.

    Each item belongs to a cluster. Each user has a primary cluster and draws
    most interactions from it; `noise` is the probability an interaction lands
    on a random out-of-cluster item. Returns:
      - logs: {subject_id: [(ts, object_id, action), ...]} time-sorted
      - item_cluster: {object_id: cluster}
    """
    items = [f"item_{i:04d}" for i in range(n_items)]
    item_cluster = {it: (idx % n_clusters) for idx, it in enumerate(items)}
    by_cluster = {}
    for it, c in item_cluster.items():
        by_cluster.setdefault(c, []).append(it)

    base = datetime.now(timezone.utc) - timedelta(days=25)
    logs = {}
    for u in range(n_users):
        sid = f"user_{u:04d}"
        primary = rng.randrange(n_clusters)
        k = rng.randint(min_inter, max_inter)
        # Pick k DISTINCT items for this user (mostly from the primary cluster).
        chosen = []
        seen = set()
        attempts = 0
        while len(chosen) < k and attempts < k * 20:
            attempts += 1
            if rng.random() < noise:
                it = rng.choice(items)
            else:
                it = rng.choice(by_cluster[primary])
            if it not in seen:
                seen.add(it)
                chosen.append(it)
        # Assign increasing timestamps so the temporal split is well-defined.
        events = []
        for j, it in enumerate(chosen):
            ts = base + timedelta(hours=j, minutes=rng.randint(0, 59))
            action = rng.choices(ACTIONS, weights=ACTION_FREQ, k=1)[0]
            events.append((ts, it, action))
        logs[sid] = events
    return logs, item_cluster


def split(logs, holdout):
    """Distinct-item temporal leave-last-N-out.

    Each event tuple is (ts, object_id, action), sorted ascending by ts. The
    `holdout` most-recently-engaged DISTINCT items become the test set; ALL
    events for those items are removed from train (so a held-out item can never
    leak into train and be filtered as 'seen'). Robust to repeated interactions
    in real data, and identical to last-N-out on the (distinct) synthetic logs.
    """
    train, test = {}, {}
    for sid, events in logs.items():
        last_seen = {}
        for ts, it, _ in events:  # ascending → last write is the last occurrence
            last_seen[it] = ts
        if len(last_seen) <= holdout:
            continue  # too few distinct items to both train and evaluate
        ordered = sorted(last_seen.items(), key=lambda x: x[1])
        test_items = {it for it, _ in ordered[-holdout:]}
        train_events = [e for e in events if e[1] not in test_items]
        if not train_events:
            continue
        train[sid] = train_events
        test[sid] = list(test_items)
    return train, test


# --- metrics ----------------------------------------------------------------


def dcg(hits):
    return sum((1.0 / math.log2(i + 2)) for i, h in enumerate(hits) if h)


def score_user(recs, test_items, k):
    """Return per-user (precision, recall, ndcg, ap, hit) for top-k recs."""
    rel = set(test_items)
    topk = recs[:k]
    hits = [1 if r in rel else 0 for r in topk]
    n_hits = sum(hits)
    precision = n_hits / k
    recall = n_hits / len(rel) if rel else 0.0
    idcg = dcg([1] * min(k, len(rel)))
    ndcg = (dcg(hits) / idcg) if idcg > 0 else 0.0
    # Average precision @ k.
    ap, running = 0.0, 0
    for i, h in enumerate(hits):
        if h:
            running += 1
            ap += running / (i + 1)
    ap = ap / min(k, len(rel)) if rel else 0.0
    return precision, recall, ndcg, ap, (1 if n_hits else 0)


def aggregate(per_user_recs, test, k, n_items):
    """Average metrics across users + catalog coverage."""
    rows = []
    covered = set()
    for sid, recs in per_user_recs.items():
        rows.append(score_user(recs, test[sid], k))
        covered.update(recs[:k])
    if not rows:
        return None
    n = len(rows)
    return {
        "precision": sum(r[0] for r in rows) / n,
        "recall": sum(r[1] for r in rows) / n,
        "ndcg": sum(r[2] for r in rows) / n,
        "map": sum(r[3] for r in rows) / n,
        "hitrate": sum(r[4] for r in rows) / n,
        "coverage": len(covered) / n_items,
        "users": n,
    }


# --- baselines (computed locally from train) --------------------------------


def popularity_recs(train, k):
    """Most globally-interacted train items, minus each user's seen items."""
    pop = {}
    for events in train.values():
        for _, it, _ in events:
            pop[it] = pop.get(it, 0) + 1
    ranked = [it for it, _ in sorted(pop.items(), key=lambda x: -x[1])]
    out = {}
    for sid, events in train.items():
        seen = {it for (_, it, _) in events}
        out[sid] = [it for it in ranked if it not in seen][:k]
    return out


def random_recs(rng, train, all_items, k):
    out = {}
    for sid, events in train.items():
        seen = {it for (_, it, _) in events}
        pool = [it for it in all_items if it not in seen]
        rng.shuffle(pool)
        out[sid] = pool[:k]
    return out


# --- pipeline steps ---------------------------------------------------------


def ingest_all(train, admin_key):
    """POST every train event over HTTP (threaded). Returns (ok, failed)."""
    jobs = []
    for sid, events in train.items():
        for ts, it, action in events:
            jobs.append(
                {
                    "subject_id": sid,
                    "object_id": it,
                    "action": action,
                    "occurred_at": ts.strftime("%Y-%m-%dT%H:%M:%SZ"),
                }
            )

    ok = 0
    failed = 0

    def post(ev):
        code, _, _ = api(
            "POST", f"/v1/namespaces/{EVAL_NS}/events", body=ev, token=admin_key
        )
        return code == 202

    with ThreadPoolExecutor(max_workers=16) as pool:
        for success in pool.map(post, jobs):
            if success:
                ok += 1
            else:
                failed += 1
    return ok, failed


def fetch_recs(subjects, admin_key, k):
    """GET recommendations per subject (threaded). Returns {sid: [object_id,...]}, source_counts."""
    out = {}
    sources = {}

    def get(sid):
        code, body, _ = api(
            "GET",
            f"/v1/namespaces/{EVAL_NS}/subjects/{sid}/recommendations?limit={k}",
            token=admin_key,
        )
        if code != 200 or not isinstance(body, dict):
            return sid, [], "error"
        items = [i.get("object_id") for i in (body.get("items") or [])]
        return sid, items, body.get("source", "?")

    with ThreadPoolExecutor(max_workers=16) as pool:
        for sid, items, src in pool.map(get, subjects):
            out[sid] = items
            sources[src] = sources.get(src, 0) + 1
    return out, sources


def fmt_row(label, m, color=""):
    return (
        f"  {color}{label:<14}{RESET} "
        f"P={m['precision']:.3f}  R={m['recall']:.3f}  "
        f"NDCG={m['ndcg']:.3f}  MAP={m['map']:.3f}  "
        f"Hit={m['hitrate']:.3f}  Cov={m['coverage']:.3f}"
    )


# --- real-data source -------------------------------------------------------


def parse_ts(s):
    """Parse an RFC3339 timestamp ('...Z' or '+00:00') to aware datetime, or None."""
    if not s:
        return None
    try:
        return datetime.fromisoformat(s.replace("Z", "+00:00"))
    except ValueError:
        return None


def read_namespace_events(src, max_events):
    """Page through the admin events endpoint of a REAL namespace.

    Returns (logs, total_in_namespace). logs = {sid: [(ts, oid, action), ...]}
    sorted ascending. Timestamps are re-based so the most recent event lands at
    'now', keeping interaction intervals intact — otherwise stale historical
    data would fall outside the engine's 90-day compute window after replay.
    """
    logs = {}
    offset, page, total, pulled = 0, 200, None, 0
    while True:
        code, body, _ = admin(
            "GET", f"/api/admin/v1/namespaces/{src}/events?limit={page}&offset={offset}"
        )
        if code != 200 or not isinstance(body, dict):
            die(f"read events from '{src}' -> {code} (does the namespace exist?)")
        items = body.get("items") or []
        if total is None:
            total = body.get("total", 0)
        if not items:
            break
        for ev in items:
            sid, oid = ev.get("subject_id"), ev.get("object_id")
            ts, action = parse_ts(ev.get("occurred_at")), ev.get("action")
            if sid and oid and ts is not None and action:
                logs.setdefault(sid, []).append((ts, oid, action))
        pulled += len(items)
        offset += len(items)
        if pulled >= total or pulled >= max_events:
            break

    for sid in logs:
        logs[sid].sort(key=lambda x: x[0])

    # Re-base timestamps: shift so the latest event across the dataset is ~now.
    all_ts = [e[0] for evs in logs.values() for e in evs]
    if all_ts:
        delta = datetime.now(timezone.utc) - max(all_ts)
        for sid in logs:
            logs[sid] = [(ts + delta, oid, action) for (ts, oid, action) in logs[sid]]
    return logs, total


# --- main -------------------------------------------------------------------


def main() -> int:
    global EVAL_NS
    p = argparse.ArgumentParser(description="Codohue offline recommendation eval")
    p.add_argument(
        "--from-namespace",
        default=None,
        metavar="NS",
        help="evaluate REAL events replayed from this namespace instead of synthetic data "
        "(its events are read, temporal-split, and replayed into a separate eval namespace)",
    )
    p.add_argument(
        "--max-events",
        type=int,
        default=100000,
        help="cap events pulled in --from-namespace mode",
    )
    p.add_argument(
        "--eval-namespace",
        default=None,
        help="replay/eval namespace name (must differ from source)",
    )
    # Synthetic-data knobs (ignored in --from-namespace mode).
    p.add_argument("--users", type=int, default=200)
    p.add_argument("--items", type=int, default=200)
    p.add_argument("--clusters", type=int, default=10)
    p.add_argument(
        "--min-inter", type=int, default=12, help="min interactions per user"
    )
    p.add_argument(
        "--max-inter", type=int, default=22, help="max interactions per user"
    )
    p.add_argument(
        "--noise",
        type=float,
        default=0.15,
        help="prob. an interaction is out-of-cluster",
    )
    # Shared knobs.
    p.add_argument(
        "--holdout",
        type=int,
        default=4,
        help="most-recent distinct items held out as test",
    )
    p.add_argument("--k", type=int, default=10, help="cutoff for @K metrics")
    p.add_argument("--seed", type=int, default=42)
    # Config — default None means "inherit" (synthetic: built-in defaults; real: source namespace config).
    p.add_argument(
        "--alpha",
        type=float,
        default=None,
        help="sparse weight; 1.0=pure CF, <1.0 enables dense hybrid",
    )
    p.add_argument(
        "--dense-strategy",
        default=None,
        choices=["disabled", "item2vec", "svd"],
        help="dense strategy trained during the batch run (needs alpha<1.0 to actually blend)",
    )
    p.add_argument(
        "--embedding-dim", type=int, default=None, help="dense vector dimension"
    )
    args = p.parse_args()

    admin_key = _conf("CODOHUE_ADMIN_API_KEY")
    if not admin_key:
        die("CODOHUE_ADMIN_API_KEY not set (export it or add to ./.env)")

    rng = random.Random(args.seed)
    real = args.from_namespace is not None
    EVAL_NS = args.eval_namespace or ("eval_replay" if real else "eval")
    if real and EVAL_NS == args.from_namespace:
        die("eval namespace must differ from the source namespace (it gets wiped)")

    print(f"{BOLD}Codohue offline eval{RESET}")
    note(f"api={API_URL}  admin={ADMIN_URL}  eval_ns={EVAL_NS}")

    # 0. admin session (needed to read source events/config)
    section("Login")
    code, _, _ = admin("POST", "/api/v1/auth/sessions", {"api_key": admin_key})
    if code != 201:
        die(f"admin login -> {code} (check key / is cmd/admin running on {ADMIN_URL}?)")
    note("session established")

    # 1. acquire data + resolve config
    if real:
        section(f"Read real events from namespace '{args.from_namespace}'")
        code, scfg, _ = admin("GET", f"/api/admin/v1/namespaces/{args.from_namespace}")
        if code != 200 or not isinstance(scfg, dict):
            die(f"source namespace '{args.from_namespace}' not found ({code})")
        logs, total = read_namespace_events(args.from_namespace, args.max_events)
        if not logs:
            die(f"namespace '{args.from_namespace}' has no events to evaluate")
        note(
            f"pulled {sum(len(v) for v in logs.values())}/{total} events, {len(logs)} subjects"
        )
        # Inherit source config; CLI flags override. BYOE can't be replayed (no
        # external vectors) so it degrades to pure sparse.
        action_weights = scfg.get("action_weights") or ACTION_WEIGHTS
        src_dense = scfg.get("dense_strategy") or "disabled"
        if src_dense not in ("item2vec", "svd"):
            src_dense = "disabled"
        alpha = args.alpha if args.alpha is not None else scfg.get("alpha", 1.0)
        dense_strategy = (
            args.dense_strategy if args.dense_strategy is not None else src_dense
        )
        embedding_dim = (
            args.embedding_dim
            if args.embedding_dim is not None
            else scfg.get("embedding_dim", 64)
        )
        lam = scfg.get("lambda", 0.05)
    else:
        section("Generate structured dataset")
        note(
            f"users={args.users} items={args.items} clusters={args.clusters} "
            f"noise={args.noise} seed={args.seed}"
        )
        logs, _ = generate(
            rng,
            args.users,
            args.items,
            args.clusters,
            args.min_inter,
            args.max_inter,
            args.noise,
        )
        action_weights = ACTION_WEIGHTS
        alpha = args.alpha if args.alpha is not None else 1.0
        dense_strategy = (
            args.dense_strategy if args.dense_strategy is not None else "disabled"
        )
        embedding_dim = args.embedding_dim if args.embedding_dim is not None else 64
        lam = 0.01

    train, test = split(logs, args.holdout)
    if not train:
        die("no eligible users — too few distinct items per user vs --holdout")
    all_items = sorted({it for evs in logs.values() for (_, it, _) in evs})
    n_items = len(all_items)

    # Ensure the eval config can weight every action present in the data.
    actions_present = {a for evs in logs.values() for (_, _, a) in evs}
    for a in actions_present:
        action_weights.setdefault(a, 1.0)

    dense_active = dense_strategy not in ("disabled", "byoe", "")
    hybrid = dense_active and alpha < 1.0
    if dense_active and alpha >= 1.0:
        print(
            f"{YELLOW}note:{RESET} dense_strategy={dense_strategy} but alpha>=1.0 — dense trained but unused (pure CF). Pass --alpha<1.0."
        )
    if not dense_active and alpha is not None and alpha < 1.0:
        print(
            f"{YELLOW}note:{RESET} alpha={alpha} but no dense strategy — nothing to blend (pure CF)."
        )

    mode = "hybrid (sparse+dense)" if hybrid else "pure sparse CF"
    n_events_train = sum(len(v) for v in train.values())
    note(f"mode={mode}  alpha={alpha}  dense_strategy={dense_strategy}")
    note(
        f"{len(train)} eligible users, {n_events_train} train events, {len(test)} test users, {n_items} items"
    )

    # 2. fresh eval namespace (wipe clears events + qdrant + rec cache)
    section(f"Provision eval namespace ({mode}: alpha={alpha}, dense={dense_strategy})")
    admin("DELETE", f"/api/admin/v1/namespaces/{EVAL_NS}")  # ignore 404
    cfg = {
        "action_weights": action_weights,
        "alpha": alpha,
        "dense_strategy": dense_strategy,
        "embedding_dim": embedding_dim,
        "seen_items_days": 60,
        "max_results": max(100, args.k),
        "lambda": lam,
    }
    code, _, raw = admin("PUT", f"/api/admin/v1/namespaces/{EVAL_NS}", cfg)
    if code not in (200, 201):
        die(f"namespace upsert -> {code}: {raw[:200]}")
    note("namespace ready")

    # 3. ingest train events
    section("Ingest train events")
    t0 = time.time()
    ok, failed = ingest_all(train, admin_key)
    note(f"ingested {ok} events ({failed} failed) in {time.time() - t0:.1f}s")
    if failed:
        print(f"  {YELLOW}WARN{RESET} {failed} events failed to ingest")

    # 4. batch run
    section("Batch recompute (sparse CF)")
    code, body, _ = admin("POST", f"/api/admin/v1/namespaces/{EVAL_NS}/batch-runs")
    body = body or {}
    status = body.get("status", "unknown")
    run_id = body.get("id", "?")
    tries = 0
    while status == "running" and tries < 60:
        time.sleep(1)
        tries += 1
        _, lst, _ = admin(
            "GET", f"/api/admin/v1/namespaces/{EVAL_NS}/batch-runs?limit=1"
        )
        status = ((lst or {}).get("items") or [{}])[0].get("status", "unknown")
    if status != "succeeded":
        die(f"batch run #{run_id} -> {status}")
    note(f"batch run #{run_id} succeeded")

    # 5. fetch recommendations
    section("Query recommendations")
    subjects = list(test.keys())
    cf_recs, sources = fetch_recs(subjects, admin_key, args.k)
    src_str = ", ".join(
        f"{s}={c}" for s, c in sorted(sources.items(), key=lambda x: -x[1])
    )
    note(f"sources: {src_str}")
    expected_source = "hybrid" if hybrid else "collaborative_filtering"
    cold = sum(c for s, c in sources.items() if s != expected_source)
    if cold:
        print(
            f"  {YELLOW}WARN{RESET} {cold} users not on the expected '{expected_source}' path "
            f"(cold-start/fallback) — raise --min-inter for cleaner signal"
        )

    # 6. score CF + baselines
    section("Results")
    cf = aggregate(cf_recs, test, args.k, n_items)
    pop = aggregate(popularity_recs(train, args.k), test, args.k, n_items)
    rnd = aggregate(random_recs(rng, train, all_items, args.k), test, args.k, n_items)
    if not (cf and pop and rnd):
        die("no scoreable users — check holdout / data")

    print(
        f"  {DIM}metric  P=Precision R=Recall NDCG MAP Hit=HitRate Cov=Coverage  (all @{args.k}){RESET}\n"
    )
    cf_label = "Codohue Hyb" if hybrid else "Codohue CF"
    print(fmt_row(cf_label, cf, GREEN))
    print(fmt_row("Popularity", pop, CYAN))
    print(fmt_row("Random", rnd, DIM))

    # 7. verdict
    print(f"\n{DIM}──────────────────────────────────────────{RESET}")
    lift = (cf["ndcg"] / pop["ndcg"]) if pop["ndcg"] > 0 else float("inf")
    if cf["ndcg"] <= rnd["ndcg"]:
        # Genuinely bad in any mode: the model adds nothing over chance.
        print(
            f"{RED}Codohue is no better than random — recommender or pipeline is broken.{RESET}"
        )
        rc = 1
    elif cf["ndcg"] > pop["ndcg"]:
        print(
            f"{GREEN}Codohue beats both baselines{RESET} "
            f"(NDCG lift over popularity: {lift:.2f}x)."
        )
        rc = 0
    elif real:
        # On real data, CF≈popularity is a legitimate finding, not a bug — it
        # means this dataset has weak collaborative signal (little co-engagement
        # structure beyond raw popularity).
        print(
            f"{YELLOW}Codohue beats random but not popularity.{RESET} "
            f"This real dataset has weak collaborative signal beyond popularity — "
            f"more/denser interactions or tuning may help."
        )
        rc = 0
    else:
        print(
            f"{YELLOW}CF does not clearly beat popularity.{RESET} "
            f"On strongly clustered synthetic data it should — inspect config / data structure."
        )
        rc = 1

    tip = (
        "vary --alpha / --dense-strategy on the SAME source to compare configs"
        if real
        else "tune action_weights / alpha / lambda and re-run to compare"
    )
    note(tip)
    return rc


if __name__ == "__main__":
    # Resolve URLs after .env parse (allow API_URL/ADMIN_URL override).
    API_URL = (
        os.environ.get("API_URL")
        or f"http://localhost:{_conf('CODOHUE_API_PORT', '2001')}"
    ).rstrip("/")
    ADMIN_URL = (
        os.environ.get("ADMIN_URL")
        or f"http://localhost:{_conf('CODOHUE_ADMIN_PORT', '2002')}"
    ).rstrip("/")
    rc = 1
    try:
        rc = main()
    finally:
        if not os.environ.get("KEEP_EVAL"):
            admin("DELETE", f"/api/admin/v1/namespaces/{EVAL_NS}")
    sys.exit(rc)
