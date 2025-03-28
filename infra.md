Architecture Fonctionnelle

```
┌─────────────────────────────┐
│ Sources Darkweb (.onion)    │
│ - Forums                    │
│ - Marketplaces clandestins  │
│ - Canaux Telegram chiffrés  │
│ - Pastebins anonymes        │
└───────────────┬─────────────┘
                │
                │ Accès via TOR/I2P/VPN
                │
                ▼
┌─────────────────────────────┐
│ Queue Redis                 │
│ (URLs prioritaires à crawler)│
└───────────────┬─────────────┘
                │
                │ Pop URL prioritaire
                │
                ▼
┌─────────────────────────────┐
│ Crawler Darkweb Sécurisé    │
│ - Rotation d'IPs            │
│ - Délais aléatoires         │
│ - Télécharge le contenu HTML│
│ - Extrait nouvelles URLs    │
└───────────────┬─────────────┘
                │
                │ Publie HTML et métadonnées
                ▼
┌─────────────────────────────┐
│ RabbitMQ / Kafka            │
│ (File de pages à analyser)  │
│ - Persistant                │
│ - Hautement disponible      │
└───────────────┬─────────────┘
                │
                │ Consomme les messages à son rythme
                ▼
┌─────────────────────────────┐
│ Filter/Analyseur CTI        │
│ - Extrait données structurées│
│ - Détecte les IOCs          │
│ - Identifie signaux de menace│
│ - Évalue pertinence         │
└──────┬────────────┬─────────┘
       │            │
       │            │ Nouvelles URLs
       │            ▼
       │    ┌───────────────────┐
       │    │ Redis Queue       │
       │    │ (Nouvelles URLs   │
       │    │  prioritaires)    │
       │    └───────────────────┘
       │
       │ Données pertinentes
       ▼
┌─────────────────────────────┐
│ Storage Layer               │
├─────────────────────────────┤
│ MongoDB        Elasticsearch│
│ (Stockage CTI) (Recherche)  │
└───────────────┬─────────────┘
                │
                │ API d'accès
                │
                ▼
┌─────────────────────────────┐
│ Backend API                 │
│ - Endpoints de recherche    │
│ - Filtrage avancé           │
└───────────────┬─────────────┘
                │
                │
                ▼
┌─────────────────────────────┐
│ Frontend / Dashboards CTI   │
│ - Visualisations            │
│ - Alertes                   │
│ - Rapports                  │
└─────────────────────────────┘
```

Flux de données détaillé

1.  Initialisation de la file d'attente

    Les URLs seeds sont chargées dans la file d'attente prioritaire Redis
    Format: ZADD "crawler:url_queue" [score] [url_data_json]
    Les scores plus élevés indiquent une priorité plus grande

2.  Extraction d'URL

    Le crawler vérifie si le taux de requête pour le domaine respecte les limites
    Vérification si l'URL a déjà été visitée: SISMEMBER "crawler:visited_urls" [url]
    Extraction de l'URL prioritaire: ZPOPMAX "crawler:url_queue" 1
    Ajout de l'URL aux URLs visitées: SADD "crawler:visited_urls" [url]

3.  Récupération de la page web

    Le crawler établit une connexion via TOR/I2P/VPN pour l'anonymat
    Il envoie une requête HTTP GET avec des en-têtes aléatoires et user-agents variés
    Gère les redirections, erreurs HTTP et timeouts
    Respecte un délai aléatoire entre les requêtes pour éviter la détection

4.  Publication dans la file d'attente RabbitMQ

    Le crawler, au lieu d'envoyer directement le contenu aux filters:
    Crée un message contenant HTML, URL et métadonnées
    Publie dans RabbitMQ avec des propriétés de persistance
    Retourne immédiatement chercher une nouvelle URL
    Format:

        {
            "url": "http://darkforum.onion/thread/12345",
            "html_content": "<!DOCTYPE html><html>...",
            "headers": {"content-type": "text/html", ...},
            "crawl_time": "2023-10-15T14:32:55Z",
            "metadata": {
            "ip": "unknown (Tor)",
            "status_code": 200,
            "response_time_ms": 1243
            }
        }

