import { randomBytes } from "node:crypto";

// CSP stricte à nonce, régénérée à chaque rendu SSR. Nuxt insère des balises
// <script> et <style> inline (configuration publique, styles critiques) : elles
// reçoivent le nonce, tout autre script inline est refusé par le navigateur.
// Inactive en dev, où Vite a besoin d'eval et du WebSocket HMR.
const tag = /<(script|style)\b(?![^>]*\bnonce=)/g;

function withNonce(chunks: string[], nonce: string): string[] {
  return chunks.map((chunk) => chunk.replace(tag, `<$1 nonce="${nonce}"`));
}

function apiOrigin(apiBase: string): string {
  try {
    return new URL(apiBase).origin;
  } catch {
    return "";
  }
}

export default defineNitroPlugin((nitro) => {
  if (import.meta.dev) return;

  nitro.hooks.hook("render:html", (html, { event }) => {
    const nonce = randomBytes(16).toString("base64");
    html.head = withNonce(html.head, nonce);
    html.bodyPrepend = withNonce(html.bodyPrepend, nonce);
    html.body = withNonce(html.body, nonce);
    html.bodyAppend = withNonce(html.bodyAppend, nonce);

    const api = apiOrigin(useRuntimeConfig(event).public.apiBase as string);
    const policy = [
      "default-src 'none'",
      `script-src 'self' 'nonce-${nonce}'`,
      `style-src 'self' 'nonce-${nonce}'`,
      `style-src-elem 'self' 'nonce-${nonce}'`,
      // Attributs style="" (couleurs calculées, vis-network) : pas d'exécution de code possible.
      "style-src-attr 'unsafe-inline'",
      "img-src 'self' data:",
      "font-src 'self'",
      `connect-src 'self' ${api}`.trim(),
      "manifest-src 'self'",
      "base-uri 'none'",
      "form-action 'self'",
      "frame-ancestors 'none'",
      "object-src 'none'",
    ].join("; ");

    setResponseHeader(event, "Content-Security-Policy", policy);
    // Une page porte un nonce unique : elle ne doit jamais être mise en cache.
    setResponseHeader(event, "Cache-Control", "no-store");
  });

  // Ne pas annoncer le framework utilisé.
  nitro.hooks.hook("render:response", (response) => {
    if (response.headers) delete response.headers["x-powered-by"];
  });
});
