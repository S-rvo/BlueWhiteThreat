# Classifieur de pertinence

`cmd/main` lance le service de production. Le reste de `cmd/`, `internal/`
et `scripts/` contient le code partagé et les outils maintenus : annotation,
évaluation, relecture et entraînement. Les comparaisons et diagnostics historiques
sont regroupés dans [experiments/](experiments/README.md), hors du contexte Docker.
Avant de reprendre l’amélioration du modèle, lire le [registre des essais](EXPERIMENTS.md) :
résultats positifs et négatifs, tests déjà consommés et pistes encore ouvertes.

Pour un entraînement explicitement décidé, écrire le candidat à part depuis ce
répertoire :

```sh
TRAINING_DATA=/chemin/corpus.csv CANDIDATE_MODEL_PATH=/chemin/candidat.json go run ./cmd/train
```

`TRAINING_DATA` choisit le CSV local (`text,label` ou `id,text,label`) ;
`CANDIDATE_MODEL_PATH` choisit le nouveau fichier de sortie. `MODEL_PATH`
désigne le modèle en service, pas la sortie. Les labels doivent être 0 ou 1.
L’entraînement ne modifie ni le fichier source ni le modèle en service.

Le pipeline retire les documents vides et les doublons après tokenisation ;
il exclut entièrement les groupes dont les labels se contredisent. Il sépare
ensuite les groupes en environ 70 % entraînement, 15 % validation et 15 % test
(graine 42).

Le vocabulaire et les IDF sont appris sur l’entraînement uniquement. La commande
cherche sur la validation le seuil au meilleur rappel parmi ceux atteignant
99 % de précision et au moins 3 % de rappel. À défaut, elle maximise F1. Elle
affiche ensuite les métriques de test ; le modèle candidat conserve le bilan de
validation. Les métriques d’un tel découpage ne qualifient pas le modèle en
production. Le fichier historique `model.json.evaluation.json` correspond au
modèle versionné, pas à une nouvelle exécution de `cmd/train`.

Les corpus et les files de revue contiennent du texte collecté : le CSV local
`training_data.csv` produit par `cmd/bootstrap` et le fichier
`review_queue.jsonl` produit par `cmd/export` ne font pas partie du dépôt courant.

Pour enrichir cet export local avec CISA KEV et des références CERT-FR :
`go run ./cmd/export -cti-context`. Le fichier séparé
`review_queue.cti.jsonl` reste privé et ignoré par Git.
Voir [le guide de revue CTI](../../docs/validation/2026-11-24-review-cti.md).

Les labels d’un corpus peuvent mélanger corrections humaines et
heuristiques de bootstrap : les métriques mesurent l'accord avec ces labels.
Pour démontrer une qualité réelle de 95 %, il faut aussi un corpus indépendant,
validé humainement, provenant de nouvelles sources et de nouvelles périodes.
La déduplication actuelle ne garantit pas la séparation des pages quasi
identiques ou d'un même site. Ne pas ajuster les paramètres en fonction du test.

En production, seul `classifier-prd` est nécessaire pour le NLP : le dossier
`apps/classifier` est monté en lecture seule dans `/data`, avec
`MODEL_PATH=/data/model.json`. Aucun conteneur de synchronisation n'est requis.
Le montage du dossier rend visibles les remplacements du fichier modèle.

Le modèle est chargé au démarrage puis rechargé à chaque cycle
(`CLASSIFIER_INTERVAL_MIN`, 10 minutes par défaut). Un nouvel artefact valide
est donc pris en compte sans rebuild ni redémarrage. Publier le modèle par
remplacement atomique du fichier pour éviter une lecture partielle.
Si le rechargement échoue, le cycle est ignoré et une nouvelle tentative est
faite au cycle suivant. Les articles déjà classés sont recalculés lorsque la version du modèle ou de
la politique change. Quand le scraper enregistre un texte différent, il retire
atomiquement l'ancien verdict ; le prochain cycle NLP classe ce nouveau texte.
Un contenu identique conserve son verdict. Une inférence calculée sur un ancien
texte est rejetée si la page a été modifiée entre la lecture et l'écriture. `RELEVANCE_THRESHOLD`, s'il est défini pour le
service, remplace le seuil évalué et rend les métriques du rapport inapplicables.

