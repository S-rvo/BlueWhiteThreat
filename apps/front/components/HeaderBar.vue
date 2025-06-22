<template>
  <header class="bg-black text-white p-4 flex">
  <h1 class="text-xl font-bold flex-1">
    <span class="text-blue-500">Blue</span>
    <span class="text-white">White</span>
    <span class="text-red-500">Threat</span>
  </h1>
  <NuxtLink to="http://localhost:7474/" class="datamap text-white-400 flex-1 hover:text-blue-500">DataMap</NuxtLink>
  <NuxtLink to="ioc" class="ioc text-white-400 flex-1 hover:text-white-500 neon-hover">IOC</NuxtLink>
  <NuxtLink to="Menaces" class="Menaces text-white-400 flex-1 hover:text-red-500">Menaces</NuxtLink>
  </header>
</template>
