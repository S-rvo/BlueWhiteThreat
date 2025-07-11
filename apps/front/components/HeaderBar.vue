<template>
  <header
    class="bg-black text-white px-8 py-4 flex items-center shadow-lg z-10"
  >
    <NuxtLink to="/" class="flex items-center gap-2">
      <span class="text-2xl font-extrabold tracking-tight flex items-center">
        <span class="text-blue-500">Blue</span>
        <span class="text-white">White</span>
        <span class="text-red-500">Threat</span>
        <span
          class="ml-2 w-5 h-5 rounded-full border-2 border-white flex items-center justify-center bg-gradient-to-r from-blue-500 via-white to-red-500"
        ></span>
      </span>
    </NuxtLink>
    <nav class="ml-auto flex gap-8">
      <NuxtLink to="/" class="hover:text-blue-500 font-semibold transition"
        >Accueil</NuxtLink
      >
      <NuxtLink
        to="/menaces"
        class="hover:text-red-500 font-semibold transition"
        >Menaces</NuxtLink
      >
      <NuxtLink to="/ioc" class="hover:text-white font-semibold transition"
        >IOC</NuxtLink
      >
      <NuxtLink
        to="http://localhost:7474/"
        class="hover:text-blue-500 font-semibold transition"
        >DataMap</NuxtLink
      >
    </nav>
  </header>
</template>
