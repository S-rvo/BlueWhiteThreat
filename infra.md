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
                │ Envoie le HTML et métadonnées
                │
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

1. Initialisation de la file d'attente

   Les URLs seeds sont chargées dans la file d'attente prioritaire Redis
   Format: ZADD "crawler:url_queue" [score] [url_data_json]
   Les scores plus élevés indiquent une priorité plus grande

2. Extraction d'URL

   Le crawler vérifie si le taux de requête pour le domaine respecte les limites
   Vérification si l'URL a déjà été visitée: SISMEMBER "crawler:visited_urls" [url]
   Extraction de l'URL prioritaire: ZPOPMAX "crawler:url_queue" 1
   Ajout de l'URL aux URLs visitées: SADD "crawler:visited_urls" [url]

3. Récupération de la page web

   Le crawler établit une connexion via TOR/I2P/VPN pour l'anonymat
   Il envoie une requête HTTP GET avec des en-têtes aléatoires et user-agents variés
   Gère les redirections, erreurs HTTP et timeouts
   Respecte un délai aléatoire entre les requêtes pour éviter la détection

4. Analyse et filtrage du contenu

   Le filter extrait les données structurées pertinentes du HTML
   Identifie automatiquement les IoCs (adresses IP, domaines, hashes, CVE, etc.)
   Évalue la pertinence du contenu en fonction de mots-clés et patterns CTI
   Détecte les doublons potentiels pour éviter la duplication de données

5. Extraction de nouvelles URLs

   Le filter identifie les liens présents dans la page
   Convertit les URLs relatives en URLs absolues
   Filtre les URLs selon des règles prédéfinies (même domaine, patterns spécifiques)
   Calcule un score de priorité pour chaque URL basé sur:
   Profondeur de crawl
   Pertinence contextuelle
   Domaine source
   Présence de mots-clés CTI dans l'URL

6. Stockage des données pertinentes

   Les données pertinentes sont formatées en documents MongoDB
   Insertion ou mise à jour: db.ctidata.updateOne({url: [url]}, {$set: [data]}, {upsert: true})
   Les données sont également indexées dans Elasticsearch pour des recherches avancées
   Indexation: PUT /ctidata/\_doc/[url_encoded] [json_data]

7. Mise à jour de la file d'attente

   Les nouvelles URLs découvertes sont vérifiées:
   Contre le set d'URLs déjà visitées
   Pour leur pertinence CTI potentielle
   Les URLs pertinentes sont ajoutées à la file prioritaire:
   ZADD "crawler:url_queue" [score] [url_data_json]

8. Accès aux données via API

   Le backend fournit des endpoints RESTful pour accéder aux données
   Fonctionnalités de recherche avancée via Elasticsearch
   Filtrage multi-critères (date, source, type d'IoC, score de pertinence)
   Format de réponse structuré et normalisé

9. Visualisation et exploitation

   Le frontend présente les données CTI de manière exploitable pour les analystes
   Tableaux de bord interactifs pour le suivi des menaces
   Visualisations des relations entre différents IoCs
   Système d'alertes pour les menaces prioritaires

Structure des données
Document CTI dans MongoDB

```
{
    "\_id": "ObjectId()",
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

Optimisation des performances
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

Sécurité et conformité
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

Extensions futures
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
