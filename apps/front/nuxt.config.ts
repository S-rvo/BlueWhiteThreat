// En-têtes communs à toutes les réponses (pages et fichiers statiques).
// La CSP, qui dépend d'un nonce par requête, est posée par server/plugins/csp.ts.
const securityHeaders = {
  "X-Content-Type-Options": "nosniff",
  "X-Frame-Options": "DENY",
  "Referrer-Policy": "no-referrer",
  "Cross-Origin-Opener-Policy": "same-origin",
  "Cross-Origin-Resource-Policy": "same-origin",
  "Permissions-Policy":
    "camera=(), microphone=(), geolocation=(), payment=(), usb=(), interest-cohort=()",
};

export default defineNuxtConfig({
  compatibilityDate: "2025-05-01",
  devtools: { enabled: false },

  css: ["~/assets/css/tailwind.css"],

  postcss: {
    plugins: {
      tailwindcss: {},
      autoprefixer: {},
    },
  },

  modules: ["@nuxt/eslint", "@nuxt/fonts"],

  // Polices téléchargées au build et servies depuis /_fonts : aucun appel
  // tiers à l'exécution, compatible avec font-src 'self'.
  fonts: {
    defaults: { weights: [400, 500, 600, 700], styles: ["normal"] },
    families: [
      { name: "Chakra Petch", provider: "google" },
      { name: "IBM Plex Sans", provider: "google" },
      { name: "IBM Plex Mono", provider: "google", weights: [400, 500] },
    ],
  },

  app: {
    head: {
      htmlAttrs: { lang: "fr" },
      titleTemplate: "%s · BlueWhiteThreat",
      meta: [
        { name: "description", content: "Veille CTI : menaces, indicateurs de compromission et graphe d'entités." },
        { name: "theme-color", content: "#070b12" },
        { name: "color-scheme", content: "dark" },
      ],
      link: [{ rel: "icon", href: "/favicon.ico" }],
    },
  },

  runtimeConfig: {
    // API vue depuis le conteneur front (relais server/api) ; NUXT_API_INTERNAL.
    apiInternal: "http://api:8083",
    public: {
      // Base appelée par le navigateur : le relais même origine par défaut.
      // Surchargeable via NUXT_PUBLIC_API_BASE (URL absolue = appel direct, CORS requis).
      apiBase: "/api",
    },
  },

  routeRules: {
    "/**": { headers: securityHeaders },
    "/_nuxt/**": { headers: { "Cache-Control": "public, max-age=31536000, immutable" } },
    "/_fonts/**": { headers: { "Cache-Control": "public, max-age=31536000, immutable" } },
  },

  nitro: {
    compressPublicAssets: true,
  },

  sourcemap: { client: false, server: false },

  devServer: {
    host: "0.0.0.0",
    port: 8080,
  },

  // Serveur de dev uniquement (Dockerfile.dev, volume monté) : le polling est
  // nécessaire pour voir les modifications à travers le montage Docker.
  vite: {
    server: {
      hmr: { protocol: "ws", clientPort: 8080, port: 8080 },
      watch: { usePolling: true },
    },
  },
});
