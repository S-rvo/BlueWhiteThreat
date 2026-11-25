# Datasheet opérationnelle — services BlueWhiteThreat

> Public : technicien, analyste CTI et développeur  
> État de référence : 25 novembre 2026
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
6. Les ports Compose sont publiés uniquement sur `127.0.0.1`. Renseigner des identifiants dédiés hors Git. Si une ancienne installation a utilisé des identifiants par défaut ou exposé ses ports, prévoir une rotation séparée et vérifier les accès. La modification du YAML et de `.env` ne change pas les conteneurs déjà en cours ni le mot de passe des volumes existants.

**Migration des images locales.** L’image de développement du scraper déjà
construite avant ce correctif peut contenir les fichiers `.env` et
`.credentials.env`. Ne pas redistribuer les anciennes images ni exporter
leur cache de build. Recréer l’image avec le contexte filtré lors d’une
maintenance ; si une image a été partagée, traiter les identifiants comme
potentiellement exposés. Préparer une migration des sessions avant toute
rotation de leur clé de chiffrement.

## 3. Carte rapide des services

| Service Compose | Répertoire | État | Port hôte | Dépendances principales | Rôle |
|---|---|---|---|---|---|
| `tor1`, `tor2`, `tor3` | `apps/proxy` | Existant | `9051`, `9052`, `9053` | réseau Tor | Proxies SOCKS5 dédiés à la collecte Tor |
| `redis` | image officielle | Existant | `6379` | aucune | Files et ensembles de déduplication |
| `mongodb` | image officielle | Existant | `27017` | aucune | Contenu brut, classification et entités |
| `neo4j` | image officielle | Existant | `7474`, `7687` | aucune | Graphe des URL, entités et relations |
| `crawler`, `crawler2`, `crawler3` | `apps/crawler` | Existant | aucun | Redis, Neo4j, Tor | Découverte et revisites ; file et baux partagés |
| `collectseeds` | `apps/crawler/cmd/collectseeds` | Profil facultatif `seeds` | aucun | Redis, catalogues publics | Import quotidien de points de départ |
| `scraper` | `apps/scraper` | Continu, Chromium cloisonné | aucun (8080 interne) | Redis, MongoDB, Tor | Rendu et sauvegarde des pages |
| `redis-backup` | `apps/crawler/scripts/redis-backup.sh` | Existant | aucun | Redis, emplacement de sauvegarde | Export RDB et rotation locale |
| `classifier` | `apps/classifier` | Existant, modèle requis | aucun | MongoDB, `model.json` | Pertinence CTI et boucle de revue humaine |
| `api` | `apps/api` | Profil facultatif `api` | `8083` | MongoDB | Métadonnées pertinentes paginées et métriques agrégées |
| `front` | `apps/front` | Existant, partiel | `8080` | API, futur graphe | Interface Nuxt analyste |
| `parser` | `apps/parser` | Existant | aucun | MongoDB, Ollama, Neo4j | Extraction d'entités et projection dans le graphe |
| registre des sources | à définir | **Planifié** | à définir | API, base de données | Validation analyste des sources candidates |
| connecteur forums | à définir | **Planifié** | aucun prévu | registre, MongoDB | Collecte de forums validés |
| connecteur Telegram | futur service Go | **Planifié** | aucun prévu | registre, MongoDB | Collecte de canaux publics validés |
| export STIX/TAXII | à définir | **Planifié** | à définir | Neo4j | Interopérabilité MISP/SIEM |
| alerting | à définir | **Planifié** | à définir | Neo4j, API | Alertes acteur/TTP surveillé |

## 4. Préparation initiale

### 4.1 Prérequis poste

- Docker avec la commande `docker compose` ;
- Go 1.24 pour travailler hors conteneur sur les services Go ;
- Node.js 22 pour travailler hors conteneur sur `front` ;
- Git ;
- au moins les ports 6379, 7474, 7687, 8080, 8083 et 9051 à 9053 libres.

### 4.2 Fichiers d'environnement locaux

Depuis la racine du dépôt :

```bash
cp .env.example .env
cp apps/scraper/.env.example apps/scraper/.env
cp apps/classifier/.env.example apps/classifier/.env
```

