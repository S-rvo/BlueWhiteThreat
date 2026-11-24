# Crawler : file durable par page

Le crawler collecte les liens HTML et les sitemaps XML sur Tor. Le scraper
reste responsable du rendu Chromium et de l'extraction de contenu. Le scraper transmet les liens du DOM rendu des pages publiques au crawler via une file Redis bornée.

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
- robots.txt activé ; redirections limitées à 10 sauts ; les destinations inter-hôtes sont mises en découverte sans être suivies immédiatement, afin de respecter leur politique robots et leur budget.
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
- Cache robots par URL d'origine : TTL 1 heure, maximum 128 entrées de 256 Kio.
- Sélection des hôtes disponibles par échéance, au maximum 64 candidats par réservation. Réconciliation historique au démarrage uniquement.
- Liens Neo4j écrits par lots de 250, délai de transaction de 30 secondes.
- /ready vérifie Redis et Neo4j ; /health reste une sonde de vie.
- processing_total et processing_milliseconds_total permettent de calculer
  le temps moyen de traitement ; ils ne mesurent pas la latence réseau seule.

### Couverture et limites de validation

- Files physiques Redis par hôte, sélection équitable entre hôtes ; la file
  historique est réconciliée au démarrage pour compatibilité, hors réservation.
- Les empreintes exactes sont regroupées sous content_canonical (TTL de l'index :
  30 jours). Les liens de chaque URL restent explorés, car des liens relatifs
  identiques peuvent désigner des destinations différentes. Pas de déduplication
  sémantique ni de suppression automatique des documents du scraper.
- Les liens JavaScript proviennent du DOM rendu des pages publiques déjà
  visitées par le scraper. La file crawler:rendered_links est bornée à 512
  messages de 64 Kio ; saturation = erreur, pas suppression silencieuse. Les pages
  authentifiées sont exclues. Les liens apparus après la capture nécessitent
  un nouveau passage.
- SIGINT/SIGTERM annulent les requêtes et arrêtent les workers ; les baux non
  acquittés sont repris après expiration. Les transactions Neo4j déjà engagées
  peuvent terminer avant l'arrêt (délai borné par lot).
- /stats/host?host=example.onion expose requêtes, millisecondes cumulées,
  erreurs et compteurs de statut ; rétention 30 jours.
- Les MERGE Neo4j restent idempotents, sans transaction distribuée Redis/Neo4j.

### Tests reproductibles

`scripts/integration.sh` crée et supprime uniquement ses conteneurs temporaires
Redis 7 et Neo4j 5.19, sans volume de production. Le 10 novembre 2026 :
100 tâches durables + écritures du graphe validées en environ 1,13 seconde.
Ce résultat local n'inclut ni réseau Tor ni rendu Chromium.

```sh
CRAWLER_TEST_ONION=http://FIXTURE.onion/ \
CRAWLER_TEST_SOCKS=socks5://127.0.0.1:9050 \
go test -v -count=1 ./internal/crawler -run '^TestTorFixture$'
```

Résultats réels : voir [validation Tor et Chromium](docs/validation-tor-2026-11-10.md).
Le test exige également CRAWLER_TEST_REDIS (instance jetable).
Ce test Tor effectue cinq visites séquentielles espacées, pas un stress test
du réseau. Il reste ignoré sans fixture configurée. La capacité de production
multi-instance et le rendu Chromium bout en bout ne sont pas certifiés par
les tests unitaires ou le test Redis/Neo4j.

### Admission bornée et annulation

Les nouvelles découvertes immédiates sont limitées à 10 000 par hôte et
par jour UTC. Le surplus est conservé dans crawler:scheduled jusqu'au lendemain,
pas rejeté. Ce quota d'admission n'est pas une limite de visites par jour :
le débit de reprise est contrôlé par la file active et le délai par hôte.
L'ancien compteur cumulatif est ignoré, sans effacement des données historiques.
Les URL de plus de 4 096 caractères, plus de 12 paramètres, plus de 24
segments, segments répétés plus de quatre fois et paramètres de session
usuels sont rejetés avec la raison trap ; budget indique un report durable au lendemain.
Ce filtre conservateur peut écarter certaines pages légitimes.
La perte d'un renouvellement de bail annule le fetch et interrompt la boucle
de découverte ; la protection atomique des métadonnées reste en place.


