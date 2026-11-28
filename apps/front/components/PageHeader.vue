<script setup lang="ts">
defineProps<{ eyebrow: string; title: string; lead?: string }>();
</script>

<template>
  <div class="flex flex-col gap-4 pb-8 sm:flex-row sm:items-end sm:justify-between">
    <div class="max-w-2xl animate-rise">
      <p class="eyebrow">{{ eyebrow }}</p>
      <h1 class="mt-2 text-3xl font-semibold sm:text-4xl">{{ title }}</h1>
      <p v-if="lead" class="mt-3 text-muted">{{ lead }}</p>
    </div>
    <div class="flex shrink-0 flex-wrap items-center gap-2">
      <slot name="actions" />
    </div>
  </div>
</template>
