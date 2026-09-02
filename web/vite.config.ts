import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// The dashboard is compiled into the Go binary, so it builds straight into
// the embed directory. `manhattan serve` then needs nothing on disk, which is
// what lets a judge run the whole demo from a single file.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "../cmd/manhattan/dist",
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    // In development the interface runs on Vite and the API on the Go
    // binary, so the two are proxied together.
    proxy: { "/api": "http://localhost:8080" },
  },
});
