# Checklist de mise en production — lot de novembre 2026

Couvre le passage de `cfe4595` à `188de22` sur `main` : PR #67 (hook
pre-commit), #64 (intégrité des données), #66 (contrat `pkg/contract`) et #65
(endpoint `/graph`). Ce lot a passé la CI, mais il n'a été testé ni en
préproduction ni en production. Le [runbook](runbook-services.md) décrit
l'exploitation courante. Ce document ne liste que ce qui change.

Les commandes supposent qu'on est à la racine du dépôt et que les variables du
`.env` racine sont exportées (`set -a; . ./.env; set +a`).

```sh
mongo() { docker compose exec -T mongodb mongosh -u "$MONGO_INITDB_ROOT_USERNAME" \
  -p "$MONGO_INITDB_ROOT_PASSWORD" --quiet BlueWhiteThreat --eval "$1"; }
cypher() { docker compose exec -T neo4j cypher-shell -u neo4j -p "$NEO4J_PASSWORD" "$1"; }
```

## 1. Avant le déploiement

### Configuration

- [ ] `.env` à la racine : `NEO4J_PASSWORD` est renseigné. `docker compose config`
      échoue sans lui.
- [ ] `apps/scraper/.env` et `.credentials.env` sont présents.
- [ ] `apps/api/.env` contient :
  - `MONGODB_URI` avec un compte MongoDB en **lecture seule**. L'API s'arrête
    au démarrage si cette variable manque.
  - Les variables `NEO4J_*` sont déjà fournies par `docker-compose.yaml`.
    Si on les surcharge sans `NEO4J_URI`, `/graph` répond 503.
- [ ] Le profil `api` est activé si l'API doit tourner (`--profile api`).

### Sauvegardes (obligatoire)

Au démarrage, le parser réécrit des labels et des identifiants dans Neo4j. Le
retour arrière passe donc par les sauvegardes.

- [ ] Faire un dump MongoDB (`mongodump`) et vérifier sa taille.
- [ ] Faire un dump Neo4j, base arrêtée :
      `neo4j-admin database dump neo4j --to-path=…`.
- [ ] Noter l'emplacement des deux dumps : …

### Doublons qui feraient échouer les nouveaux index et contraintes

Chaque requête doit renvoyer **zéro ligne**. Sinon, dédupliquer avant de
déployer.

- [ ] URL d'articles en double (index unique `scraper_url_unique`) :
      ```sh
      mongo 'db.article.aggregate([{$match:{url:{$type:"string"}}},{$group:{_id:"$url",n:{$sum:1}}},{$match:{n:{$gt:1}}},{$limit:20}]).toArray()'
      ```
- [ ] Identifiants `Article` qui entreraient en collision après la migration
      `ObjectID("…")` → `…` :
      ```sh
      cypher 'MATCH (a:Article) WITH CASE WHEN a.id STARTS WITH "ObjectID(\"" THEN substring(a.id,10,size(a.id)-12) ELSE a.id END AS k, count(*) AS c WHERE c > 1 RETURN k, c LIMIT 20'
      ```
- [ ] Valeurs d'entités en double. Lancer cette requête pour chaque label :
      `IP`, `Hash`, `Domain`, `URLIndicator`, `CVE`, `Malware`, `ThreatActor`,
      `Infrastructure`.
      ```sh
      cypher 'MATCH (n:IP) WITH n.value AS v, count(*) AS c WHERE c > 1 RETURN v, c LIMIT 20'
      ```
- [ ] Collisions `URL` → `URLIndicator`. La migration renomme les nœuds `URL`
      qui ont une `value` et pas de `url` :
      ```sh
      cypher 'MATCH (u:URL) WHERE u.value IS NOT NULL AND u.url IS NULL MATCH (i:URLIndicator {value: u.value}) RETURN u.value LIMIT 20'
      ```

## 2. Déploiement

- [ ] `git checkout 188de22` (ou un tag posé sur ce commit).
- [ ] Reconstruire toutes les images : `docker compose build`. Les builds
      partent désormais de la racine (`context: .`) et utilisent les
      `Dockerfile.*.dockerignore`. Une image construite avant ce lot ne contient
      pas `pkg/contract`.
- [ ] Démarrer par étage, comme décrit au [§5.3 du runbook](runbook-services.md) :
      stockage, puis crawler et scraper, puis classifier, parser et API, puis
      front.

## 3. Après le déploiement

### Journaux de démarrage

Aucune de ces lignes ne doit apparaître :

