export interface ArticleSummary {
  id: string;
  source_type?: string;
  scraped_at?: string;
}

// Liste paginée des articles pertinents (/data), curseur porté par X-Next-Cursor.
export function useArticles(pageSize = 24) {
  const { public: { apiBase } } = useRuntimeConfig();
  const items = useState<ArticleSummary[]>("articles", () => []);
  const cursor = useState<string | null>("articles-cursor", () => null);
  const done = useState("articles-done", () => false);
  const pending = ref(false);
  const error = ref<string | null>(null);

  async function load(reset = false) {
    if (pending.value || (done.value && !reset)) return;
    if (reset) {
      items.value = [];
      cursor.value = null;
      done.value = false;
    }
    pending.value = true;
    error.value = null;
    try {
      const query: Record<string, string | number> = { limit: pageSize };
      if (cursor.value) query.cursor = cursor.value;
      const res = await $fetch.raw<ArticleSummary[]>("/data", { baseURL: apiBase, query, timeout: 8000 });
      items.value = [...items.value, ...(res._data ?? [])];
      cursor.value = res.headers.get("X-Next-Cursor");
      done.value = !cursor.value;
    } catch {
      error.value = "API injoignable : vérifier que le service api tourne.";
    } finally {
      pending.value = false;
    }
  }

  return { items, pending, error, done, load };
}
