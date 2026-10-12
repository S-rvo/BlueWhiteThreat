# Datasheet opérationnelle — services BlueWhiteThreat

> Public : technicien, analyste CTI et développeur  
> État de référence : branche `main`, 12 octobre 2026  
> Portée : environnement local de développement et de démonstration, pas production

## 1. Objectif du document

Cette datasheet indique, pour chaque composant :

- son rôle et son état réel ;
- comment le préparer, le lancer et l'arrêter ;
- comment vérifier qu'il fonctionne ;
- où le modifier et quels tests exécuter ;
- ce que doivent contrôler un technicien, un analyste et un développeur ;
- les limites connues avant de conclure qu'un test de bout en bout est valide.

Elle complète `docs/mvp-2027-cadrage.md`. Elle décrit l'existant observable dans le dépôt ; un service marqué **planifié** n'a volontairement aucune commande de lancement.

## 2. Règles de sécurité avant toute utilisation

1. Ne jamais utiliser une source réelle qui n'a pas été validée dans le cadre du projet.
2. Ne jamais placer d'identifiant, token, contenu sensible ou donnée personnelle dans Git, un ticket ou un jeu de démonstration.
3. Utiliser des fixtures sûres pour les tests, le développement frontend et la soutenance.
4. Ne pas contourner de captcha, authentification, rate-limit ou mesure anti-bot.
5. Arrêter la collecte et le réentraînement si le garde-fou de contenu interdit signale une correspondance ; une revue humaine est alors obligatoire.
6. Considérer les identifiants `root/root`, `neo4j/password` et `user/user` présents dans la configuration comme des valeurs locales de développement à remplacer avant tout environnement partagé.

## 3. Carte rapide des services

| Service Compose | Répertoire | État | Port hôte | Dépendances principales | Rôle |
|---|---|---|---|---|---|
| `tor1`, `tor2`, `tor3` | `apps/proxy` | Existant | `9051`, `9052`, `9053` | réseau Tor | Proxies SOCKS5 dédiés à la collecte Tor |
| `redis` | image officielle | Existant | `6379` | aucune | Files et ensembles de déduplication |
| `mongodb` | image officielle | Existant | `27017` | aucune | Contenu brut, classification et entités |
| `mongo-express` | image officielle | Existant, accès à vérifier | `8082` déclaré | MongoDB | Inspection de développement de MongoDB |
| `neo4j` | image officielle | Existant | `7474`, `7687` | aucune | Graphe des URL, entités et relations |
| `crawler` | `apps/crawler` | Existant, seed codée en dur | aucun port hôte | Redis, Neo4j, Tor | Découverte des URL `.onion` |
| `scraper` | `apps/scraper` | Existant | aucun | Redis, MongoDB, Tor | Récupération des pages découvertes |
| `classifier` | `apps/classifier` | Existant, modèle requis | aucun | MongoDB, `model.json` | Pertinence CTI et boucle de revue humaine |
| `api` | `apps/api` | Existant, minimal | `8083` | MongoDB | Lecture brute de la collection `article` |
| `front` | `apps/front` | Existant, partiel | `8080` | API, futur graphe | Interface Nuxt analyste |
| `target` | `apps/target` | Existant, fixture | `8081` | aucune fonctionnelle | Site Vue de test, jamais une source réelle |
| parser CTI | futur `apps/parser` | **Planifié** | à définir | MongoDB, Ollama, Neo4j | Extraction d'entités structurées |
| registre des sources | à définir | **Planifié** | à définir | API, base de données | Validation analyste des sources candidates |
| connecteur forums | à définir | **Planifié** | aucun prévu | registre, MongoDB | Collecte de forums validés |
| connecteur Telegram | futur service Go | **Planifié** | aucun prévu | registre, MongoDB | Collecte de canaux publics validés |
| export STIX/TAXII | à définir | **Planifié** | à définir | Neo4j | Interopérabilité MISP/SIEM |
| alerting | à définir | **Planifié** | à définir | Neo4j, API | Alertes acteur/TTP surveillé |

