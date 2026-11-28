<script setup lang="ts">
import { ENTITY_KINDS, KINDS, countByKind, entityRows } from "~/utils/entities";

useHead({ title: "Vue d'ensemble" });

const { data: graph, status, refresh } = useGraph();
const rows = computed(() => entityRows(graph.value));
const counts = computed(() => countByKind(graph.value?.nodes ?? []));
const top = computed(() => rows.value.slice(0, 8));
const maxMentions = computed(() => Math.max(1, ...top.value.map((r) => r.mentions)));

const cards = [
  { to: "/menaces", icon: "list", title: "Menaces", text: "Documents jugés pertinents par le classifieur, du plus récent au plus ancien." },
  { to: "/ioc", icon: "target", title: "Indicateurs", text: "IP, domaines, empreintes et CVE extraits et vérifiés dans le texte source." },
  { to: "/datamap", icon: "graph", title: "DataMap", text: "Graphe des entités et des articles qui les mentionnent." },
] as const;
</script>

<template>
  <div class="space-y-12">
    <section class="grid gap-10 lg:grid-cols-[1.2fr_1fr] lg:items-end">
      <div class="animate-rise">
        <p class="eyebrow flex items-center gap-2">
          <span class="relative flex h-2 w-2" aria-hidden="true">
            <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-signal opacity-60" />
            <span class="relative inline-flex h-2 w-2 rounded-full bg-signal" />
          </span>
          Veille CTI · collecte continue
        </p>
        <h1 class="mt-4 text-4xl font-semibold leading-[1.05] sm:text-6xl">
          Voir la menace<br >
          <span class="text-muted">avant qu'elle</span> <span class="text-rouge">frappe.</span>
        </h1>
        <p class="mt-6 max-w-xl text-lg text-muted">
          Le crawler collecte, le classifieur trie, le parser extrait les entités.
          Chaque indicateur affiché ici est vérifié dans le texte de l'article qui le cite.
        </p>
        <div class="mt-8 flex flex-wrap gap-3">
          <NuxtLink to="/ioc" class="btn-primary">
            Explorer les indicateurs <AppIcon name="arrow" :size="16" />
          </NuxtLink>
          <NuxtLink to="/datamap" class="btn-ghost">Ouvrir la DataMap</NuxtLink>
        </div>
      </div>

      <div class="panel animate-rise p-5 [animation-delay:120ms]">
        <div class="flex items-center justify-between">
          <h2 class="text-sm font-medium">Entités projetées</h2>
          <button type="button" class="btn-ghost px-2 py-1" :disabled="status === 'pending'" @click="() => refresh()">
            <AppIcon name="refresh" :size="14" /><span class="sr-only">Actualiser</span>
          </button>
        </div>
        <p class="mt-1 text-xs text-muted">Sur les 200 articles les plus récents du graphe.</p>
        <dl class="mt-5 grid grid-cols-2 gap-px overflow-hidden rounded-lg border border-line bg-line sm:grid-cols-4">
          <div v-for="kind in ENTITY_KINDS" :key="kind" class="bg-surface p-3">
            <dt class="flex items-center gap-1.5 text-[0.7rem] uppercase tracking-wider text-muted">
              <span class="h-1.5 w-1.5 rounded-full" :style="{ backgroundColor: KINDS[kind].color }" aria-hidden="true" />
              {{ KINDS[kind].label }}
            </dt>
            <dd class="mt-1 font-display text-2xl tabular-nums">
              <span v-if="!graph && status !== 'error'" class="text-muted">—</span>
              <span v-else>{{ counts[kind] }}</span>
            </dd>
          </div>
          <div class="bg-surface p-3">
            <dt class="text-[0.7rem] uppercase tracking-wider text-muted">Articles</dt>
            <dd class="mt-1 font-display text-2xl tabular-nums">{{ graph ? counts.Article : "—" }}</dd>
          </div>
        </dl>
        <p v-if="status === 'error'" class="mt-4 text-sm text-rouge" role="alert">Graphe indisponible : vérifier l'API et Neo4j.</p>
      </div>
    </section>

    <section class="grid gap-6 lg:grid-cols-[1fr_1.2fr]">
      <div class="panel p-5">
        <h2 class="text-sm font-medium">Les plus cités</h2>
        <p class="mt-1 text-xs text-muted">Nombre d'articles mentionnant l'entité.</p>
        <ol v-if="top.length" class="mt-5 space-y-3">
          <li v-for="row in top" :key="row.id" class="grid grid-cols-[auto_1fr_auto] items-center gap-3">
            <KindBadge :kind="row.kind" />
            <div class="min-w-0">
              <p class="truncate font-mono text-sm" :title="row.label">{{ row.label }}</p>
              <div class="mt-1 h-1 rounded-full bg-line">
                <div
                  class="h-1 rounded-full"
                  :style="{ width: `${(row.mentions / maxMentions) * 100}%`, backgroundColor: KINDS[row.kind].color }"
                />
              </div>
            </div>
            <span class="font-mono text-sm tabular-nums text-muted">{{ row.mentions }}</span>
          </li>
        </ol>
        <p v-else class="mt-5 text-sm text-muted">{{ !graph && status !== "error" ? "Chargement…" : "Aucune entité pour le moment." }}</p>
      </div>

      <div class="grid gap-4 sm:grid-cols-3 lg:grid-cols-1">
        <NuxtLink
          v-for="(card, i) in cards"
          :key="card.to"
          :to="card.to"
          class="panel group flex animate-rise items-start gap-4 p-5 transition hover:border-muted/60 hover:bg-raised/80"
          :style="{ animationDelay: `${200 + i * 80}ms` }"
        >
          <span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg border border-line bg-ink text-bleu">
            <AppIcon :name="card.icon" />
          </span>
          <span>
            <span class="flex items-center gap-2 font-display text-lg">
              {{ card.title }}
              <AppIcon name="arrow" :size="16" class="-translate-x-1 opacity-0 transition group-hover:translate-x-0 group-hover:opacity-100" />
            </span>
            <span class="mt-1 block text-sm text-muted">{{ card.text }}</span>
          </span>
        </NuxtLink>
      </div>
    </section>
  </div>
</template>
