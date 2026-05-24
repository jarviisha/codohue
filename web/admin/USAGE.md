# Using Davinci in Your Project

> 🇻🇳 Bản tiếng Việt: [USAGE.vn.md](./USAGE.vn.md)

This document is for **end users** who want to install the `@jarviisha/davinci-*` packages from npm and use them in their own project (React app, Next.js, Vite, plain HTML, …).

For the design philosophy behind these packages — when to use which variant, what each token means, anti-patterns — see [DESIGN.md](./DESIGN.md).

---

## Available packages

| Package | What it is | Required? |
| --- | --- | --- |
| `@jarviisha/davinci-tokens` | CSS variables (`--davinci-*`) + typed JS tokens. Framework-agnostic. | Yes — every other package depends on these CSS variables. |
| `@jarviisha/davinci-tailwind-preset` | Tailwind CSS v4 preset mapping tokens to utility classes (`bg-background`, `text-foreground`, …). | Only if you use Tailwind. |
| `@jarviisha/davinci-react-theme-provider` | `<ThemeProvider>` + `<ThemeScript>` for light / dark / system. | Only if you need theme switching. |
| `@jarviisha/davinci-react-ui` | React components (`Button`, `Dialog`, `Input`, …) styled with CSS tokens. **No Tailwind required.** | Only if you want ready-made components. |

Baseline requirements: **Node ≥ 20**, React 18.3.1 or 19 (for the React packages).

---

## Install

Pick the combo that matches your stack.

### Combo A — Tokens only (raw CSS variables)

For: plain HTML, Vue, Svelte, or any framework where you write your own CSS using `var(--davinci-*)`.

```bash
npm install @jarviisha/davinci-tokens
# or
pnpm add @jarviisha/davinci-tokens
```

### Combo B — Tokens + Tailwind preset

For: projects using Tailwind CSS v4.

```bash
npm install @jarviisha/davinci-tokens @jarviisha/davinci-tailwind-preset
npm install -D tailwindcss@^4.1.0
```

### Combo C — Tokens + React UI (no Tailwind needed)

For: React projects that want to use the components out of the box.

```bash
npm install @jarviisha/davinci-tokens @jarviisha/davinci-react-ui @jarviisha/davinci-react-theme-provider
```

(`react` and `react-dom` are peer dependencies — they must already exist in your project.)

---

## Step 1 — Import the CSS variables

Required for every combo. Add this to your app's CSS entry file (usually `src/index.css`, `src/main.css`, or `app/globals.css`):

```css
@import "@jarviisha/davinci-tokens/css/variables.css";
@import "@jarviisha/davinci-tokens/css/light.css";
@import "@jarviisha/davinci-tokens/css/dark.css";
```

After this import, the document has all of `--davinci-color-*`, `--davinci-spacing-*`, `--davinci-radius-*`, etc.

If you also use the **Tailwind preset (Combo B)**, add one more line:

```css
@import "@jarviisha/davinci-tokens/css/variables.css";
@import "@jarviisha/davinci-tokens/css/light.css";
@import "@jarviisha/davinci-tokens/css/dark.css";
@import "tailwindcss";
```

If you also use **React UI (Combo C)**:

```css
@import "@jarviisha/davinci-tokens/css/variables.css";
@import "@jarviisha/davinci-tokens/css/light.css";
@import "@jarviisha/davinci-tokens/css/dark.css";
@import "@jarviisha/davinci-react-ui/styles.css";
```

Order matters: `variables.css` must come before `light.css` / `dark.css`; the UI or Tailwind import goes last.

---

## Step 2 — Configure the Tailwind preset (Combo B only)

In `tailwind.config.ts` (or `.js`):

```ts
import preset from "@jarviisha/davinci-tailwind-preset";
import type { Config } from "tailwindcss";

export default {
  presets: [preset],
  content: ["./index.html", "./src/**/*.{ts,tsx,js,jsx}"]
} satisfies Config;
```

After this, you get utility classes mapped to semantic tokens:

```tsx
<div className="bg-background text-foreground">
  <button className="rounded-md bg-primary text-primary-foreground px-4 py-2">
    Save
  </button>
</div>
```

---

## Step 3 — Add the ThemeProvider (light / dark / system)

Wrap your app with `ThemeProvider`:

```tsx
import { ThemeProvider } from "@jarviisha/davinci-react-theme-provider";

export function Root() {
  return (
    <ThemeProvider defaultTheme="system" storageKey="my-app-theme">
      {/* your app */}
    </ThemeProvider>
  );
}
```

Main props:

- `defaultTheme`: `"light" | "dark" | "system"` (default `"system"`).
- `storageKey`: `localStorage` key used to remember the user's choice (default `"davinci-theme"`).