## Simulation de la conservation

Depuis `apps/classifier`, avec `RETENTION_MONGODB_URI` injectée depuis la
configuration privée d’un compte MongoDB en lecture seule :

```sh
go run ./cmd/main -retention-report
```

Ce mode n’exécute pas de cycle de classification. Il compte les verdicts
existants pour le modèle et la politique actifs, sans lire les textes côté
client ni modifier la base. Voir le [protocole et le relevé](../../docs/validation/2026-11-24-retention-shadow.md).

## Validation locale

Depuis `apps/classifier` :

```sh
go test ./cmd/... ./internal/...
go vet ./cmd/... ./internal/...
python3 -m venv .venv-test
.venv-test/bin/python -m pip install -r requirements-test.txt
.venv-test/bin/python -B -m unittest discover -s scripts -p 'test_*.py'
```

Ces tests n’utilisent pas les artefacts d’expériences locales. Le contexte Docker
du classifier n’embarque que le code Go et la fixture de mots-clés. Pour une
exécution hors Compose, monter le modèle en lecture seule et définir `MODEL_PATH`.

## Résultats et provenance

Résultat local du 16 octobre 2026 : 2 003 documents retenus après nettoyage,
dont 401 au test (94 positifs). Le candidat choisi uniquement sur la validation
utilise les mots/bigrammes, les fréquences brutes, une régularisation de 0,00001
et des classes non pondérées. Seuil : 0,6291371502542825.

| Mesure au test | Résultat |
| --- | ---: |
| Accuracy | 90,77 % |
| Précision | 93,85 % |
| Rappel | 64,89 % |
| F1 | 76,73 % |

L'objectif global de 95 % n'est pas atteint. Le modèle manque notamment
33 des 94 pages positives du test. Les corrections humaines des erreurs et
un enrichissement des exemples positifs restent nécessaires avant de revendiquer
ce niveau de qualité. Ces résultats ne constituent pas une comparaison directe
avec l'ancien score, qui utilisait le même jeu pour régler et mesurer le seuil.

Le store persistant `data/reviewed_labels.csv` utilise désormais les colonnes
`id,label,source` : `human` pour les relectures humaines (y compris toutes les
lignes de l'ancien format) et `llm_reviewed` pour les décisions de Codex.
`cmd/applyreview` accepte `source` dans chaque décision JSONL ; son absence
vaut `human` pour compatibilité. Le batch LLM et ses justifications sont conservés
respectivement dans `data/llm_review_decisions.jsonl` et `data/llm_review_log.jsonl`.
Tant qu'un échantillon des labels `llm_reviewed` n'a pas été audité par un humain,
un `target_95_reached` calculé sur un corpus contenant ces labels ne doit pas
être présenté comme équivalent à la vérification humaine complète décrite
ci-dessus : il mesure aussi l'accord avec les décisions d'un LLM.

## Pilote d'évaluation automatique

Le suivi de l'évaluation autonome est dans [l'issue #34](https://github.com/S-rvo/BlueWhiteThreat/issues/34)
et le [protocole automatique](../../docs/validation/nlp-automatic-evaluation.md).
`cmd/prepareautoeval` fige le modèle et les références, puis prépare une sélection
sans scores. Le collecteur de pilote est `apps/scraper/cmd/collecteval` ; les deux
modèles d'annotation sont exécutés localement via
`scripts/annotate_autoeval.py`. Les données restent sous
`data/automatic-eval-2026-10-19/`, ignoré par Git. Ne pas utiliser `confirmfinal`
pour présenter des annotations IA comme une confirmation humaine.

La [campagne autonome v2](../../docs/validation/nlp-automatic-v2-2026-10-19.md)
ajoute des annotations locales structurées, un corpus nouveau et une calibration
sur validation séparée d'un test réservé. Les données et modèles candidats restent
locaux ; les mesures utilisent une référence IA explicitement identifiée.
