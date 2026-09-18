import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

export default defineConfig({
  root: __dirname,
  plugins: [react()],
  build: {
    outDir: resolve(__dirname, "../smoke-dist"),
    emptyOutDir: true,
    minify: false,
    rollupOptions: { input: resolve(__dirname, "smoke.html") },
  },
});