Use the `useTheme` hook to toggle:

```tsx
import { useTheme } from "@jarviisha/davinci-react-theme-provider";

export function ThemeToggle() {
  const { theme, resolvedTheme, setTheme } = useTheme();
  return (
    <button onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}>
      Current: {theme} (resolved: {resolvedTheme})
    </button>
  );
}
```

### Prevent theme flash on reload (matters for dark-mode users)

`ThemeProvider` applies the class after React mounts, which produces a brief light → dark flash. Inject a synchronous script in `<head>` so the class is set **before first paint**.

**Vite / CRA / plain HTML** — call `getThemeScript()` and inline the returned string into `index.html`:

```html
<head>
  <script>
    /* paste the string returned by getThemeScript({ defaultTheme: "system", storageKey: "my-app-theme" }) */
  </script>
</head>
```

**Next.js / Remix / Astro** — render `<ThemeScript />` inside `<head>`:

```tsx
import { ThemeScript } from "@jarviisha/davinci-react-theme-provider";

export default function RootLayout({ children }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        <ThemeScript defaultTheme="system" storageKey="my-app-theme" />
      </head>
      <body>{children}</body>
    </html>
  );
}
```

> Pass **exactly the same** `defaultTheme` and `storageKey` to both `<ThemeScript />` and `<ThemeProvider>` — if they drift, the theme reads incorrectly.

---

## Step 4 — Use the React UI components (Combo C)

```tsx
import { Button, Card, CardHeader, CardTitle, CardContent } from "@jarviisha/davinci-react-ui";

export function Example() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Hello</CardTitle>
      </CardHeader>
      <CardContent>
        <Button variant="primary">Save</Button>
      </CardContent>
    </Card>
  );
}
```

Available components (full props show up via TypeScript on import):

- **Form**: `Button`, `IconButton`, `Input`, `Textarea`, `Select`, `Checkbox`, `Radio`, `Switch`, `Combobox`, `SearchInput`, `Label`, `FormField`, `FormHelpText`, `FormErrorText`.
- **Layout**: `AppShell` (slots: `AppShellTopBar`, `AppShellSidebar`, `AppShellHeader`, `AppShellMain`, `AppShellAside`), `DetailLayout` (`DetailLayoutMain` + `DetailLayoutAside`), `Container`, `Stack`, `Inline`, `Divider`, `Card`.
- **Navigation**: `Nav`, `Breadcrumbs`, `Pagination`, `Tabs`, `DropdownMenu`.
- **Overlay**: `Dialog`, `Drawer`, `Popover`, `Tooltip`, `Toast` (via `ToastProvider` + `useToast`).
- **Feedback**: `Alert`, `Badge`, `Skeleton`, `EmptyState`, `Avatar`.
- **Data**: `Table` (`TableHeader`, `TableBody`, `TableRow`, …).
- **Hook**: `useFocusTrap`.

### Thin hover-reveal scrollbar

`@jarviisha/davinci-react-ui/styles.css` ships a thin, hover-reveal scrollbar utility. The thumb is transparent at rest and fades in when the container is hovered or receives focus, using `--davinci-component-scrollbar-*` tokens so it follows light / dark automatically.

Already auto-applied to `AppShellSidebar`, `AppShellMain`, `AppShellAside`, `Dialog`, `Drawer`, `Combobox`'s listbox, and `TableContainer` — no extra class needed.

Opt in on any other scroll container with the `davinci-scrollbar` class:

```tsx
<aside className="davinci-scrollbar" style={{ overflow: "auto" }}>
  {/* sidebar items */}
</aside>
```

Add `davinci-scrollbar--always` if you want the thumb visible at rest:

```tsx
<div className="davinci-scrollbar davinci-scrollbar--always">{/* … */}</div>
```

Override per app by redefining the tokens:

```css
:root {
  --davinci-component-scrollbar-size: 0.625rem;
  --davinci-component-scrollbar-thumb-background: var(--davinci-semantic-color-border-bold);
}
```

> Firefox supports only `scrollbar-width: thin` (no precise sizing), so the size token is honored on Chromium / WebKit and approximated on Firefox.

---

Toast needs a `ToastProvider` near the root:

```tsx
import { ToastProvider, useToast } from "@jarviisha/davinci-react-ui";

function Root() {
  return (
    <ToastProvider position="top-right">
      <App />
    </ToastProvider>
  );
}

function SaveButton() {
  const toast = useToast();
  return <Button onClick={() => toast.success("Saved")}>Save</Button>;
}
```

---

## Using tokens from JavaScript / TypeScript

When you need a token value outside CSS (e.g. chart library, canvas, inline style):

