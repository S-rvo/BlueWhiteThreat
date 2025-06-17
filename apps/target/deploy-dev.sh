#!/bin/bash

echo "🚀 Déploiement de l'application Vue.js Target en mode développement..."

# Vérifier si Docker est installé
if ! command -v docker &> /dev/null; then
    echo "❌ Docker n'est pas installé. Veuillez installer Docker d'abord."
    exit 1
fi

# Vérifier si Docker Compose est installé
if ! command -v docker-compose &> /dev/null; then
    echo "❌ Docker Compose n'est pas installé. Veuillez installer Docker Compose d'abord."
    exit 1
fi

# Arrêter les conteneurs existants
echo "🛑 Arrêt des conteneurs existants..."
docker-compose -f docker-compose.dev.yml down

# Construire l'image
echo "🔨 Construction de l'image Docker..."
docker-compose -f docker-compose.dev.yml build

# Démarrer les services
echo "▶️  Démarrage des services..."
docker-compose -f docker-compose.dev.yml up -d

# Attendre que le service soit prêt
echo "⏳ Attente du démarrage du service..."
sleep 5

# Vérifier le statut
if docker-compose -f docker-compose.dev.yml ps | grep -q "Up"; then
    echo "✅ Application déployée avec succès !"
    echo "🌐 Votre application est accessible sur : http://localhost:5173"
    echo ""
    echo "📋 Commandes utiles :"
    echo "   - Voir les logs : docker-compose -f docker-compose.dev.yml logs -f"
    echo "   - Arrêter : docker-compose -f docker-compose.dev.yml down"
    echo "   - Redémarrer : docker-compose -f docker-compose.dev.yml restart"
else
    echo "❌ Erreur lors du déploiement. Vérifiez les logs :"
    docker-compose -f docker-compose.dev.yml logs
fi 