# Codohue Admin Design System

Design system for `web/admin`, the Codohue operations console. Companion to [BUILD_PLAN.md](BUILD_PLAN.md) (implementation roadmap).

## 1. Product Voice

Codohue Admin is an internal operations console for monitoring namespaces, events, batch runs, recommendations, catalog embedding, and service health.

The new design language is **terminal/console-influenced**:

- Quiet, dense, professional. Operator-first.
- Optimized for scan, comparison, and repeated operational use.
- Data is the hero. Chrome recedes.
- Numbers, IDs, timestamps, and codes use monospace. Prose uses sans.
- Borders and dividers do the structural work. Shadows are exceptional, not decorative.
- Status is expressed by **glyph + color + text**, never color alone.
- No marketing-style hero content, no nested decorative cards, no oversized surfaces.
- Readability beats density. If a compact choice makes operators slow down,
  squint, or re-read, increase contrast, size, or spacing before adding more UI.

If a design choice would not look at home next to `kubectl`, `htop`, `psql`, or Grafana — reconsider it.

## 2. Token System

Tokens live in [src/index.css](src/index.css). Page code consumes them via Tailwind v4 semantic utilities (`bg-base`, `text-primary`, …). **Never introduce raw hex in page-level JSX.** If a new semantic color is needed, add a token first.

### 2.1 Palette

Light theme uses Tailwind **Slate** neutrals with **Meta Blue** accent. Dark theme is aligned to **GitHub Primer** dark tokens — neutrals follow Primer's `bgColor.*` / `borderColor.*` / `fgColor.*` scale, accent follows Primer's `accent` family.

| Role | Light | Dark | Reference |
|------|-------|------|-----------|
| `bg-base` | `#F8FAFC` | `#0d1117` | app canvas |
| `bg-subtle` | `#F1F5F9` | `#151b23` | alternate bands, sidebar, low-emphasis regions |
| `bg-surface` | `#FFFFFF` | `#161b22` | panels, tables, inputs, modals |
| `bg-surface-raised` | `#E2E8F0` | `#21262d` | hover, selected-adjacent, raised controls |
| `border-default` | `#CBD5E1` | `#3d444d` | resting dividers and panel boundaries |
| `border-strong` | `#94A3B8` | `#6e7681` | hover, focus-adjacent, high-importance dividers |
| `text-primary` | `#0F172A` | `#f0f6fc` | dark = Primer fgColor-default |
| `text-secondary` | `#334155` | `#d1d7e0` | default body, form labels, table body |
| `text-muted` | `#475569` | `#aeb6c2` | metadata, hints, secondary timestamps |
| `text-disabled` | `#94A3B8` | `#6e7681` | disabled controls only |
| `accent` | `#0866FF` | `#4493f8` | **text on neutral bg** (links, accent labels) |
| `accent-emphasis` | `#0866FF` | `#1f6feb` | **solid bg with white text** (Button primary) |
| `accent-subtle` | `#DBEAFE` | `#1b2f4a` | selected / active row bg |
| `accent-text` | `#FFFFFF` | `#FFFFFF` | text on solid accent (`bg-accent-emphasis`) |
| `success` | `#10B981` | `#3fb950` | dark = Primer fgColor-success |
| `warning` | `#F59E0B` | `#d29922` | dark = Primer fgColor-attention |
| `danger`  | `#EF4444` | `#f85149` | dark = Primer fgColor-danger |

**Palette notes**

- **`accent` vs `accent-emphasis`.** Dark theme splits the accent into two values because GitHub does: `accent` `#4493f8` is for accent-colored **text** on a neutral background (6.11:1 vs `bg-base` — passes AA-normal); `accent-emphasis` `#1f6feb` is for solid accent **backgrounds** with white text (4.63:1 — passes AA-normal). Using `accent` itself as a button bg with white text would fail (3.10:1). Light theme uses the same value for both because Meta Blue on white passes either way (4.82:1). Always pair `bg-accent-emphasis` with `text-accent-text`; use `text-accent` only on neutral surfaces.
- **Layer pattern.** Both themes use visible surface separation: `bg-base` is the page canvas, `bg-surface` is the content surface, `bg-subtle` is the secondary region, and `bg-surface-raised` is reserved for hover/raised states. Panels and tables use both `bg-surface` and `border-default`; they do not rely on hairline borders alone. This is intentionally less flat than the original border-only pass because the app must stay readable during long operator sessions.
- **Text contrast tiers.** `text-primary` is for titles, IDs, numbers, and current values. `text-secondary` is the default body/readable UI text. `text-muted` is only for metadata, hints, empty-state detail, and secondary timestamps; it must not carry required form labels, table body values, or primary navigation labels.
- **`text-muted` still passes WCAG AA-normal, but it is no longer the default small-text color.** Small text (`text-xs`, 11px mono labels, timestamps) should prefer `text-secondary` unless the information is genuinely optional.
- **No status-bg tokens.** `Notice` uses a left-border pattern, not a tinted background — see §6 (Notice primitive).