## Corrections du second audit

- Redirections de pages, y compris internes : découverte séparée, robots et
  budget revérifiés avant de visiter la destination. Les boucles sont dédupliquées
  par URL ; les pages de redirection ne sont pas suivies en chaîne dans la tâche.
- 429/5xx de robots : statut et Retry-After transmis au scheduler.
- Cache robots : au maximum 32 Mio de corps, hors en-têtes et structures Go.
- File de rendu : au maximum 32 Mio de payloads pour les nouveaux producteurs.
  Les anciens messages préexistants ne sont pas effacés. Un message trop gros
  ou une extraction en erreur fait échouer la collecte pour permettre une reprise.
- Provenance JavaScript persistée dans Neo4j avant acquittement Redis ;
  une panne du graphe conserve le message.
- Empreinte content_id indépendante de l'URL ; pointeur canonique revérifié
  lors d'une nouvelle association. Ce mécanisme reste exact, pas sémantique.
- Délai entre tâches d'un hôte adapté à la moyenne exponentielle des durées
  de fetch, entre 2 et 60 secondes, avec priorité au backoff en cas d'échec.
- Histogrammes cumulatifs de durée par hôte et retard cumulé des promotions.
  Le retard de promotion ne mesure pas toute l'attente jusqu'au fetch.
- Métadonnée opérateur cti_priority (0..3) dans crawler:page:URL :
  plafonds de revisite stable de 30 jours / 7 jours / 24 heures / 6 heures.
  Aucun modèle entraîné ni verdict du classifier modifié.

Pour migrer : arrêter les anciens workers avant de démarrer cette version ;
le protocole d'indexation incrémentale ne couvre pas les admissions faites
par une ancienne version après la réconciliation de démarrage.
Les tests réels utilisent scripts/integration.sh et des bases jetables.
Ils couvrent quatre workers concurrents, pas plusieurs machines ni la
durabilité après panne Redis/Neo4j. Restent des essais de charge prolongée,
une politique de stockage global et une priorisation CTI apprise/évaluée.

### Durcissement de la frontier et du rendu (2026-11-10)

- L'identité d'hôte est calculée avec `net/url`, indépendamment des paramètres
  de requête. HTTP/HTTPS et les ports explicites 80/443 partagent volontairement
  le même budget. Migration au démarrage des URLs actives, visitées, différées,
  en vol et mortes ; compteurs d'admission idempotents.
- Admission bornée à **1 000 000 URLs connues**, dont **50 000 par hôte**,
  toutes catégories de frontier confondues. Les nouvelles admissions dépassant
  le plafond retournent `storage` et incrémentent `storage_rejected_total`.
  Les URLs rejetées ne sont pas ajoutées au graphe par le worker HTTP.
  Le stock historique n'est pas supprimé : s'il dépasse déjà le plafond,
  l'admission reste fermée. Ce plafond en URLs n'est pas une limite mémoire
  Redis en octets ni une politique de rétention pour MongoDB.
- Chromium public (`cfg == nil`) partage `crawler:host_ready` avec le crawler,
  utilise le même User-Agent, vérifie robots via le proxy Tor, bloque les
  ressources interdites et les autres origines, et espace les requêtes d'au
  moins 2 s (ou `Crawl-delay`). WebSockets et service workers sont désactivés
  pour ce parcours. Bail de 180 s, contexte de rendu de 120 s, libération
  conditionnelle ; robots indisponible ferme le rendu avec délai de reprise.
  Les parcours authentifiés conservent leur politique d'accès distincte et
  n'alimentent pas la découverte publique.
- Les liens rendus sont découpés en messages de 64 KiB, admission atomique
  de tous les morceaux, avec 512 messages maximum (prêts + en vol + reprises).
  Bail de consommation de 10 min, reprise après crash, backoff exponentiel,
  dead-letter après 5 échecs (`crawler:rendered_dead`, 512 messages maximum).
  Livraison **au moins une fois** : admission des URLs et `MERGE` du graphe
  sont idempotents. Un événement défectueux ne bloque plus les autres pages.
