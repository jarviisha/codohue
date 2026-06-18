# Davinci — Design Rationale

This document captures the **why** behind the system: the visual philosophy, the decision rules, and the patterns that hold it together. Read this before adding components, picking variants, or theming a consumer app.

> 🇻🇳 Một số quyết định gốc được thảo luận bằng tiếng Việt trong PR / commit history. Tài liệu này là bản tiếng Anh để dễ chia sẻ.

---

## Philosophy

Davinci is a **single-canvas, border-led** design system.

1. **One canvas, not stacked surfaces.** Sidebar, header, main, default cards all share `background`. The whole app reads as one color.
2. **Borders do the layering work.** Separation between regions, panels, and cards comes from borders — alpha-based in light, alpha-based in dark — never from background tiers.
3. **Fills are reserved for intent.** A surface gets a background fill only when its content is deliberately emphasized: a status message, a selected item, a sub-panel, an overlay. Decorative tinting is forbidden.
4. **Shadow = true elevation only.** Shadows are not visual decoration. They appear only on floating overlays (popover, dropdown, dialog, toast) to signal "this is above the canvas."

This bias toward flat + bordered comes from how Jira, Linear, and GitHub render dense product UI: low visual noise, predictable in dark mode, and the user's content gets the contrast budget — not the chrome.

---

## Token system

Three layers, referenced top-down:

| Layer | Examples | Use from app code? |
|---|---|---|
| **Primitive** | `color.blue.700`, `spacing.200`, `radius.md` | No — these are raw values, not rebrandable. |
| **Semantic** | `semantic.color.primary`, `semantic.color.foreground` | Yes — these are the stable API. |
| **Component** | `component.button.height.md`, `component.card.default.background` | Internal — only `react-ui` components reference these. |

**Rules:**

- App code uses **semantic** tokens. Never hardcode hex values, never reference primitive tokens directly.
- Custom themes override **semantic** tokens. They don't touch primitive values.
- Component tokens exist for component authors and themers who need to deviate per component.

---

## Surfaces — decision tree

Davinci ships four surface roles. In the default theme they collapse aggressively onto one canvas; in custom themes consumers can break the aliases to introduce tiers.

| Token | Light default | Dark default | When to use |
|---|---|---|---|
| `background` | `#FFFFFF` | `#18191A` | The canvas. Page wrappers, AppShell regions, default cards. **Default to this.** |
| `surface` | `#FFFFFF` (= background) | `#18191A` (= background) | Alias of `background`. Use when semantic intent is "a generic surface" rather than the page canvas. |
| `surface-raised` | `#FFFFFF` (= background) | `#303134` (lighter) | True float overlays — popover, dropdown, dialog, toast. **Always pair with `shadow.overlay` or `shadow.raised`.** |
| `surface-sunken` | `#F0F1F2` (darker) | `#111213` (darker) | Inset wells — code blocks, comparison panes, zebra rows. One step deeper than canvas in both modes. |

**Anti-pattern:** Using `surface-raised` on a card to make it "stand out". Use a card emphasis variant instead (see below). `surface-raised` is reserved for popovers/menus/dialogs.

---

## Card emphasis ladder

When picking a Card variant, **start at `default` and escalate only if intent demands it.**

| Variant | Background | Border | When to use |
|---|---|---|---|
| `default` | canvas (no fill) | `border` | The baseline. Most grouped content — sections, lists, settings groups. |
| `outlined` | transparent | `border` (subtle/default/bold weight) | Same as default but lets you tune border emphasis explicitly. |
| `filled` | `backgroundSubtle` | none | Mild emphasis. Sub-panels nested inside another container; neutral callouts. |
| `flat` | transparent | none | When the surrounding context already provides the frame (e.g., inside another bordered region). |
| `floating` | `surface-raised` + `shadow.raised` | `border` | True overlays — popover, menu, command palette. |
| `tone="info|success|warning|danger"` | 8% color-mix on canvas | tinted border | **Status IS the message.** Build failed, deployment succeeded, action requires attention. |
| `selected` | 8% primary mix on canvas | `border-focused` | Current selection in a list/grid/picker. Composes with any variant. |
| `interactive` | unchanged | bumps to `border-bold` on hover | Hover/focus signal. Composes with any variant. |

