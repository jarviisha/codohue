# Personal Design System — Minimal SaaS

**Identity**: Clean, precise, professional. A SaaS tool that respects the user's focus.  
**Primary Font**: Inter  
**Primary Color**: Meta Blue  
**Modes**: Light + Dark (semantic tokens, no hardcoded values in components)

---

## 1. Visual Theme & Atmosphere

This system is built around clarity and trust — the two things users need from a SaaS dashboard. The light mode uses crisp white surfaces with cool blue-gray neutrals that keep the interface airy without feeling sterile. The dark mode shifts to a deep navy canvas (never pure black) that pairs naturally with Meta Blue, creating a premium, focused workspace aesthetic.

Inter is the single typeface throughout. It excels at small sizes (crucial for dashboards), has a full numeric tabular set for data alignment, and carries enough personality in its letterforms to feel modern without being decorative.

Meta Blue (`#0866FF`) is the sole accent — it handles all interactive states, CTAs, focus rings, and brand moments. A constrained palette means every blue element carries visual weight and intent.

**Key Characteristics:**

- White / cool-gray-blue surfaces in light mode — never warm beige or yellow
- Deep navy backgrounds in dark mode (`#0B1120`, `#101C33`) — never pure black
- Single accent: Meta Blue (`#0866FF`) with a clean tonal range
- Inter at every level — weight and size do the heavy lifting
- 4px base unit, 8-column grid
- Borders over shadows wherever possible — minimal depth system
- Rounded but restrained: 6px–12px radius range

---

## 2. Color Palette

### Semantic Token System

All components reference **semantic tokens**, not raw hex values. This makes light/dark switching trivial.

| Token                 | Light Value          | Dark Value        | Role                          |
| --------------------- | -------------------- | ----------------- | ----------------------------- |
| `--bg-base`           | `#FFFFFF`            | `#0B1120`         | Page background               |
| `--bg-subtle`         | `#F8FAFC`            | `#0F1A2E`         | Sidebar, secondary areas      |
| `--bg-surface`        | `#FFFFFF`            | `#101C33`         | Cards, panels, modals         |
| `--bg-surface-raised` | `#F1F5F9`            | `#152240`         | Hover states, nested surfaces |
| `--bg-overlay`        | `rgba(15,23,42,0.5)` | `rgba(0,0,0,0.6)` | Modals backdrop               |
| `--border-default`    | `#E2E8F0`            | `#1E3A5F`         | Cards, containers, dividers   |
| `--border-strong`     | `#CBD5E1`            | `#2A4F7A`         | Inputs focus-unfocused        |
| `--text-primary`      | `#0F172A`            | `#F1F5F9`         | Headlines, important content  |
| `--text-secondary`    | `#475569`            | `#94A3B8`         | Descriptions, subtitles       |
| `--text-muted`        | `#94A3B8`            | `#4A6380`         | Timestamps, captions          |
| `--text-disabled`     | `#CBD5E1`            | `#243A55`         | Disabled labels               |
| `--text-on-accent`    | `#FFFFFF`            | `#FFFFFF`         | Text on blue buttons          |

### Brand Blue (Meta-Inspired)

| Name         | Hex       | Use                                                    |
| ------------ | --------- | ------------------------------------------------------ |
| **Blue 50**  | `#EBF3FF` | Hover backgrounds, alert backgrounds                   |
| **Blue 100** | `#C3DAFE` | Focus rings (light mode)                               |
| **Blue 200** | `#93C5FD` | Disabled states, light badges                          |
| **Blue 400** | `#3B82F6` | Secondary interactive, lighter accent                  |
| **Blue 500** | `#0866FF` | **Primary — Meta Blue. Buttons, links, active states** |
| **Blue 600** | `#0052CC` | Hover on primary button                                |
| **Blue 700** | `#0040AA` | Active/pressed state                                   |
| **Blue 900** | `#001A4D` | Deep accent for dark surfaces                          |

### Neutral (Cool Blue-Gray)

