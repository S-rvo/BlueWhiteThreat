<script setup lang="ts">
useHead({ title: "Menaces" });

const { items, pending, error, done, load } = useArticles();
onMounted(() => {
  if (!items.value.length) load();
});

const dateFmt = new Intl.DateTimeFormat("fr-FR", { dateStyle: "medium", timeStyle: "short" });
const formatDate = (iso?: string) => (iso ? dateFmt.format(new Date(iso)) : "Date inconnue");

const sources = computed(() => {
  const counts = new Map<string, number>();
  for (const a of items.value) {
    const key = a.source_type || "inconnue";
    counts.set(key, (counts.get(key) ?? 0) + 1);
  }
  return [...counts.entries()].sort((a, b) => b[1] - a[1]);
});
const filter = ref<string | null>(null);
const visible = computed(() =>
  filter.value ? items.value.filter((a) => (a.source_type || "inconnue") === filter.value) : items.value,
);
</script>

<template>
  <div>
    <PageHeader
      eyebrow="Flux qualifié"
      title="Menaces"
      lead="Documents classés pertinents et validés. L'API n'expose ni le contenu brut ni l'URL source."
    >
      <template #actions>
        <button type="button" class="btn-ghost" :disabled="pending" @click="load(true)">
          <AppIcon name="refresh" :size="16" /> Actualiser
        </button>
      </template>
    </PageHeader>

    <div v-if="sources.length > 1" class="mb-6 flex flex-wrap gap-2" role="group" aria-label="Filtrer par source">
      <button type="button" class="chip" :aria-pressed="filter === null" @click="filter = null">
        Toutes <span class="text-muted">{{ items.length }}</span>
      </button>
      <button
        v-for="[name, count] in sources"
        :key="name"
        type="button"
        class="chip"
        :aria-pressed="filter === name"
        @click="filter = name"
      >
        {{ name }} <span class="text-muted">{{ count }}</span>
      </button>
    </div>

    <StatusNote v-if="error && !items.length" tone="error">{{ error }}</StatusNote>
    <StatusNote v-else-if="pending && !items.length" tone="loading">Chargement des documents…</StatusNote>
    <StatusNote v-else-if="!items.length" tone="empty">Aucun document pertinent pour le moment.</StatusNote>

    <ul v-else class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      <li v-for="article in visible" :key="article.id" class="panel group p-4 transition hover:border-muted/60">
        <div class="flex items-center justify-between gap-3">
          <span class="rounded-md border border-line bg-ink/60 px-2 py-0.5 font-mono text-[0.7rem] uppercase tracking-wider text-bleu">
            {{ article.source_type || "source inconnue" }}
          </span>
          <time v-if="article.scraped_at" :datetime="article.scraped_at" class="text-xs text-muted">
            {{ formatDate(article.scraped_at) }}
          </time>
        </div>
        <p class="mt-4 font-display text-base">Document pertinent</p>
        <p class="mt-1 truncate font-mono text-xs text-muted" :title="article.id">ID {{ article.id }}</p>
      </li>
    </ul>

    <div v-if="items.length" class="mt-8 flex flex-col items-center gap-2">
      <p v-if="error" class="text-sm text-rouge" role="alert">{{ error }}</p>
      <button v-if="!done" type="button" class="btn-ghost" :disabled="pending" @click="load()">
        {{ pending ? "Chargement…" : "Charger plus" }}
      </button>
      <p v-else class="text-xs text-muted">Fin du flux · {{ items.length }} documents</p>
    </div>
  </div>
</template>

<style scoped>
.chip {
  @apply inline-flex items-center gap-2 rounded-full border border-line px-3 py-1 text-sm text-fg transition hover:border-muted;
}
.chip[aria-pressed="true"] {
  @apply border-bleu bg-bleu/15;
}
</style>
