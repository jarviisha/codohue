# Codohue Admin Design Contract

This contract defines the visual and interaction rules for `web/admin`. Use it as the source of truth when creating or refactoring admin pages and shared UI components.

## Product Tone

Codohue Admin is an internal operations interface for monitoring namespaces, events, batch runs, recommendations, and service health.

- Prefer quiet, dense, professional UI over marketing-style presentation.
- Optimize for scanning, comparison, and repeated operational use.
- Avoid decorative backgrounds, oversized hero content, nested cards, and purely ornamental visuals.
- Keep text direct and task-oriented. Do not add in-app copy explaining obvious UI behavior.

## Theme And Tokens

Use semantic tokens from `src/index.css` instead of raw color utilities whenever possible.

- Backgrounds: `bg-base`, `bg-subtle`, `bg-surface`, `bg-surface-raised`.
- Borders: `border-default`, `border-strong`.
- Text: `text-primary`, `text-secondary`, `text-muted`, `text-disabled`.
- Accent: `bg-accent`, `text-accent`, `bg-accent-subtle`.
- Status: `success`, `warning`, `danger` tokens for all operational states.

Light and dark themes must both remain usable. Light mode is the primary review target; dark mode should preserve contrast, hierarchy, and state meaning.

Do not introduce new one-off colors in page files. If a new semantic color is needed, add a token first.

## Layout

Admin pages share one app frame: fixed sidebar, content area, breadcrumbs, page header, then page content.

- Content max width: use the app shell width, currently `max-w-7xl`.
- Page padding: `px-8 py-8` on desktop.
- Page content spacing: prefer `gap-6` or `mb-6` between major sections.
- Use responsive grids:
  - Metric rows: `grid-cols-1 sm:grid-cols-2 xl:grid-cols-4`.
  - Two-column panels: `grid-cols-1 xl:grid-cols-2`.
  - Namespace cards or repeated items: responsive columns, never fixed desktop-only columns.
- Tables must be inside a horizontal overflow wrapper when columns may exceed viewport width.
- Avoid fixed widths in page content unless the component has a stable operational reason, such as compact numeric inputs.

Mobile is not the primary target, but pages must not visually break on small screens. Sidebar behavior may remain desktop-first until a dedicated navigation pass.

## Spacing And Shape

Use restrained spacing and small radius.

- Panel padding: `p-5`.
- Compact table cell padding: around `px-2 py-2.5`.
- Form field vertical spacing: `mb-3` or controlled by a form layout component.
- Default radius: `rounded` or `rounded-lg`.
- Maximum card/control radius: `8px`.
- Pills and status badges may use `rounded-full`.
- Shadows should be rare. Use borders and background hierarchy first.
- Do not nest cards inside cards. A panel can contain rows, tables, forms, or inline metric tiles, but not another decorative card shell.

## Typography

Use clear hierarchy without display-style typography inside operational pages.

- Page title: `text-2xl font-bold text-primary leading-tight`.
- Panel title: `text-sm font-semibold text-primary`.
- Section/meta label: `text-[11px] font-semibold uppercase tracking-[0.06em] text-muted`.
- Body text: `text-sm text-secondary`.
- Muted helper text: `text-sm text-muted` or `text-xs text-muted`.
- Numeric values: use `tabular-nums`.
- Code-like IDs: use the shared code badge or mono styling.

Do not use negative letter spacing in admin content. Use uppercase labels only for metadata, table headers, and compact section labels.

## Shared UI Components

Page files should compose shared primitives instead of repeating Tailwind class strings.

Preferred primitives:

- `PageHeader`: title and page-level actions.
- `Panel`: bordered surface with optional title, actions, footer, and tone.
- `Button`: all clickable command buttons.
- `Field` and shared input classes: all forms.
- `Table`, `Thead`, `Th`, `Tbody`, `Tr`, `Td`: all data tables.
- `EmptyState`: all empty data states.
- `ErrorBanner` or future `Notice`: all error and status notices.
- `CodeBadge`: all IDs and code-like values.

Primitives to add during refactor:

- `PageShell`: consistent page wrapper and vertical rhythm.
- `Badge`: shared status/source/TTL variants.
- `MetricTile`: shared metric/stat cards.
- `LoadingState`: consistent loading rows and page placeholders.
- `Toolbar`: filter and action rows.
- `FormGrid`, `TextInput`, `NumberInput`, `Select`: form consistency without duplicated class names.

Domain-specific components may live under page folders, but they should still use shared UI primitives for visual structure.

## Buttons And Actions

Use buttons only for commands. Links should be actual navigation links.

- Primary button: one main page or panel action.
- Secondary button: standard non-destructive action.
- Ghost button: low-emphasis inline action.
- Destructive actions must have a dedicated danger variant before use.
- Buttons must have stable dimensions and not shift layout when loading.
- Loading labels may change text, but should not materially resize surrounding controls.

Use icons only when they improve scan speed. Use the existing icon system consistently before adding another icon source.

## Forms

Forms should be compact, aligned, and predictable.

- Label placement should be consistent within a form.
- Use inline label/value rows for dense settings forms.
- Use top labels for filter bars and short input groups.
- Numeric inputs should use compact widths and `tabular-nums`.
- Selects and inputs should share the same height, radius, border, focus, and disabled behavior.
- Group long forms with section headers, not extra card shells.
- Validation and save errors appear near the form top via a shared notice component.

## Tables

Tables are the default presentation for logs, events, batch runs, and ranked recommendation data.

- Header labels use compact uppercase metadata styling.
- Rows use subtle borders and optional hover background.
- Right-align numeric columns when comparing magnitudes.
- Use mono or `tabular-nums` for timestamps, durations, scores, and counts.
- Keep table actions visually lightweight.
- Do not replace dense tabular operational data with card grids.

## Metrics And Status

Metrics and statuses must be consistent across pages.

- Use a shared `MetricTile` for dashboard counts and summary values.
- Use a shared `Badge` for statuses such as `ok`, `degraded`, `failed`, `running`, `manual`, `cron`, TTL, and recommendation source.
- Status colors:
  - Success: healthy, completed, cache present.
  - Warning: degraded, manual, partial or attention-needed state.
  - Danger: failed, missing, unreachable.
  - Accent: active, selected, running, primary context.
- Avoid mixing checkmark/cross glyph styles across pages. Pick one badge pattern and reuse it.

## State Patterns

Use the same ordering and placement on every page:

1. `PageHeader`
2. Page-level error or notice
3. Loading state if no data is available yet
4. Empty state if loaded data is empty
5. Main content
6. Pagination or footer actions

Do not use custom paragraph-only loading states in new code. Use the shared loading component once it exists.

## Accessibility

- Use semantic elements: `button`, `a`, `table`, `form`, `label`.
- Every input must have a visible or programmatic label.
- Focus states must remain visible in both light and dark themes.
- Color must not be the only status signal; pair status color with text or a dot.
- Interactive controls must have a minimum practical hit area, especially sidebar and toolbar controls.

## Refactor Checklist

Use this checklist for each migrated page:

- Page uses shared page frame and `PageHeader`.
- Major sections use `Panel`, `Toolbar`, `MetricTile`, or table primitives.
- No one-off raw colors in page-level JSX.
- No negative tracking.
- No nested decorative cards.
- Loading, empty, error, and success states use shared patterns.
- Grids are responsive.
- Tables handle overflow.
- Light and dark themes remain legible.
- `npm run lint` and `npm run build` pass from `web/admin`.
