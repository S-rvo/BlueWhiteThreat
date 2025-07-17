<template>
  <div class="articles-cards">
    <div v-for="article in articles" :key="article.id" class="card group">
      <div class="card-content">
        <h2 class="card-title">
          <span>{{ article.title }}</span>
        </h2>
        <p class="card-description">{{ article.description }}</p>
        <a :href="article.link" class="card-link">Lire l'article</a>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from "vue";

const articles = ref([]);

const fetchArticles = async () => {
  try {
    const response = await fetch("http://localhost:8083/data");
    if (!response.ok)
      throw new Error("Erreur lors de la récupération des articles");
    console.log(response);
    const data = await response.json();
    articles.value = data;
  } catch (error) {
    console.error(error);
  }
};

onMounted(fetchArticles);
</script>

<style scoped>
.articles-cards {
  display: flex;
  gap: 2rem;
  flex-wrap: wrap;
  justify-content: center;
  padding: 1rem;
}

.card {
  background: #181818;
  border-radius: 16px;
  border: 2px solid #222;
  box-shadow: 0 4px 24px 0 #000a;
  overflow: hidden;
  width: 100%;
  max-width: 400px;
  display: flex;
  flex-direction: column;
  transition: box-shadow 0.2s, border 0.2s, transform 0.2s;
}
.card:hover {
  box-shadow: 0 8px 32px 0 #2563eb55, 0 0 0 4px #ef444455;
  transform: translateY(-4px) scale(1.03);
}
.card-image {
  width: 100%;
  height: 180px;
  object-fit: cover;
}
.card-content {
  padding: 1rem;
  flex: 1;
  display: flex;
  flex-direction: column;
}
.card-title {
  font-size: 1.25rem;
  font-weight: bold;
  margin: 0 0 0.5rem 0;
  color: white;
  letter-spacing: 1px;
}
.card-title span {
  text-shadow: 0 0 6px #2563eb33, 0 0 2px #ef444433;
}
.card-description {
  flex: 1;
  color: #47e0ff;
  margin-bottom: 1rem;
}
.card-link {
  background: white;
  color: #101014;
  padding: 0.5rem 1.5rem;
  border-radius: 8px;
  font-weight: bold;
  text-decoration: none;
  align-self: flex-start;
  transition: background 0.2s, color 0.2s;
}
.card-link:hover {
  background: #2563eb;
  color: #fff;
}
</style>