## 4. Préparation initiale

### 4.1 Prérequis poste

- Docker avec la commande `docker compose` ;
- Go 1.24 pour travailler hors conteneur sur les services Go ;
- Node.js 22 pour travailler hors conteneur sur `front` et `target` ;
- Git ;
- au moins les ports 6379, 7474, 7687, 8080 à 8083 et 9051 à 9053 libres.

### 4.2 Fichiers d'environnement locaux

Depuis la racine du dépôt :

```bash
cp apps/scraper/.env.example apps/scraper/.env
cp apps/classifier/.env.example apps/classifier/.env
```

Renseigner les fichiers sans les commiter. Pour l'environnement Compose actuel, les valeurs de développement doivent être cohérentes entre MongoDB, le scraper et le classifieur :

```dotenv
MONGO_HOST=mongodb
MONGO_PORT=27017
MONGO_INITDB_ROOT_USERNAME=root
MONGO_INITDB_ROOT_PASSWORD=root
MONGO_DB=BlueWhiteThreat
```

Ajouter côté scraper :

```dotenv
SCRAPER_INTERVAL_MIN=30
REDIS_HOST=redis
REDIS_PORT=6379
SITES_CONFIG_PATH=./data/sites.json
```

Ajouter côté classifieur :

```dotenv
CLASSIFIER_INTERVAL_MIN=10
MODEL_PATH=model.json
RELEVANCE_THRESHOLD=
```

Laisser `RELEVANCE_THRESHOLD` vide pour utiliser le seuil enregistré dans le modèle. Ne renseigner un token ou des identifiants de forum que dans le fichier local ou dans un gestionnaire de secrets.

### 4.3 Contrôle avant démarrage

```bash
docker compose config
docker compose config --services
git status --short
```

Résultat attendu : la configuration Compose est valide et les fichiers `.env` n'apparaissent pas dans Git.

## 5. Parcours technicien — exploiter l'environnement

### 5.1 Démarrage prudent de l'infrastructure

Ne pas commencer par `docker compose up` sur toute la stack : le classifieur s'arrête si `model.json` n'existe pas et le crawler utilise actuellement une seed `.onion` codée en dur.

```bash
docker compose up -d redis mongodb neo4j tor1 tor2 tor3 target
docker compose ps
```

### 5.2 Vérifications d'infrastructure

```bash
docker compose exec redis redis-cli ping
docker compose exec redis redis-cli HGETALL crawler:stats
docker compose exec neo4j cypher-shell -u neo4j -p password "RETURN 1 AS ok;"
docker compose logs --tail=50 mongodb neo4j tor1 tor2 tor3
```

Résultats attendus :

- Redis répond `PONG` ;
- Neo4j retourne `ok = 1` ;
- MongoDB et les trois proxies restent en état `running` ;
- aucune boucle d'erreur rapide n'apparaît dans les logs.

### 5.3 Démarrage applicatif par étage

Uniquement après validation du périmètre des sources :

```bash
docker compose up -d crawler
docker compose up -d scraper
docker compose up -d api front
```

Le classifieur se lance seulement après création de `apps/classifier/model.json`, décrite dans sa fiche.

### 5.4 Supervision courante

```bash
docker compose ps
docker compose logs --tail=100 crawler scraper classifier api front
docker compose logs -f scraper
```

Contrôles utiles :

```bash
docker compose exec redis redis-cli SCARD urls_to_crawl
docker compose exec redis redis-cli SCARD urls_crawled
docker compose exec redis redis-cli SCARD urls_scraped
docker compose exec crawler curl -fsS http://localhost:8080/health
docker compose exec crawler curl -fsS http://localhost:8080/stats
curl -fsS http://localhost:8083/data
```

### 5.5 Arrêt et reprise

