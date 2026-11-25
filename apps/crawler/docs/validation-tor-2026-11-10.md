# Validation Tor et rendu — 10 novembre 2026

Tests effectués sur un service onion temporaire créé pour cette validation.
La page injectait par JavaScript un lien /javascript-link absent du HTML
statique. Redis était une instance temporaire distincte de la production.

## Résultats

- TestTorFixture, détecteur de courses actif : PASS, cinq visites séquentielles
  en 13,585 s, incluant quatre pauses de deux secondes.
- TestTorRenderedFixture dans un conteneur temporaire de l'image du scraper,
  avec /usr/bin/chromium : PASS en 29,223 s. Vérifie le texte produit par JS,
  la source et l'URL absolue du lien transmis dans crawler:rendered_links.
- Tests crawler complets et go vet : PASS.
- Tests scraper internal/sites et internal/db avec -race et go vet : PASS.

## Anomalies révélées

1. Le premier test Tor utilisait le filtre de fermes sans initialiser Redis :
   le test exige désormais CRAWLER_TEST_REDIS et utilise uniquement cette base.
2. Colly v1 modifie la requête reçue par son helper proxy, provoquant une
   course avec net/http même avec une seule visite en cours. Le helper reçoit
   désormais un clone de la requête, dans le crawler et le test.
3. Le navigateur macOS a retourné ERR_BLOCKED_BY_CLIENT, y compris avec
   le chemin Chrome explicite. La cause locale reste non déterminée ; aucune
   politique du navigateur n'a été modifiée. Le test a réussi avec Chromium
   Linux dans un conteneur temporaire de l'image du scraper.

## Reproduction

- Crawler : CRAWLER_TEST_ONION, CRAWLER_TEST_SOCKS (socks5://...), et
  CRAWLER_TEST_REDIS (hôte:port) puis :
  go test -race -count=1 -v ./internal/crawler -run '^TestTorFixture$'
- Scraper : CRAWLER_TEST_ONION, CRAWLER_TEST_SOCKS_HOST (hôte:port),
  CRAWLER_TEST_REDIS, CRAWLER_TEST_CHROME (chemin du binaire) puis :
  go test -race -count=1 -v ./internal/sites -run '^TestTorRenderedFixture$'

Utiliser une page fixture injectant le lien /javascript-link et le texte
rendered. Les tests sont ignorés sans configuration explicite.

Ces résultats valident un parcours Tor/rendu/handoff Redis, pas la charge
multi-instance, la couverture du Web ni une chaîne complète jusqu'au classifier.
Aucun déploiement, commit, push ou écriture dans les bases de production.

## Complément : correctifs de frontier et rendu public

Validation locale du lot suivant (sans commit, déploiement ni mutation des
services de production) :

- `go test -race -count=1 ./...` et `go vet ./...` dans le crawler : PASS.
- `go test -race -count=1 ./internal/sites ./internal/db` et `go vet` sur ces
  packages dans le scraper : PASS.
- Redis 7 / Neo4j 5.19 jetables : PASS, 200 tâches dont 100 traitées par
  quatre workers concurrents ; contrainte réappliquée puis 12 écritures
  simultanées du même nœud, un seul nœud obtenu. Dernière exécution : 2,04 s
  pour le test d'intégration (hors démarrage des services).
- Chromium Linux / Redis jetable : PASS en 6,24 s, attente d'un verrou HTTP
  préexistant, espacement des requêtes, JS exécuté, lien rendu transmis,
  ressource interdite par robots et script tiers jamais reçus par les serveurs.
- Tests de régression : hôte avec query/ports par défaut, migration idempotente
  du stock différé, plafond par hôte, message poison sans blocage du suivant,
  dead-letter après cinq échecs, récupération d'un bail expiré, découpage sans
  perte de 5 000 liens et contrôle robots (429/503/redirect/surdimensionnement).

Le dernier test Chromium utilise des serveurs HTTP locaux dans un conteneur ;
ce n'est pas une nouvelle mesure de performance Tor. Chromium émet des
avertissements de décodage CDP pour la valeur `IPAddressSpace: Loopback`,
absente du schéma du client épinglé ; les assertions réseau/rendu passent.
Ce décalage de versions reste à traiter lors de la mise à jour coordonnée de
Chromium et du client CDP, et n'est pas masqué.

Le plafond porte sur les URLs admises, pas sur la consommation totale en
GiB des bases. Les stocks historiques ne sont pas effacés automatiquement.
Si Neo4j contient déjà des doublons `URL.url`, le démarrage échoue avec une
instruction de migration plutôt que de fusionner arbitrairement les données.
