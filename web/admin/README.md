# web/admin

Operator-facing SPA for the Codohue admin plane. React 19 + TypeScript + Vite + Astryx (Meta's design system) + Tailwind v4 + React Router v7 + TanStack Query v5. Compiled into `cmd/admin` at build time via `embed.FS`; the binary is what production runs.

This directory is its own npm workspace and is *not* a Go package. Tooling and conventions are described in the docs below; this README only exists to point you at them.

## Read first

| Doc | What it covers |
|---|---|
| [.claude/CLAUDE.md](.claude/CLAUDE.md) | Astryx cheat sheet: setup, workflow, and the rules for adding UI. |
| `npm run astryx docs <topic>` | Astryx reference — `color`, `spacing`, `layout`, `styling`, `theme`, `tokens`, `migration`. Run `npm run astryx component <Name>` for a component's props. |
| [BUILD_PLAN.md](BUILD_PLAN.md) | Phase outline, build order, route table, code organisation, risks. The product-level plan. |
| [PHASE_2_TODO.md](PHASE_2_TODO.md) | Granular checklist for Phase 2 (page implementations). |
| [PHASE_3_TODO.md](PHASE_3_TODO.md) | Granular checklist for Phase 3 (polish + release prep). |
| Project [AGENTS.md](../../AGENTS.md) | Shared repository conventions, commands, testing, and commit style. |
| Project [ARCHITECTURE.md](../../ARCHITECTURE.md) | Ports, storage, data flow, authentication, and REST API contracts. |

## Commands

All commands run from this directory unless noted. From the repo root, `make web-admin-*` targets wrap the equivalents (`make web-admin-deps`, `web-admin-lint`, `web-admin-test`, `web-admin-build`).

```
npm ci                  # install deps from package-lock (CI + first checkout)
npm run dev             # Vite dev server on http://localhost:5173
npm run lint            # eslint (zero-warning gate)
npm test                # contract and operator UX unit tests
npm run build           # tsc -b && vite build → dist/
```

`npm test` also checks operator navigation, action-weight validation and URL
pagination rules. `make web-admin-test-browser` builds the SPA and runs the
Playwright operator workflow suite against a temporary local preview server with
mocked APIs. Install its browser once with `npx playwright install chromium` from
this directory, or set `BROWSER_EXECUTABLE` to an existing Chrome executable.
`ADMIN_TEST_URL` optionally targets an already running local frontend instead.
The suite covers configuration drafts during background refresh, validation,
command-palette jumps, namespace switching, filter restoration, failed telemetry,
and narrow-screen table containment. CI runs it after building the SPA.

The sidebar shows only the active namespace’s destinations while inside a
namespace. “All namespaces” returns to global navigation; the top-bar namespace
switcher remains available.

The Events live tail retains 1000 events but renders only a window of 60.
Arrivals are batched on a 100 ms timer and flash expiry is one sweeping
interval, so a burst costs a handful of renders and timers rather than one of
each per event. Stepping back through retained history pauses the tail, because
the window is an offset into a buffer that live append keeps shifting.

Catalog and Subjects apply text searches explicitly and keep applied filters,
sorting and pagination in the URL. Configuration preserves drafts when a newer
server snapshot arrives. Each tab saves independently with generation and group
revision preconditions; conflicts preserve drafts for explicit reconciliation. See the [UX audit and implementation notes](../../specs/admin-console-ux-audit.md).

For dev work that also runs the embedded admin binary, use the repo-root targets:

```
make dev-admin          # this Vite dev server (proxies API to localhost:2002)
make dev-all            # air-reloaded cmd/api + cmd/admin + this dev server
make build-admin-embed  # canonical production sequence: npm ci → build → go build -tags=embedui
```

## Architecture in three sentences

1. **Routes** live in [src/routes.tsx](src/routes.tsx). The URL identifies the active namespace (`:ns`) and page; the sidebar and namespace switcher derive their context from it. Each page is a separate chunk loaded through React Router's own `lazy`, so the download is part of the navigation: `useNavigation()` reports it, the shell draws a progress bar and marks the outlet `aria-busy`, the sidebar selects the pending destination, and [RouterLink](src/components/RouterLink.tsx) warms the module on hover or focus.
2. **Services** under [src/services/](src/services/) own one domain each: types, request functions, and TanStack Query hooks all colocated in the same file. Every HTTP call goes through [services/http.ts](src/services/http.ts) — the `urls.test.mjs` smoke enforces no raw `fetch(`.
3. **UI primitives** come from [@astryxdesign/core](https://github.com/facebook/astryx); pages compose them and reach for Tailwind only for local layout. Tailwind utilities are bound to Astryx tokens through `@astryxdesign/core/tailwind-theme.css` (see [src/index.css](src/index.css)), so `text-secondary` / `bg-surface` follow the active theme — never hardcode a colour.

## Embedding into the Go binary

[embed.go](embed.go) / [embed_prod.go](embed_prod.go) split is by build tag. The default (no tag) ships an empty embed FS so Go-only development doesn't need a built SPA. The `-tags=embedui` build ships `dist/`. Production / Docker / CI use the `-tags=embedui` path; `make build-admin-embed` is the canonical sequence.

A `web-admin` CI job in [.github/workflows/ci.yml](../../.github/workflows/ci.yml) runs `npm ci → lint → test → build → browser tests → go build -tags=embedui` end-to-end on every PR; the final step greps the binary for `<!doctype html>` to prove the SPA bytes were embedded.

## Out of scope

No i18n, no RBAC, no theming beyond light/dark on Astryx's `neutral` theme, no density toggle, no icon system yet (every spot uses a text label until the icon set lands).

### Configuration and runtime

Namespace Settings has four independently saved tabs: Recommendations, Event
signals, Trending and Embeddings. Catalog strategy and overrides are inline in
Embeddings. Each tab keeps its draft while switching; saving only clears that
section. Review before/after values or reconcile stale revisions before saving.
The grouped API returns canonical values in its save response. Explicit system
default toggles reset nullable overrides; blank override inputs are invalid.
See [rollout and concurrency notes](../../deploy/namespace-settings.md).

System Runtime (`/system/runtime`) shows allowlisted effective startup settings
reported by each process, without credentials or connection strings. Reports
refresh every 30 seconds and expire after 120 seconds. Missing reports are not
proof of service failure. Edit deployment configuration and restart the affected
process to change these settings; the page is read-only.