Renseigner les fichiers sans les commiter. Dans le `.env` racine, définir `NEO4J_PASSWORD`. Si le volume Neo4j existe déjà, utiliser son mot de passe actuel ; le modifier dans ce fichier ne le fait pas tourner dans Neo4j. Pour l'environnement Compose actuel, les valeurs de développement doivent être cohérentes entre MongoDB, le scraper et le classifieur :

```dotenv
MONGO_HOST=mongodb
MONGO_PORT=27017
MONGO_INITDB_ROOT_USERNAME=
MONGO_INITDB_ROOT_PASSWORD=
MONGO_DB=BlueWhiteThreat
```

Ajouter côté scraper :

```dotenv
SCRAPER_WORKERS_PER_PROXY=2
REDIS_HOST=redis
REDIS_PORT=6379
```

Le scraper charge aussi `apps/scraper/.credentials.env`, réservé à sa clé `BWT_CREDENTIALS_KEY`. Ce fichier privé est transmis uniquement au service scraper par Compose ; ne pas placer la clé dans le `.env` partagé avec MongoDB et le parser. Sur une nouvelle installation, suivre la [préparation de la clé](scraper-access.md#préparer-une-nouvelle-installation). Ne jamais remplacer une clé existante pour un simple redémarrage.

Le scraper n’a plus d’intervalle de traitement à configurer. N’ajouter
`SITES_CONFIG_PATH` que pour une configuration explicite montée en lecture seule.
Si le poste a besoin de réglages spécifiques, garder
`docker-compose.override.yaml` local et ignoré par Git. Pour un disque de
sauvegarde séparé, voir [isolation et sauvegarde](scraper-isolation-backup.md).

Ajouter côté classifieur :

```dotenv
CLASSIFIER_INTERVAL_MIN=10
MODEL_PATH=model.json
RELEVANCE_THRESHOLD=
```

Laisser `RELEVANCE_THRESHOLD` vide pour utiliser le seuil enregistré dans le modèle. Ne renseigner un token ou des identifiants de forum que dans le fichier local ou dans un gestionnaire de secrets.

### 4.3 Contrôle avant démarrage

```bash
docker compose config --quiet
docker compose config --services
git status --short
```

Résultat attendu : la configuration Compose est valide et les fichiers `.env` n'apparaissent pas dans Git.

## 5. Parcours technicien — exploiter l'environnement

### 5.1 Démarrage prudent de l'infrastructure

Démarrer l’infrastructure séparément afin de contrôler les bases avant les workers et de ne pas lancer de collecte involontaire. Le classifieur demande un `model.json` présent.

```bash
docker compose up -d redis redis-backup mongodb neo4j tor1 tor2 tor3
docker compose ps
```

### 5.2 Vérifications d'infrastructure

```bash
docker compose exec redis redis-cli ping
docker compose exec redis redis-cli HGETALL crawler:stats
docker compose exec neo4j cypher-shell -u neo4j "RETURN 1 AS ok;"
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
docker compose --profile api up -d api front
```

Le conteneur scraper contient déjà six workers Chromium par défaut. Vérifier
d’abord son débit, les échecs et la RAM avant d’augmenter la concurrence.
`collectseeds` est un service distinct, facultatif ; son absence n’arrête pas
le traitement des files existantes. Voir [les états des files](redis-crawler-scraper.md).

Le classifieur charge `apps/classifier/model.json` en lecture seule en production.
Le modèle versionné suffit pour le démarrage ; aucun entraînement n’est nécessaire.

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
docker compose exec redis wget -qO- http://crawler:8080/health
docker compose exec redis wget -qO- http://crawler:8080/stats
docker compose exec scraper /usr/local/bin/scraper --check-health
docker compose exec redis redis-cli ZCARD scraper:pending
docker compose exec redis redis-cli HGETALL scraper:stats
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
| API articles | `http://localhost:8083/data?limit=25` | Métadonnées uniquement ; curseur dans les en-têtes |
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

Le fichier local `review_queue.jsonl`, ignoré par Git, contient les candidats non
encore revus et du texte collecté. L'analyste crée un fichier séparé de décisions au
format JSONL :

```json
{"id":"identifiant-du-document","label":1}
{"id":"autre-identifiant","label":0}
```

Puis le développeur ou l'opérateur applique les décisions :

```bash
DECISIONS_PATH=decisions.jsonl go run ./cmd/applyreview
```

Convention : `1` signifie pertinent CTI, `0` non pertinent. Les décisions doivent être prises sur des lots bornés et relues ; elles ne doivent pas contenir le texte source dans le fichier de décisions.
L’application de décisions ne déclenche aucun entraînement. La procédure de
préparation d’un candidat figure dans la fiche du classifieur.

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

Frontend :

```bash
cd apps/front && npm ci && npm run build
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

**Modification.** Les schémas applicatifs sont dans `apps/scraper/internal/sites/page.go`, `apps/classifier/internal/db/mongo.go` et `apps/parser/internal/db/mongo.go`. Toute évolution de champ doit préserver provenance et compatibilité de lecture.

### 8.4 Neo4j — `neo4j`

**Responsabilité.** Stocker actuellement les URL et liens du crawler ; cible : entités et relations CTI.

**Lancer/tester.**

```bash
docker compose up -d neo4j
docker compose exec neo4j cypher-shell -u neo4j "RETURN 1 AS ok;"
```

**Inspection.** Ouvrir `http://localhost:7474` puis utiliser des requêtes bornées, par exemple `MATCH (n) RETURN n LIMIT 25`.

**Modification.** Code actuel : `apps/crawler/internal/db/neo4j.go`. La projection CTI planifiée doit utiliser `MERGE`, des contraintes d'unicité et une provenance documentaire.

### 8.5 Crawler — `crawler`

**Responsabilité.** Visiter les pages planifiées, découvrir des liens, dédupliquer et réserver via Redis, puis écrire les relations URL dans Neo4j.

**Fichiers principaux.** `apps/crawler/cmd/main.go`, `internal/crawler`, `internal/db`, `internal/api`.

**Lancer.**

```bash
docker compose up -d crawler
docker compose logs -f crawler
```

**Smoke test.**

```bash
docker compose exec redis wget -qO- http://crawler:8080/health
docker compose exec redis wget -qO- http://crawler:8080/stats
```

**Tests développeur.**

```bash
cd apps/crawler
go test ./...
```

**Critère prêt.** `/ready` vérifie aussi Redis et Neo4j, contrairement à
`/health` qui indique seulement que l’API répond. Quand la file est vide,
le crawler attend : la progression doit être lue avec les échéances.

**Collectseeds.** Le profil facultatif `seeds` consulte toutes les 24 heures des
catalogues publics (dont DeepDarkCTI et Ahmia) pour soumettre des points de
départ à Redis. Il dénombre les résultats, mais c’est le crawler qui vérifie
si les adresses répondent. Distinguer `collectseeds_last_found`,
`collectseeds_last_added` et les admissions différées dans Redis. Ni ces
chiffres ni la découverte n’engagent le futur registre de sources.

### 8.6 Scraper — `scraper`

**Responsabilité.** Réserver une tâche Redis, rendre la page puis sauvegarder le contenu dans MongoDB avant de confirmer le succès.

**Fichiers principaux.** `apps/scraper/cmd/main.go`, `internal/sites`, `internal/db`, `data/sites.json`.

**Lancer.**

```bash
docker compose up -d scraper
docker compose logs -f scraper
```

Le scraper travaille en continu, abandonne Chromium quand il reste inactif
et réessaie après une panne d’infrastructure. À l’arrêt propre, un worker
libère sa réservation ; après un arrêt brutal, la file la récupère à
expiration. Sur un Mac en vraie veille, tout traitement est suspendu.
Voir le [guide Mac et les contrôles de bascule](redis-crawler-scraper.md).

**Tests développeur.**

```bash
cd apps/scraper
go test ./...
```

**Critère prêt.** `--check-health` vérifie Redis et MongoDB ; un succès métier
se mesure dans `scraper:stats` après écriture MongoDB. Un conteneur sain
n’indique pas à lui seul que des pages sont sauvegardées.

**Configuration de connexion optionnelle.** Partir de `apps/scraper/examples/sites.json` et monter un fichier local dédié en lecture seule, jamais le dépôt. Le fichier contient des sélecteurs, chemins d’état et noms de variables, pas les identifiants eux-mêmes. Voir le [guide d’accès du scraper](scraper-access.md).

- Avec `state_file`, `credentials_key_env` vaut par défaut `BWT_CREDENTIALS_KEY`.
- Le fichier d’état lie les identifiants chiffrés à une origine ; ne pas le partager entre sites.
- Les cookies sont restaurés en premier ; après expiration, le compte enregistré sert à la connexion.
- `registration_attempted` évite les inscriptions répétées, même après un timeout.
- Le solveur est un adaptateur HTTP à configurer, pas un service de résolution livré avec le projet.
- Les clés et fichiers d’état sont à sauvegarder séparément ; les cookies ne sont pas chiffrés par cette implémentation.
- La présence d’un compte n’influence pas les verdicts NLP/IA.

Après modification de l’environnement, recréer le scraper :

```bash
docker compose up -d --no-deps --force-recreate scraper
```

Tests complémentaires depuis `apps/scraper` :

```bash
go test -race ./internal/sites
go vet ./...
```

Les tests unitaires couvrent le chiffrement et la restauration, le lien au site, les clés invalides, les permissions, la configuration par défaut et le contrat HTTP du solveur. Les formulaires réels nécessitent encore une validation navigateur.

**Limite importante.** Le code marque actuellement une URL comme scrapée même après un échec, afin d'éviter les boucles infinies. L'opérateur doit donc utiliser les logs pour distinguer succès et abandon ; un statut de tentative persistant reste à concevoir.

### 8.7 Classifieur — `classifier`

**Responsabilité.** Ajouter `relevance.relevant`, `score` et `model_version` aux articles non classés.

**Fichiers principaux.** `apps/classifier/cmd`, `internal`, `data/seed_keywords.json`.

**Préparer un candidat (sur décision explicite).** Depuis `apps/classifier`,
avec MongoDB accessible et un corpus identifié :

```bash
BOOTSTRAP_OUTPUT=/chemin/corpus.csv go run ./cmd/bootstrap
go run ./cmd/checkfilter
```

Attendre la fin du contrôle et lire le résultat : le code de sortie seul
n’indique pas le nombre de correspondances. Si le contrôle échoue ou trouve
une correspondance, arrêter la procédure. Sinon, écrire le candidat et son
rapport hors de l’artefact déployé :

```bash
TRAINING_DATA=/chemin/corpus.csv CANDIDATE_MODEL_PATH=/chemin/candidat.json go run ./cmd/train
```

**Lancer.**

```bash
docker compose up -d classifier
docker compose logs -f classifier
```

**Tests développeur.**

```bash
cd apps/classifier
go test ./cmd/... ./internal/...
```

**Diagnostic.**

```bash
go run ./cmd/diagfp --threshold 0.99 --limit 30
```

**Reprendre une amélioration.** Consulter le [registre des essais](../apps/classifier/EXPERIMENTS.md)
avant toute nouvelle campagne. L’ancienne boucle automatique est archivée et
ne fait plus partie du démarrage ni du runbook opérateur.

**Critère prêt.** Un modèle est chargé, les articles non classés reçoivent un verdict et les logs indiquent sa version et le nombre de documents traités.

### 8.8 API — `api`

**Responsabilité.** `GET /data` renvoie une page JSON de métadonnées des
articles positifs : `id`, `source_type`, `scraped_at`. Ni contenu, ni URL
source, ni IOC, ni détail Mongo ne font partie du contrat. `limit` vaut 25
par défaut, maximum 100. S’il existe une page suivante, transmettre
`X-Next-Cursor` dans le paramètre `cursor` de la requête suivante.

**Configurer.** Copier `apps/api/.env.example` vers `apps/api/.env`.
Avec un compte opérateur MongoDB, créer un utilisateur distinct dans
`BlueWhiteThreat` avec le rôle `read` de cette base, pas de rôle admin ni
d’écriture. Renseigner `MONGODB_URI` avec ce compte, le nom de service
`mongodb` et la base d’authentification correspondante. Encoder dans l’URI
les caractères réservés des identifiants. Le service ne démarre pas sans URI.
Le profil `api` n’est pas lancé par défaut.

```bash
docker compose --profile api up -d api
curl -fsS http://localhost:8083/healthz
curl -fsS http://localhost:8083/readyz
curl -fsS -i 'http://localhost:8083/data?limit=25'
```

`/healthz` teste le serveur ; `/readyz` vérifie que la lecture Mongo est
possible. `/metrics` fournit les agrégats. Cette API locale n’a pas
d’authentification HTTP : ne pas la publier sur une interface externe ni
derrière un proxy accessible à des tiers. Les en-têtes CORS n’autorisent que le front local sur le port 8080.

**Tester.** `(cd apps/api && go test ./...)` ou le
[scénario isolé complet](validation/2026-11-24-pipeline-smoke.md).

### 8.9 Frontend analyste — `front`

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

### 8.10 Inspection locale de MongoDB

Mongo Express et ses identifiants par défaut ont été retirés du Compose.
Pour une inspection ponctuelle, ouvrir `mongosh` via `docker compose exec
mongodb mongosh` avec un compte opérateur approprié. Ne pas publier les
sorties contenant les textes, URL ou données personnelles.

### 8.11 Projection CTI — `parser`

Le parser ne traite automatiquement que les documents classés pertinents
et non marqués pour revue. Ollama local est le chemin par défaut ; Claude
n’est activé qu’avec `PARSER_ENABLE_REMOTE_FALLBACK=true` et une clé fournie
au parser. Cette option peut transmettre le texte au fournisseur : laisser
désactivée pour les données sensibles et la démo hors ligne. Les contextes
de build excluent les fichiers `.env` et les données ; la configuration
privée est injectée au lancement, pas copiée dans les images.

L’extraction est conservée dans `article.entities` et rejetée si le texte
a changé avant l’écriture. La projection Neo4j peut être rejouée sans
relancer l’extraction. Les échecs de projection sont journalisés et remis
en file. Ce rejeu ne supprime pas les anciennes relations après modification
ou déclassement d’un document : cette réconciliation est un chantier distinct.

### 8.12 Services planifiés

Les composants suivants ne sont pas présents dans le code : registre des sources, connecteurs forums/Telegram dédiés, export STIX/TAXII et alertes CTI. Le profil `alerting` existant ne surveille que la file du crawler ; son webhook facultatif `ALERT_WEBHOOK_URL` accepte uniquement une URL Slack HTTPS, sans redirection. Leur fiche opérationnelle devra être ajoutée lorsque leur premier exécutable et leur contrat de configuration existeront.

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
| Le scraper ne traite rien | sonde `--check-health`, files `scraper:pending` et `scraper:inflight` | contrôler MongoDB, Redis, Chromium et la progression, sans purge |
| Une URL est en retard ou échoue | échéance, `scraper:retry_due`, `scraper:dead` et `scraper:blocked` | distinguer retard, échec temporaire, quarantaine et refus |
| Le frontend affiche zéro article | API `/data`, logs API, MongoDB | vérifier l'API avant le frontend |
| La vue graphe échoue | requête `/graph` dans le navigateur | endpoint non disponible actuellement ; utiliser Neo4j Browser pour le diagnostic |
| Mongo Express est inaccessible | logs et mapping de port | corriger le port interne du service avant usage |
| Le crawler ne prend plus de tâche | `/ready`, file prête et échéances | une file vide ou future est normale ; diagnostiquer un processus arrêté |
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

Le premier panneau Grafana conserve un inventaire explicite de `crawler`, `scraper`, `api`, `classifier`, `tor1`, `tor2`, `tor3`, `front`, `mongodb`, `redis`, `neo4j` et des composants de monitoring. Les services non démarrés apparaissent **HORS LIGNE** au lieu de disparaître. Les filtres **Service** et **Conteneur** pilotent ensemble les graphes de performance et les logs.

Cette visibilité ne constitue pas une autorisation de collecte : ne pas démarrer automatiquement le crawler, le scraper ou les proxies sur des sources réelles. Appliquer d'abord les contrôles de périmètre, d'environnement et de contenu décrits dans ce runbook.

**Persistance.** Un arrêt avec `docker compose down` conserve les volumes. L'option `down -v` les supprime : l'historique Prometheus/Loki, les réglages locaux Grafana et les données MongoDB/Neo4j sont alors perdus. Au prochain `up -d`, Docker recrée des volumes vides et Grafana reprovisionne automatiquement le dashboard et ses sources depuis le dépôt. Utiliser `-v` uniquement pour une remise à zéro volontaire.

Le tableau **Crawler et scraper — files Redis** utilise les métriques agrégées du scraper. La cible Prometheus `bwt-crawl-queues` est disponible lorsque le scraper et le monitoring partagent le réseau d’observation ; aucun port scraper n’est publié sur le poste.