```ts
import { tokens } from "@jarviisha/davinci-tokens/js/tokens";

const red500 = tokens["color.red.500"];
// { type: "color", value: "#F15B50", cssVar: "--davinci-color-red-500" }

element.style.color = `var(${red500.cssVar})`;
// or the hard-coded value (not theme-aware):
element.style.color = red500.value;
```

`tokens` is typed `as const`, so your IDE gets full autocomplete.

---

## Upgrading

Package versions are released together via changesets. Upgrade them in lockstep to avoid peer-dep mismatches:

```bash
npm install \
  @jarviisha/davinci-tokens@latest \
  @jarviisha/davinci-tailwind-preset@latest \
  @jarviisha/davinci-react-theme-provider@latest \
  @jarviisha/davinci-react-ui@latest
```

The changelog lives in each package's GitHub Release, or in the `CHANGELOG.md` file shipped inside each package on npm.

---

## Troubleshooting

### The `dark` class isn't applied → colors don't change

- Check that you imported all three CSS files (`variables.css`, `light.css`, `dark.css`).
- Open DevTools and confirm `<html>` has a `light` or `dark` class once `ThemeProvider` mounts.
- If only `variables.css` is imported: semantic values are missing — you must also import `light.css` and `dark.css`.

### Light → dark flash still happens on reload

- You haven't added `<ThemeScript />` (Next/Remix/Astro) or inlined `getThemeScript()` (Vite/CRA).
- `<ThemeScript />` is rendered in `<body>` instead of `<head>` — it must be in `<head>` to run before paint.
- `storageKey` or `defaultTheme` is out of sync between `ThemeScript` and `ThemeProvider`.

### Tailwind doesn't recognize `bg-background`, `text-foreground`, …

- Missing `presets: [preset]` in `tailwind.config`.
- The `content` glob doesn't cover the file using those classes.
- You're on Tailwind v3 — the preset requires Tailwind **v4.1+**.

### TypeScript can't resolve `@jarviisha/davinci-tokens/js/tokens`

- Your `tsconfig.json` needs `"moduleResolution": "Bundler"` or `"NodeNext"` to understand subpath exports.

### `peer dependency` warning for React

- React UI requires React 18.3.1+ or 19. Upgrade if your project is on 18.2 or below.

---

## Migration: token naming consistency (0.x minor with breaking renames)

This release consolidates vocabulary across the three layers and fixes architectural debt. While the packages are still pre-1.0, breaking changes ship as a minor bump per semver. Run the find-replace table below across your codebase.

### Search & replace

| Old | New | Why |
| --- | --- | --- |
| `destructive` (any case in token / class / prop / Tailwind utility) | `danger` | Unifies the red role — previously split between `destructive`, `danger`, and `error` |
| `toast.error(` | `toast.danger(` | `useToast` API renamed for consistency |
| `<Toast variant="error">` | `<Toast variant="danger">` | Same |
| `<Badge variant="destructive">` | `<Badge variant="danger">` | Same |
| `<Card variant="surface">` | `<Card variant="default">` | `surface` collided with the semantic color role; `default` is also the default value when prop is omitted |
| `--davinci-focus-ring-color` | `--davinci-semantic-focus-ring-color` | Focus moved from primitive to semantic layer (also `-width`, `-offset`, `-style`) |
| `--davinci-radius-control` | `--davinci-semantic-radius-control` | Radius semantic aliases moved out of primitive (also `-card`, `-panel`, `-pill`) |
| `--davinci-component-card-surface-*` | `--davinci-component-card-default-*` | Card variant rename |
| `--davinci-spacing-1` / `-2` / `-3` / `-4` / `-6` / `-8` | `--davinci-spacing-050` / `-100` / `-150` / `-200` / `-300` / `-400` | Numeric alias shortcuts removed in favor of the explicit scale |

### Tailwind utility renames (handled by the preset)

- `bg-destructive`, `text-destructive`, `border-destructive`, `bg-destructive-hovered`, `bg-destructive-pressed`, `text-destructive-foreground` → swap `destructive` → `danger`.
- `rounded-control`, `rounded-card`, `rounded-panel`, `rounded-pill` — **unchanged**; the preset re-wires them to the new CSS vars.

### No-op for most users

If you only consume the documented Tailwind utilities and React components and you never wrote `variant="destructive"`, `variant="surface"`, or referenced `--davinci-focus-ring-*` / `--davinci-radius-control` directly, the only change you need is bumping the package versions.

### New tokens worth knowing

- `component.appShell.*` and `component.detailLayout.*` — layout dimensions are now tokenized; override them at `:root` to customize sidebar/aside widths.
- `component.card.toneInfo/toneSuccess/toneWarning/toneDanger` — Card tone backgrounds and borders are first-class tokens; override per-theme to retune the tint intensity.
