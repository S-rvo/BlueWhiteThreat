# Expériences du classifier

Ce dossier conserve les outils hors du pipeline courant pour pouvoir relire et
reproduire les anciens audits. Il est exclu du contexte de build Docker et les
commandes actives ne dépendent pas de ses paquets Go.

- `cmd/` : diagnostics ponctuels, comparaisons de modèles ou de vocabulaire,
  validation croisée et tests de parité. Les commandes se lancent depuis
  `apps/classifier` avec `go run ./experiments/cmd/<commande>` ; vérifier les
  options et les chemins de sortie avant exécution.
- `internal/` : paquets Go utilisés exclusivement par ces commandes.
- `annotations/` : politiques d’annotation historiques v3 à v8 et leurs tests.
  Les versions appelées par les orchestrateurs restent dans `scripts/`.
- `python/` : ablations, audits de développement, comparaisons d’encodeurs et
  scores de candidats expérimentaux. Les dépendances spécifiques aux encodeurs
  sont dans `requirements-embeddings.txt`, séparées des tests courants.
- `legacy/` : ancien script de synchronisation et ancienne boucle de
  réentraînement, conservés en texte et hors des procédures opérationnelles.
- `results/` : résultats agrégés relus, sans documents ni scores individuels.

Le [registre des essais](../EXPERIMENTS.md) donne les résultats et les limites.
Ces archives gardent les scripts et les tests, mais ne remplacent pas les
corpus et les manifestes privés d’un run gelé.

Vérifications séparées, depuis `apps/classifier` et avec les dépendances de
test Python installées :

```sh
go test ./experiments/...
go vet ./experiments/...
PYTHONPATH=scripts python3 -B -m unittest discover -s experiments/annotations -p 'test_*.py'
PYTHONPATH=scripts python3 -B -m unittest discover -s experiments/python -p 'test_*.py'
```