**Anti-pattern:** Using `tone="info"` to "make a card blue" because it looks nice. Tones encode meaning — if the user doesn't need to know it's informational, use `default`.

---

## Color tones — status semantics

Tone tokens (`success`, `warning`, `danger`, `info`, `discovery`) communicate **state of the underlying thing**, not visual flavor.

| Tone | Meaning |
|---|---|
| `success` | Operation completed; system is healthy. |
| `warning` | Caution needed; partial degradation; non-blocking issue. |
| `danger` | Destructive action; error state; blocking failure. |
| `info` | Neutral notification; passive information. |
| `discovery` | New / experimental feature surface. |

**Anti-pattern:** A "neutral" badge or alert tinted blue or purple "for branding". Use `neutral` variant or rely on layout/typography hierarchy instead.

---

## Borders — ladder

| Token | Use |
|---|---|
| `border-subtle` | Dividers between rows in dense tables; faint separators inside cards. Alpha-based — survives over tinted surfaces. |
| `border` | The default. Card outlines, input borders, panel edges. |
| `border-bold` | Emphasized borders — hovered interactive cards, prominent dividers. |
| `border-boldest` | Strong dividers between major sections; rarely used in product UI. |
| `border-hovered` | Input fields on hover. Solid color (not alpha) for crispness. |
| `border-focused` | The focus ring and the `selected` card border. Always primary blue. |

**Rule:** Border-subtle and border are alpha-based so they render predictably over any background. Bold/boldest/hovered are solid greys — only use them where the surface beneath is known to be canvas.

---

## Spacing — rhythm

The scale revolves around an **8px grid** for layout, with 2/4px fine-tuning for in-control nudges and three half-steps (6, 12, 20) for compact controls that fall between grid steps.

| Step | Pixels | Use |
|---|---|---|
| `025` | 2px | Icon-text nudges, tight inline gaps. |
| `050` | 4px | Icon-label gap, focus-ring offset. |
| `075` | 6px | Compact button inline gap. |
| `100` | 8px | Default form gap, dense list gap. |
| `150` | 12px | Form field internal gap, button gap. |
| `200` | 16px | Card padding, section gap. |
| `250` | 20px | Larger card padding. |
| `300` | 24px | Page gutter, large card padding. |
| `400` | 32px | Major section gap. |
| `500` | 40px | Page-section separation. |
| `600+` | 48, 64, 80px | Page-level padding, vertical rhythm. |

**Rule:** Pick the smallest step that satisfies the design. Never introduce arbitrary values outside this set.

---

## Canonical patterns

### App shell (Pulseboard-style dashboard)

```tsx
<AppShell>
  <AppShellTopBar>{/* logo, search, avatar */}</AppShellTopBar>
  <AppShellSidebar>{/* primary nav */}</AppShellSidebar>
  <AppShellHeader>{/* page title, page actions */}</AppShellHeader>
  <AppShellMain>{/* page content */}</AppShellMain>
  <AppShellAside>{/* metadata, recent activity */}</AppShellAside>
</AppShell>
```

All regions share `background`. Border lines between grid areas provide separation. Sidebar, main, and aside scroll independently; top-bar/header are pinned.

### Detail page (issue, settings page)

```tsx
<DetailLayout asideSticky>
  <DetailLayoutMain>{/* primary content */}</DetailLayoutMain>
  <DetailLayoutAside>{/* meta rail — labels, assignees, dates */}</DetailLayoutAside>
</DetailLayout>
```

Two columns above 1024px, single column below. `asideSticky` keeps the rail in view as main scrolls.

### Form