Light is the primary review target. Dark must preserve contrast, hierarchy, and state meaning.

### 2.1.1 Readability Targets

Before shipping a page, verify both themes against these targets:

- Main body text and table body values use at least `text-sm` with readable line-height (`leading-5` or browser-normal). Avoid `text-xs` for values operators must compare.
- Required labels, active nav items, table body values, and command names use `text-secondary` or stronger.
- `text-muted` is allowed for helper text, section metadata, timestamps when adjacent to a primary value, and disabled/empty context only.
- Adjacent surfaces must differ by either background or a strong enough border. A panel on the app canvas cannot rely on `#E2E8F0`-style hairline contrast alone.
- Compact controls are scoped to dense toolbars and table actions. Forms and primary actions use the default control size.

### 2.2 Type tokens

Two families. No third.

- **Sans**: `IBM Plex Sans` — prose, labels, button text, navigation, page titles. Humanist neo-grotesque with measurable character; matches the operations engineering tone and reads well at small sizes.
- **Mono**: `JetBrains Mono` — IDs, timestamps, durations, counts, scores, JSON, code, namespace names, **table column headers**, **table numeric cells**, status brackets, PS1 prompt. Wide latin character coverage, distinct disambiguated glyphs (`0/O`, `1/l/I`), strong identity in dev/ops contexts.

Both fonts are open-source and Google Fonts hosted. Load via `<link>` in `index.html` and reference in `@theme`.

System fallback chain: `JetBrains Mono` → `ui-monospace` → `SFMono-Regular` → `Menlo` → `Courier New`; `IBM Plex Sans` → `ui-sans-serif` → `-apple-system` → `system-ui`.

### 2.3 Radius scale

Terminal feel = sharper corners.

| Token | Value | Where |
|-------|-------|-------|
| `rounded-none` | 0px | Tables, table cells, sidebar items, top-bar |
| `rounded-sm` | 2px | **Default for inputs, selects, buttons, panels** |
| `rounded` | 4px | Modals, dropdown menus, tooltips |
| `rounded-full` | full | Only the avatar/status dot — nothing else |

Default is `rounded-sm`. **No `rounded-lg` or `rounded-md` anywhere.**

### 2.4 Shadows

Shadows live in `@theme` as `--shadow-raised`, `--shadow-floating`, `--shadow-overlay`, `--shadow-focus`.

- `shadow-overlay` — modals, popovers, dropdowns. **Only** these.
- `shadow-focus` — focus ring on interactive controls.
- `shadow-raised` / `shadow-floating` — avoid in page content. Border + background hierarchy first.

### 2.5 Status indicator

Status uses **bracketed dmesg-style tokens**, not pill badges. Fixed 6-character width (`[XXXX]`), mono font, color token carries the semantic. Self-aligns in tables and column views, scans like `journalctl`/`dmesg` output.

| State | Token | Color token | Example use |
|-------|-------|-------------|-------------|
| ok / healthy / success | `[ OK ]` | `text-success` | health probe, completed batch run, cache present |
| running / active | `[ RUN]` | `text-accent` | in-progress batch run, live tail |
| idle / unknown | `[IDLE]` | `text-muted` | namespace with no events, no data yet |
| warn / degraded / manual | `[WARN]` | `text-warning` | degraded health, manual run trigger, partial result |
| fail / error | `[FAIL]` | `text-danger` | failed run, unreachable infra, dead-letter |
| pending / queued | `[PEND]` | `text-muted` | catalog item queued, scheduled run |

Rules:

- Token is always followed by a descriptive label outside the brackets: `[ OK ]  cron heartbeat 2m ago`. Never use the token alone except inside a dedicated single-column "status" cell.
- Brackets are part of the token. Do not render as background fill or pill. The whole token is `text-{token-color} font-mono`.
- One state per row. No combined `[OK ][WARN]`.
- Hover tooltip on the token gives the verbose state phrase ("degraded since 14:02 — qdrant heartbeat missed").