| Name            | Hex       | Use                      |
| --------------- | --------- | ------------------------ |
| **Neutral 50**  | `#F8FAFC` | Page bg (light)          |
| **Neutral 100** | `#F1F5F9` | Sidebar bg (light)       |
| **Neutral 200** | `#E2E8F0` | Borders (light)          |
| **Neutral 300** | `#CBD5E1` | Strong borders (light)   |
| **Neutral 400** | `#94A3B8` | Muted text, placeholders |
| **Neutral 500** | `#64748B` | Secondary text           |
| **Neutral 600** | `#475569` | Body text (light)        |
| **Neutral 800** | `#1E293B` | Headlines (light)        |
| **Neutral 900** | `#0F172A` | Primary text (light)     |
| **Navy 900**    | `#0B1120` | Page bg (dark)           |
| **Navy 800**    | `#0F1A2E` | Sidebar bg (dark)        |
| **Navy 700**    | `#101C33` | Card surface (dark)      |
| **Navy 600**    | `#152240` | Raised surface (dark)    |
| **Navy 500**    | `#1E3A5F` | Borders (dark)           |
| **Navy 400**    | `#2A4F7A` | Strong borders (dark)    |

### Functional Colors

| Purpose        | Light     | Dark      | Token                |
| -------------- | --------- | --------- | -------------------- |
| **Success**    | `#10B981` | `#34D399` | `--color-success`    |
| **Success bg** | `#ECFDF5` | `#052E1F` | `--color-success-bg` |
| **Warning**    | `#F59E0B` | `#FBBF24` | `--color-warning`    |
| **Warning bg** | `#FFFBEB` | `#2D1B00` | `--color-warning-bg` |
| **Danger**     | `#EF4444` | `#F87171` | `--color-danger`     |
| **Danger bg**  | `#FEF2F2` | `#2A0A0A` | `--color-danger-bg`  |
| **Info**       | `#0866FF` | `#60A5FA` | `--color-info`       |
| **Info bg**    | `#EBF3FF` | `#001A4D` | `--color-info-bg`    |

---

## 3. Typography

**Single family: Inter** — loaded via `font-feature-settings: "cv11", "ss01"` for better legibility.  
Enable tabular numbers for all numeric data: `font-variant-numeric: tabular-nums`.

### Type Scale

| Role        | Size             | Weight | Line Height | Letter Spacing | Notes                                                                |
| ----------- | ---------------- | ------ | ----------- | -------------- | -------------------------------------------------------------------- |
| **Display** | 48px / 3rem      | 700    | 1.15        | −0.02em        | Landing hero, empty states                                           |
| **H1**      | 36px / 2.25rem   | 700    | 1.20        | −0.02em        | Page titles                                                          |
| **H2**      | 28px / 1.75rem   | 600    | 1.25        | −0.01em        | Section headings                                                     |
| **H3**      | 22px / 1.375rem  | 600    | 1.30        | −0.01em        | Card titles, drawer headers                                          |
| **H4**      | 18px / 1.125rem  | 600    | 1.35        | 0              | Sub-sections                                                         |
| **H5**      | 16px / 1rem      | 600    | 1.40        | 0              | Labels, column headers                                               |
| **Body L**  | 16px / 1rem      | 400    | 1.60        | 0              | Primary content text                                                 |
| **Body**    | 14px / 0.875rem  | 400    | 1.57        | 0              | Default body, table cells                                            |
| **Body S**  | 13px / 0.8125rem | 400    | 1.54        | 0              | Secondary content                                                    |
| **Caption** | 12px / 0.75rem   | 500    | 1.50        | 0.01em         | Timestamps, metadata                                                 |
| **Label**   | 11px / 0.6875rem | 600    | 1.45        | 0.06em         | `text-transform: uppercase`. Nav section headers, table column chips |
| **Code**    | 13px / 0.8125rem | 400    | 1.60        | 0              | `font-family: 'JetBrains Mono', monospace`                           |

### Principles

- **Weight does the work**: 400 for reading, 500 for emphasis, 600 for headings, 700 for display. No bold on body text.
- **Negative tracking at scale**: Large headings tighten letter-spacing (−0.02em) for a premium feel.
- **Uppercase only for system labels**: Navigation section titles, table column chips — never for body or button text.
- **Tabular numbers always**: Any column with numbers uses `font-variant-numeric: tabular-nums` so decimals align.

---

## 4. Spacing System

**Base unit: 4px.** All spacing values are multiples of 4.

