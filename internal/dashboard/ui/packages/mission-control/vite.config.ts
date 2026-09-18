import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vite";
// @ts-expect-error -- untyped JS plugin
import mcScope from "./postcss-mc-scope.js";

const pkg = JSON.parse(readFileSync(resolve(__dirname, "package.json"), "utf8")) as {
  dependencies: Record<string, string>;
  peerDependencies: Record<string, string>;
};
const bare = [...Object.keys(pkg.dependencies), ...Object.keys(pkg.peerDependencies)];
const isExternal = (id: string) => !id.endsWith(".css") && bare.some((d) => id === d || id.startsWith(d + "/"));

function emitTheme(): Plugin {
  return {
    name: "mc-emit-theme",
    generateBundle() {
      this.emitFile({
        type: "asset",
        fileName: "theme.css",
        source: readFileSync(resolve(__dirname, "src/theme.css"), "utf8"),
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), emitTheme()],
  css: { postcss: { plugins: [mcScope()] } },
  build: {
    target: "es2022",
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: true,
    cssCodeSplit: false,
    lib: { entry: resolve(__dirname, "src/index.ts"), formats: ["es"], cssFileName: "style" },
    rollupOptions: {
      external: isExternal,
      output: {
        preserveModules: true,
        preserveModulesRoot: "src",
        entryFileNames: "[name].js",
        chunkFileNames: "[name].js",
        assetFileNames: "[name][extname]",
      },
    },
  },
});
