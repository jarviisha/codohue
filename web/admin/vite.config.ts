import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const devPort = Number(env.VITE_ADMIN_DEV_PORT ?? 5173);

  return {
    plugins: [react(), tailwindcss()],
    server: {
      port: devPort,
      proxy: {
        "/api": {
          target: env.VITE_ADMIN_PROXY_TARGET ?? "http://localhost:2002",
          changeOrigin: true,
        },
      },
    },
  };
});