Arrêt en conservant les données :

```bash
docker compose stop
```

Arrêt et suppression des conteneurs en conservant les volumes :

```bash
docker compose down
```

Ne pas utiliser `docker compose down -v` dans le workflow normal : cette option supprime les volumes MongoDB et Neo4j. Elle doit être réservée à une remise à zéro explicitement décidée et précédée d'une sauvegarde utile.

## 6. Parcours analyste — examiner et valider

### 6.1 Interfaces actuelles

| Usage | Adresse locale | État |
|---|---|---|
| Interface analyste Nuxt | `http://localhost:8080` | Partielle |
| API articles | `http://localhost:8083/data` | Fonctionnelle mais non paginée |
| Fixture de test | `http://localhost:8081` | Développement uniquement |
| Neo4j Browser | `http://localhost:7474` | Fonctionnel |
| Mongo Express | `http://localhost:8082` | À vérifier avant usage |

### 6.2 Contrôles analyste sur un résultat

Pour tout article ou entité, vérifier :

- la source et l'URL/message d'origine ;
- la date de collecte ;
- la version du modèle de classification ;
- le score et le seuil appliqué ;
- l'absence de contenu interdit ou de données personnelles inutiles ;
- la cohérence entre l'extrait, l'entité proposée et la relation dans le graphe ;
- la distinction entre fait observé, corrélation calculée et hypothèse analyste.

### 6.3 Cycle de revue du classifieur

Depuis `apps/classifier`, avec MongoDB accessible :

```bash
go run ./cmd/export
```

Le fichier `review_queue.jsonl` contient les candidats non encore revus. L'analyste crée un fichier séparé de décisions au format JSONL :

```json
{"id":"identifiant-du-document","label":1}
{"id":"autre-identifiant","label":0}
```

Puis le développeur ou l'opérateur applique les décisions :

```bash
DECISIONS_PATH=decisions.jsonl go run ./cmd/applyreview
go run ./cmd/bootstrap
go run ./cmd/checkfilter
go run ./cmd/train
```

Convention : `1` signifie pertinent CTI, `0` non pertinent. Les décisions doivent être prises sur des lots bornés et relues ; elles ne doivent pas contenir le texte source dans le fichier de décisions.

### 6.4 Workflow des nouvelles sources — cible non encore implémentée

Le workflow cible est :

1. découverte d'une source candidate ;
2. récupération d'un aperçu minimal sûr ;
3. alerte dans l'interface ;
4. revue de la provenance, du thème, de la légalité et de la fiabilité ;
5. décision approuvée, rejetée ou révoquée, avec auteur, date et justification ;
6. collecte régulière uniquement si la source est approuvée.

Tant que le registre n'est pas implémenté, cette validation reste manuelle et doit être consignée dans le ticket ou le journal de décisions avant de modifier les seeds/configurations.

## 7. Parcours développeur — modifier sans casser le pipeline

### 7.1 Cycle standard

1. choisir un ticket et vérifier ses dépendances ;
2. ne modifier qu'un composant principal à la fois ;
3. lancer ses tests unitaires ;
4. reconstruire le service concerné ;
5. lancer un smoke test avec des fixtures sûres ;
6. vérifier les logs et les effets dans Redis/MongoDB/Neo4j ;
7. demander une revue croisée.

### 7.2 Tests locaux vérifiés

```bash
cd apps/crawler && go test ./...
cd apps/scraper && go test ./...
cd apps/classifier && go test ./...
cd apps/api && go test ./...
```

Au 12 octobre 2026, ces quatre commandes passent. L'API ne contient toutefois aucun test applicatif ; sa commande valide uniquement la compilation.

Frontend et fixture :

```bash
cd apps/front && npm ci && npm run build
cd apps/target && npm ci && npm run build
```

Ces builds nécessitent le téléchargement des dépendances si `node_modules` est absent.

### 7.3 Reconstruction ciblée

