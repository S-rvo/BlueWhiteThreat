#!/bin/bash

echo "🛑 Arrêt de l'application Vue.js Target en PRODUCTION..."

CONTAINER_NAME="vue-target-container"
IMAGE_NAME="vue-target-prod"

# Arrêter le conteneur
echo "⏹️  Arrêt du conteneur..."
docker stop $CONTAINER_NAME 2>/dev/null || echo "Conteneur déjà arrêté"

# Supprimer le conteneur
echo "🗑️  Suppression du conteneur..."
docker rm $CONTAINER_NAME 2>/dev/null || echo "Conteneur déjà supprimé"

# Supprimer l'image (optionnel)
read -p "Voulez-vous aussi supprimer l'image ? (y/N): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "🗑️  Suppression de l'image..."
    docker rmi $IMAGE_NAME 2>/dev/null || echo "Image déjà supprimée"
fi

echo "✅ Application arrêtée avec succès !" 