import { defineConfig, loadEnv } from 'vite'
import type { ServerResponse } from 'node:http'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// vite.config.ts itself does NOT see env vars automatically — Vite only
// auto-injects them into client code via import.meta.env.*. To use them in
// this config (dev port, proxy target), call loadEnv() explicitly.
//
// Honored env (set in web/admin/.env; see .env.example):
//   VITE_ADMIN_DEV_PORT      Vite dev server port    (default 5174)
//   VITE_ADMIN_PROXY_TARGET  /api proxy target       (default http://localhost:2002)
//   VITE_ADMIN_API_BASE_URL  SPA fetch base prefix   (default '' — same-origin)
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), 'VITE_')

  const devPort = Number(env.VITE_ADMIN_DEV_PORT) || 5174
  const proxyTarget = env.VITE_ADMIN_PROXY_TARGET || 'http://localhost:2002'

  return {
    plugins: [react(), tailwindcss()],
    server: {
      port: devPort,
      proxy: {
        // /api -> cmd/admin. When cmd/admin is down we quiet the
        // AggregateError stack trace and reply with a clean 503 so the SPA
        // can render its error state without flooding the terminal.
        '/api': {
          target: proxyTarget,
          changeOrigin: true,
          configure: (proxy) => {
            let warned = false
            proxy.on('error', (err, _req, res) => {
              const code = (err as NodeJS.ErrnoException).code
              if (code !== 'ECONNREFUSED' && code !== 'ENOTFOUND') {
                // Real error: let Vite log it.
                return
              }
              if (!warned) {
                console.warn(
                  `\n[vite proxy] cmd/admin is not reachable on ${proxyTarget}.\n` +
                    `  Start it with: make run-admin  (run \`make up-infra\` first if infra is not up)\n` +
                    `  Or override the target via VITE_ADMIN_PROXY_TARGET in web/admin/.env\n`,
                )
                warned = true
              }
              if ('writeHead' in res && !(res as ServerResponse).headersSent) {
                const r = res as ServerResponse
                r.writeHead(503, { 'Content-Type': 'application/json' })
                r.end(
                  JSON.stringify({
                    error: {
                      code: 'backend_unreachable',
                      message: `cmd/admin not running on ${proxyTarget}`,
                    },
                  }),
                )
              }
            })
          },
        },
      },
    },
    build: {
      outDir: 'dist',
      emptyOutDir: true,
    },
  }
})