```bash
docker compose build crawler
docker compose build scraper
docker compose build classifier
docker compose build api
docker compose build front
docker compose up -d --no-deps SERVICE
docker compose logs --tail=100 SERVICE
```

Remplacer `SERVICE` par le service modifié. Les volumes de développement du Compose permettent le hot-reload de plusieurs services, mais une modification de dépendance ou de Dockerfile exige un rebuild.

## 8. Fiches par service

### 8.1 Proxies Tor — `tor1`, `tor2`, `tor3`

**Responsabilité.** Exposer trois proxies SOCKS5 indépendants sur le port 9050 interne.

**Fichiers.** `apps/proxy/Dockerfile.dev`, `apps/proxy/torrc`.

**Lancer.**

```bash
docker compose up -d tor1 tor2 tor3
```

**Observer.**

```bash
docker compose ps tor1 tor2 tor3
docker compose logs --tail=50 tor1 tor2 tor3
```

**Critère prêt.** Les trois conteneurs restent actifs et les logs montrent l'initialisation de Tor sans boucle d'échec.

**Modifier/tester.** Toute modification de `torrc` impose `docker compose build tor1 tor2 tor3` puis un redémarrage. Un test réseau vers Tor ne doit être réalisé que dans un cadre autorisé.

### 8.2 Redis — `redis`

**Responsabilité.** Stocker `urls_to_crawl`, `urls_crawled`, `urls_scraped` et `crawler:stats`.

**Lancer et tester.**

```bash
docker compose up -d redis
docker compose exec redis redis-cli ping
docker compose exec redis redis-cli --scan
```

**Critère prêt.** `PING` retourne `PONG`.

**Incident courant.** Si les compteurs paraissent incohérents, inspecter les trois ensembles avant toute suppression. Ne jamais lancer `FLUSHALL` sur un environnement contenant des données utiles.

### 8.3 MongoDB — `mongodb`

**Responsabilité.** Conserver la collection `BlueWhiteThreat.article` et ses enrichissements.

**Lancer.**

```bash
docker compose up -d mongodb
docker compose logs --tail=50 mongodb
```

**Tester depuis le conteneur.**

```bash
docker compose exec mongodb sh -lc 'mongosh -u "$MONGO_INITDB_ROOT_USERNAME" -p "$MONGO_INITDB_ROOT_PASSWORD" --authenticationDatabase admin --eval "db.adminCommand({ ping: 1 })"'
```

**Critère prêt.** La commande retourne `ok: 1`.

**Modification.** Les schémas applicatifs sont dans `apps/scraper/internal/sites/page.go`, `apps/classifier/internal/db/mongo.go` et, à terme, le parser. Toute évolution de champ doit préserver provenance et compatibilité de lecture.

### 8.4 Neo4j — `neo4j`

**Responsabilité.** Stocker actuellement les URL et liens du crawler ; cible : entités et relations CTI.

**Lancer/tester.**

```bash
docker compose up -d neo4j
docker compose exec neo4j cypher-shell -u neo4j -p password "RETURN 1 AS ok;"
```

**Inspection.** Ouvrir `http://localhost:7474` puis utiliser des requêtes bornées, par exemple `MATCH (n) RETURN n LIMIT 25`.

**Modification.** Code actuel : `apps/crawler/internal/db/neo4j.go`. La projection CTI planifiée doit utiliser `MERGE`, des contraintes d'unicité et une provenance documentaire.

### 8.5 Fixture web — `target`

**Responsabilité.** Fournir une interface locale contrôlée pour des essais de scraping/front ; elle ne représente pas une source fiable réelle.

**Lancer/tester.**

```bash
docker compose up -d target
curl -I http://localhost:8081
```

**Modifier.** Répertoire `apps/target`, puis `npm run dev` ou `npm run build`.