```tsx
<Stack gap="200">
  <FormField>
    <Label required>Project name</Label>
    <Input />
    <FormHelpText>Must be unique within the workspace.</FormHelpText>
  </FormField>
  <FormField>
    <Label>Owner</Label>
    <Combobox options={people} />
  </FormField>
  <Inline gap="100" justify="end">
    <Button tone="neutral" variant="ghost">Cancel</Button>
    <Button>Create</Button>
  </Inline>
</Stack>
```

Use `FormField` for label + input + help/error grouping. Inline action rows for buttons.

### Status callout (when status IS the message)

```tsx
<Alert variant="danger">
  <p><strong>Deploy failed.</strong> Migration 0042 timed out at 14:32.</p>
</Alert>
```

Or as a card when there's more structure:

```tsx
<Card tone="warning" variant="outlined">
  <CardHeader>
    <CardTitle>Approaching quota</CardTitle>
    <CardDescription>You've used 89% of your monthly API budget.</CardDescription>
  </CardHeader>
  <CardContent>{/* details */}</CardContent>
</Card>
```

---

## Anti-patterns — explicit list

| Don't | Do instead | Why |
|---|---|---|
| Hardcode hex (`#3B82F6`) | `semantic.color.primary` token | Breaks theme switching, breaks rebranding. |
| Use `tone="info"` on a card for visual variety | `variant="default"` or `variant="filled"` | Tones encode status meaning. Decorative use trains users to ignore them. |
| Add `box-shadow` to grouped content cards | Use border + (optional) `variant="filled"` | Shadow signals true z-axis lift. Page-level cards stay flat. |
| Set `background: white` on a panel | Token-driven `background` | Breaks dark mode. |
| Override `--davinci-semantic-color-surface` to bring back tiered chrome | Wrap your AppShell in a custom class, override only there | Global override breaks the single-canvas contract everywhere else. |
| Use `border-bold` for routine card outlines | `border` (the default) | `border-bold` is for hover/emphasis — overuse flattens its meaning. |
| Use arbitrary spacing (`gap-[18px]`) | Pick the closest spacing token | Arbitrary values fragment vertical rhythm. |
| Wrap every section in `Card variant="filled"` | `variant="default"` for the baseline; escalate per-section | Filled-everywhere produces a "Bootstrap card soup" look that contradicts the flat philosophy. |

---

## Adding a new component

1. **Check tokens first.** If your component needs a color/shadow/radius that doesn't exist as a semantic token, the right move is usually adding the token, not hardcoding.
2. **Component tokens for component-specific decisions.** If your component has its own size variants or paddings, define `component.<name>.*` tokens — don't reach into primitives.
3. **Reference semantic tokens for color/border/shadow.** Never primitives.
4. **Use the `.davinci-<component>` BEM-ish class convention** so consumers can override via the layer mechanism.
5. **Mirror the variant naming conventions:** `variant`, `tone`, `size` props with the value vocab already used by Card/Button/Badge.
6. **Add JSDoc to the variant/tone union types** explaining intent — that's what AI assistants and IDE tooltips read.

---

## Migration / theming hooks

Consumers who need to deviate from the flat single-canvas defaults override semantic tokens after importing Davinci's CSS:

```css
@import "@jarviisha/davinci-tokens/css/variables.css";
@import "@jarviisha/davinci-tokens/css/light.css";

:root {
  /* Re-introduce a tiered chrome layer */
  --davinci-semantic-color-surface: #F8F8F8;
}
```

Override scopes:
- **Global** (`:root`) — affects the whole app.
- **Subtree** (scoped class) — affects only the wrapped region.
- **Custom theme class** — pair with `<ThemeProvider>` to switch dynamically.

Never patch primitive (`--davinci-color-*`) variables — those are the palette source of truth.

---

## See also

- [USAGE.md](./USAGE.md) — installation, framework integration, troubleshooting.
- Per-package READMEs — quick API surface for each package.
