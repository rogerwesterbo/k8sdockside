import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import wails from "@wailsio/runtime/plugins/vite";

// https://vitejs.dev/config/
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  build: {
    // The bundle is read from the binary's own embedded filesystem, not fetched
    // over a network, so the 500 kB default -- a budget for download time --
    // is measuring a cost this app does not pay. CodeMirror and xterm account
    // for most of the weight and both are needed on the first screen that uses
    // them, so splitting them out would move the parse rather than avoid it.
    // The limit is raised to keep a real regression visible instead of leaving
    // a warning that is always on and therefore never read.
    chunkSizeWarningLimit: 1500,
  },
  plugins: [svelte(), wails("./bindings")],
});