- [ ] Scraper : `ATTENTION index unique article.url non créé`. Si elle apparaît,
      dédupliquer puis redémarrer le scraper.
- [ ] Parser : `Schéma Neo4j incomplet`.
- [ ] Parser : `Index MongoDB du parser`.
- [ ] API : `NEO4J_URI not set`, sauf si c'est voulu.

Ces lignes doivent apparaître :

- [ ] Classifier : `Classifier service started (model version …)`. Noter la
      version : …
- [ ] Classifier : `Processed N articles needing classification by model …`.
- [ ] Parser : `Parser: N articles à traiter`.

### Index, contraintes et migrations

- [ ] Index MongoDB : la commande suivante doit lister `scraper_url_unique`,
      `parser_extraction_queue`, `parser_projection_queue` et
      `parser_graph_projected`.
      ```sh
      mongo 'db.article.getIndexes().map(i => i.name)'
      ```
- [ ] Contraintes Neo4j : `parser_article_id` et une contrainte
      `parser_<label>_value` par label d'entité.
      ```sh
      cypher 'SHOW CONSTRAINTS YIELD name WHERE name STARTS WITH "parser_" RETURN name'
      ```
- [ ] Migrations appliquées. Les deux requêtes doivent renvoyer `0` :
      ```sh
      cypher 'MATCH (a:Article) WHERE a.id STARTS WITH "ObjectID(" RETURN count(a)'
      cypher 'MATCH (u:URL) WHERE u.value IS NOT NULL AND u.url IS NULL RETURN count(u)'
      ```

### Pipeline (contrôler après 30 à 60 min)

- [ ] Les files Redis avancent : `urls_to_crawl`, `scraper:pending` et
      `crawler:stats` évoluent entre deux relevés. Voir le
      [§5.4 du runbook](runbook-services.md).
- [ ] De nouveaux articles arrivent dans MongoDB (`scraped_at` récent).
- [ ] Les extractions en échec restent marginales. Si beaucoup d'articles sont
      en échec ou si `Parser: N échecs consécutifs, cycle d'extraction
      interrompu` revient en boucle, vérifier qu'Ollama répond et que le modèle
      `qwen2.5:14b-instruct` est chargé.
      ```sh
      mongo 'db.article.countDocuments({extraction_failures:{$gte:1}})'
      mongo 'db.article.aggregate([{$match:{extraction_last_error:{$exists:true}}},{$group:{_id:"$extraction_last_error",n:{$sum:1}}},{$sort:{n:-1}},{$limit:5}]).toArray()'
      ```
- [ ] La projection vers le graphe avance : le nombre d'articles avec
      `graph_projected_at` augmente.
      ```sh
      mongo 'db.article.countDocuments({graph_projected_at:{$exists:true}})'
      ```

### API et front

- [ ] `curl -i http://127.0.0.1:8083/healthz` répond `204`.
- [ ] `curl -i http://127.0.0.1:8083/readyz` répond `204`.
- [ ] `curl -s 'http://127.0.0.1:8083/data?limit=5'` renvoie un JSON non vide.
      L'en-tête `X-Next-Cursor` est présent s'il y a plus de 5 articles.
- [ ] `curl -s 'http://127.0.0.1:8083/graph?limit=50'` renvoie des nœuds et des
      liens, sans 503.
- [ ] Sur le front (`:8080`), la page datamap affiche le graphe et la console
      du navigateur ne montre aucune erreur CORS.

## 4. Limite connue

Les entités devenues obsolètes **avant** #64 ne sont pas détectées
rétroactivement. Seuls les changements de contenu postérieurs au déploiement
déclenchent le nettoyage et la rétractation. Pour un graphe parfaitement
propre, il faut une reprojection complète, à décider à part.

## 5. Retour arrière

Le parser de `cfe4595` ne connaît ni `URLIndicator` ni les nouveaux
identifiants `Article`. Revenir au code sans restaurer Neo4j laisserait donc
un graphe incohérent.

1. Arrêter `parser`, `classifier`, `scraper` et `api`.
2. Restaurer le dump Neo4j, puis le dump MongoDB si des écritures posent
   problème.
3. `git checkout cfe4595`, puis `docker compose build` et redémarrer.
4. Les index ajoutés dans MongoDB peuvent rester en place, l'ancien code les
   ignore. Exception : supprimer `scraper_url_unique` si l'ancien scraper doit
   réinsérer des doublons.

## Validation

| Étape | Fait par | Date | Remarques |
| --- | --- | --- | --- |
| Avant déploiement | | | |
| Déploiement | | | |
| Après déploiement | | | |
