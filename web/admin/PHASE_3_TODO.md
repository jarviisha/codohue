# Phase 3 — Task Breakdown

Granular working checklist for Phase 3 of the `web/admin` build — Polish & QA. Companion to [BUILD_PLAN.md](BUILD_PLAN.md) §1 and §6 (risks) and to [DESIGN.md](DESIGN.md) (token/state contracts).

## Process

- One commit per checked task.
- Each task lands as a self-contained change: `npm run build`, `npm test`, `npm run lint`, and `go build -tags=embedui ./cmd/admin` must all be green before commit.
- Tasks are executed sequentially. After each commit, we stop so the user can preview the change. The next task only starts after the user gives the go-ahead.
- Items inside a sub-phase can be reordered as findings dictate; sub-phases themselves are sequential (3.1 first, then 3.2, then 3.3).

## Conventions

- Commit message prefix follows the affected surface:
  - `fix(web/admin):` for a11y / behaviour fixes,
  - `refactor(web/admin):` for structural cleanup that does not change visible behaviour,
  - `chore(web/admin):` for build / CI / dependency / config work,
  - `docs(web/admin):` for documentation refresh.
- Subject still follows Conventional Commits (≤72 chars); body is 2–4 lines WHY, no per-file enumeration. See the user's commit-brevity rule.
- Every behavioural change must be exercised via a manual smoke (and a `tests/urls.test.mjs` extension where possible).
- No new design tokens, no new UI primitives. Phase 3 is *finishing*, not feature work — if a task needs a new primitive, surface it first instead of inventing one inline.
- The list in [BUILD_PLAN.md §8 Out of scope] stays out of scope: no mobile sidebar, no i18n, no RBAC, no extra themes, no density toggle, no atmosphere overlays.

---

## 3.1 — Foundation polish

Architectural fixes that influence every page. Land these first so 3.2 sweeps benefit from them.

- [x] **3.1.1 `NotFoundPage` + catch-all route**
  - New `pages/not-found/Page.tsx` rendering inside `AppShell`. Mirrors the PS1 prompt for the unknown path so the operator can see exactly what they typed.
  - Add `<Route path="*" element={<NotFoundPage />} />` in [routes/index.tsx](src/routes/index.tsx). Also add a nested `*` inside `ns/:name` — without it, unknown sub-paths under `/ns/:name` render an empty namespace shell (Outlet with no match), not the overview.
  - Update `tests/urls.test.mjs` ROUTE_PATHS with `path="*"`.
  - **Done when:** typing `/ns/prod/nonsense` renders the not-found page inside the shell, not a blank `<main>`.

- [x] **3.1.2 App-level `ErrorBoundary`**
  - New `components/layout/ErrorBoundary.tsx` (class component — React error boundaries cannot be functional). Renders a `Notice tone="fail"` with the error message and a Reload button.
  - Wrap `<Outlet />` inside `AppShell` so a crashed page does not take the sidebar down with it.
  - **Done when:** throwing an error inside a page renders the boundary fallback, sidebar + top bar still interactive.

- [x] **3.1.3 `document.title` per route**
  - New `components/layout/useDocumentTitle.ts` (colocated — [BUILD_PLAN.md §4.2](BUILD_PLAN.md) forbids a dedicated `hooks/` directory). Sets `${page} · ${ns?} · Codohue Admin`.
  - Wire it into `AppShell` reading the active route + `useParams<{ name }>()`, so each page does not have to opt in.
  - **Done when:** every route updates the browser tab title without per-page boilerplate.

- [x] **3.1.4 Modal focus polish**
  - [Modal.tsx](src/components/ui/Modal.tsx) already cycles Tab/Shift-Tab. What's missing: focus restoration to the trigger element on close, and reliably focusing the first interactive control on open (currently focuses the panel itself).
  - Remove the "focus is NOT yet trapped" comment in the file header once polished.
  - Verify with `Cancel` button in `InjectEventModal`, `Redrive` button in `CatalogItemDetailModal`, and `Seed/Clear` confirmations in `DemoDataPage`.
  - **Done when:** opening then closing a modal returns focus to the button that opened it; opening focuses the first input/button inside.

- [x] **3.1.5 Skip-to-content link in `AppShell`**
  - Hidden-until-focused link at the top of `AppShell` that jumps to the page content landmark. Add `id="main-content"` on the `<main>` wrapper.
  - **Done when:** Tab from a fresh page load surfaces the skip link first; Enter on it moves focus to the page content.

- [x] **3.1.6 Gate `_kitchen-sink` to dev builds**
  - The current static `import KitchenSinkPage from '@/pages/_kitchen-sink/Page'` keeps the module in the production bundle even if the `<Route>` is gated. Replace the static import with `React.lazy(() => import('@/pages/_kitchen-sink/Page'))`, then only render the route when `import.meta.env.DEV` is true — Vite folds `DEV` to a literal `false` at build time and Rollup tree-shakes the dynamic chunk out.
  - Wrap the `<Route element>` in `<Suspense fallback={<LoadingState />}>`.
  - Verify: `npm run build && grep -ri 'kitchen' dist/` produces no matches.
  - **Done when:** the production bundle has no kitchen-sink chunk and no reference to `KitchenSinkPage`.

