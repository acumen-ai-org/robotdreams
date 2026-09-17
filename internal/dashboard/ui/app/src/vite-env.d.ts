/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_DREAM_DEV_TOKEN?: string;
}

declare module "*.webp" {
  const src: string;
  export default src;
}
