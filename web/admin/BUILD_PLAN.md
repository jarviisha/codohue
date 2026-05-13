# Codohue Admin Build Plan

Implementation plan for `web/admin`, the Codohue operations console. Companion to [DESIGN.md](DESIGN.md) (visual + interaction rules).

- **Stack**: React 19 + TypeScript + Vite 8 + Tailwind v4 + React Router v7 + TanStack Query v5.
- **Distribution**: Compiled SPA is embedded into the `cmd/admin` Go binary at build time.

## 1. Phase summary

| Phase | Output | Target |
|-------|--------|--------|
| 0 — Design | [DESIGN.md](DESIGN.md), this plan, ASCII anchor mockups (§7) | 1–2 days |
| 1 — Foundation | App shell, all UI primitives, services, auth, kitchen-sink route | 2–3 days |
| 2 — Pages | All 13 pages on namespace-first IA | 5–8 days |
| 3 — Polish & QA | a11y pass, empty/error/loading, dual-theme sweep, release prep | 2–3 days |

Total: **~10–16 working days**, solo. Phase 2 parallelizable across two developers once Foundation merges.

## 2. Hard constraints

1. **`cmd/admin` embed**: the Go binary at [cmd/admin/main.go](../../cmd/admin/main.go) imports the SPA via `github.com/jarviisha/codohue/web/admin` and embeds `web/admin/dist/` when built with `-tags=embedui` (see [embed_prod.go](embed_prod.go)). Local dev builds use the default tag and leave the embed FS empty (the SPA is served by `make dev-admin` via the Vite dev server). Production / Docker builds must run `npm install && npm run build` to produce `dist/` before `go build -tags=embedui ./cmd/admin`. The Vite output path stays `web/admin/dist/` — this is the contract.
2. **`tests/urls.test.mjs`**: enumerates every route under `adminRoutes`. Updated alongside route changes in the same commit, never after.
3. **Backend API**: data-plane (`cmd/api`, port 2001) and admin-plane (`cmd/admin`, port 2002) endpoints are fixed. Frontend consumes them; it does not propose API changes from this plan.

## 3. Routes

Frontend routes, fully namespace-first.

**Global routes**

| Path | Page |
|------|------|
| `/login` | Login |
| `/` | Health (default landing) |
| `/namespaces` | All namespaces list |
| `/namespaces/new` | Namespace create form |

**Namespace-scoped routes** (under `<NamespaceLayout>`)

| Path | Page |
|------|------|
| `/ns/:name` | Overview |
| `/ns/:name/config` | Config (action weights, decay, dense hybrid, scoring, trending, gamma, seen-items) |
| `/ns/:name/catalog` | Catalog config + status + ops |
| `/ns/:name/catalog/items` | Catalog items browse |
| `/ns/:name/catalog/items/:id` | Catalog item detail (modal route) |
| `/ns/:name/events` | Events + inject |
| `/ns/:name/trending` | Trending |
| `/ns/:name/batch-runs` | Batch runs |
| `/ns/:name/debug` | Recommend debug |
| `/ns/:name/demo-data` | Demo data seeder |

Two structural choices:

- Path segment shortened to `/ns/:name/...` so PS1 prompt segments stay tight (`codohue@prod:~/events $`).
- Catalog has its own page distinct from `config` because it owns embedding strategy, backlog, and re-embed lifecycle — different concerns from action-weights config.

## 4. Code organization

### 4.1 Services layer

- `services/http.ts` — fetch transport + `ApiError`. Single entry point for HTTP.
- Per-domain files own request functions, types, **and** TanStack Query hooks:
  - `services/auth.ts`
  - `services/namespaces.ts`
  - `services/events.ts`
  - `services/catalog.ts`
  - `services/batchRuns.ts`
  - `services/trending.ts`
  - `services/recommend.ts`
  - `services/health.ts`
- `services/queryKeys.ts` re-exports keys from each domain file. Hierarchical convention: `[domain, ns?, ...params]`.

### 4.2 Hooks

No dedicated `hooks/` directory. TanStack Query hooks live next to their service file as named exports. Components import `useBatchRunsQuery` from `services/batchRuns.ts`. Co-locating query, mutation, and request function makes the data flow legible in one place.

### 4.3 Namespace state

Read from URL via `useParams<{ name: string }>()`. **No `NamespaceContext`.** The route is the only source of truth for the active namespace; `Ps1Prompt` and `Sidebar` both read from the same hook. This eliminates a class of bugs where context state diverges from URL.

### 4.4 Layout primitives

`AppShell` is the top-level layout primitive. Sub-components:

- `AppShell` (composes the others into the shell grid)
- `Sidebar`, `SidebarNavGroup`, `SidebarNavItem`
- `TopBar`, `Ps1Prompt`, `ThemeToggle`, `UserMenu`

The namespace picker is folded into `Ps1Prompt` (click the `@ns` segment → namespace popover). There is no standalone namespace picker chip.

### 4.5 Pages

Each page is single-purpose. Forms live in their own module file, separated from the page component:

```
pages/
├── login/LoginPage.tsx
├── health/HealthPage.tsx
├── namespaces/
│   ├── ListPage.tsx
│   └── CreatePage.tsx
└── ns/
    ├── NamespaceLayout.tsx
    ├── OverviewPage.tsx
    ├── ConfigPage.tsx
    ├── configForm.ts
    ├── catalog/
    │   ├── ConfigPage.tsx
    │   ├── StrategyPicker.tsx
    │   ├── DimMismatchNotice.tsx
    │   └── items/
    │       ├── ListPage.tsx
    │       └── DetailModal.tsx
    ├── events/
    │   ├── ListPage.tsx
    │   └── InjectEventModal.tsx
    ├── trending/Page.tsx
    ├── batch-runs/
    │   ├── ListPage.tsx
    │   └── RunDetailPanel.tsx
    ├── debug/Page.tsx
    └── demo-data/Page.tsx
```

### 4.6 UI primitives

All shared primitives live in `components/ui/` and are documented in [DESIGN.md §6](DESIGN.md). Page files compose primitives — never repeat Tailwind class strings.

### 4.7 Index / CSS

`src/index.css`:

- Loads `IBM Plex Sans` and `JetBrains Mono` from Google Fonts (latin subset only — bundle size discipline).
- Declares `@theme` tokens for fonts, shadows, and the `--spacing-90` / `--spacing-140` layout widths.
- Declares `:root` (light) and `.dark` color tokens.
- Maps tokens to Tailwind utilities via `@theme inline { --color-* }`.

### 4.8 Tests

- `tests/urls.test.mjs` enumerates every route under `adminRoutes` and asserts every API call goes through `services/http.ts`.
- Smoke test for `Ps1Prompt` formatting: `(namespace, pathSegments) → "codohue@{ns}:~/{path} $"`.
- Smoke test for `CommandPalette` index registration: every page module registers ≥1 command on load.

## 5. Build order

Each entry has a clear "done" definition. Build the foundation first, then iterate pages.

| # | Page | Route | Depends on | Done when |
|---|------|-------|------------|-----------|
| 0 | Login | `/login` | `services/http.ts`, `services/auth.ts`, AppShell-less layout | Session cookie set on success, redirects to `/` |
| 1 | Health | `/` | AppShell, MetricTile, StatusToken | All probes render with token; `[WARN]` reflects degraded; Sidebar Health item shows live token |
| 2 | Namespaces (list) | `/namespaces` | Table, Panel, StatusToken, `services/namespaces.ts` | Lists every namespace with last-run status token; Create button routes to `/namespaces/new` |
| 3 | Namespace Create | `/namespaces/new` | Form, FormGrid, namespaces service | Form validates locally + server-side; POST creates the namespace; redirect to `/ns/:name` on success |
| 4 | Namespace Overview | `/ns/:name` | health subset, trending top-5, last batch run, embedding panel | Matches §7.1 mockup; "Run batch now" via primary button + CommandPalette action |
| 5 | Namespace Config | `/ns/:name/config` | Form, FormGrid, NumberInput, Notice, namespaces service | All config fields editable; dirty-state guard; save returns to Overview |
| 6 | Events | `/ns/:name/events` | Table, Toolbar, Modal (inject), events service | Matches §7.3 mockup; inject event modal works; subject filter persists in URL query |
| 7 | Trending | `/ns/:name/trending` | Table, window selector, trending service | Window selector (1h / 6h / 24h / 7d) in URL query; Redis TTL surfaced in panel header |
| 8 | Recommend Debug | `/ns/:name/debug` | Form, results Table, recommend service | Subject ID + limit input → results with score breakdown; `debug=true` shows source attribution |
| 9 | Batch Runs | `/ns/:name/batch-runs` | Table, run detail panel, batchRuns service | List + filters (status, source); detail view with per-phase tokens `[ OK ]` / `[FAIL]` and timing |
| 10 | Catalog Config | `/ns/:name/catalog` | Form, status panel, ops panel, catalog service | Config editable; dim-mismatch 400 surfaces inline; "Re-embed all" button (with 409 handling) |
| 11 | Catalog Items | `/ns/:name/catalog/items` | Table, filter toolbar, catalog service | Matches §7.2 mockup; redrive single + bulk deadletter; URL query for state filter |
| 12 | Catalog Item Detail | `/ns/:name/catalog/items/:id` | Modal, catalog item service | Modal opens via deep link; content + metadata rendered; redrive / delete from modal |
| 13 | Demo Data | `/ns/:name/demo-data` | Form, demo service | Seed/clear actions; existing demo dataset state surfaced |