- [x] **3.1.7 Strengthen `tests/urls.test.mjs`**
  - Replace the manual `ROUTE_PATHS` array with a parse of [routes/index.tsx](src/routes/index.tsx) so route drift fails the test automatically rather than silently aging.
  - Also derive `COMMAND_PAGE_MODULES` from the same parse — the current manual list is already missing `CatalogStatusPage`, `CatalogLayout`, `CfRunsPage`, and `ReEmbedsPage`. Mark layout-only files (no business surface of their own) as opt-out via a small allow-list constant.
  - Keep `EXPECTED_BUILDERS` as-is — it catches a different class of bug ([paths.ts](src/routes/path.ts) drift, not route drift).
  - **Done when:** adding or removing a route in `routes/index.tsx` without touching the test still leaves a green test if the change is consistent, and missing-command-registration is caught on every non-layout route.

---

## 3.2 — Sweep

Walk every page once, light + dark, and fix each finding in the smallest commit that makes sense. Land 3.1 first so sweep fixes do not regress on a missing 404 / error boundary.

- [x] **3.2.1 a11y audit — keyboard nav**
  - Tab through every page: Sidebar, TopBar, PageHeader actions, Toolbar, Table rows, Pagination, Form fields. Note anything that traps focus or is unreachable.
  - Verify `CommandPalette` keyboard flow: open with `Cmd+K`, arrow keys navigate, Enter runs, Esc closes.
  - **Done when:** the audit notes are landed as one fix-per-finding commit.

- [x] **3.2.2 a11y audit — ARIA + screen reader**
  - Run `axe-core` (manually or via a temporary devtool) on every page. Cross-check `Notice`, `StatusToken`, `Pagination`, `Tabs`, `Toolbar`, `Switch`.
  - Add `aria-live="polite"` to the events live-tail strip and to inline `Notice` regions.
  - **Done when:** axe reports zero serious/critical violations on every page.

- [x] **3.2.3 a11y audit — color contrast**
  - Verify WCAG AA contrast for body text, secondary text, muted text, and every `StatusToken` variant in both themes.
  - Fix any token that fails by adjusting `--color-*` tokens in [index.css](src/index.css), not by patching component-level classes.
  - **Done when:** Lighthouse a11y score ≥ 95 on every page in both themes.

- [x] **3.2.4 Empty / error / loading sweep**
  - For every list page (Namespaces, Events, Trending, BatchRuns, CatalogItems, etc.) verify the empty, error, and loading paths render correctly without flicker.
  - Standardise empty-state copy tone: "No X match" + actionable description; standardise error notice titles: "Failed to load X".
  - **Done when:** every list page shows the expected state under each condition and copy reads consistently.

- [x] **3.2.5 Stale-while-refetch indicator**
  - `isFetching && !isLoading` is already surfaced on Refresh buttons. Audit panel headers: when a query is silently refetching (interval poll, focus refetch), the user should see a subtle indicator (e.g. dim spinner on the panel header), not nothing.
  - **Done when:** background refetches are visible across every page; no page does a silent stale-then-jump update.

- [x] **3.2.6 Dual-theme walkthrough**
  - Screenshot every route in light then dark. Flag any non-token color, any low-contrast border, any unreadable status badge.
  - Fix by tightening tokens in [index.css](src/index.css); avoid component-level overrides.
  - **Done when:** the entire app is presentable in both themes with no token deviations.

- [x] **3.2.7 Form label + dirty-state audit**
  - Verify every form has labels properly associated via `htmlFor` (in particular the namespace-form tabs and the CatalogConfig form).
  - Verify the namespace `ConfigPage` and `CatalogConfigPage` dirty-state guard fires on browser back/close.
  - **Done when:** every form is keyboard-only navigable, labels are properly announced, and unsaved-change navigation prompts the operator.

---

## 3.3 — Release prep

Last-mile work before declaring the SPA shippable. Land 3.2 first — release prep is about *verifying* polish, not adding it.

- [x] **3.3.1 Bundle size verification**
  - Measured 2026-05-19: `dist/assets/index-*.js` = 372.3 kB raw / **111.0 kB gzip** (26% under the 150 kB gzip budget; ~14 kB raw smaller than the Phase 2 baseline thanks to kitchen-sink gating in 3.1.6).
  - No route-split applied — the budget has comfortable headroom. If a future surface (charts, editor, viz) pushes us close, the next candidates in order are `DebugPage` (operator-only), `CatalogItemDetailModal` (modal-route, on-demand), the batch-runs pages.