This is the single signature operational element. Every status surface in the app — sidebar health dot, health page, batch run list, catalog item state, namespace card, namespace picker — uses the same token.

## 3. Layout & Information Architecture

### 3.1 App shell

```
┌──────────────────┬──────────────────────────────────────────────┐
│ codohue          │ codohue@prod:~/events $   [Cmd+K] [theme] [user]│
├──────────────────┼──────────────────────────────────────────────┤
│ GLOBAL           │                                              │
│   Health   [ OK ]│  Page title                                  │
│   Namespaces     │                                              │
│                  │  Page content                                │
│ prod             │                                              │
│   Overview       │  (active row has bg-accent-subtle)           │
│   Config         │                                              │
│   Catalog        │                                              │
│   Events         │                                              │
│   Trending       │                                              │
│   Batch Runs     │                                              │
│   Debug          │                                              │
│   Demo Data      │                                              │
└──────────────────┴──────────────────────────────────────────────┘
```

- **Sidebar**: fixed, ~240px wide, always expanded on desktop. Two sections: **GLOBAL** and **{namespace name}** (when selected). Sidebar `Health` item carries an inline status token `[ OK ]` / `[WARN]` / `[FAIL]` — health is always-on awareness.
- **Top bar**: holds the **PS1 prompt** (§3.1.1), the command palette trigger (label `Cmd+K`), the theme toggle, and the user menu. Namespace picker is invoked through the PS1 prompt or `Cmd+K` — not a dropdown chip.
- **Section labels** in sidebar use mono uppercase, slightly larger tracking (`tracking-[0.12em]`).
- **Active nav item**: `bg-accent-subtle` background + `text-accent`. No bold weight change — the bg and color carry the active signal. A leading glyph/icon marker is deferred until the icon system is in place (see §15).
- **Inactive nav item**: same row height, no decoration — `text-secondary` until hover.
- Mobile is not the primary target. Pages must not visually break under 1024px, but the sidebar may stay desktop-first until a dedicated nav pass.

### 3.1.1 PS1 prompt

The top bar's primary element is a **shell-style location prompt** that replaces both the namespace picker chip and the breadcrumb:

```
codohue@prod:~/events $
codohue@prod:~/catalog/items $
codohue@prod:~/catalog/items/sku_42 $
codohue@(no-ns):~/health $
codohue@~:~/namespaces $        ← when no namespace is selected
```

Format: `codohue@{namespace}:~/{path-segments} $`, mono `JetBrains Mono`, no background fill, `text-primary` weight regular. The `@{namespace}` segment is **clickable** — opens an inline namespace picker (popover). The `~/path` segments are **clickable** — each segment navigates back to that level.

This is one of three memorability anchors (§16). It encodes the entire location state in one scannable line and lets the user click any token. Operators learn the format on day 1 and never lose context.

### 3.2 Routing map

Namespace-scoped routes share an `<NamespaceLayout>` that reads `:name` from the URL via `useParams<{ name: string }>()`. **There is no `NamespaceContext`** — the route is the only source of truth for the active namespace (see [BUILD_PLAN.md §4.3](BUILD_PLAN.md)).

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
| `/ns/:name/events` | Events table + inject event |
| `/ns/:name/trending` | Trending |
| `/ns/:name/batch-runs` | Batch runs |
| `/ns/:name/debug` | Recommend debug |
| `/ns/:name/demo-data` | Demo data seeder |

The namespace picker writes to `:name` in the URL (replace, not push). Deep links survive page reload.

### 3.3 Page frame

Every page wraps its content in `<PageShell>` and starts with `<PageHeader>`. Location is encoded by the PS1 prompt in the top bar (§3.1.1) — there is **no separate breadcrumb component** inside the page frame.

```
PageShell (vertical rhythm, px-6 py-6)
├── PageHeader        (title · meta · actions)
├── Page-level Notice (error or status, optional)
├── Loading / Empty state (when applicable)
└── Main content      (Panels, Tables, Toolbars)
```

Width: full app shell width after the sidebar. Constrain only pages with a clear form-reading reason (login card, narrow config form) using `max-w-140` token.

`PageHeader` itself stays light — title + optional meta + actions. No duplicated location chips.

### 3.4 Grids