**Limite.** Le scraper darkweb actuel force un proxy Tor pour les URL de sa file. La fixture n'est donc pas encore un corpus de bout en bout directement compatible avec ce chemin.

### 8.6 Crawler — `crawler`

**Responsabilité.** Lire une seed, découvrir des liens, dédupliquer via Redis et écrire les relations URL dans Neo4j.

**Fichiers principaux.** `apps/crawler/cmd/main.go`, `internal/crawler`, `internal/db`, `internal/api`.

**Lancer.**

```bash
docker compose up -d crawler
docker compose logs -f crawler
```

**Smoke test.**

```bash
docker compose exec crawler curl -fsS http://localhost:8080/health
docker compose exec crawler curl -fsS http://localhost:8080/stats
```

**Tests développeur.**

```bash
cd apps/crawler
go test ./...
```

**Critère prêt.** Connexions Redis/Neo4j établies, endpoint `/health` à `ok`, progression ou fin contrôlée de la file.

**Limite bloquante.** La seed est codée en dur dans `cmd/main.go` et le crawler s'arrête quand la file est vide. Ne pas le considérer comme une collecte continue ni comme compatible avec le futur registre avant correction.

### 8.7 Scraper — `scraper`

**Responsabilité.** Calculer `urls_crawled - urls_scraped`, charger les pages via les trois proxies Tor et persister le contenu dans MongoDB.

**Fichiers principaux.** `apps/scraper/cmd/main.go`, `internal/sites`, `internal/db`, `data/sites.json`.

**Lancer.**

```bash
docker compose up -d scraper
docker compose logs -f scraper
```

Le scraper exécute un passage au démarrage puis un nouveau passage toutes les `SCRAPER_INTERVAL_MIN` minutes.

**Tests développeur.**

```bash
cd apps/scraper
go test ./...
```

**Critère prêt.** Les URL non traitées sont consommées, les documents sont upsertés dans `BlueWhiteThreat.article` et les URL terminées rejoignent `urls_scraped`.

**Configuration de connexion optionnelle.** `apps/scraper/data/sites.json` ne doit contenir que les noms des variables d'environnement, jamais les identifiants eux-mêmes.

**Limite importante.** Le code marque actuellement une URL comme scrapée même après un échec, afin d'éviter les boucles infinies. L'opérateur doit donc utiliser les logs pour distinguer succès et abandon ; un statut de tentative persistant reste à concevoir.

### 8.8 Classifieur — `classifier`

**Responsabilité.** Ajouter `relevance.relevant`, `score` et `model_version` aux articles non classés.

**Fichiers principaux.** `apps/classifier/cmd`, `internal`, `data/seed_keywords.json`, `retrain_loop.sh`.

**Préparer le modèle.** Depuis `apps/classifier`, avec MongoDB accessible et un corpus autorisé :

```bash
go run ./cmd/bootstrap
go run ./cmd/checkfilter
go run ./cmd/train
```

Ne pas exécuter `train` si `checkfilter` trouve une correspondance. Le fichier `model.json` généré est local et ignoré par Git.

**Lancer.**

```bash
docker compose up -d classifier
docker compose logs -f classifier
```

**Tests développeur.**

```bash
cd apps/classifier
go test ./...
```

**Diagnostic.**

```bash
go run ./cmd/diagfp --threshold 0.99 --limit 30
```

**Boucle automatique.** `./retrain_loop.sh` est un processus continu. Ne le lancer que sous supervision, après validation du corpus et de la politique de promotion du modèle.

**Critère prêt.** Un modèle est chargé, les articles non classés reçoivent un verdict et les logs indiquent sa version et le nombre de documents traités.

### 8.9 API — `api`

**Responsabilité actuelle.** Exposer `GET /data`, qui retourne tous les documents de `BlueWhiteThreat.article`.

**Lancer/tester.**

```bash
docker compose up -d api
curl -fsS http://localhost:8083/data
```

**Modifier.** `apps/api/cmd/main.go`, puis :

