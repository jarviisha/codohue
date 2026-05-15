/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_ADMIN_API_BASE_URL?: string
  readonly VITE_ADMIN_DEV_PORT?: string
  readonly VITE_ADMIN_PROXY_TARGET?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