- Metric rows: `grid-cols-1 sm:grid-cols-2 xl:grid-cols-4`
- Two-column panels: `grid-cols-1 xl:grid-cols-2`
- Cards / repeated items: responsive, never fixed desktop-only columns

## 4. Typography

| Role | Class | Notes |
|------|-------|-------|
| Page title | `text-xl font-semibold text-primary leading-tight` | sans |
| Panel title | `text-sm font-semibold text-primary` | sans |
| Section / meta label | `text-xs font-mono uppercase tracking-[0.04em] text-secondary` | mono uppercase; use `text-muted` only for low-importance labels |
| Body | `text-sm text-secondary leading-5` | sans |
| Muted body | `text-sm text-muted leading-5` | sans; avoid for required content |
| Numeric value | `font-mono tabular-nums text-primary` | always mono |
| ID / code inline | `font-mono text-xs bg-surface-raised px-1.5 py-0.5 rounded-sm` | use `CodeBadge` |
| Timestamp / duration | `font-mono tabular-nums` | always mono |
| Table column header | `font-mono text-xs uppercase text-secondary` | mono |

No negative letter spacing in admin content. Uppercase is reserved for metadata, section labels, and table headers.

Small text rule: 11px mono is allowed only for sidebar section labels, status-adjacent metadata, and extremely compact table headers. If the text is a value or an action, use `text-xs` minimum; if it is a paragraph, use `text-sm`.

## 5. Spacing & Shape

| Surface | Rule |
|---------|------|
| Panel padding | `p-5` default; `p-4` only for dense repeated panels |
| Compact table cell | `px-3 py-2.5` (sans label cells), `px-3 py-2.5 font-mono tabular-nums` (numeric) |
| Form field vertical spacing | `gap-2` inside field, `gap-4` between form rows |
| Default control height | `h-9` |
| Compact toolbar / filter control | `h-8` |
| Default radius | `rounded-sm` (see §2.3) |
| Panel border | `border border-default` |
| Panel divider (in-panel sections) | `border-t border-default` |

Vertical rhythm between major sections: prefer `gap-5` or `mb-5`; dense dashboards may use `gap-4` when the page remains easy to scan.

**Hard rules**:
- No `rounded-lg`, no `rounded-md`. Only `rounded-none`, `rounded-sm`, `rounded`, `rounded-full`.
- No nested decorative cards. A panel can contain rows, tables, forms, inline metric tiles. Not another panel shell.
- No shadow on resting page content. Shadow only on overlays (modal, popover, dropdown) and focus.

## 6. Shared UI Primitives

Page files compose primitives. Don't repeat Tailwind class strings.

**Layout & structure**

- `AppShell` — top-level layout primitive that composes `Sidebar` + `TopBar` + content slot
- `Sidebar`, `SidebarNavGroup`, `SidebarNavItem` — match §3.1 visual rules. `SidebarNavItem` accepts an optional inline `StatusToken` (used by Health item).
- `TopBar`, `Ps1Prompt` (location prompt — reads `:name` and current route from React Router, renders `codohue@{ns}:~/{path} $`, makes `@ns` and each path segment clickable), `ThemeToggle`, `UserMenu`
- `PageShell`, `PageHeader`

**Content**

- `Panel` (bordered surface, optional title / actions / footer). Use for **card semantics** — a distinct content block standing on the canvas.
- `Section` (borderless content group, optional mono-uppercase title / actions). Use for **grouping under a heading** without nesting a bordered card — pick this for form sections so a page doesn't become a stack of rectangles.
- `Toolbar` (filter and action rows)
- `Table`, `Thead`, `Th`, `Tbody`, `Tr`, `Td`
- `MetricTile`, `Badge`, `KeyValueList`, `CodeBadge` (mono inline IDs)
- `EmptyState`, `LoadingState`, `Notice`

**Forms**

- `Field`, `Input`, `Select`, `NumberInput`, `Form`, `FormGrid`

**Overlays & interaction**

- `Modal`, `ConfirmDialog`, `Dropdown`
- `Button` (primary / secondary / ghost / danger; `h-7` `h-8` `h-9`)
- `CommandPalette` — global modal triggered by `Cmd+K` / `Ctrl+K`. **Primary action interface** (§16). Index includes:
  - Namespace switching (`prod`, `staging`, `demo`)
  - Page navigation (jump to any route in §3.2)
  - Recent items (last viewed catalog item, last batch run)
  - Quick actions (`Run batch now`, `Inject test event`, `Redrive deadletter`, `Toggle theme`, `Logout`)