Entries 0–1 belong to Phase 1 Foundation (auth + shell validation). Entries 2–13 are Phase 2.

Order can parallelize across two developers once Foundation merges: one takes 2–6, the other 7–13.

## 6. Risks & mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| `tests/urls.test.mjs` drifts from `adminRoutes` | medium | medium | Update test in the same commit as the route change. Pre-commit hook to flag mismatches. |
| Form behavior gaps (dirty state, validation) on Config / Catalog Config | medium | medium | Smoke test critical forms (NamespaceConfig, CatalogConfig) before promoting entries 5 and 10 to "done". |
| PS1 prompt UX confuses non-engineer users | low | low | Operators are engineers; this is internal tooling. Verify with one screen-share session in Phase 3. |
| Command palette empty index on first build | low | low | Bake palette index into route registration. CI test asserts every page registers ≥1 command. |
| Bundle size regrows after font load | low | low | Subset JetBrains Mono and IBM Plex Sans to latin only via Google Fonts URL params. Verify bundle size in Phase 3 before release. |
| `cmd/admin` embed fails because `dist/` is missing | low | high | `npm run build` produces `dist/`. The `-tags=embedui` build step must run after `npm run build` in CI. Verify `make build-admin` in Phase 1. |

## 7. Anchor page mockups

Three reference pages. Phase 2 entries use these as the visual brief alongside DESIGN.md.

### 7.1 Namespace Overview (`/ns/prod`)

Top bar PS1 prompt: `codohue@prod:~ $`