- Neo4j crée la contrainte unique `crawler_url_unique` sur `URL.url` au
  démarrage. Des doublons historiques entraînent un échec explicite : aucune
  fusion/destruction automatique de données existantes.

Vérifications reproductibles, sur services jetables uniquement :

```sh
(cd apps/crawler && go test -race -count=1 ./... && go vet ./...)
(cd apps/scraper && go test -race -count=1 ./internal/sites ./internal/db && go vet ./internal/sites ./internal/db)
apps/crawler/scripts/integration.sh
apps/scraper/scripts/render-integration.sh
```

Le script Chromium utilise l'image locale `bluewhitethreat-scraper` (ou
`SCRAPER_TEST_IMAGE`), monte le code en lecture seule et supprime ses propres
conteneurs/réseau. Aucun service de production n'est redémarré.

## Découverte multi-source (2026-11-24)

- `collectseeds` admet les seeds sans sonde Tor préalable : un timeout ne
  supprime plus une découverte avant les retries durables du crawler.
- Extraction des URLs onion v3 (forme 56 caractères, sans validation du
  checksum), HTTPS, chemins et paramètres préservés ; mentions nues converties
  en racines HTTP. La sortie `-dry-run` est triée, sans connexion Tor ni Redis
  (elle consulte toujours les sources clearnet).
- `Onion-Location` conserve son URL complète dans le collecteur et le crawler.
- Les champs de formulaire Ahmia sont lus avec un parseur HTML. Les paramètres
  de session sont retirés des libellés source journalisés.
- Corps de source >8 Mio : erreur explicite, pas de succès sur corps tronqué.
- SIGINT/SIGTERM annulent la collecte et l'attente entre cycles.
- Compose conserve le profil `seeds` opt-in, toutes les catégories par défaut
  et une cadence quotidienne. Aucune activation automatique en production.

### Onion Search Engine (optionnel)

Le connecteur utilise `/api.php`, `X-API-Key`, `q` et `page` selon la
[documentation officielle](https://onionsearchengine.com/api_documentation.php).
Configurer `ONION_SEARCH_API_KEY` dans l'environnement transmis à Compose
(ne pas committer la clé). Sans clé, aucun appel à ce fournisseur.

24 requêtes maximum par défaut (`-search-budget=1..100`), espacées de 2 secondes ; une page de chaque
mot-clé avant les pages suivantes, afin de diversifier les résultats. Avec
24 mots-clés, le budget par défaut couvre donc leur première page. Arrêt du
fournisseur sur erreur HTTP, JSON invalide ou refus applicatif ; résultats
précédents conservés. Les redirections API sont bloquées pour protéger la clé.
Ce quota est par collecte, pas une garantie de quota mensuel multi-instance.
Aucun appel réel à l'API n'est effectué par les tests.

### Réactivation et supervision

`collectseeds -revive-dead=20` réactive atomiquement au plus 20 URLs mortes
depuis au moins 7 jours ; plafond 100, défaut 0 (désactivé). Elles passent par
`crawler:scheduled`, avec les filtres et budgets habituels. Aucune visite
réussie n'est inventée et aucune base n'est purgée.

- `/stats` : `known_url_count`, `known_url_limit`, `dead_revived_total`,
  `collectseeds_last_storage`, `collectseeds_last_trap`,
  `collectseeds_last_budget`, `collectseeds_last_capacity`.
- `crawler:seed_sources` : nombre de nouvelles URLs dans le lot attribuées à
  chaque source lors de sa dernière collecte (première source gagnante, pas
  un indicateur de disponibilité Tor ni le nombre d'admissions Redis).
- Métadonnées des nouvelles admissions : `seed_source`, `seed_discovered_at`
  dans `crawler:page:URL` (rétention 90 jours). Les doublons déjà connus ne
  réécrivent pas cette provenance ; ce n'est pas un historique multi-source.

Les limites globales de stockage, robots et filtres restent inchangés.
Restent à mesurer sur un déploiement réel : gain de nouveaux hôtes accessibles,
coût réseau, équilibre revisites/découvertes et politique d'archivage.
