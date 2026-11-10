# Crawler : file durable par page

Le crawler collecte les liens HTML et les sitemaps XML sur Tor. Le scraper
reste responsable du rendu Chromium et de l'extraction de contenu. Le crawler
ne découvre pas encore les liens produits uniquement par JavaScript.

## Fonctionnement

- Identité par URL complète : chemin et casse préservés, fragment supprimé.
- Une page par tâche ; 4 workers par défaut (CRAWLER_WORKERS : 1 à 32), un bail actif par hôte partagé entre instances.
- Redis Lua : admission atomique, file active limitée à 5 000 entrées ; le
  surplus est conservé dans `crawler:scheduled`, et non supprimé.
- Bail de 10 minutes renouvelé chaque minute, token aléatoire, métadonnées et acquittement protégés contre un ancien
  worker et récupération automatique des baux expirés.
- Intervalle minimum de 2 secondes entre tâches d'un même hôte. Les ressources
  robots.txt et les redirections internes peuvent être demandées au sein de
  cette tâche. Un échec reporte aussi les autres tâches de l'hôte.
- Backoff exponentiel (1 minute initiale), Retry-After jusqu'à 24 heures,
  arrêt après 10 échecs et conservation dans `crawler:dead`.
- Réévaluation : 1 jour au premier passage ; intervalle doublé si stable, divisé par deux si modifié (bornes 1 heure à 30 jours) ; 30 jours pour refus robots ou réponse 4xx définitive.
- Les horodatages de succès sont réservés aux réponses 2xx/304. La remise en
  file et les échecs ne rajeunissent pas artificiellement le contenu.
- ETag et Last-Modified conservés ; requêtes conditionnelles au prochain passage.
- robots.txt activé ; redirections limitées à 10 sauts et au même hôte, avec
  revalidation des filtres. Les migrations vers un autre hôte doivent être
  ajoutées comme nouveaux seeds (nouvelle politique robots).
- Découverte de `/sitemap.xml` depuis les racines, lecture de `loc` dans les
  sitemaps et leurs index. Lecture des directives Sitemap de robots.txt ; décompression des sitemaps `.xml.gz`.
- Maximum 2 Mio décompressés par réponse (dépassement rejeté) et 5 000 liens uniques par page. Les pages qui
  dépassent ces budgets nécessitent une politique dédiée.
- Graphe persisté avant acquittement ; échec de stockage : reprise après bail.

## Déploiement et données historiques

Reconstruire et redémarrer le service crawler avec le processus de déploiement
habituel. Aucun effacement Redis ni migration destructive n'est requis.
Les anciens nœuds de domaine restent valides ; les pages sont créées au fil
du recrawl. Les anciens chemins supprimés par la normalisation historique
seront redécouverts depuis les liens accessibles, pas restaurés rétroactivement.
Prévoir une croissance du graphe et des métadonnées : conserver les découvertes
au-delà du plafond actif consomme du stockage, à superviser et provisionner.

Le débit multi-instance reste à valider avant déploiement à grande échelle. Le verrou par hôte est partagé entre instances,
mais le débit multi-instance n'a pas fait l'objet d'un test de charge.

## Supervision

`/stats` expose la file active, les tâches planifiées, en vol et en échec final,
les compteurs succès/échecs et les admissions différées. L'âge de la dernière
visite représente une visite réussie, pas une simple tentative. `/health` est
une sonde de vie HTTP, pas une sonde de disponibilité Redis/Neo4j.
Les erreurs de stockage sont journalisées et provoquent une reprise après bail.

Pour rejouer une tâche en échec final : diagnostiquer la cause puis retirer
uniquement son membre de `crawler:dead`, son compteur dans `crawler:attempts`,
et la réadmettre avec `AddURLToQueue`. Ne pas purger les bases.

## Validation

```sh
cd apps/crawler
go test -race ./...
go vet ./...
```

Les tests Redis utilisent miniredis isolé, jamais le Redis d'exploitation.
Les tests HTTP utilisent httptest : liens, déduplication de fragments, robots,
429 et Retry-After, 304 et validateurs, redirections et sitemaps. Tests de baux,
fencing, temporisation par hôte, fraîcheur et saturation de la file inclus.
Ces tests ne remplacent pas un essai de bout en bout Tor + Redis réel + Neo4j,
ni un benchmark de couverture, de débit et de mémoire sur un corpus représentatif.


## Durcissement du crawler

- Rotation Tor partagée et synchronisée entre workers.
- Cache robots par URL d'origine : TTL 1 heure, maximum 2 048 entrées.
- Sélection de l'hôte le moins récemment servi parmi toute la file active
  (5 000 entrées maximum hors anciennes données). Ce scan Lua est borné par
  la taille de la file mais reste O(N) : mesurer la latence Redis avant extension.
- Liens Neo4j écrits par lots de 250, délai de transaction de 30 secondes.
- /ready vérifie Redis et Neo4j ; /health reste une sonde de vie.
- processing_total et processing_milliseconds_total permettent de calculer
  le temps moyen de traitement ; ils ne mesurent pas la latence réseau seule.

### Travaux encore ouverts

Ces changements ne constituent pas une validation de charge en production.
Restent à implémenter : files physiques par hôte, déduplication inter-URL, liens du rendu JavaScript, passage contrôlé
des redirections inter-hôtes, métriques de rendement et latence par hôte,
arrêt gracieux avec annulation des requêtes, ainsi que tests d'intégration sur
Redis/Neo4j réels et benchmark Tor. Les écritures Neo4j sont des MERGE
idempotents mais ne sont pas protégées par un fencing transactionnel partagé
avec Redis. Une perte de renouvellement de bail annule les requêtes réseau via contexte ; les
métadonnées et l'acquittement sont rejetés après expiration, les insertions déjà effectuées restent idempotentes.


### Admission bornée et annulation

Les nouvelles découvertes sont limitées à 10 000 URL par hôte via le compteur
persistant crawler:discovery_budget. Il s'agit d'un plafond cumulatif, pas
d'un quota quotidien : une revue opérateur est nécessaire pour l'augmenter
ou réinitialiser le compteur d'un hôte. Les URL existantes restent revisitées.
Les URL de plus de 4 096 caractères, plus de 12 paramètres, plus de 24
segments, segments répétés plus de quatre fois et paramètres de session
usuels sont rejetés avec la raison trap ; budget indique un quota atteint.
Ce filtre conservateur peut écarter certaines pages légitimes.
La perte d'un renouvellement de bail annule le fetch et interrompt la boucle
de découverte ; la protection atomique des métadonnées reste en place.
