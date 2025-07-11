<template>
  <div class="app-container">
    <HeaderBar />
    <main class="main-content">
      <NuxtPage />
    </main>
    <FooterBar />
  </div>
</template>

<script setup>
import HeaderBar from "~/components/HeaderBar.vue";
import FooterBar from "~/components/FooterBar.vue";
</script>

<style scoped>
.app-container {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background-color: #101014;
}

.main-content {
  flex: 1 1 auto;
  min-height: 0;
  /* Pour forcer la hauteur exacte : */
  height: calc(
    100vh - 64px - 56px
  ); /* Adapter les valeurs à la hauteur réelle du header/footer */
  overflow-y: auto;
}

/* Si besoin, tu peux ajuster les hauteurs ci-dessus selon la taille réelle de tes barres */
</style>