**Status & signals**

- `StatusToken` — renders the 6-char `[XXXX]` bracketed status per §2.5.
- `Kbd` — terminal-style key cap for shortcut hints (`Cmd+K`).

### 6.1 Notice rendering

`Notice` and any status-bearing surface use a **4px left border + status border color + no background fill**, not a tinted bg pill. The pattern is terminal/Unix-DNA (vim error highlights, `notify-send`, dmenu) and avoids the dark-mode problem where tinted backgrounds sit ~1:1 against `bg-base`.

```
|  [WARN]  qdrant heartbeat missed at 14:02:38 UTC
|  Embedder ran with degraded latency — investigating.

| = border-l-4 border-warning, transparent background
```

Tailwind composition:

```jsx
<aside className="border-l-4 border-warning bg-transparent pl-4 py-3">
  <StatusToken state="warn" />
  <p className="text-sm text-primary">…</p>
</aside>
```

Border-color utility maps to status:

- `border-l-4 border-success` for `[ OK ]` notices
- `border-l-4 border-warning` for `[WARN]`
- `border-l-4 border-danger`  for `[FAIL]`
- `border-l-4 border-accent`  for informational notices

Body text always uses `text-primary` so the message is fully legible; the status color appears in the border + the leading `StatusToken`. Notice may optionally include a dismiss action on the right (rendered as a text button — see §15 on icons).

Everything else — typography, spacing, radius, shadow, motion — composes Tailwind utilities backed by the tokens declared in `@theme` (§2). No component writes its own CSS rules; every visual is reachable through utility classes or a shared primitive.

Domain-specific components may live under page folders, but they must use shared primitives for visual structure.

## 7. Buttons & Actions

Use buttons only for commands. Links must be real navigation links.

| Variant | Use | Visual |
|---------|-----|--------|
| Primary | one main page or panel action | `bg-accent text-accent-text` |
| Secondary | standard non-destructive action | `bg-surface border border-default text-primary` |
| Ghost | low-emphasis inline action, table-row action | transparent, `text-secondary`, hover `bg-surface-raised` |
| Danger | destructive action (delete, drop) | `bg-danger text-white`, requires `ConfirmDialog` |

Rules:
- Default height `h-8`, compact `h-7`, prominent `h-9`.
- Stable dimensions on loading. Spinner replaces leading glyph; label may change but width must not jump.
- Icons only when they improve scan speed. Don't introduce a new icon source — use [components/icons.tsx](src/components/icons.tsx).

## 8. Forms

- Compact, aligned, predictable.
- Inline label/value rows for dense settings (use `KeyValueList`).
- Top labels for filter bars and short input groups.
- Numeric inputs are narrow + `tabular-nums` + mono.
- Inputs and selects share height, radius, border, focus, disabled.
- Long forms grouped by section header (mono uppercase per §4), not by extra card shells.
- Validation and save errors appear at the form top via `Notice`.

## 9. Tables

Tables are the default presentation for events, batch runs, recommendations, catalog items, trending.

- Column headers: mono uppercase per §4.
- Row borders subtle. Optional hover `bg-surface-raised`.
- Numeric columns right-aligned, mono, `tabular-nums`.
- Timestamps mono.
- Table actions visually lightweight (ghost buttons or icon-only ghost).
- Never replace dense tabular operational data with card grids.
- Horizontal overflow wrapper when columns may exceed viewport.

## 10. Metrics, Status, and Density

- Use `MetricTile` for dashboard counts and summary values. Value in mono, label in mono uppercase.
- Use `StatusToken` for any operational state (the bracketed dmesg token defined in §2.5).
- Use `Badge` for non-status tags (run trigger source `cron`/`admin`, TTL `5m`, source `cf`/`trending`).
- Never mix multiple status formats for the same state across pages.

## 11. Motion

Terminals snap, they don't slide. The system should feel **immediate**.

- Page transitions: none.
- Hover transitions: 100ms color/background only.
- Modal / Dropdown / CommandPalette enter: **80ms opacity, no translate**. Snap, not slide.
- Loading: shimmer skeleton via `LoadingState`. No full-page spinner where shimmer fits.
- Spinner allowed inline in buttons / row actions.
- **Pulse on `[ RUN]` status only** — 1s breathing opacity (1.0 → 0.55 → 1.0). Functional signal that the run is live, not decoration. No other state pulses. Implemented via CSS `@keyframes`, respects `prefers-reduced-motion`.
- No type-on, no number tick-up, no caret blink on titles. Decoration is rejected — this is an operations console.

