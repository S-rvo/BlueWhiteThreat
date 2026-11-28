<script setup lang="ts">
import { ENTITY_KINDS, KINDS, countByKind, defang, entityRows, type EntityKind } from "~/utils/entities";

useHead({ title: "Indicateurs" });

const { data: graph, status, refresh } = useGraph();
const rows = computed(() => entityRows(graph.value));
const counts = computed(() => countByKind(rows.value));

const query = ref("");
const active = ref<Set<EntityKind>>(new Set());
function toggle(kind: EntityKind) {
  const next = new Set(active.value);
  if (next.has(kind)) next.delete(kind);
  else next.add(kind);
  active.value = next;
}

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase();
  return rows.value.filter(
    (r) => (!active.value.size || active.value.has(r.kind)) && (!q || r.label.toLowerCase().includes(q)),
  );
});

const PAGE = 50;
const shown = ref(PAGE);
watch([query, active], () => (shown.value = PAGE));

const copied = ref<string | null>(null);
let copiedTimer: ReturnType<typeof setTimeout> | undefined;
async function copy(value: string) {
  try {
    await navigator.clipboard.writeText(value);
    copied.value = value;
    clearTimeout(copiedTimer);
    copiedTimer = setTimeout(() => (copied.value = null), 1500);
  } catch {
    copied.value = null;
  }
}
onBeforeUnmount(() => clearTimeout(copiedTimer));
</script>

<template>
  <div>
    <PageHeader
      eyebrow="Indicateurs de compromission"
      title="IOC et entités"
      lead="Valeurs extraites des articles récents. Les IP et domaines sont affichés désarmés ([.]) ; la copie restitue la valeur exacte."
    >
      <template #actions>
        <button type="button" class="btn-ghost" :disabled="status === 'pending'" @click="() => refresh()">
          <AppIcon name="refresh" :size="16" /> Actualiser
        </button>
      </template>
    </PageHeader>

    <div class="panel mb-6 flex flex-col gap-4 p-4 lg:flex-row lg:items-center">
      <label class="relative block flex-1">
        <span class="sr-only">Rechercher une valeur</span>
        <AppIcon name="search" :size="16" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted" />
        <input v-model="query" type="search" class="field pl-9" placeholder="Rechercher une IP, un domaine, une CVE…" autocomplete="off" spellcheck="false" maxlength="200" >
      </label>
      <div class="flex flex-wrap gap-2" role="group" aria-label="Filtrer par type">
        <button
          v-for="kind in ENTITY_KINDS"
          :key="kind"
          type="button"
          class="chip"
          :aria-pressed="active.has(kind)"
          :disabled="!counts[kind]"
          @click="toggle(kind)"
        >
          <span class="h-1.5 w-1.5 rounded-full" :style="{ backgroundColor: KINDS[kind].color }" aria-hidden="true" />
          {{ KINDS[kind].label }} <span class="text-muted">{{ counts[kind] }}</span>
        </button>
      </div>
    </div>

    <StatusNote v-if="status === 'error'" tone="error">Graphe indisponible : vérifier l'API et Neo4j.</StatusNote>
    <StatusNote v-else-if="!graph" tone="loading">Chargement des indicateurs…</StatusNote>
    <StatusNote v-else-if="!filtered.length" tone="empty">
      {{ rows.length ? "Aucun indicateur ne correspond aux filtres." : "Aucun indicateur extrait pour le moment." }}
    </StatusNote>

    <div v-else class="panel overflow-x-auto">
      <table class="w-full text-left text-sm">
        <caption class="sr-only">Indicateurs extraits, triés par nombre d'articles</caption>
        <thead class="border-b border-line text-[0.7rem] uppercase tracking-wider text-muted">
          <tr>
            <th scope="col" class="px-4 py-3 font-medium">Type</th>
            <th scope="col" class="px-4 py-3 font-medium">Valeur</th>
            <th scope="col" class="px-4 py-3 text-right font-medium">Articles</th>
            <th scope="col" class="px-4 py-3"><span class="sr-only">Actions</span></th>
          </tr>
        </thead>
        <tbody class="divide-y divide-line/60">
          <tr v-for="row in filtered.slice(0, shown)" :key="row.id" class="transition hover:bg-raised/50">
            <td class="whitespace-nowrap px-4 py-2.5"><KindBadge :kind="row.kind" /></td>
            <td class="max-w-[28rem] px-4 py-2.5">
              <code class="block truncate font-mono text-[0.82rem]" :title="row.label">{{ defang(row.kind, row.label) }}</code>
            </td>
            <td class="px-4 py-2.5 text-right font-mono tabular-nums text-muted">{{ row.mentions }}</td>
            <td class="px-4 py-2.5 text-right">
              <button type="button" class="btn-ghost px-2 py-1" @click="copy(row.label)">
                <AppIcon :name="copied === row.label ? 'check' : 'copy'" :size="14" />
                <span class="sr-only">Copier {{ row.label }}</span>
              </button>
            </td>
          </tr>
        </tbody>
      </table>
      <div class="flex items-center justify-between border-t border-line px-4 py-3 text-xs text-muted">
        <span>{{ Math.min(shown, filtered.length) }} / {{ filtered.length }}</span>
        <button v-if="shown < filtered.length" type="button" class="btn-ghost px-3 py-1" @click="shown += PAGE">Afficher plus</button>
      </div>
    </div>
    <p class="sr-only" aria-live="polite">{{ copied ? "Valeur copiée" : "" }}</p>
  </div>
</template>

<style scoped>
.chip {
  @apply inline-flex items-center gap-2 rounded-full border border-line px-3 py-1 text-sm text-fg transition hover:border-muted disabled:cursor-not-allowed disabled:opacity-40;
}
.chip[aria-pressed="true"] {
  @apply border-bleu bg-bleu/15;
}
</style>
