<script setup lang="ts">
import "vis-network/styles/vis-network.min.css";
import { ENTITY_KINDS, KINDS, countByKind, type EntityKind, type Graph } from "~/utils/entities";
import type { Network } from "vis-network/peer/esm/vis-network";

useHead({ title: "DataMap" });

const { data: graph, status, refresh } = useGraph();
const container = ref<HTMLElement | null>(null);
const hidden = ref<Set<EntityKind>>(new Set());
const selected = ref<{ kind: EntityKind; label: string; degree: number } | null>(null);
const counts = computed(() => countByKind(graph.value?.nodes ?? []));
let network: Network | null = null;

function label(kind: EntityKind, text: string) {
  if (kind === "Article") return `article ${text.slice(-6)}`;
  return text.length > 28 ? `${text.slice(0, 27)}…` : text;
}

async function render(g: Graph | null | undefined) {
  if (!container.value || !g) return;
  // Import différé : vis-network (~500 ko) n'est chargé que sur cette page.
  const [{ Network }, { DataSet }] = await Promise.all([
    import("vis-network/peer/esm/vis-network"),
    import("vis-data/peer/esm/vis-data"),
  ]);
  const visible = g.nodes.filter((n) => n.kind === "Article" || !hidden.value.has(n.kind));
  const ids = new Set(visible.map((n) => n.id));
  const degree = new Map<string, number>();
  for (const e of g.edges) {
    degree.set(e.from, (degree.get(e.from) ?? 0) + 1);
    degree.set(e.to, (degree.get(e.to) ?? 0) + 1);
  }
  const nodes = new DataSet(
    visible.map((n) => ({
      id: n.id,
      label: label(n.kind, n.label),
      shape: "dot",
      size: n.kind === "Article" ? 6 : 8 + Math.min(14, (degree.get(n.id) ?? 1) * 2),
      color: {
        background: KINDS[n.kind].color,
        border: KINDS[n.kind].color,
        highlight: { background: "#ffffff", border: KINDS[n.kind].color },
        hover: { background: KINDS[n.kind].color, border: "#ffffff" },
      },
      font: { color: n.kind === "Article" ? "#5f6b82" : "#e8ecf3", size: n.kind === "Article" ? 9 : 12, face: "IBM Plex Mono, monospace" },
    })),
  );
  const edges = new DataSet(
    g.edges
      .filter((e) => ids.has(e.from) && ids.has(e.to))
      .map((e, i) => ({ id: i, from: e.from, to: e.to })),
  );

  network?.destroy();
  network = new Network(
    container.value,
    { nodes, edges },
    {
      autoResize: true,
      interaction: { hover: true, tooltipDelay: 120, navigationButtons: false, keyboard: false },
      edges: { color: { color: "rgba(139,151,173,0.22)", highlight: "#4880ff", hover: "#8b97ad" }, width: 1, smooth: false },
      physics: {
        solver: "forceAtlas2Based",
        forceAtlas2Based: { gravitationalConstant: -40, springLength: 80, avoidOverlap: 0.4 },
        stabilization: { iterations: 200, updateInterval: 50 },
      },
    },
  );
  network.once("stabilizationIterationsDone", () => {
    network?.setOptions({ physics: false });
    network?.fit({ animation: { duration: 400, easingFunction: "easeInOutQuad" } });
  });
  network.on("click", (params: { nodes: string[] }) => {
    const id = params.nodes[0];
    const node = id ? g.nodes.find((n) => n.id === id) : undefined;
    selected.value = node ? { kind: node.kind, label: node.label, degree: degree.get(node.id) ?? 0 } : null;
  });
}

function toggle(kind: EntityKind) {
  const next = new Set(hidden.value);
  if (next.has(kind)) next.delete(kind);
  else next.add(kind);
  hidden.value = next;
}

const fit = () => network?.fit({ animation: { duration: 400, easingFunction: "easeInOutQuad" } });

onMounted(() => {
  watch([graph, hidden], () => render(graph.value), { immediate: true });
});
onBeforeUnmount(() => {
  network?.destroy();
  network = null;
});
</script>

<template>
  <div>
    <PageHeader
      eyebrow="Graphe d'entités"
      title="DataMap"
      lead="Chaque entité est reliée aux articles qui la citent. Cliquez un nœud pour le détail, filtrez par type dans la légende."
    >
      <template #actions>
        <button type="button" class="btn-ghost" @click="fit"><AppIcon name="fit" :size="16" /> Recadrer</button>
        <button type="button" class="btn-ghost" :disabled="status === 'pending'" @click="() => refresh()">
          <AppIcon name="refresh" :size="16" /> Actualiser
        </button>
      </template>
    </PageHeader>

    <div class="grid gap-4 lg:grid-cols-[1fr_16rem]">
      <div class="panel relative h-[65vh] min-h-[420px] overflow-hidden">
        <div ref="container" class="absolute inset-0" role="img" aria-label="Graphe des entités et des articles qui les mentionnent" />
        <div v-if="!graph && status !== 'error'" class="absolute inset-0 grid place-items-center text-sm text-muted">
          <StatusNote tone="loading">Chargement du graphe…</StatusNote>
        </div>
        <div v-else-if="status === 'error'" class="absolute inset-0 grid place-items-center p-6">
          <StatusNote tone="error">Graphe indisponible : vérifier l'API et Neo4j.</StatusNote>
        </div>
        <div v-else-if="graph && !graph.nodes.length" class="absolute inset-0 grid place-items-center p-6">
          <StatusNote tone="empty">Aucune entité projetée pour le moment.</StatusNote>
        </div>
      </div>

      <aside class="space-y-4">
        <div class="panel p-4">
          <h2 class="text-sm font-medium">Légende</h2>
          <ul class="mt-3 space-y-1">
            <li v-for="kind in ENTITY_KINDS" :key="kind">
              <button
                type="button"
                class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm transition hover:bg-raised"
                :class="hidden.has(kind) ? 'opacity-40' : ''"
                :aria-pressed="!hidden.has(kind)"
                @click="toggle(kind)"
              >
                <span class="h-2.5 w-2.5 rounded-full" :style="{ backgroundColor: KINDS[kind].color }" aria-hidden="true" />
                {{ KINDS[kind].plural }}
                <span class="ml-auto font-mono text-xs tabular-nums text-muted">{{ counts[kind] }}</span>
              </button>
            </li>
            <li class="flex items-center gap-2 px-2 py-1.5 text-sm text-muted">
              <span class="h-2 w-2 rounded-full" :style="{ backgroundColor: KINDS.Article.color }" aria-hidden="true" />
              Articles
              <span class="ml-auto font-mono text-xs tabular-nums">{{ counts.Article }}</span>
            </li>
          </ul>
        </div>
        <div class="panel p-4" aria-live="polite">
          <h2 class="text-sm font-medium">Sélection</h2>
          <div v-if="selected" class="mt-3 space-y-2">
            <KindBadge :kind="selected.kind" />
            <p class="break-all font-mono text-sm">{{ selected.label }}</p>
            <p class="text-xs text-muted">{{ selected.degree }} lien(s) dans ce graphe</p>
          </div>
          <p v-else class="mt-3 text-xs text-muted">Aucun nœud sélectionné.</p>
        </div>
      </aside>
    </div>
  </div>
</template>
