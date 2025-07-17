export default defineNuxtConfig({
  css: ["@/assets/css/tailwind.css", "assets/css/app.css"],

  postcss: {
    plugins: {
      tailwindcss: {},
      autoprefixer: {},
    },
  },

  modules: ["@nuxt/eslint", "@nuxt/fonts", "@nuxt/icon"],

  devServer: {
    host: "0.0.0.0", // Écoute sur toutes les interfaces
    port: 8080,
  },

  vite: {
    server: {
      host: true, // Autorise 0.0.0.0 pour toutes les connexions
      strictPort: false, // Évite les blocages sur les ports
      hmr: {
        protocol: "ws", // WebSocket non sécurisé
        host: "0.0.0.0", // Accepte TOUS les hosts pour HMR (workaround permissif pour OrbStack)
        clientPort: 8080, // Port externe vu par le navigateur
        port: 8080, // Port interne pour HMR
      },
      watch: {
        usePolling: true, // Pour hot-reload fiable avec Docker volumes
      },
    },
  },
});
