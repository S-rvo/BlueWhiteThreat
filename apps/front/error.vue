<script setup lang="ts">
import type { NuxtError } from "#app";

const props = defineProps<{ error: NuxtError }>();
const notFound = computed(() => props.error.statusCode === 404);
useHead({ title: notFound.value ? "Page introuvable" : "Erreur" });
</script>

<template>
  <div class="flex min-h-screen flex-col">
    <HeaderBar />
    <main id="contenu" class="mx-auto grid w-full max-w-7xl flex-1 place-items-center px-4 py-16 sm:px-6">
      <div class="text-center">
        <p class="font-mono text-sm text-rouge">{{ error.statusCode }}</p>
        <h1 class="mt-3 text-4xl font-semibold">{{ notFound ? "Page introuvable" : "Une erreur est survenue" }}</h1>
        <p class="mt-3 text-muted">{{ notFound ? "Cette adresse ne correspond à aucune page." : "Réessayez dans un instant." }}</p>
        <button type="button" class="btn-primary mt-8" @click="clearError({ redirect: '/' })">Retour à l'accueil</button>
      </div>
    </main>
    <FooterBar />
  </div>
</template>
