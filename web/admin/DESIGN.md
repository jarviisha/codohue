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

If a design choice would not look at home next to `kubectl`, `htop`, `psql`, or Grafana — reconsider it.

## 2. Token System

Tokens live in [src/index.css](src/index.css). Page code consumes them via Tailwind v4 semantic utilities (`bg-base`, `text-primary`, …). **Never introduce raw hex in page-level JSX.** If a new semantic color is needed, add a token first.

### 2.1 Palette

Accent is **Meta Blue** (with a dark-mode-tuned variant). Neutrals are Tailwind Slate in both themes.

| Role | Light | Dark | Reference |
|------|-------|------|-----------|
| `bg-base` | `#FFFFFF` | `#0F172A` | dark = slate-900 |
| `bg-subtle` | `#F8FAFC` | `#162033` | light = slate-50; dark = custom slate-850 |
| `bg-surface` | `#FFFFFF` | `#1E293B` | dark = slate-800 |
| `bg-surface-raised` | `#F1F5F9` | `#283447` | light = slate-100; dark = custom slate-750 |
| `border-default` | `#E2E8F0` | `#334155` | light = slate-200; dark = slate-700 |
| `border-strong` | `#CBD5E1` | `#475569` | light = slate-300; dark = slate-600 |
| `text-primary` | `#0F172A` | `#F1F5F9` | slate-900 / slate-100 |
| `text-secondary` | `#475569` | `#94A3B8` | slate-600 / slate-400 |
| `text-muted` | `#64748B` | `#7B8AA1` | slate-500 / custom slate-450 |
| `text-disabled` | `#CBD5E1` | `#475569` | slate-300 / slate-600 |
| `accent` | `#0866FF` | `#3B82F6` | Meta Blue / Tailwind blue-500 |
| `accent-subtle` | `#EBF3FF` | `#1E3A8A` | dark = Tailwind blue-900 |
| `accent-text` | `#FFFFFF` | `#FFFFFF` | text on solid accent |
| `success` | `#10B981` | `#34D399` | emerald-500 / emerald-400 |
| `warning` | `#F59E0B` | `#FBBF24` | amber-500 / amber-400 |
| `danger`  | `#EF4444` | `#F87171` | red-500 / red-400 |

**Palette notes**

- **Dark accent is brighter than light accent.** `#0866FF` on dark `bg-base` measures ~3.9:1 — fails WCAG AA for normal text. `#3B82F6` measures 4.85:1. The dual-token swap keeps the perceived brand color while giving accent-colored text enough contrast on dark mode.
- **Layer count differs by theme.** Dark uses four distinct background layers stepping ~1.5× in luminance for explicit depth perception. Light uses three (`bg-base = bg-surface = #FFFFFF`); the bordered-surface-on-white contrast against `text-primary` carries enough signal on its own. Same Tailwind utility names work in both themes; only the resolved CSS variable differs.
- **`text-muted` is tuned for WCAG AA-normal (4.5:1).** Light = slate-500 on white (4.76:1). Dark = custom #7B8AA1 on slate-900 (5.09:1). Helper-text-sized content must remain readable.
- **No status-bg tokens.** `Notice` uses a left-border pattern, not a tinted background. See §6 (Notice primitive).

Light is the primary review target. Dark must preserve contrast, hierarchy, and state meaning.

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
| Section / meta label | `text-[11px] font-mono uppercase tracking-[0.04em] text-muted` | mono uppercase |
| Body | `text-sm text-secondary` | sans |
| Muted body | `text-sm text-muted` or `text-xs text-muted` | sans |
| Numeric value | `font-mono tabular-nums text-primary` | always mono |
| ID / code inline | `font-mono text-xs bg-surface-raised px-1 rounded-sm` | use `CodeBadge` |
| Timestamp / duration | `font-mono tabular-nums` | always mono |
| Table column header | `font-mono text-[11px] uppercase text-muted` | mono |

No negative letter spacing in admin content. Uppercase is reserved for metadata, section labels, and table headers.

## 5. Spacing & Shape

| Surface | Rule |
|---------|------|
| Panel padding | `p-4` |
| Compact table cell | `px-2.5 py-2` (sans label cells), `px-2.5 py-2 font-mono tabular-nums` (numeric) |
| Form field vertical spacing | `mb-3` (or via `FormGrid`) |
| Default control height | `h-8` |
| Compact toolbar / filter control | `h-7` |
| Default radius | `rounded-sm` (see §2.3) |
| Panel border | `border border-default` |
| Panel divider (in-panel sections) | `border-t border-default` |

Vertical rhythm between major sections: prefer `gap-4` or `mb-4`.

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

- `Panel` (bordered surface, optional title / actions / footer / tone)
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
