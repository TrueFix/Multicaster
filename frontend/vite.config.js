import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath, URL } from "node:url";
export default defineConfig({
  plugins: [react(), tailwindcss()],
  base: "/app/",
  resolve: { alias: { "@": fileURLToPath(new URL("./src", import.meta.url)) } },
  build: { outDir: "../cmd/multicaster/web", emptyOutDir: true },
  server: {
    proxy: { "/api": "http://127.0.0.1:8080", "/hls": "http://127.0.0.1:8080" },
  },
});
