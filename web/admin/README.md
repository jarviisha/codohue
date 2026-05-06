# Codohue Admin

React admin UI for operating Codohue locally and in deployed environments. The app is built with Vite, React Router, TanStack Query, and Tailwind CSS.

## Development

Install dependencies from this directory:

```sh
npm install
```

Run the Vite dev server:

```sh
npm run dev
```

The dev server proxies `/api` requests to `http://localhost:2002` as configured in `vite.config.ts`.

## Build And Checks

```sh
npm run lint
npm run build
```

`npm run build` writes the production SPA to `dist/`. The Go package in `embed.go` embeds that directory for serving the admin UI from the API binary.

## Structure

- `src/pages`: route-level screens.
- `src/components`: layout, shared components, and small UI primitives.
- `src/hooks`: TanStack Query hooks.
- `src/services`: HTTP client, admin endpoint wrappers, and query keys.
- `src/types.ts`: admin API request and response contracts.
- `src/routes.tsx`: route and sidebar navigation config.

## Design Contract

Use [`DESIGN_CONTRACT.md`](./DESIGN_CONTRACT.md) as the source of truth for admin UI layout, spacing, component usage, state patterns, and refactor checks.

## Auth

Login uses `POST /api/auth/login` with the configured admin API key. Auth state is maintained by the backend via cookies, and admin API requests include credentials.
