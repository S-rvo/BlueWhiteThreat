<script setup lang="ts">
// État de chargement, d'erreur ou de vide, annoncé aux lecteurs d'écran.
defineProps<{ tone?: "loading" | "error" | "empty" }>();
</script>

<template>
  <div
    class="panel relative overflow-hidden px-5 py-8 text-center text-sm"
    :class="tone === 'error' ? 'border-rouge/40 text-rouge' : 'text-muted'"
    :role="tone === 'error' ? 'alert' : 'status'"
  >
    <span v-if="tone === 'loading'" class="absolute inset-x-0 top-0 h-px overflow-hidden" aria-hidden="true">
      <span class="block h-px w-1/2 animate-scan bg-bleu" />
    </span>
    <slot />
  </div>
</template>