| Token        | Value | Use                                    |
| ------------ | ----- | -------------------------------------- |
| `--space-1`  | 4px   | Icon gap, tight inline spacing         |
| `--space-2`  | 8px   | Compact padding, small gaps            |
| `--space-3`  | 12px  | Button padding-y, input padding        |
| `--space-4`  | 16px  | Card padding (compact), form field gap |
| `--space-5`  | 20px  | Button padding-x                       |
| `--space-6`  | 24px  | Card padding (default)                 |
| `--space-8`  | 32px  | Section inner spacing                  |
| `--space-10` | 40px  | Between card groups                    |
| `--space-12` | 48px  | Page section gaps                      |
| `--space-16` | 64px  | Major layout divisions                 |
| `--space-20` | 80px  | Hero section breathing room            |

---

## 5. Border Radius

| Token           | Value  | Use                          |
| --------------- | ------ | ---------------------------- |
| `--radius-sm`   | 4px    | Badges, chips, tags          |
| `--radius-md`   | 6px    | Buttons, inputs              |
| `--radius-lg`   | 8px    | Cards, dropdowns, tooltips   |
| `--radius-xl`   | 12px   | Modals, panels, larger cards |
| `--radius-2xl`  | 16px   | Floating elements, drawers   |
| `--radius-full` | 9999px | Avatar, pill badges          |

---

## 6. Shadow & Elevation

Minimal shadow usage — prefer borders. Shadows only for floating/overlay elements.

| Level          | Shadow                                                                | Use                                        |
| -------------- | --------------------------------------------------------------------- | ------------------------------------------ |
| **Flush**      | none                                                                  | Cards on same-level surface                |
| **Raised**     | `0 1px 3px rgba(0,0,0,0.08), 0 1px 2px rgba(0,0,0,0.06)`              | Cards with border removed                  |
| **Floating**   | `0 4px 6px -1px rgba(0,0,0,0.10), 0 2px 4px -2px rgba(0,0,0,0.08)`    | Dropdowns, popovers                        |
| **Overlay**    | `0 20px 25px -5px rgba(0,0,0,0.15), 0 8px 10px -6px rgba(0,0,0,0.10)` | Modals, drawers                            |
| **Focus Ring** | `0 0 0 3px rgba(8,102,255,0.25)`                                      | Keyboard focus on all interactive elements |

_Dark mode: multiply alpha by 2x on Floating and Overlay._

---

## 7. Components

### Button

**Primary**

- Background: `#0866FF`
- Text: `#FFFFFF`, 14px Inter weight 500
- Padding: `10px 20px`
- Radius: 6px
- Hover: background `#0052CC`
- Active: background `#0040AA`
- Disabled: background `#CBD5E1`, text `#94A3B8` (light) / background `#1E3A5F`, text `#4A6380` (dark)
- Focus: ring `0 0 0 3px rgba(8,102,255,0.25)`

**Secondary**

- Background: transparent
- Border: `1px solid var(--border-default)`
- Text: `var(--text-primary)`, 14px weight 500
- Hover: background `var(--bg-surface-raised)`, border `var(--border-strong)`

**Ghost**

- Background: transparent, no border
- Text: `var(--text-secondary)`
- Hover: background `var(--bg-surface-raised)`, text `var(--text-primary)`

**Danger**

- Background: `#EF4444`
- Text: `#FFFFFF`
- Hover: background `#DC2626`

**Size variants:**

- `sm`: padding `6px 14px`, font 13px, radius 5px
- `md` (default): padding `10px 20px`, font 14px, radius 6px
- `lg`: padding `12px 24px`, font 15px, radius 8px

### Input

**Text Input**

- Background: `var(--bg-surface)`
- Border: `1px solid var(--border-default)`
- Text: `var(--text-primary)`, 14px Inter weight 400
- Placeholder: `var(--text-muted)`
- Padding: `10px 12px`
- Radius: 6px
- Focus: border-color `#0866FF`, shadow `0 0 0 3px rgba(8,102,255,0.15)`
- Error: border-color `#EF4444`, shadow `0 0 0 3px rgba(239,68,68,0.15)`

**Select / Dropdown**

- Same as text input
- Arrow icon: `var(--text-muted)`

