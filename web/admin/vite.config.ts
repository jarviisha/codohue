import { defineConfig } from 'vite'
import type { ServerResponse } from 'node:http'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// Dev proxy: /api -> cmd/admin (port 2002). When cmd/admin is not running we
// quiet the AggregateError stack trace and reply with a clean 503 so the SPA
// can render its error state without flooding the terminal.
const ADMIN_TARGET = 'http://localhost:2002'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5174,
    proxy: {
      '/api': {
        target: ADMIN_TARGET,
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
                `\n[vite proxy] cmd/admin is not reachable on ${ADMIN_TARGET}.\n` +
                  `  Start it with: make run-admin  (run \`make up-infra\` first if infra is not up)\n`,
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
                    message: `cmd/admin not running on ${ADMIN_TARGET}`,
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
})
