import { fileURLToPath, URL } from "node:url";

import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import vueDevTools from "vite-plugin-vue-devtools";

// https://vite.dev/config/
export default defineConfig({
  plugins: [vue(), vueDevTools()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  server: {
    host: "0.0.0.0",
    port: 8081,
    allowedHosts: [
      "localhost",
      "127.0.0.1",
      "vue-target-dev.target.orb.local",
      "vue-target-dev",
      ".orb.local",
    ],
    watch: {
      usePolling: true,
    },
  },
});