**Label**

- 13px Inter weight 500, `var(--text-primary)`
- Margin-bottom: 6px

**Helper text**

- 12px Inter weight 400, `var(--text-muted)`
- Margin-top: 4px

### Card

**Default**

- Background: `var(--bg-surface)`
- Border: `1px solid var(--border-default)`
- Radius: 8px
- Padding: 24px
- No shadow by default

**Clickable card**

- Same as default
- Hover: border-color `var(--border-strong)`, background `var(--bg-surface-raised)`
- Cursor: pointer
- Transition: 150ms ease

**Stat card (dashboard metric)**

- Padding: 24px
- Label: 11px uppercase weight 600, `var(--text-muted)`, letter-spacing 0.06em
- Value: 32px Inter weight 700, `var(--text-primary)`, tabular-nums
- Delta: 13px weight 500, green/red depending on direction

### Badge / Chip

- Padding: `3px 8px`
- Radius: 4px
- Font: 11px weight 600, uppercase, letter-spacing 0.04em

| Variant | Light bg  | Light text | Dark bg   | Dark text |
| ------- | --------- | ---------- | --------- | --------- |
| Default | `#F1F5F9` | `#475569`  | `#152240` | `#94A3B8` |
| Blue    | `#EBF3FF` | `#0866FF`  | `#001A4D` | `#60A5FA` |
| Green   | `#ECFDF5` | `#059669`  | `#052E1F` | `#34D399` |
| Yellow  | `#FFFBEB` | `#B45309`  | `#2D1B00` | `#FBBF24` |
| Red     | `#FEF2F2` | `#DC2626`  | `#2A0A0A` | `#F87171` |

### Navigation (Sidebar)

- Width: 240px (collapsed: 64px)
- Background: `var(--bg-subtle)`
- Border-right: `1px solid var(--border-default)`

**Nav item**

- Height: 36px
- Padding: `0 12px`
- Radius: 6px
- Font: 14px weight 500, `var(--text-secondary)`
- Icon: 16px, `var(--text-muted)`
- Hover: background `var(--bg-surface-raised)`, text `var(--text-primary)`, icon `var(--text-secondary)`
- Active: background `#EBF3FF` / dark `#001A4D`, text `#0866FF`, icon `#0866FF`

**Section header**

- 11px uppercase weight 600, `var(--text-muted)`, letter-spacing 0.06em
- Padding: `16px 12px 4px`

### Table

**Header row**

- Background: `var(--bg-subtle)`
- Border-bottom: `2px solid var(--border-default)`
- Font: 11px uppercase weight 600, `var(--text-muted)`, letter-spacing 0.06em
- Padding: `10px 16px`

**Data row**

- Border-bottom: `1px solid var(--border-default)`
- Font: 14px weight 400, `var(--text-primary)`, tabular-nums for numbers
- Padding: `12px 16px`
- Hover: background `var(--bg-surface-raised)`

**Compact row**

- Padding: `8px 16px`

### Avatar

- Shape: circle (`--radius-full`)
- Sizes: 24px (xs), 32px (sm), 40px (md), 48px (lg)
- Fallback: blue background `#EBF3FF`, initials in `#0866FF`, weight 600
- Border: none (use spacing to separate)

### Tooltip

- Background: `#0F172A` (light) / `#F1F5F9` (dark)
- Text: `#F8FAFC` (light) / `#0F172A` (dark), 12px weight 500
- Padding: `6px 10px`
- Radius: 6px
- Shadow: Floating level
- Max-width: 240px

---

## 8. Layout & Grid

### Page Structure (SaaS Dashboard)

```
┌─────────────────────────────────────────┐
│ TopBar (56px, optional)                 │
├──────────┬──────────────────────────────┤
│ Sidebar  │ Main Content                 │
│ (240px)  │ padding: 32px               │
│          │                              │
│          │ ┌──────────────────────────┐ │
│          │ │ Page Header (title+CTA)  │ │
│          │ ├──────────────────────────┤ │
│          │ │ Content Area             │ │
│          │ └──────────────────────────┘ │
└──────────┴──────────────────────────────┘
```

### Content Max Widths

