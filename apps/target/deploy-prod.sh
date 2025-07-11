#!/bin/bash

echo "🚀 Déploiement de l'application Vue.js Target en PRODUCTION..."

# Vérifier si Docker est installé
if ! command -v docker &> /dev/null; then
    echo "❌ Docker n'est pas installé. Veuillez installer Docker d'abord."
    exit 1
fi

# Nom de l'image et du conteneur
IMAGE_NAME="vue-target-prod"
CONTAINER_NAME="vue-target-container"
PORT="80"

# Arrêter et supprimer le conteneur existant
echo "🛑 Arrêt du conteneur existant..."
docker stop $CONTAINER_NAME 2>/dev/null || true
docker rm $CONTAINER_NAME 2>/dev/null || true

# Supprimer l'ancienne image
echo "🧹 Suppression de l'ancienne image..."
docker rmi $IMAGE_NAME 2>/dev/null || true

# Construire l'image de production
echo "🔨 Construction de l'image Docker de production..."
docker build -f apps/target/dockerfile.prod -t $IMAGE_NAME .

# Vérifier si la construction a réussi
if [ $? -ne 0 ]; then
    echo "❌ Erreur lors de la construction de l'image"
    exit 1
fi

# Démarrer le conteneur
echo "▶️  Démarrage du conteneur de production..."
docker run -d \
  --name $CONTAINER_NAME \
  -p $PORT:80 \
  --restart unless-stopped \
  $IMAGE_NAME

# Vérifier si le conteneur a démarré
if [ $? -eq 0 ]; then
    echo "⏳ Attente du démarrage du service..."
    sleep 5
    
    # Vérifier le statut du conteneur
    if docker ps | grep -q $CONTAINER_NAME; then
        echo "✅ Application déployée avec succès en PRODUCTION !"
        echo "🌐 Votre application est accessible sur : http://localhost"
        echo "🏥 Health check : http://localhost/health"
        echo ""
        echo "📋 Commandes utiles :"
        echo "   - Voir les logs : docker logs -f $CONTAINER_NAME"
        echo "   - Arrêter : docker stop $CONTAINER_NAME"
        echo "   - Redémarrer : docker restart $CONTAINER_NAME"
        echo "   - Voir les stats : docker stats $CONTAINER_NAME"
        echo "   - Accéder au conteneur : docker exec -it $CONTAINER_NAME sh"
        echo ""
        echo "🔒 Configuration de production :"
        echo "   - Nginx avec compression gzip"
        echo "   - Headers de sécurité"
        echo "   - Cache optimisé"
        echo "   - Health checks intégrés"
        echo "   - Utilisateur non-root"
        echo "   - Redémarrage automatique"
    else
        echo "❌ Le conteneur n'a pas démarré correctement"
        docker logs $CONTAINER_NAME
    fi
else
    echo "❌ Erreur lors du démarrage du conteneur"
    exit 1
fi 