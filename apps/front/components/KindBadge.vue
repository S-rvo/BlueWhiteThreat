<script setup lang="ts">
import { KINDS, type EntityKind } from "~/utils/entities";

const props = defineProps<{ kind: EntityKind }>();
const meta = computed(() => KINDS[props.kind]);
</script>

<template>
  <span class="inline-flex items-center gap-1.5 rounded-md border border-line bg-ink/60 px-2 py-0.5 font-mono text-[0.7rem] uppercase tracking-wider text-muted">
    <span class="kind-dot h-1.5 w-1.5 rounded-full" :style="{ backgroundColor: meta.color }" aria-hidden="true" />
    {{ meta.label }}
  </span>
</template>
