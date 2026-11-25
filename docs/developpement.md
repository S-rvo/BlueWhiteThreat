# Reprendre le développement de BlueWhiteThreat

## Où chercher

| Dossier | Rôle |
| --- | --- |
| `apps/crawler`, `apps/scraper` | Découverte, file d’attente, récupération et extraction du texte. |
| `apps/classifier`, `apps/parser` | Pertinence du document, extraction d’entités, projection vers Neo4j. |
| `apps/api`, `apps/front` | API et interface Nuxt. |
| `apps/proxy`, `monitoring/`, `docker-compose*.yaml` | Services d’infrastructure et d’observabilité. |
| `pkg/csamfilter` | Module Go partagé par les services qui filtrent les URL. |

Le [runbook](runbook-services.md) documente le démarrage des services. Pour le
classifier, commencer par le [registre des essais](../apps/classifier/EXPERIMENTS.md),
puis distinguer les outils maintenus de [`experiments/`](../apps/classifier/experiments/README.md).

## Vérifier sans démarrer la stack

Depuis la racine, avec la version Go déclarée par les modules :

```sh
set -e
for module in apps/api apps/crawler apps/parser apps/scraper pkg/csamfilter; do
  (cd "$module" && go test -mod=readonly ./... && go vet -mod=readonly ./...)
done
(cd apps/classifier && go test -mod=readonly ./cmd/... ./internal/... ./experiments/... &&
  go vet -mod=readonly ./cmd/... ./internal/... ./experiments/...)
```

`go.mod`/`go.sum` restent propres à chaque module. Après une modification des
imports, lancer `go mod tidy` dans le module concerné et relire le diff. Les
suites Go du classifier excluent `data/`, qui peut contenir des snapshots de code.
Les suites d’intégration Docker documentées dans le runbook sont séparées des tests
ci-dessus. Pour le front, utiliser [Node 22 et `npm ci`](../apps/front/README.md).
Pour les tests Python, suivre la [validation du classifier](../apps/classifier/README.md#validation-locale)
et les [vérifications des archives](../apps/classifier/experiments/README.md).

## Garder le dépôt lisible

- Mettre une commande dans `cmd/` lorsqu’elle sert à une opération maintenue,
  avec un exemple dans le runbook. Une analyse ponctuelle du classifier va dans
  `experiments/`, avec son protocole et son verdict dans le registre.
- Conserver les tests de non-régression près du code. Séparer les expériences
  qui prennent des données privées, le réseau ou un modèle local des tests CI.
- Versionner les Dockerfiles, réglages partagés, exemples sans secrets et
  procédures ; garder les `.env`, `docker-compose.override.yaml`, données
  Redis et fichiers d’exploitation propres à un poste hors de Git.
- Préférer une fixture synthétique et un résultat agrégé aux dumps et corpus.
  Un nettoyage ne doit ni réentraîner ni remplacer `model.json`.
- Avant de supprimer un outil, vérifier ses appels dans le code, Compose, la
  CI et les procédures ; un script lancé manuellement n’a pas forcément d’import.