- [x] **3.3.2 Font payload verification**
  - Measured 2026-05-19. The CSS2 response (15 kB raw, 0.9 kB gzip) declares six `unicode-range` blocks per font/weight: latin, latin-ext, cyrillic, cyrillic-ext, greek, vietnamese. Per-subset woff2: latin **44.6 kB**, latin-ext **30.2 kB**, cyrillic 28.8, cyrillic-ext 23.0, greek 19.0, vietnamese 12.9.
  - Browser smart-loading via `unicode-range` only fetches woff2 files whose ranges contain rendered glyphs. An English-only operator pulls **only latin** (6 weights × ~45 kB ≈ 270 kB), or latin + latin-ext if the page renders Eastern-European accents (~450 kB worst case).
  - **Decision: keep Google Fonts, no narrowing.** `&text=` is unsafe because admin UI surfaces dynamic user content (namespace names, object_ids). Self-hosting a subset would save ~190 kB on first load but adds binary commits + a glyph-subsetting build step — disproportionate for an internal tool with `display=swap` (no FOUT) and CDN-cached forever after first visit. Revisit if the SPA is air-gapped or first-load latency becomes an operator complaint.

- [x] **3.3.3 CI verifies the embedded build sequence**
  - Added a `web-admin` job to [.github/workflows/ci.yml](../../.github/workflows/ci.yml) that runs the canonical embed sequence end-to-end: `npm ci` → `npm run lint` → `npm test` → `npm run build` → verify `dist/index.html` and `dist/assets/index-*.js` are present and non-empty → `go build -tags=embedui` → grep the binary for `<!doctype html>` to prove the SPA bytes are embedded. CI now fails at the precise step that breaks instead of in one opaque command.
  - Added a `build-admin-embed` Makefile target (plus split `web-admin-deps` / `web-admin-lint` / `web-admin-test` / `web-admin-build` helpers) so local dev hits the same sequence the Dockerfile and CI already use. `make build-admin` still produces the unembedded binary for fast Go-only iteration.

- [x] **3.3.4 Smoke test for `Ps1Prompt`**
  - Split the PS1 helpers (`parsePs1`, `segmentTo`) plus a new `formatPs1` formatter out of [routes/nav.ts](src/routes/nav.ts) into [routes/ps1.mjs](src/routes/ps1.mjs) with a [.d.mts](src/routes/ps1.d.mts) sibling. The .mjs file is importable by `node --test` without a TypeScript runtime; nav.ts re-exports so component imports stay on `@/routes/nav`.
  - Added four smokes to [tests/urls.test.mjs](tests/urls.test.mjs): parsePs1 segment splitting, formatPs1 against representative inputs (empty path, deep path, no namespace), a parse→format round-trip across the real route table, and segmentTo URL building.
  - CommandPalette registration coverage was already widened in 3.1.7 (parse-derived), so it is intentionally not re-tested here.

- [x] **3.3.5 Documentation refresh**
  - Updated [DESIGN.md](DESIGN.md) §2.1 palette table with the post-3.2 token values (light border/status colors darkened, dark border bumped, new `danger-emphasis`), added matching Palette notes, fixed §6 Panel description for the new `busy` prop, expanded §12 Accessibility to cover Modal focus restoration / skip link / CommandPalette keyboard flow, and added an Out-of-Scope entry recording that the in-app dirty-state nav guard is deferred until a React Router data-router migration.
  - Picked the per-phase TODO files as the authoritative DoD trackers — see [BUILD_PLAN.md §9](BUILD_PLAN.md). PHASE_2_TODO is fully ticked; PHASE_3_TODO §3.1 + §3.2 ticks landed alongside this audit (each task already shipped in its own commit but the boxes weren't crossed off until now).
  - Landed [README.md](README.md) as a short pointer to DESIGN/BUILD_PLAN/PHASE_*_TODO + canonical commands + the three-sentence architecture summary.

- [ ] **3.3.6 Operator UX verification** *(deferred — operator session pending)*
  - One screen-share session with an operator running through Overview → Config → Events (inject) → CatalogItems → BatchRuns. Capture friction.
  - This addresses the [BUILD_PLAN §6 risk] for PS1 prompt UX. The risk is rated *low* (operators are engineers familiar with shell prompts), so the session is scheduled out-of-band rather than gating Phase 3 close-out.
  - **Done when:** the session is logged and any showstoppers are tracked as fix tasks (this checklist may grow as a result).

---

## Phase 3 — Definition of done

- Every checkbox above ticked or explicitly deferred with a written reason.
- `npm run build`, `npm run lint`, `npm test`, and `go build -tags=embedui ./cmd/admin` all green at the end of every commit.
- Lighthouse accessibility score ≥ 95 in both themes for at least the Overview, Events, and CatalogItems routes.
- axe-core reports zero serious/critical violations on every page.
- Bundle size at or below 150 kB gzip.
- `tests/urls.test.mjs` no longer depends on a manually-maintained route list.
- [BUILD_PLAN.md](BUILD_PLAN.md) Phase 0 + 1 + 2 + 3 Definition-of-done sections all ticked.
