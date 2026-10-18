# Classifieur de pertinence

Entraîner depuis ce répertoire :

```sh
go run ./cmd/train
```

`TRAINING_DATA` choisit le CSV (`text,label` ou `id,text,label`). `MODEL_PATH`
choisit le modèle de sortie. Les labels doivent être exactement 0 ou 1.
Le fichier source n'est jamais modifié par l'entraînement.

Le pipeline retire les documents vides et les doublons après tokenisation ;
il exclut entièrement les groupes dont les labels se contredisent. Il sépare
ensuite les documents uniques en environ 60 % entraînement, 20 % validation,
20 % test, en conservant les proportions de classes (graine 42).

Le vocabulaire, les IDF et la sélection des caractéristiques sont appris sur
l'entraînement uniquement. La validation compare 24 configurations : mots et
bigrammes avec ou sans fragments de caractères, fréquences brutes ou
logarithmiques, trois régularisations, classes équilibrées ou non. Les anciens
modèles gardent leur vectorisation d'origine. Le calcul parcimonieux du gradient
évite de parcourir les coordonnées nulles à chaque itération.

Pour chaque candidat, le seuil maximise le rappel sous une contrainte de
précision de 95 % sur la validation. Si aucun seuil ne satisfait cette contrainte,
il maximise F1. La sélection des candidats privilégie ceux qui satisfont la
contrainte, puis leur F1. Ce choix favorise la fiabilité des alertes et peut
réduire sensiblement le rappel. Les 95 % constituent un objectif de validation,
pas une garantie sur de nouvelles données.

Le test est évalué après la sélection. `<MODEL_PATH>.evaluation.json` contient
l'accuracy, la précision, le rappel, F1, les effectifs et les erreurs identifiées
par SHA-256 du texte source. `target_95_reached` n'est vrai que si précision,
rappel et F1 atteignent tous 95 %. Les empreintes permettent de retrouver les
pages à revoir sans recopier leur contenu dans le rapport.

Les labels de `training_data.csv` peuvent mélanger corrections humaines et
heuristiques de bootstrap : les métriques mesurent l'accord avec ces labels.
Pour démontrer une qualité réelle de 95 %, il faut aussi un corpus indépendant,
validé humainement, provenant de nouvelles sources et de nouvelles périodes.
La déduplication actuelle ne garantit pas la séparation des pages quasi
identiques ou d'un même site. Ne pas ajuster les paramètres en fonction du test.

Les modèles sont chargés au démarrage du service : redémarrer le classifieur
pour charger un nouvel artefact. Les articles déjà classés ne sont pas
recalculés automatiquement. `RELEVANCE_THRESHOLD`, s'il est défini pour le
service, remplace le seuil évalué et rend les métriques du rapport inapplicables.

Validation du code : `go test ./...` et `go vet ./...`.

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
