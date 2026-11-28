<script setup lang="ts">
const links = [
  { to: "/", label: "Vue d'ensemble", icon: "radar" },
  { to: "/menaces", label: "Menaces", icon: "list" },
  { to: "/ioc", label: "IOC", icon: "target" },
  { to: "/veille", label: "Ma veille", icon: "eye" },
  { to: "/datamap", label: "DataMap", icon: "graph" },
] as const;

const open = ref(false);
const route = useRoute();
watch(() => route.path, () => (open.value = false));
</script>

<template>
  <header class="sticky top-0 z-30 border-b border-line/70 bg-ink/80 backdrop-blur-md">
    <div class="mx-auto flex h-16 max-w-7xl items-center gap-6 px-4 sm:px-6">
      <NuxtLink to="/" class="shrink-0" aria-label="BlueWhiteThreat, accueil">
        <BrandMark />
      </NuxtLink>

      <nav aria-label="Navigation principale" class="ml-auto hidden md:block">
        <ul class="flex items-center gap-1">
          <li v-for="link in links" :key="link.to">
            <NuxtLink
              :to="link.to"
              class="nav-link flex items-center gap-2 rounded-lg px-3 py-2 text-sm text-muted transition hover:bg-raised hover:text-fg"
              exact-active-class="is-active"
            >
              <AppIcon :name="link.icon" :size="16" />
              {{ link.label }}
            </NuxtLink>
          </li>
        </ul>
      </nav>

      <button
        type="button"
        class="btn-ghost ml-auto px-2.5 md:hidden"
        :aria-expanded="open"
        aria-controls="mobile-nav"
        @click="open = !open"
      >
        <AppIcon :name="open ? 'x' : 'menu'" />
        <span class="sr-only">Menu</span>
      </button>
    </div>

    <nav v-show="open" id="mobile-nav" aria-label="Navigation principale" class="border-t border-line/70 px-4 pb-4 md:hidden">
      <ul class="grid gap-1 pt-3">
        <li v-for="link in links" :key="link.to">
          <NuxtLink
            :to="link.to"
            class="nav-link flex items-center gap-3 rounded-lg px-3 py-2.5 text-muted hover:bg-raised hover:text-fg"
            exact-active-class="is-active"
          >
            <AppIcon :name="link.icon" />
            {{ link.label }}
          </NuxtLink>
        </li>
      </ul>
    </nav>
  </header>
</template>

<style scoped>
.nav-link.is-active {
  color: rgb(var(--fg));
  background: rgb(var(--raised));
  box-shadow: inset 0 -2px 0 rgb(var(--bleu));
}
</style>