| Context        | Max Width                                 |
| -------------- | ----------------------------------------- |
| Full dashboard | unlimited (fills sidebar-subtracted area) |
| Settings pages | 720px                                     |
| Form pages     | 560px                                     |
| Reading / docs | 680px                                     |
| Wide tables    | 1200px                                    |

### Breakpoints

| Name    | Width       | Behavior                                       |
| ------- | ----------- | ---------------------------------------------- |
| Mobile  | < 640px     | Sidebar collapses to bottom nav, single column |
| Tablet  | 640–1024px  | Sidebar icon-only (64px)                       |
| Desktop | 1024–1440px | Full sidebar (240px), standard layout          |
| Wide    | > 1440px    | Max content width, centered                    |

### Spacing Within Pages

- Page padding: `32px` (desktop), `20px` (tablet), `16px` (mobile)
- Between page sections: `32px`
- Between cards in a grid: `16px`
- Card internal padding: `24px` (default), `16px` (compact)

---

## 9. Icon System

- Library: **Lucide** (consistent with SaaS tools — clean, 24px grid, 1.5px stroke)
- Default size: 16px in nav, 18px in headers, 14px inline
- Stroke width: 1.5px (default)
- Color: inherits from text token unless intentional (success/danger icons use functional colors)
- Never fill icons — always stroke-only

---

## 10. Motion & Transitions

| Context                 | Duration | Easing                          |
| ----------------------- | -------- | ------------------------------- |
| Hover state (color, bg) | 150ms    | `ease`                          |
| Focus ring appear       | 100ms    | `ease-out`                      |
| Dropdown / popover open | 150ms    | `cubic-bezier(0.16, 1, 0.3, 1)` |
| Modal enter             | 200ms    | `cubic-bezier(0.16, 1, 0.3, 1)` |
| Modal exit              | 150ms    | `ease-in`                       |
| Sidebar collapse        | 250ms    | `cubic-bezier(0.4, 0, 0.2, 1)`  |
| Skeleton shimmer        | 1.5s     | `linear` (infinite)             |

**Principle**: Fast micro-interactions (under 150ms), slightly slower entrance animations. Never animate layout shifts.

---

## 11. Do's and Don'ts

### Do

- Reference semantic tokens (`--bg-surface`, `--text-primary`) — never hardcode hex in components
- Use borders to define boundaries — reserve shadows for floating elements
- Apply Inter's tabular numbers on all numeric data
- Keep uppercase labels to system chrome only (nav section headers, table column chips)
- Use `#0866FF` as the single CTA accent — don't introduce secondary accent colors
- Maintain 4px spacing discipline — every gap is a multiple of 4
- Use `−0.02em` letter-spacing on headings 28px and above
- Respect both modes — test every new component in light and dark before shipping

### Don't

- Don't use pure black (`#000000`) or pure white (`#FFFFFF`) as the only dark mode surface
- Don't use more than 2 weights in a single component
- Don't use uppercase on button labels — only system chrome labels get uppercase
- Don't mix functional colors for decoration (red is danger only, green is success only)
- Don't add box-shadow to cards that already have a visible border
- Don't go below 11px for any visible text — minimum font size is 11px (uppercase label)
- Don't use the blue accent for non-interactive elements — it signals "clickable"
- Don't use animations longer than 300ms — this is a tool, not a showcase

---

## 12. Agent Prompt Guide

### Quick Token Reference

**Light mode:**

- Page bg: `#F8FAFC`, Surface: `#FFFFFF`, Border: `#E2E8F0`
- Text: `#0F172A` (primary), `#475569` (secondary), `#94A3B8` (muted)

**Dark mode:**

- Page bg: `#0B1120`, Surface: `#101C33`, Border: `#1E3A5F`
- Text: `#F1F5F9` (primary), `#94A3B8` (secondary), `#4A6380` (muted)

**Always:**

- Accent: `#0866FF` (hover `#0052CC`, active `#0040AA`)
- Font: Inter, tabular-nums on all numbers
- Focus ring: `0 0 0 3px rgba(8,102,255,0.25)`

### Example Prompts

