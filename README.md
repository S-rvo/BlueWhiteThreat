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

## Pipeline cible du MVP

Les scrapers d'exploration découvrent des pages. Après contrôle de sûreté, le NLP puis l'IA décident automatiquement des pages à conserver et à suivre, avec extraction et validation des IOC. Des scrapers de suivi distincts reviennent sur ces pages à cadence configurable ; aucune validation humaine initiale ou périodique n'est requise. Chaque contenu modifié repasse par le même pipeline. Les restrictions d'accès restent applicables indépendamment de la pertinence CTI.

Ce schéma décrit la cible ; le [cadrage MVP](docs/mvp-2027-cadrage.md) et l'[architecture](docs/sidequest-architecture.md) précisent les contrats et l'état d'implémentation.

```mermaid
flowchart LR
    S[Seeds et liens admissibles] --> QE[(Redis exploration)]
    QE --> E["Scrapers exploration<br/>Découverte de nouvelles pages"]
    E --> F{Filtre de sûreté CSAM}
    U["Scrapers suivi<br/>Workers dédiés aux pages retenues"] --> F
    F -->|Suspect ou indéterminé| B[Blocage sans contenu exposé]
    F -->|Autorisé| M[("MongoDB<br/>Versions et provenance")]
    M --> N{NLP pertinent ?}
    N -->|Non| A["Exclu du suivi actif<br/>Métadonnées et rétention bornée"]
    N -->|Incertain ou erreur| W["Attente et reprise bornée"]
    N -->|Oui| L["IA locale<br/>Verdict CTI et extraction IOC"]
    L --> V["Validation déterministe<br/>Sortie et formats IOC"]
    V --> T{"NLP positif et IA positive<br/>Résultat valide ?"}
    T -->|Oui| R[("Registre des pages suivies<br/>Admission automatique")]
    T -->|Non| A
    L -->|Erreur ou résultat incertain| W
    T -->|Oui| G[("Neo4j<br/>Entités valides et provenance")]
    R --> P[Planificateur de suivi]
    P --> QS[(Redis suivi)]
    QS --> U
    E --> D["Liens découverts<br/>Contrôles accès et destination"]
    D --> QE
    U --> D
    G --> API[API et interface analyste]
    G --> STIX[STIX 2.1 / TAXII et alertes]
```
