<script setup lang="ts">
import { KINDS, defang, entityRows } from "~/utils/entities";

useHead({ title: "Ma veille" });

// Termes conservés uniquement dans ce navigateur (localStorage) : rien n'est envoyé à l'API.
const STORAGE_KEY = "bwt:watchlist";
const MAX_TERMS = 50;
const terms = ref<string[]>([]);
const draft = ref("");
const notice = ref("");

onMounted(() => {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    const parsed: unknown = raw ? JSON.parse(raw) : [];
    if (Array.isArray(parsed)) {
      terms.value = parsed.filter((t): t is string => typeof t === "string" && t.length <= 200).slice(0, MAX_TERMS);
    }
  } catch {
    terms.value = [];
  }
});

function persist() {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(terms.value));
  } catch {
    notice.value = "Stockage local indisponible : la liste ne sera pas conservée.";
  }
}

function add() {
  const term = draft.value.trim();
  notice.value = "";
  if (!term) return;
  if (terms.value.some((t) => t.toLowerCase() === term.toLowerCase())) {
    notice.value = "Terme déjà suivi.";
    return;
  }
  if (terms.value.length >= MAX_TERMS) {
    notice.value = `Limite de ${MAX_TERMS} termes atteinte.`;
    return;
  }
  terms.value = [...terms.value, term];
  draft.value = "";
  persist();
}

function remove(term: string) {
  terms.value = terms.value.filter((t) => t !== term);
  persist();
}

const { data: graph, status } = useGraph();
const rows = computed(() => entityRows(graph.value));
const matches = computed(() =>
  terms.value.map((term) => {
    const q = term.toLowerCase();
    return { term, hits: rows.value.filter((r) => r.label.toLowerCase().includes(q)) };
  }),
);
const hitCount = computed(() => matches.value.filter((m) => m.hits.length).length);
</script>

<template>
  <div>
    <PageHeader
      eyebrow="Surveillance personnelle"
      title="Ma veille"
      lead="Suivez vos marques, domaines, plages IP ou fournisseurs. La liste reste dans ce navigateur et est comparée aux entités extraites."
    />

    <form class="panel mb-6 flex flex-col gap-3 p-4 sm:flex-row" @submit.prevent="add">
      <label class="flex-1">
        <span class="sr-only">Terme à surveiller</span>
        <input v-model="draft" class="field" placeholder="ex. exemple.fr, 203.0.113., Fortinet…" maxlength="200" autocomplete="off" spellcheck="false" >
      </label>
      <button type="submit" class="btn-primary" :disabled="!draft.trim()">
        <AppIcon name="plus" :size="16" /> Suivre
      </button>
    </form>
    <p v-if="notice" class="-mt-3 mb-6 text-sm text-rouge" role="status">{{ notice }}</p>

    <StatusNote v-if="!terms.length" tone="empty">Aucun terme suivi. Ajoutez-en un pour être alerté dès qu'il apparaît.</StatusNote>

    <template v-else>
      <p class="mb-4 text-sm text-muted">
        <span v-if="!graph && status !== 'error'">Comparaison en cours…</span>
        <span v-else-if="status === 'error'" class="text-rouge">Graphe indisponible : comparaison impossible.</span>
        <span v-else><strong class="text-fg">{{ hitCount }}</strong> terme(s) sur {{ terms.length }} retrouvé(s) dans les articles récents.</span>
      </p>
      <ul class="grid gap-3 md:grid-cols-2">
        <li
          v-for="m in matches"
          :key="m.term"
          class="panel p-4"
          :class="m.hits.length ? 'border-rouge/50' : ''"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <p class="truncate font-mono text-sm" :title="m.term">{{ m.term }}</p>
              <p class="mt-1 text-xs" :class="m.hits.length ? 'text-rouge' : 'text-muted'">
                {{ m.hits.length ? `${m.hits.length} correspondance(s)` : "Aucune mention" }}
              </p>
            </div>
            <button type="button" class="btn-ghost px-2 py-1" @click="remove(m.term)">
              <AppIcon name="x" :size="14" /><span class="sr-only">Ne plus suivre {{ m.term }}</span>
            </button>
          </div>
          <ul v-if="m.hits.length" class="mt-3 space-y-1.5 border-t border-line pt-3">
            <li v-for="hit in m.hits.slice(0, 5)" :key="hit.id" class="flex items-center gap-2 text-sm">
              <KindBadge :kind="hit.kind" />
              <code class="truncate font-mono text-[0.8rem]">{{ defang(hit.kind, hit.label) }}</code>
              <span class="ml-auto shrink-0 text-xs text-muted">{{ hit.mentions }} art.</span>
            </li>
            <li v-if="m.hits.length > 5" class="text-xs text-muted">+ {{ m.hits.length - 5 }} autre(s) — voir <NuxtLink to="/ioc" class="text-bleu underline-offset-2 hover:underline">Indicateurs</NuxtLink></li>
          </ul>
        </li>
      </ul>
    </template>
    <p class="mt-8 text-xs text-muted">Couleur d'un type : <span v-for="(meta, k) in KINDS" :key="k" class="mr-2 whitespace-nowrap"><span class="mr-1 inline-block h-1.5 w-1.5 rounded-full align-middle" :style="{ backgroundColor: meta.color }" />{{ meta.label }}</span></p>
  </div>
</template>