- "Dashboard page: sidebar 240px with `#F8FAFC` bg and `1px solid #E2E8F0` right border. Main content area `32px` padding. Page title `28px Inter 600 #0F172A`. Primary CTA button `#0866FF`, `10px 20px` padding, `6px` radius, white Inter 500 text."
- "Stat card grid: 3 columns, `16px` gap. Each card: white bg, `1px solid #E2E8F0` border, `8px` radius, `24px` padding. Label `11px uppercase 600 #94A3B8 0.06em tracking`. Value `32px 700 #0F172A tabular-nums`."
- "Data table: header row `#F8FAFC` bg, `2px solid #E2E8F0` bottom border, `11px uppercase 600 #94A3B8`. Body rows `1px solid #E2E8F0` divider, `14px 400 #0F172A`, hover `#F8FAFC`."
- "Badge component: `3px 8px` padding, `4px` radius, `11px uppercase 600`. Blue variant: bg `#EBF3FF` text `#0866FF`."
- "Dark mode card: bg `#101C33`, border `1px solid #1E3A5F`, radius `8px`, `24px` padding. Title `18px 600 #F1F5F9`. Body `14px 400 #94A3B8`."
- "Nav item active state: bg `#EBF3FF`, text `#0866FF`, icon `#0866FF`, `6px` radius, `36px` height, `0 12px` padding."

---

## 13. Tailwind CSS Token System

### Rule: Zero Hardcoded Values in Class Names

**Every color, every background, every border used in Tailwind classes MUST come from a semantic token — never a raw hex, never an arbitrary value.**

```html
<!-- ✅ CORRECT -->
<div class="bg-surface text-primary border border-default rounded-lg p-6">
  <!-- ❌ FORBIDDEN — never do this -->
  <div
    class="bg-[#FFFFFF] text-[#0F172A] border border-[#E2E8F0] rounded-lg p-6"
  >
    <div
      class="bg-white text-slate-900 border border-slate-200 rounded-lg p-6"
    ></div>
  </div>
</div>
```

The only exception: spacing, radius, and sizing values that map directly to Tailwind's default scale (`p-4`, `gap-2`, `rounded-lg`) are allowed. All **color values** must be tokens.

---

### Step 1 — CSS Custom Properties

Define all tokens in your global CSS file. Light mode in `:root`, dark mode under `.dark` (Tailwind `darkMode: 'class'`).

```css
/* globals.css */
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer base {
  :root {
    /* Backgrounds */
    --bg-base: 248 250 252; /* #F8FAFC */
    --bg-subtle: 241 245 249; /* #F1F5F9 */
    --bg-surface: 255 255 255; /* #FFFFFF */
    --bg-surface-raised: 241 245 249; /* #F1F5F9 */

    /* Borders */
    --border-default: 226 232 240; /* #E2E8F0 */
    --border-strong: 203 213 225; /* #CBD5E1 */

    /* Text */
    --text-primary: 15 23 42; /* #0F172A */
    --text-secondary: 71 85 105; /* #475569 */
    --text-muted: 148 163 184; /* #94A3B8 */
    --text-disabled: 203 213 225; /* #CBD5E1 */

    /* Accent — Meta Blue */
    --accent: 8 102 255; /* #0866FF */
    --accent-hover: 0 82 204; /* #0052CC */
    --accent-active: 0 64 170; /* #0040AA */
    --accent-subtle: 235 243 255; /* #EBF3FF */
    --accent-text: 255 255 255; /* #FFFFFF */

    /* Functional */
    --success: 16 185 129; /* #10B981 */
    --success-bg: 236 253 245; /* #ECFDF5 */
    --warning: 245 158 11; /* #F59E0B */
    --warning-bg: 255 251 235; /* #FFFBEB */
    --danger: 239 68 68; /* #EF4444 */
    --danger-bg: 254 242 242; /* #FEF2F2 */
  }

  .dark {
    /* Backgrounds */
    --bg-base: 11 17 32; /* #0B1120 */
    --bg-subtle: 15 26 46; /* #0F1A2E */
    --bg-surface: 16 28 51; /* #101C33 */
    --bg-surface-raised: 21 34 64; /* #152240 */

    /* Borders */
    --border-default: 30 58 95; /* #1E3A5F */
    --border-strong: 42 79 122; /* #2A4F7A */

    /* Text */
    --text-primary: 241 245 249; /* #F1F5F9 */
    --text-secondary: 148 163 184; /* #94A3B8 */
    --text-muted: 74 99 128; /* #4A6380 */
    --text-disabled: 36 58 85; /* #243A55 */

    /* Accent — stays same hue, lightens slightly */
    --accent: 8 102 255; /* #0866FF */
    --accent-hover: 0 82 204; /* #0052CC */
    --accent-active: 0 64 170; /* #0040AA */
    --accent-subtle: 0 26 77; /* #001A4D */
    --accent-text: 255 255 255;

    /* Functional */
    --success: 52 211 153; /* #34D399 */
    --success-bg: 5 46 31; /* #052E1F */
    --warning: 251 191 36; /* #FBBF24 */
    --warning-bg: 45 27 0; /* #2D1B00 */
    --danger: 248 113 113; /* #F87171 */
    --danger-bg: 42 10 10; /* #2A0A0A */
  }
}
```