5.  Consommation et traitement par les filters

    Les filters consomment les messages à leur propre rythme:
    Chaque filter s'abonne à la file RabbitMQ
    Traite le contenu HTML et extrait les données importantes
    Confirme la réception uniquement après traitement réussi
    En cas d'erreur, le message est remis en file ou envoyé dans une file d'erreur

6.  Stockage des données pertinentes

    Les données pertinentes sont formatées en documents MongoDB
    Insertion ou mise à jour: db.ctidata.updateOne({url: [url]}, {$set: [data]}, {upsert: true})
    Les données sont également indexées dans Elasticsearch pour des recherches avancées
    Indexation: PUT /ctidata/\_doc/[url_encoded] [json_data]

7.  Mise à jour de la file d'attente

    Les nouvelles URLs découvertes sont vérifiées:
    Contre le set d'URLs déjà visitées
    Pour leur pertinence CTI potentielle
    Les URLs pertinentes sont ajoutées à la file prioritaire:
    ZADD "crawler:url_queue" [score] [url_data_json]

8.  Accès aux données via API

    Le backend fournit des endpoints RESTful pour accéder aux données
    Fonctionnalités de recherche avancée via Elasticsearch
    Filtrage multi-critères (date, source, type d'IoC, score de pertinence)
    Format de réponse structuré et normalisé

9.  Visualisation et exploitation

    Le frontend présente les données CTI de manière exploitable pour les analystes
    Tableaux de bord interactifs pour le suivi des menaces
    Visualisations des relations entre différents IoCs
    Système d'alertes pour les menaces prioritaires

### Structure des données

Document CTI dans MongoDB

```
{
    "_id": "ObjectId()",
    "url": "http://darkforum.onion/thread/12345",
    "source_type": "forum",
    "crawl_time": "2023-10-15T14:32:55Z",
    "title": "New Zero-Day Vulnerability in Popular Framework",
    "content": "We have discovered a critical vulnerability...",
    "extracted_iocs": {
        "ip_addresses": ["192.168.1.1", "10.0.0.23"],
        "domains": ["malicious-domain.com"],
        "hashes": {
            "md5": ["d41d8cd98f00b204e9800998ecf8427e"],
            "sha256": ["e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"]
        },
        "cves": ["CVE-2023-1234"]
    },
    "relevance_score": 0.87,
    "language": "en",
    "processed": true,
    "references": ["http://darkforum.onion/thread/12340"]
}
```

Format d'URL dans la file Redis

```
{
    "url": "http://darkforum.onion/section/exploits",
    "depth": 2,
    "referrer": "http://darkforum.onion/main",
    "priority_factors": {
        "keyword_match": 0.7,
        "domain_priority": 0.8,
        "referrer_relevance": 0.5
    },
    "crawl_after": "2023-10-15T14:32:55Z"
}
```

### Optimisation des performances

Stratégies de crawl intelligent

    Crawling adaptatif basé sur la fraîcheur et pertinence du contenu
    Ajustement dynamique des délais entre les requêtes selon la réponse du serveur
    Alternance entre différents circuits TOR pour éviter les blocages
    Politesse du crawler adaptée à chaque source

Gestion des ressources

    Limitation du nombre de connexions simultanées par domaine
    Pool de connexions TOR avec rotation automatique
    Mise en cache des résultats fréquemment demandés
    Distribution des charges sur plusieurs instances de crawler en fonction du trafic

Haute disponibilité

    Déploiement en Kubernetes avec auto-scaling
    Persistance des files d'attente Redis pour reprendre après un arrêt
    Réplication MongoDB pour la sauvegarde des données
    Circuit-breakers pour isoler les composants défaillants

### Sécurité et conformité

Protection de l'infrastructure

    Isolation réseau complète des nœuds de crawling
    Utilisation exclusive de TOR/VPN pour toutes les connexions sortantes
    Rotation régulière des identités réseau
    Monitoring des tentatives de détection de crawling

Anonymat et discrétion

    Empreinte digitale du navigateur aléatoire pour chaque requête
    Délais variables entre les requêtes pour simuler un comportement humain
    En-têtes HTTP randomisés
    User-agents diversifiés et mis à jour régulièrement

### Extensions futures

Intelligence artificielle

    Classification automatique des menaces par ML
    Extraction d'entités nommées pour identifier de nouveaux acteurs
    Analyse de sentiment pour évaluer l'intention des acteurs malveillants
    Prédiction de tendances de menaces basée sur l'évolution des discussions

Intégration externe

    Connecteurs vers des plateformes SIEM
    API pour l'intégration avec des outils SOAR
    Enrichissement automatique avec des sources OSINT
    Export au format STIX/TAXII pour le partage standardisé de CTI

### Implémentation dans Kubernetes

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                              Kubernetes Cluster                                     │
│                                                                                     │
│  ┌─────────────────────────────────────────────────────────────────────────────┐    │
│  │                        Namespace: cti-darkweb                               │    │
│  │                                                                             │    │
│  │                           Internet / Dark Web                               │    │
│  │                                 │                                           │    │
│  │                                 │                                           │    │
│  │                                 ▼                                           │    │
│  │  ┌────────────────────┐     ┌────────────────────┐                          │    │
│  │  │   Deployment:      │     │   Deployment:      │                          │    │
│  │  │   tor-proxy        │     │   vpn-gateway      │                          │    │
│  │  │                    │     │                    │                          │    │
│  │  │  ┌──────────────┐  │     │  ┌──────────────┐  │                          │    │
│  │  │  │  TOR Pod     │  │     │  │   VPN Pod    │  │                          │    │
│  │  │  └──────────────┘  │     │  └──────────────┘  │                          │    │
│  │  │  ┌──────────────┐  │     │  ┌──────────────┐  │                          │    │
│  │  │  │  TOR Pod     │  │     │  │   VPN Pod    │  │                          │    │
│  │  │  └──────────────┘  │     │  └──────────────┘  │                          │    │
│  │  │                    │     │                    │                          │    │
│  │  └─────────┬──────────┘     └────────┬───────────┘                          │    │
│  │            │                         │                                      │    │
│  │            └─────────────┬───────────┘                                      │    │
│  │                          │                                                  │    │
│  │                          │                                                  │    │
│  │                          ▼                                                  │    │
│  │  ┌────────────────────────────────────────┐                                 │    │
│  │  │         StatefulSet: redis             │     Stockage URLs prioritaires  │    │
│  │  │                                        │                                 │    │
│  │  │  ┌──────────────┐    ┌──────────────┐  │                                 │    │
│  │  │  │ Redis Master │    │ Redis Replica│  │                                 │    │
│  │  │  └──────────────┘    └──────────────┘  │                                 │    │
│  │  │                                        │                                 │    │
│  │  └───────────────────┬────────────────────┘                                 │    │
│  │                      │                                                      │    │
│  │                      │                                                      │    │
│  │                      ▼                                                      │    │
│  │  ┌──────────────────────────────────────────┐                               │    │
│  │  │        Deployment: crawler               │                               │    │
│  │  │                                          │                               │    │
│  │  │   ┌────────────────┐ ┌────────────────┐  │                               │    │
│  │  │   │ Crawler Pod    │ │ Crawler Pod    │  │                               │    │
│  │  │   │ - HPA          │ │ - HPA          │  │                               │    │
│  │  │   │ - PDB          │ │ - PDB          │  │                               │    │
│  │  │   └────────────────┘ └────────────────┘  │                               │    │
│  │  │   ┌────────────────┐ ┌────────────────┐  │                               │    │
│  │  │   │ Crawler Pod    │ │ Crawler Pod    │  │                               │    │
│  │  │   │ - HPA          │ │ - HPA          │  │                               │    │
│  │  │   │ - PDB          │ │ - PDB          │  │                               │    │
│  │  │   └────────────────┘ └────────────────┘  │                               │    │
│  │  │                                          │                               │    │
│  │  └─────────────────────┬────────────────────┘                               │    │
│  │                        │                                                    │    │
│  │                        │                                                    │    │
│  │                        ▼                                                    │    │
│  │  ┌────────────────────────────────────────┐                                 │    │
│  │  │       StatefulSet: rabbitmq            │     File de messages HTML       │    │
│  │  │                                        │                                 │    │
│  │  │  ┌──────────────┐  ┌──────────────┐    │                                 │    │
│  │  │  │ RabbitMQ     │  │ RabbitMQ     │    │                                 │    │
│  │  │  │ Node 1       │  │ Node 2       │    │                                 │    │
│  │  │  └──────────────┘  └──────────────┘    │                                 │    │
│  │  │  ┌──────────────┐                      │                                 │    │
│  │  │  │ RabbitMQ     │                      │                                 │    │
│  │  │  │ Node 3       │                      │                                 │    │
│  │  │  └──────────────┘                      │                                 │    │
│  │  │                                        │                                 │    │
│  │  └───────────────────┬────────────────────┘                                 │    │
│  │                      │                                                      │    │
│  │                      │                                                      │    │
│  │                      ▼                                                      │    │
│  │  ┌──────────────────────────────────────────┐                               │    │
│  │  │        Deployment: filter                │                               │    │
│  │  │                                          │                               │    │
│  │  │   ┌────────────────┐ ┌────────────────┐  │                               │    │
│  │  │   │ Filter Pod     │ │ Filter Pod     │  │                               │    │
│  │  │   │ - HPA          │ │ - HPA          │  │                               │    │
│  │  │   │ - PDB          │ │ - PDB          │  │                               │    │
│  │  │   └────────────────┘ └────────────────┘  │                               │    │
│  │  │   ┌────────────────┐ ┌────────────────┐  │                               │    │
│  │  │   │ Filter Pod     │ │ Filter Pod     │  │                               │    │
│  │  │   │ - HPA          │ │ - HPA          │  │                               │    │
│  │  │   │ - PDB          │ │ - PDB          │  │                               │    │
│  │  │   └────────────────┘ └────────────────┘  │                               │    │
│  │  │                      │                   │                               │    │
│  │  └──────────────────────┼───────────────────┘                               │    │
│  │                         │                                                   │    │
│  │      ┌──────────────────┼──────────────────────┐                            │    │
│  │      │                  │                      │                            │    │
│  │      ▼                  │                      ▼                            │    │
│  │  ┌─────────────────┐    │               ┌─────────────────────┐             │    │
│  │  │ StatefulSet:    │    │               │   StatefulSet:      │             │    │
│  │  │ mongodb         │◄───┼───────────────┤   elasticsearch     │             │    │
│  │  │                 │    │               │                     │             │    │
│  │  │ ┌─────────────┐ │    │               │  ┌─────────────┐    │             │    │
│  │  │ │MongoDB Prim.│ │    │               │  │ES Master    │    │             │    │
│  │  │ └─────────────┘ │    │               │  └─────────────┘    │             │    │
│  │  │ ┌─────────────┐ │    │               │  ┌─────────────┐    │             │    │
│  │  │ │MongoDB Sec. │ │    │               │  │ES Data Node │    │             │    │
│  │  │ └─────────────┘ │    │               │  └─────────────┘    │             │    │
│  │  │                 │    │               │  ┌─────────────┐    │             │    │
│  │  │                 │    │               │  │ES Data Node │    │             │    │
│  │  │                 │    │               │  └─────────────┘    │             │    │
│  │  └────────┬────────┘    │               └──────────┬──────────┘             │    │
│  │           │             │                          │                        │    │
│  │           │             │                          │                        │    │
│  │           │             │                          │                        │    │
│  │           └─────────────┼──────────────────────────┘                        │    │
│  │                         │                                                   │    │
│  │                         │                                                   │    │
│  │                         ▼                                                   │    │
│  │  ┌────────────────────────────────────────┐                                 │    │
│  │  │        Deployment: api-backend         │                                 │    │
│  │  │                                        │                                 │    │
│  │  │   ┌────────────────┐ ┌────────────────┐│                                 │    │
│  │  │   │ API Pod        │ │ API Pod        ││                                 │    │
│  │  │   │                │ │                ││                                 │    │
│  │  │   └────────────────┘ └────────────────┘│                                 │    │
│  │  │                                        │                                 │    │
│  │  └───────────────────┬────────────────────┘                                 │    │
│  │                      │                                                      │    │
│  │                      │                                                      │    │
│  │                      ▼                                                      │    │
│  │  ┌────────────────────────────────────────┐                                 │    │
│  │  │        Deployment: frontend            │                                 │    │
│  │  │                                        │                                 │    │
│  │  │   ┌────────────────┐ ┌────────────────┐│                                 │    │
│  │  │   │ Frontend Pod   │ │ Frontend Pod   ││                                 │    │
│  │  │   │                │ │                ││                                 │    │
│  │  │   └────────────────┘ └────────────────┘│                                 │    │
│  │  │                                        │                                 │    │
│  │  └────────────────────────────────────────┘                                 │    │
│  │                                                                             │    │
│  │                                                                             │    │
│  │  ┌────────────────────────────────────────┐                                 │    │
│  │  │         Horizontals & Services         │                                 │    │
│  │  │                                        │                                 │    │
│  │  │ • HPA: Crawler & Filter pods           │                                 │    │
│  │  │ • Service: TOR/VPN proxy               │                                 │    │
│  │  │ • Service: RabbitMQ                    │                                 │    │
│  │  │ • Service: Redis                       │                                 │    │
│  │  │ • Service: MongoDB                     │                                 │    │
│  │  │ • Service: Elasticsearch               │                                 │    │
│  │  │ • Service: API-Backend                 │                                 │    │
│  │  │ • Ingress: Frontend Dashboard          │                                 │    │
│  │  │                                        │                                 │    │
│  │  └────────────────────────────────────────┘                                 │    │
│  │                                                                             │    │
│  └─────────────────────────────────────────────────────────────────────────────┘    │
│                                                                                     │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

### Détails des composants Kubernetes

Namespace: cti-darkweb

Espace isolé pour tous les composants de la solution CTI Darkweb.

#### StatefulSets

Redis

    Service de file d'attente prioritaire et cache
    Configuration en mode maître-réplique pour haute disponibilité
    Volume persistant pour les données
    Exposition de service uniquement en interne

RabbitMQ

    File d'attente persistante entre les crawlers et les filters
    Configuration en cluster avec 3 nodes pour haute disponibilité
    Garantit la livraison des messages même en cas de panne temporaire
    Mécanisme d'acknowledgment pour assurer le traitement des messages

MongoDB

    Stockage persistant des données CTI structurées
    Configuration avec réplication pour redondance
    Volumes persistants pour les données
    Sauvegardes automatisées via CronJob

Elasticsearch

    Indexation et recherche avancée des données CTI
    Cluster avec nœuds maître et de données
    Volumes persistants pour les index
    Configuration optimisée pour les recherches textuelles

#### Deployments

Crawler

    Pods scalables pour le crawling du darkweb
    Tous les pods passent par le service tor-proxy
    AutoScaling basé sur la taille de la file d'attente
    Limites de ressources ajustées pour éviter la surconsommation
    Affinité avec les nœuds disposant de plus de bande passante

Filter

    Pods scalables pour l'analyse et le filtrage du contenu
    AutoScaling basé sur la taille de la file d'attente RabbitMQ
    Optimisé pour le traitement parallèle de données
    Consomme les messages de RabbitMQ à son propre rythme

Tor-proxy

    Service dédié pour gérer les connexions TOR
    Rotation automatique des circuits
    Isolation réseau stricte
    Exposition d'un proxy SOCKS accessible uniquement aux crawlers

API-backend

    Service API RESTful pour accéder aux données CTI
    Authentification et autorisation via JWT
    Rate limiting pour éviter les abus
    Cache Redis pour les requêtes fréquentes

Frontend

    Interface utilisateur pour analyser les données CTI
    Servie via NGINX
    Build statique pour performance optimale
    Configuration via ConfigMap

#### Services

    redis-service: ClusterIP pour accès interne uniquement, points d'accès pour les fonctions queue et cache
    rabbitmq-service: ClusterIP pour accès interne uniquement, service de file d'attente entre crawlers et filters
    mongodb-service: ClusterIP pour accès interne uniquement, accès séparé pour lecture/écriture
    elasticsearch-service: ClusterIP pour accès interne uniquement, points d'accès pour recherche et indexation
    crawler-service: ClusterIP pour accès interne uniquement, service de découverte pour les crawlers
    filter-service: ClusterIP pour accès interne uniquement, point d'entrée pour les données à analyser
    api-service: Service exposé via Ingress, sécurisé avec TLS
    frontend-service: Service exposé via Ingress, sécurisé avec TLS
    tor-proxy-service: ClusterIP pour accès interne uniquement, exposition du proxy SOCKS aux crawlers

#### ConfigMaps, Secrets et HPA

    ConfigMaps: Configuration des comportements de crawling, règles de filtrage, configuration API
    Secrets: Identifiants pour MongoDB, Redis, RabbitMQ, Elasticsearch, et clés API
    HorizontalPodAutoscalers:
        crawler-hpa: Scaling basé sur la taille de queue Redis
        filter-hpa: Scaling basé sur la taille de queue RabbitMQ
        api-hpa: Scaling basé sur le nombre de requêtes

#### Sécurité et NetworkPolicies

    restrict-crawler-egress: Limite les sorties réseau des crawlers uniquement via tor-proxy
    protect-database-access: Limite l'accès aux bases de données
    isolate-tor-network: Isole le réseau TOR du reste de l'infrastructure
    protect-queue-access: Contrôle l'accès à RabbitMQ

#### PersistentVolumes

    mongodb-data: Stockage persistant pour MongoDB
    elasticsearch-data: Stockage persistant pour Elasticsearch
    redis-data: Stockage persistant pour Redis
    rabbitmq-data: Stockage persistant pour RabbitMQ

#### Flux de données dans l'infrastructure Kubernetes

    Les URLs seed sont initialement chargées dans Redis via un Job Kubernetes
    Les pods crawler récupèrent les URLs de Redis et les traitent via le proxy TOR
    Le contenu HTML récupéré est publié dans la file d'attente RabbitMQ
    Les pods filter consomment les messages de RabbitMQ à leur propre rythme
    Les filtres analysent le contenu et stockent les données pertinentes dans MongoDB
    Les données sont également indexées dans Elasticsearch
    Les nouvelles URLs découvertes sont renvoyées à Redis
    L'API backend accède aux données via MongoDB et Elasticsearch
    Le frontend interagit avec l'API pour afficher les données aux utilisateurs

#### Monitoring et Observabilité

    Prometheus: Collecte les métriques de tous les composants
    Grafana: Visualise les métriques et alertes
    AlertManager: Gère les alertes et notifications
    Loki: Agrégation de logs centralisée
    Jaeger: Traçage distribué pour suivre les requêtes

#### Avantages de cette architecture

    Découplage entre composants: Les crawlers et les filters peuvent fonctionner à des rythmes différents sans blocage grâce à RabbitMQ
    Haute disponibilité: Réplication pour toutes les bases de données et file d'attente
    Scalabilité dynamique: Scaling automatique basé sur la charge réelle des composants
    Sécurité renforcée: Isolation réseau stricte, tout le trafic passant par TOR
    Résilience: Capacité à reprendre après des pannes sans perte de données
    Observabilité complète: Monitoring détaillé de chaque composant du système
    Performance optimisée: Distribution de charge et parallélisme pour un traitement efficace
