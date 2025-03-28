#!/bin/bash

# Couleurs pour les messages
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # Pas de couleur

# Variables
NAMESPACE="bluewhitethreat"
DOCKER_REGISTRY="localhost:5000"

# Fonction pour vérifier une commande
check_command() {
    if ! command -v "$1" &> /dev/null; then
        echo -e "${RED}Erreur : $1 n'est pas installé.${NC}"
        exit 1
    fi
}

# Étape 1 : Vérifier les prérequis
echo "Vérification des prérequis..."
check_command "docker"
check_command "kubectl"

# Étape 2 : Vérifier le fonctionnement de Docker
echo "Vérification du service Docker..."
if ! docker info &> /dev/null; then
    echo -e "${RED}Erreur : Docker ne fonctionne pas.${NC}"
    exit 1
fi
echo -e "${GREEN}Docker fonctionne correctement.${NC}"

# Étape 3 : Vérifier si Kubernetes est actif
echo "Vérification de Kubernetes..."
kubectl cluster-info &> /dev/null
if [ $? -ne 0 ]; then
    echo -e "${RED}Erreur : Kubernetes ne fonctionne.${NC}"
    exit 1
fi
echo -e "${GREEN}Kubernetes est actif.${NC}"

# Étape 4 : Construire les images Docker
echo "Construction des images Docker..."
for app in "crawler" "scraper"; do
    echo "Construction de $app..."
    docker build -f "../apps/$app/Dockerfile.dev" -t "$app:latest" "../apps/$app"
    if [ $? -ne 0 ]; then
        echo -e "${RED}Erreur : Échec de la construction de $app.${NC}"
        exit 1
    fi
    echo -e "${GREEN}Image $app construite avec succès.${NC}"
done

echo -e "${GREEN}Toutes les images ont été construites avec succès.${NC}"

# Étape 5 : Créer le namespace s'il n'existe pas
echo "Vérification du namespace $NAMESPACE..."
if ! kubectl get namespace "$NAMESPACE" &> /dev/null; then
    echo "Namespace $NAMESPACE non trouvé, création en cours..."
    kubectl create namespace "$NAMESPACE"
    echo -e "${GREEN}Namespace $NAMESPACE créé.${NC}"
else
    echo -e "${GREEN}Namespace $NAMESPACE déjà existant.${NC}"
fi

# Étape 6 : Appliquer les manifests Kubernetes
echo "Déploiement des manifests Kubernetes..."
for dir in "mongodb" "redis" "elasticsearch"; do
    if [ -d "../infra/k8s/$dir" ]; then
        echo "Déploiement de $dir..."
        kubectl apply -f "../infra/k8s/$dir/" --namespace="$NAMESPACE"
        if [ $? -ne 0 ]; then
            echo -e "${RED}Erreur : Échec du déploiement de $dir.${NC}"
            exit 1
        fi
        echo -e "${GREEN}$dir déployé avec succès.${NC}"
    else
        echo -e "${RED}Avertissement : Le dossier infra/k8s/$dir est introuvable. Skipping.${NC}"
    fi

done

# Étape 7 : Appliquer la NetworkPolicy
if [ -f "infra/network-policy.yaml" ]; then
    echo "Application de la NetworkPolicy..."
    kubectl apply -f "infra/network-policy.yaml" --namespace="$NAMESPACE"
    echo -e "${GREEN}NetworkPolicy appliquée.${NC}"
fi

# Étape 8 : Vérifier le déploiement
echo "Vérification des pods..."
sleep 10 # Attendre que les pods démarrent
kubectl get pods --namespace="$NAMESPACE"

# Étape 9 : Accéder au service Web
echo "Récupération de l’URL du service Web..."
if [ "$(uname)" == "Darwin" ]; then
    # macOS
    IP=$(kubectl get svc web-service -n "$NAMESPACE" -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')
else
    # Linux
    IP=$(kubectl get svc web-service -n "$NAMESPACE" -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
fi
echo "IP du service Web : $IP"
if [ -z "$IP" ]; then
    echo "Aucune IP externe détectée."
    echo "Utilisez : kubectl port-forward svc/web-service 8080:80 -n $NAMESPACE"
fi

echo -e "${GREEN}Déploiement terminé ! Accédez au site via l’URL ci-dessus.${NC}"

# Instructions finales
echo "Pour voir les logs :"
echo "  - Crawler : kubectl logs -l job-name=crawler-<hash> -n $NAMESPACE"
echo "  - Scraper : kubectl logs -l job-name=scraper-<hash> -n $NAMESPACE"
echo "Pour arrêter :"
echo "  docker stop $(docker ps -q)"
