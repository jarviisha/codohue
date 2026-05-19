# web/admin

Operator-facing SPA for the Codohue admin plane. React 19 + TypeScript + Vite + Tailwind v4 + React Router v7 + TanStack Query v5. Compiled into `cmd/admin` at build time via `embed.FS`; the binary is what production runs.

This directory is its own npm workspace and is *not* a Go package. Tooling and conventions are described in the docs below; this README only exists to point you at them.

## Read first

| Doc | What it covers |
|---|---|
| [DESIGN.md](DESIGN.md) | Design tokens, palette, type, primitives, motion, accessibility — the visual + interaction contract every page must follow. |
| [BUILD_PLAN.md](BUILD_PLAN.md) | Phase outline, build order, route table, code organisation, risks. The product-level plan. |
| [PHASE_2_TODO.md](PHASE_2_TODO.md) | Granular checklist for Phase 2 (page implementations). |
| [PHASE_3_TODO.md](PHASE_3_TODO.md) | Granular checklist for Phase 3 (polish + release prep). |
| Project [CLAUDE.md](../../CLAUDE.md) | Repo-wide conventions, ports, env vars, commit-message style. Read for Go-side context. |

## Commands

All commands run from this directory unless noted. From the repo root, `make web-admin-*` targets wrap the equivalents (`make web-admin-deps`, `web-admin-lint`, `web-admin-test`, `web-admin-build`).

```
npm ci                  # install deps from package-lock (CI + first checkout)
npm run dev             # Vite dev server on http://localhost:5173
npm run lint            # eslint (zero-warning gate)
npm test                # node --test tests/urls.test.mjs
npm run build           # tsc -b && vite build → dist/
```

For dev work that also runs the embedded admin binary, use the repo-root targets:

```
make dev-admin          # this Vite dev server (proxies API to localhost:2002)
make dev-all            # air-reloaded cmd/api + cmd/admin + this dev server
make build-admin-embed  # canonical production sequence: npm ci → build → go build -tags=embedui
```

## Architecture in three sentences

1. **Routes** live in [src/routes/index.tsx](src/routes/index.tsx); the URL is the only source of truth for which namespace + page is active — child components read `:name` via `useParams`. The PS1 prompt and Sidebar both derive from the URL via [routes/ps1.mjs](src/routes/ps1.mjs).
2. **Services** under [src/services/](src/services/) own one domain each: types, request functions, and TanStack Query hooks all colocated in the same file. Every HTTP call goes through [services/http.ts](src/services/http.ts) — the `urls.test.mjs` smoke enforces no raw `fetch(`.
3. **UI primitives** under [src/components/ui/](src/components/ui/) are the only place Tailwind class strings live; pages compose primitives and never repeat utility class soup. See [DESIGN.md §6](DESIGN.md) for the catalogue.

## Embedding into the Go binary

[embed.go](embed.go) / [embed_prod.go](embed_prod.go) split is by build tag. The default (no tag) ships an empty embed FS so Go-only development doesn't need a built SPA. The `-tags=embedui` build ships `dist/`. Production / Docker / CI use the `-tags=embedui` path; `make build-admin-embed` is the canonical sequence.

A `web-admin` CI job in [.github/workflows/ci.yml](../../.github/workflows/ci.yml) runs `npm ci → lint → test → build → go build -tags=embedui` end-to-end on every PR; the final step greps the binary for `<!doctype html>` to prove the SPA bytes were embedded.

## Out of scope

Same list as [DESIGN.md §15](DESIGN.md) and [BUILD_PLAN.md §8](BUILD_PLAN.md): no mobile sidebar, no i18n, no RBAC, no theming beyond light/dark, no density toggle, no atmosphere overlays, no icon system yet (every spot uses a text label until the icon set lands).
