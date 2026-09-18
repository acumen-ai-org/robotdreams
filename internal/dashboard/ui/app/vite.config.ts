import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import react from "@vitejs/plugin-react";
import { defineConfig, type Plugin } from "vite";
// @ts-expect-error -- untyped JS plugin
import mcScope from "../packages/mission-control/postcss-mc-scope.js";

const API_BASE_MARKER = "<!--DREAM_API_BASE-->";

const LIB_SRC = resolve(__dirname, "../packages/mission-control/src");
const OUT = resolve(__dirname, "../../web/dist");

function assertApiBaseMarker(): Plugin {
  return {
    name: "assert-api-base-marker",
    closeBundle() {
      if (!readFileSync(resolve(OUT, "index.html"), "utf8").includes(API_BASE_MARKER)) {
        throw new Error(`built index.html lost the ${API_BASE_MARKER} marker`);
      }
    },
  };
}

export default defineConfig({
  base: "./",
  plugins: [react(), assertApiBaseMarker()],
  css: { postcss: { plugins: [mcScope({ skip: (f: string) => f.replace(/\\/g, "/").includes("/app/src/") })] } },
  resolve: {
    alias: [
      { find: /^@robotdreams\/mission-control$/, replacement: resolve(LIB_SRC, "index.ts") },
      { find: /^@robotdreams\/mission-control\/(.*)$/, replacement: LIB_SRC + "/$1" },
    ],
    dedupe: ["react", "react-dom"],
  },
  build: {
    outDir: OUT,
    emptyOutDir: true,
  },
  server: {
    fs: { allow: [resolve(__dirname, "..")] },
    allowedHosts: ["celin-p1.tail567a32.ts.net"],
    proxy: {
      "/api": {
        target: process.env.DREAM_API ?? "http://127.0.0.1:8420",
        changeOrigin: true,
      },
    },
  },
});
