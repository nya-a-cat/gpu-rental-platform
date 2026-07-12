/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string;
  readonly VITE_RUNTIME_MODE?: "api" | "demo";
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
