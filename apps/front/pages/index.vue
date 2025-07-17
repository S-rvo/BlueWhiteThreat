<script setup lang="ts"></script>

<template>
  <div class="min-h-screen bg-[#101014] flex flex-col">
    <main class="flex-1 flex flex-col items-center justify-center px-4">
      <div class="text-center max-w-2xl">
        <h1 class="text-5xl font-extrabold mb-6 tracking-tight">
          <span class="text-blue-500">Blue</span>
          <span class="text-white">White</span>
          <span class="text-red-500">Threat</span>
        </h1>
        <p class="text-lg text-gray-300 mb-8">
          Projet académique de Cyber Threat Intelligence :<br />
          <span class="text-white font-semibold">Crawler Go</span> pour
          collecter, analyser et visualiser les menaces du darkweb.<br />
          Interface web moderne pour explorer facilement les informations
          extraites.
        </p>
        <NuxtLink
          to="/Menaces"
          class="inline-block px-8 py-3 rounded-lg bg-gradient-to-r from-blue-500 via-white to-red-500 text-black font-bold shadow-lg hover:scale-105 transition"
        >
          Explorer les Menaces
        </NuxtLink>
      </div>
    </main>
  </div>
</template>

<style scoped>
/* Ajoute un léger effet de glow sur le titre */
h1 span {
  text-shadow: 0 0 8px #2563eb33, 0 0 2px #ef444433;
}
</style>