> **Why RGB triplets?** Tailwind's opacity modifier (`bg-surface/50`) requires the value to be channel numbers, not hex. Write `255 255 255` not `#FFFFFF`.

---

### Step 2 — tailwind.config.ts

Map every CSS variable into a Tailwind color key. Use the `rgb(var(...) / <alpha-value>)` pattern so opacity modifiers work.

```ts
// tailwind.config.ts
import type { Config } from "tailwindcss";

const config: Config = {
  darkMode: "class",
  content: ["./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        /* Backgrounds */
        base: "rgb(var(--bg-base) / <alpha-value>)",
        subtle: "rgb(var(--bg-subtle) / <alpha-value>)",
        surface: "rgb(var(--bg-surface) / <alpha-value>)",
        "surface-raised": "rgb(var(--bg-surface-raised) / <alpha-value>)",

        /* Borders */
        default: "rgb(var(--border-default) / <alpha-value>)",
        strong: "rgb(var(--border-strong) / <alpha-value>)",

        /* Text */
        primary: "rgb(var(--text-primary) / <alpha-value>)",
        secondary: "rgb(var(--text-secondary) / <alpha-value>)",
        muted: "rgb(var(--text-muted) / <alpha-value>)",
        disabled: "rgb(var(--text-disabled) / <alpha-value>)",

        /* Accent */
        accent: "rgb(var(--accent) / <alpha-value>)",
        "accent-hover": "rgb(var(--accent-hover) / <alpha-value>)",
        "accent-active": "rgb(var(--accent-active) / <alpha-value>)",
        "accent-subtle": "rgb(var(--accent-subtle) / <alpha-value>)",
        "accent-text": "rgb(var(--accent-text) / <alpha-value>)",

        /* Functional */
        success: "rgb(var(--success) / <alpha-value>)",
        "success-bg": "rgb(var(--success-bg) / <alpha-value>)",
        warning: "rgb(var(--warning) / <alpha-value>)",
        "warning-bg": "rgb(var(--warning-bg) / <alpha-value>)",
        danger: "rgb(var(--danger) / <alpha-value>)",
        "danger-bg": "rgb(var(--danger-bg) / <alpha-value>)",
      },
      fontFamily: {
        sans: ["Inter", "-apple-system", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "Menlo", "monospace"],
      },
      borderRadius: {
        sm: "4px",
        md: "6px",
        lg: "8px",
        xl: "12px",
        "2xl": "16px",
      },
      boxShadow: {
        raised: "0 1px 3px rgb(0 0 0 / 0.08), 0 1px 2px rgb(0 0 0 / 0.06)",
        floating:
          "0 4px 6px -1px rgb(0 0 0 / 0.10), 0 2px 4px -2px rgb(0 0 0 / 0.08)",
        overlay:
          "0 20px 25px -5px rgb(0 0 0 / 0.15), 0 8px 10px -6px rgb(0 0 0 / 0.10)",
        focus: "0 0 0 3px rgb(8 102 255 / 0.25)",
      },
    },
  },
  plugins: [],
};

export default config;
```

---

### Step 3 — Class Name Mapping Reference

Use this table to translate design tokens into Tailwind classes.

