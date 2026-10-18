# BlueWhiteThreat
Ce projet est un crawler écrit en Go, conçu pour collecter, analyser et visualiser des informations sur les menaces provenant de sources du darkweb, avec une interface web dédiée permettant une visualisation simple et intuitive des données extraites.

Le système repose sur une architecture modulaire intégrant plusieurs mécanismes complémentaires :

**Crawler** : explore automatiquement les sources ciblées pour identifier des pages ou ressources pertinentes.

**Scraper** : extrait le contenu brut des pages identifiées (HTML, JSON, etc.).

**Parser** : transforme le contenu extrait en entités exploitables (IOCs, noms de malware, groupes, infrastructures, etc.).

**Redis** : gère la file d’attente des URLs à explorer, permettant un traitement asynchrone et scalable.

**MongoDB** : stocke les données extraites sous forme de documents structurés et flexibles.

**Neo4j** : modélise les relations entre les entités pour une analyse orientée graphe.

**Frontend web** : permet la consultation des menaces collectées, l’exploration des graphes d’interconnexion et l’analyse des entités.

Ce projet vise à fournir un outil de veille automatisée sur les menaces issues du darkweb, et à faciliter leur analyse contextuelle à travers une visualisation graphique et interactive.

## Documentation opérationnelle

- [`docs/mvp-2027-cadrage.md`](docs/mvp-2027-cadrage.md) : périmètre et contrats du MVP de janvier 2027.
- [`docs/runbook-services.md`](docs/runbook-services.md) : datasheet pour préparer, lancer, tester, diagnostiquer et modifier chaque service selon les rôles technicien, analyste et développeur.
- [`monitoring/README.md`](monitoring/README.md) : démarrage et exploitation du dashboard Grafana, des métriques Prometheus et des logs Loki.

#### Schémas Fonctionnel Crawler:
```mermaid
flowchart TB
    onion["Sources .onion"] --> redis[(Redis)] --> crawler["Crawler Go"]
    crawler --> tor1["Proxy Tor 1"] --> onion
    crawler --> tor2["Proxy Tor 2"] --> onion
    crawler --> tor3["Proxy Tor 3"] --> onion
    crawler --> neo4j[(Neo4j)]
    api["API Go"] --> front["Frontend Nuxt"]
    api --> neo4j --> api

    subgraph OBS["Logs et monitoring Docker"]
        docker["Docker Engine\nlogs stdout/stderr"] --> alloy["Grafana Alloy"] --> loki[(Loki)] --> grafana["Grafana"]
        docker --> cadvisor["cAdvisor"] --> prometheus[(Prometheus)] --> grafana
    end

    crawler -. observé .-> docker
    tor1 -. observé .-> docker
    tor2 -. observé .-> docker
    tor3 -. observé .-> docker
    api -. observé .-> docker
    front -. observé .-> docker
    redis -. observé .-> docker
    neo4j -. observé .-> docker
```

#### Schémas Fonctionnel Scraping, Parsing :

```mermaid
flowchart TB
    scraper["Scraper Go"] --> extraction["Extraction du contenu"] --> mongo[(MongoDB)] --> api["API Go"] --> front["Frontend Nuxt"]
    mongo --> classifier["Classifieur NLP"] --> parser["Parser CTI"] --> neo4j[(Neo4j)]

    subgraph OBS["Logs et monitoring Docker"]
        docker["Docker Engine\nlogs stdout/stderr"] --> alloy["Grafana Alloy"] --> loki[(Loki)] --> grafana["Grafana"]
        docker --> cadvisor["cAdvisor"] --> prometheus[(Prometheus)] --> grafana
    end

    scraper -. observé .-> docker
    classifier -. observé .-> docker
    parser -. observé .-> docker
    api -. observé .-> docker
    mongo -. observé .-> docker
    neo4j -. observé .-> docker
```
