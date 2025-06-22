<template>
    <div class="articles-cards">
        <div
            v-for="article in articles"
            :key="article.id"
            class="card"
        >
            <img :src="article.image" alt="Article image" class="card-image" />
            <div class="card-content">
                <h2 class="card-title">{{ article.title }}</h2>
                <p class="card-description">{{ article.description }}</p>
                <a :href="article.link" class="card-link">Lire l'article</a>
            </div>
        </div>
    </div>
</template>

<script setup>
import { ref } from 'vue'

const articles = ref([
    {
        id: 1,
        title: 'Premier Article',
        description: 'Ceci est la description du premier article.',
        image: 'https://via.placeholder.com/300x180',
        link: '#'
    },
    {
        id: 2,
        title: 'Deuxième Article',
        description: 'Voici un aperçu du deuxième article.',
        image: 'https://via.placeholder.com/300x180',
        link: '#'
    },
    {
        id: 3,
        title: 'Troisième Article',
        description: 'Résumé du troisième article présenté ici.',
        image: 'https://via.placeholder.com/300x180',
        link: '#'
    },
    {
        id: 4,
        title: '4 Article',
        description: 'Résumé du troisième article présenté ici.',
        image: 'https://via.placeholder.com/300x180',
        link: '#'
    },
    {
        id: 4,
        title: '4 Article',
        description: 'Résumé du troisième article présenté ici.',
        image: 'https://via.placeholder.com/300x180',
        link: '#'
    },
        {
        id: 4,
        title: '4 Article',
        description: 'Résumé du troisième article présenté ici.',
        image: 'https://via.placeholder.com/300x180',
        link: '#'
    },
    {
        id: 4,
        title: '4 Article',
        description: 'Résumé du troisième article présenté ici.',
        image: 'https://via.placeholder.com/300x180',
        link: '#'
    }
])
</script>

<style scoped>
.articles-cards {
    display: flex;
    gap: 1.5rem;
    flex-wrap: wrap;
    justify-content: center;
}

.card {
    background: #1b1a1a;
    border-radius: 12px;
    box-shadow: 0 2px 8px rgba(0,0,0,0.08);
    overflow: hidden;
    width: 400px;
    height: 250px;
    display: flex;
    flex-direction: column;
    transition: box-shadow 0.2s;
}

.card:hover {
    box-shadow: 0 4px 16px rgba(0,0,0,0.15);
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
    margin: 0 0 0.5rem 0;
    color: white;
}

.card-description {
    flex: 1;
    color: #47e0ff;
    margin-bottom: 1rem;
}

.card-link {
    color: #ffffff;
    text-decoration: none;
    font-weight: bold;
    align-self: flex-start;
}

.card-link:hover {
    text-decoration: underline;
}
</style>