```
┌─────────────────────────────────────────────────────────────────────┐
│ Overview                                       [Run batch now]      │  ← PageHeader
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│ ┌─ HEALTH ───────────────┐  ┌─ LAST BATCH RUN ──────────────────┐   │
│ │ [ OK ]  postgres       │  │ [ OK ]  cron #1847                │   │
│ │ [ OK ]  redis          │  │ started   14:02:38 UTC            │   │
│ │ [ OK ]  qdrant         │  │ duration  4.812s                  │   │
│ │ [WARN]  embedder       │  │ trigger   cron                    │   │
│ └────────────────────────┘  │ phase 1   [ OK ]  sparse  1.012s  │   │
│                             │ phase 2   [ OK ]  dense   2.487s  │   │
│ ┌─ VOLUME (24h) ─────────┐  │ phase 3   [ OK ]  trending  113ms │   │
│ │ events         12,418  │  └───────────────────────────────────┘   │
│ │ subjects        1,204  │                                          │
│ │ objects         4,891  │  ┌─ EMBEDDING ───────────────────────┐   │
│ │ dead-letter         0  │  │ strategy           item2vec       │   │
│ └────────────────────────┘  │ dim                     128       │   │
│                             │ catalog auto-embed   enabled      │   │
│                             │ catalog backlog            0      │   │
│                             └───────────────────────────────────┘   │
│                                                                     │
│ ┌─ TRENDING TOP 5 ──────────────────────────────────────────────┐   │
│ │ RANK   OBJECT ID            SCORE       LAST EVENT            │   │
│ │    1   sku_42               2451.8      2m ago                │   │
│ │    2   sku_19               1922.4      3m ago                │   │
│ │    3   sku_88               1604.3      1m ago                │   │
│ │    4   sku_03                987.0      8m ago                │   │
│ │    5   sku_17                812.5      11m ago               │   │
│ │                                          [view all trending →]│   │
│ └───────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

Notes:
- Section labels (`HEALTH`, `VOLUME (24h)`, etc.) are mono uppercase, `tracking-[0.12em]`, `text-muted`.
- All numeric values are mono `tabular-nums`.
- No "hero" metric. Four uniform panels in a 2×2 layout at xl breakpoint, single column on smaller widths.
- "Run batch now" button is also available via `⌘K → run batch`.

### 7.2 Catalog Items (`/ns/prod/catalog/items`)

Top bar PS1 prompt: `codohue@prod:~/catalog/items $`

```
┌─────────────────────────────────────────────────────────────────────┐
│ Catalog Items                          [Redrive deadletter (3)]     │  ← PageHeader
├─────────────────────────────────────────────────────────────────────┤
│ ┌─ TOOLBAR ─────────────────────────────────────────────────────┐   │
│ │ state: [ all     ▾]   object_id: [_________]   [Refresh]      │   │
│ │ Showing 1–50 of 4,891                                         │   │
│ └───────────────────────────────────────────────────────────────┘   │
│                                                                     │
│ ┌────────┬────────────────┬─────────────────┬───────────┬────────┐  │
│ │ STATE  │ OBJECT ID      │ UPDATED         │ ATTEMPTS  │        │  │
│ ├────────┼────────────────┼─────────────────┼───────────┼────────┤  │
│ │ [ OK ] │ sku_4291       │ 14:02 today     │       1   │  ⋯     │  │
│ │ [ OK ] │ sku_4290       │ 14:02 today     │       1   │  ⋯     │  │
│ │ [ RUN] │ sku_4289       │ 14:01 today     │       1   │  ⋯     │  │
│ │ [PEND] │ sku_4288       │ 14:01 today     │       0   │  ⋯     │  │
│ │ [FAIL] │ sku_4042       │ 13:55 today     │       5   │ redrive│  │
│ │ [WARN] │ sku_3987       │ 13:51 today     │       2   │  ⋯     │  │
│ │ [ OK ] │ sku_3986       │ 13:51 today     │       1   │  ⋯     │  │
│ │  …                                                              │  │
│ └────────┴────────────────┴─────────────────┴───────────┴────────┘  │
│                                                   [← prev | next →] │
└─────────────────────────────────────────────────────────────────────┘
```

Notes:
- `STATE` column is fixed-width (8ch). Status tokens self-align because they are 6 chars.
- `OBJECT ID` is mono. Click → opens `/ns/prod/catalog/items/sku_4291` as a modal route over this list.
- `⋯` is a ghost overflow menu (redrive, hard-delete). `[FAIL]` rows show `redrive` inline as a ghost button — failed rows are the most common action target.
- Filter state (`state=fail`, `object_id=sku_4042`) goes to URL query params for shareability.
- `[ RUN]` row pulses ([DESIGN.md §11](DESIGN.md)).

### 7.3 Events (`/ns/prod/events`)

Top bar PS1 prompt: `codohue@prod:~/events $`

```
┌─────────────────────────────────────────────────────────────────────┐
│ Events                                              [Inject event]  │  ← PageHeader
├─────────────────────────────────────────────────────────────────────┤
│ ┌─ TOOLBAR ─────────────────────────────────────────────────────┐   │
│ │ subject_id: [_________]   [Refresh]                  ▶ live  │   │
│ │ Showing recent 100                                            │   │
│ └───────────────────────────────────────────────────────────────┘   │
│                                                                     │
│ ┌──────────────────────┬──────────┬──────────────┬────────────┬────┐│
│ │ TIME                 │ ACTION   │ SUBJECT      │ OBJECT     │ Δ  ││
│ ├──────────────────────┼──────────┼──────────────┼────────────┼────┤│
│ │ 14:02:38.412 UTC     │ click    │ user_19283   │ sku_42     │ 1s ││
│ │ 14:02:37.220 UTC     │ like     │ user_19281   │ sku_19     │ 2s ││
│ │ 14:02:35.001 UTC     │ share    │ user_18992   │ sku_88     │ 5s ││
│ │ 14:02:34.118 UTC     │ click    │ user_19283   │ sku_03     │ 6s ││
│ │ 14:02:33.001 UTC     │ skip     │ user_18992   │ sku_17     │ 7s ││
│ │ 14:02:31.500 UTC     │ comment  │ user_19283   │ sku_19     │ 9s ││
│ │  …                                                                ││
│ └──────────────────────┴──────────┴──────────────┴────────────┴────┘│
│                                                                     │
│ [ RUN] streaming · 12 evts/s · last @ 14:02:38.412      [ pause ]  │
└─────────────────────────────────────────────────────────────────────┘
```

Notes:
- Live tail strip at the bottom — always-on telemetry. `[ RUN]` pulses while streaming; `[IDLE]` when paused.
- `TIME` column is mono with millisecond precision, no row hover bg (it would fight the scan during live tail).
- `Δ` column is computed client-side (age delta) — updates every 1s; mono.
- Inject event button is also reachable via `⌘K → inject event`.

## 8. Out of scope

Deferred to a later pass:

- Mobile/responsive sidebar (drawer pattern)
- Internationalization. UI text stays English.
- RBAC / multi-user. Session model is single global key.
- Theming beyond light/dark (no system-themed variants)
- Customizable density toggle (default density is the only density)
- Atmosphere overlays (grain, scanlines, vignettes) — see [DESIGN.md §15](DESIGN.md)

## 9. Definition of done — Phase 0

- [x] [DESIGN.md](DESIGN.md) written and reviewed
- [x] [BUILD_PLAN.md](BUILD_PLAN.md) written
- [x] Anchor mockups for Overview, Catalog Items, Events (§7)