| Intent                    | Tailwind Class                                          |
| ------------------------- | ------------------------------------------------------- |
| Page background           | `bg-base`                                               |
| Sidebar / secondary bg    | `bg-subtle`                                             |
| Card / panel              | `bg-surface`                                            |
| Hovered card              | `bg-surface-raised`                                     |
| Default border            | `border-default`                                        |
| Strong border             | `border-strong`                                         |
| Primary text              | `text-primary`                                          |
| Secondary text            | `text-secondary`                                        |
| Muted / timestamp         | `text-muted`                                            |
| Disabled text             | `text-disabled`                                         |
| Primary button bg         | `bg-accent`                                             |
| Primary button hover      | `hover:bg-accent-hover`                                 |
| Primary button active     | `active:bg-accent-active`                               |
| Link / interactive accent | `text-accent`                                           |
| Accent subtle bg (badge)  | `bg-accent-subtle`                                      |
| Focus ring                | `focus-visible:shadow-focus focus-visible:outline-none` |
| Success text              | `text-success`                                          |
| Success background        | `bg-success-bg`                                         |
| Warning text              | `text-warning`                                          |
| Warning background        | `bg-warning-bg`                                         |
| Danger text               | `text-danger`                                           |
| Danger background         | `bg-danger-bg`                                          |

---

### Component Examples

**Card**

```html
<div class="bg-surface border border-default rounded-lg p-6">
  <h3 class="text-primary font-semibold text-lg">Title</h3>
  <p class="text-secondary text-sm mt-1">Description text</p>
</div>
```

**Primary Button**

```html
<button
  class="
  bg-accent hover:bg-accent-hover active:bg-accent-active
  text-accent-text font-medium text-sm
  px-5 py-2.5 rounded-md
  focus-visible:outline-none focus-visible:shadow-focus
  transition-colors duration-150
"
>
  Save changes
</button>
```

**Secondary Button**

```html
<button
  class="
  bg-transparent border border-default hover:border-strong hover:bg-surface-raised
  text-primary font-medium text-sm
  px-5 py-2.5 rounded-md
  focus-visible:outline-none focus-visible:shadow-focus
  transition-colors duration-150
"
>
  Cancel
</button>
```

**Text Input**

```html
<input
  class="
  w-full bg-surface border border-default hover:border-strong
  focus:border-accent focus:shadow-focus
  text-primary placeholder:text-muted text-sm
  px-3 py-2.5 rounded-md
  focus:outline-none transition-shadow duration-100
"
  placeholder="Enter value..."
/>
```

**Badge — Blue**

```html
<span
  class="bg-accent-subtle text-accent text-[11px] font-semibold uppercase tracking-wide px-2 py-0.5 rounded-sm"
>
  Active
</span>
```

**Nav Item — Active**

```html
<a
  class="flex items-center gap-2 h-9 px-3 rounded-md bg-accent-subtle text-accent text-sm font-medium"
>
  <Icon class="w-4 h-4" />
  Dashboard
</a>
```

**Stat Card**

```html
<div class="bg-surface border border-default rounded-lg p-6">
  <p class="text-muted text-[11px] font-semibold uppercase tracking-widest">
    Total Revenue
  </p>
  <p class="text-primary text-4xl font-bold tabular-nums mt-1">$48,295</p>
  <p class="text-success text-sm font-medium mt-1">+12.5% from last month</p>
</div>
```

---

### AI Generation Rules (enforce these in every prompt)

1. **Never use `bg-[#...]`, `text-[#...]`, `border-[#...]`** — always use a semantic token class
2. **Never use Tailwind's built-in color palette** (`bg-white`, `text-slate-900`, `border-gray-200`) — they bypass the token system and break dark mode
3. **For dark mode**: add the `.dark` class to `<html>` — do NOT use `dark:` prefixed classes on individual elements unless overriding a specific token behavior
4. **Opacity modifiers are allowed**: `bg-surface/80`, `text-primary/60` — these work because tokens are defined as RGB triplets
5. **Spacing, sizing, radius**: use Tailwind's default scale freely (`p-4`, `gap-3`, `rounded-lg`, `w-full`)
6. **Hardcoded exceptions** (the only ones allowed):
   - `text-[11px]` — Tailwind has no `text-11` step
   - `tracking-[0.06em]` — custom letter-spacing for uppercase labels
   - `tabular-nums` — numeric alignment utility
