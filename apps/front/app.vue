<template>
  <div class="flex min-h-dvh flex-col">
    <a href="#contenu" class="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-50 btn-primary">
      Aller au contenu
    </a>
    <HeaderBar />
    <main id="contenu" class="mx-auto w-full max-w-7xl flex-1 px-4 py-10 sm:px-6">
      <NuxtPage />
    </main>
    <FooterBar />
  </div>
</template>