```bash
cd apps/api
go test ./...
```

**Limites bloquantes.** L'URI MongoDB est codée en dur, `MONGODB_URI` n'est pas utilisée, et `/data` n'a ni pagination ni limite. L'API ne doit pas être exposée hors du poste local avant correction.

### 8.10 Frontend analyste — `front`

**Responsabilité.** Afficher les articles et, à terme, les entités, graphes, métriques, sources candidates et alertes.

**Lancer.**

```bash
docker compose up -d front
```

Ouvrir `http://localhost:8080`.

**Développement local.**

```bash
cd apps/front
npm ci
npm run dev
npm run build
```

**Fichiers principaux.** `pages/menaces.vue`, `pages/datamap.vue`, `pages/ioc.vue`, `components`.

**Limites connues.** La page menaces dépend de `http://localhost:8083/data`. La page graphe appelle actuellement `http://localhost:8080/graph`, alors qu'aucun endpoint hôte correspondant n'est exposé par le Compose actuel. La vue graphe n'est donc pas un critère de réussite tant que le contrat API n'est pas corrigé.

### 8.11 Mongo Express — `mongo-express`

**Responsabilité.** Inspection locale de MongoDB, jamais interface analyste.

**Lancer.**

```bash
docker compose up -d mongo-express
docker compose logs --tail=50 mongo-express
```

**Limite connue.** Le Compose déclare `8082:8082`, alors que le port interne habituel de l'image est 8081. Vérifier les logs et corriger le mapping avant de compter sur `http://localhost:8082`.

### 8.12 Services planifiés

Les composants suivants ne sont pas présents dans le code : parser CTI, registre des sources, connecteurs forums/Telegram dédiés, export STIX/TAXII et alerting. Leur fiche opérationnelle devra être ajoutée lorsque leur premier exécutable et leur contrat de configuration existeront.

Pour chacun, la Definition of Done documentaire minimale sera :

- une section Compose ou une commande de lancement reproductible ;
- un `.env.example` sans secret ;
- un endpoint ou une commande de santé ;
- des métriques et logs sans contenu sensible ;
- des tests unitaires et un smoke test sur fixture sûre ;
- une procédure de reprise après échec ;
- une mise à jour de cette datasheet.

## 9. Matrice de validation par rôle

| Étape | Technicien | Analyste | Développeur |
|---|---|---|---|
| Infrastructure | conteneurs actifs, ports et volumes contrôlés | aucune donnée interdite dans les interfaces | configuration reproductible |
| Collecte | files, erreurs et retries observables | source approuvée et provenance visible | déduplication et limites testées |
| Classification | modèle chargé, version tracée | labels et erreurs revus | métriques, seuil et tests reproductibles |
| Extraction | service disponible et rejouable | entités vérifiées sur échantillon | validation déterministe testée |
| Graphe | Neo4j disponible et sauvegardable | relations et confiance interprétables | `MERGE`, unicité et provenance testés |
| Interface | API/front accessibles localement | parcours d'investigation compréhensible | erreurs, vide et chargement traités |
| Démonstration | procédure et mode hors ligne prêts | narration factuelle et sourcée | version, fixtures et résultats figés |

## 10. Diagnostic rapide

| Symptôme | Vérification | Action sûre |
|---|---|---|
| Le classifieur redémarre en boucle | logs : modèle absent | générer `model.json` après bootstrap, contrôle sécurité et entraînement |
| Le scraper ne traite rien | `SCARD urls_crawled`, `SCARD urls_scraped` | vérifier les ensembles Redis et les logs, ne pas vider les données à l'aveugle |
| Une URL échoue une fois puis disparaît | logs scraper et ensemble `urls_scraped` | documenter l'échec ; le statut persistant est une dette connue |
| Le frontend affiche zéro article | API `/data`, logs API, MongoDB | vérifier l'API avant le frontend |
| La vue graphe échoue | requête `/graph` dans le navigateur | endpoint non disponible actuellement ; utiliser Neo4j Browser pour le diagnostic |
| Mongo Express est inaccessible | logs et mapping de port | corriger le port interne du service avant usage |
| Le crawler s'arrête | taille `urls_to_crawl` | comportement actuel normal quand la file est vide |
| Un test Go échoue sur le cache dans un sandbox | permissions du cache Go | définir temporairement un `GOCACHE` dans un répertoire autorisé, sans changer le projet |

