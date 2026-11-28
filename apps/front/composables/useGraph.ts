import type { Graph } from "~/utils/entities";

// Graphe CTI exposé par l'API (/graph), via le relais même origine. Appel côté navigateur :
// inutile de bloquer le rendu SSR sur Neo4j.
// La clé partagée évite de recharger le graphe à chaque page.
export function useGraph(limit = 200) {
  const { public: { apiBase } } = useRuntimeConfig();
  return useLazyFetch<Graph>("/graph", {
    baseURL: apiBase,
    query: { limit },
    key: `graph-${limit}`,
    server: false,
    timeout: 8000,
    getCachedData: (key, nuxtApp) => nuxtApp.payload.data[key] ?? nuxtApp.static.data[key],
  });
}