## 12. Accessibility

- Semantic elements: `button`, `a`, `table`, `form`, `label`.
- Every input has a visible or programmatic label.
- Focus state must be visible in both themes — use `shadow-focus`.
- Color is never the only status signal. Pair with glyph + text.
- Interactive controls have a practical hit area. Sidebar items min `h-8`, toolbar `h-7`.
- Keyboard nav: tab order matches visual order. `Esc` closes modals/dropdowns. `Enter` submits forms.
- `Modal`, `Dropdown`, `ConfirmDialog` must trap focus and restore on close.

## 13. State Patterns

Same ordering and placement on every page:

1. `PageHeader` (location is in the top-bar PS1 prompt, not here)
2. Page-level `Notice` (error or status)
3. `LoadingState` if no data yet
4. `EmptyState` if loaded data is empty
5. Main content
6. Pagination or footer actions

No custom paragraph-only loading states in new code.

## 14. Per-page Checklist

Use this for every page built:

- [ ] Uses `<PageShell>` + `<PageHeader>`. No standalone breadcrumb component.
- [ ] Sections use `Panel`, `Toolbar`, `MetricTile`, or table primitives.
- [ ] No one-off raw colors in page JSX (only token utilities).
- [ ] No `rounded-lg` / `rounded-md` anywhere.
- [ ] No nested decorative cards.
- [ ] No `font-bold` on numeric values — use mono instead.
- [ ] Numeric values, IDs, timestamps, durations use mono per §4.
- [ ] All status surfaces use `StatusToken` (`[ OK ]` / `[ RUN]` / `[IDLE]` / `[WARN]` / `[FAIL]` / `[PEND]`).
- [ ] Loading, empty, error states use shared patterns.
- [ ] Grids are responsive.
- [ ] Tables have overflow wrapper.
- [ ] Both themes legible; focus visible in both.
- [ ] Routes follow §3.2; route is the source of truth for namespace (`useParams`), not context state.
- [ ] All quick actions reachable via `CommandPalette` (`Cmd+K`). Adding a new action means registering it in the palette index.
- [ ] `npm run lint`, `npm run build`, and `tests/urls.test.mjs` pass from `web/admin`.

## 15. Out of Scope

Decisions deliberately deferred:

- Mobile/responsive sidebar (drawer pattern).
- Internationalization. UI text stays English.
- RBAC / multi-user. Session model stays single global key per current backend.
- Theming beyond light/dark (no system-themed variants).
- Customizable density toggle (default density is the only density).
- Decorative atmosphere effects (grain overlays, scanlines, vignettes). Operational tools earn identity through ergonomics, not texture.
- **Icon system.** Every spot that would normally take an icon (sidebar active-row marker, button glyphs, theme/user/menu/close affordances, dropdown chevrons) uses a **plain text label** for now. No SVG drawing, no decorative Unicode glyphs. An `Icon` primitive lands in a separate change once the icon set is supplied; status tokens such as `[ OK ]` / `[FAIL]` stay text (brackets + letters are the designed status format, not icons).

## 16. Memorability Anchors

Three signature elements give the admin a recognizable identity. Every page reinforces all three; none is optional.

1. **PS1 prompt in top bar** (§3.1.1) — `codohue@prod:~/events $`. Location is shell context, not breadcrumb chips. Clickable segments.
2. **Bracketed dmesg status tokens** (§2.5) — `[ OK ]` `[ RUN]` `[IDLE]` `[WARN]` `[FAIL]` `[PEND]`. Same 6-char token everywhere, color carries semantic, table-aligns naturally.
3. **Command palette as primary action interface** (§6) — `Cmd+K` opens a fuzzy palette indexing every route, every quick action, every recently-viewed item. Mouse paths exist (sidebar, page buttons), but keyboard is the **intended** path. Every new action must register in the palette.

These earn the product's memorability the way operations tools should: by making the operator faster, not by visual surprise. A new hire learns all three on day one and stops needing the sidebar by week two.

Anti-anchors (deliberately not chosen so the three above stay clean):

- No type-on title, no caret blink — decoration, not function.
- No giant hero metric tile — operators want uniform scan.
- No grain / scanlines / vignettes — fights data display.
- No staggered page-load reveal — annoying on the 50th visit.