## 11. Checklist avant une démonstration

- [ ] Commit ou tag de démonstration identifié.
- [ ] Fichiers `.env` locaux présents et absents de Git.
- [ ] Corpus sûr figé et contrôlé.
- [ ] Images et dépendances déjà téléchargées.
- [ ] Redis, MongoDB et Neo4j démarrent sans erreur.
- [ ] Modèle du classifieur présent, version et seuil notés.
- [ ] Chaque étape produit le résultat attendu sur les fixtures.
- [ ] Aucun appel réseau réel n'est indispensable au scénario principal.
- [ ] Sauvegarde des données de démonstration disponible.
- [ ] Captures ou enregistrement de secours disponibles.
- [ ] Répartition technicien/analyste/développeur répétée par les deux étudiants.

## 12. Maintenance de cette datasheet

Toute modification qui ajoute un service, un port, une variable obligatoire, un schéma de données ou un nouveau chemin de test doit mettre à jour ce document dans la même pull request. Une commande n'est considérée fiable que si elle a été exécutée sur la branche concernée ou explicitement marquée comme cible future.

## 13. Monitoring centralisé

Le dépôt fournit une extension `docker-compose.monitoring.yaml` composée de Grafana, Prometheus, Loki, Alloy et cAdvisor.

Elle apporte immédiatement :

- état et activité des conteneurs ;
- CPU, mémoire et réseau par conteneur ;
- état des cibles Prometheus ;
- logs Docker centralisés et filtrables ;
- compteur de lignes d'erreur ;
- historique local de sept jours pour les métriques et les logs.

Démarrage :

```bash
export GRAFANA_ADMIN_USER=admin
export GRAFANA_ADMIN_PASSWORD='choisir-un-mot-de-passe-local'
docker compose -p bluewhitethreat -f docker-compose.monitoring.yaml up -d
```

Ouvrir ensuite `http://localhost:3000`, puis le dossier **BlueWhiteThreat**. La procédure complète, les contrôles de santé, les ports, l'arrêt et les limites sont documentés dans `monitoring/README.md`.

Le premier panneau Grafana conserve un inventaire explicite de `crawler`, `scraper`, `api`, `classifier`, `tor1`, `tor2`, `tor3`, `front`, `target`, `mongodb`, `redis`, `neo4j`, `mongo-express` et des composants de monitoring. Les services non démarrés apparaissent **HORS LIGNE** au lieu de disparaître. Les filtres **Service** et **Conteneur** pilotent ensemble les graphes de performance et les logs.

Cette visibilité ne constitue pas une autorisation de collecte : ne pas démarrer automatiquement le crawler, le scraper ou les proxies sur des sources réelles. Appliquer d'abord les contrôles de périmètre, d'environnement et de contenu décrits dans ce runbook.

**Persistance.** Un arrêt avec `docker compose down` conserve les volumes. L'option `down -v` les supprime : l'historique Prometheus/Loki, les réglages locaux Grafana et les données MongoDB/Neo4j sont alors perdus. Au prochain `up -d`, Docker recrée des volumes vides et Grafana reprovisionne automatiquement le dashboard et ses sources depuis le dépôt. Utiliser `-v` uniquement pour une remise à zéro volontaire.

Les métriques métier restent une étape suivante : crawler, scraper, classifier et API devront exposer `/metrics` sans inclure de contenu ou d'identifiant sensible.
