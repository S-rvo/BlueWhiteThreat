# Test NLP sur des pages de la vraie pipeline — 29 octobre 2026

## Résumé

Test rétrospectif sur 1 537 pages brutes réellement collectées par la pipeline
BlueWhiteThreat entre le 22 et le 27 octobre 2026. Les textes ont été gelés
sans utiliser les verdicts de production. Cinq pages dont le texte avait déjà été
inspecté et leurs groupes hôtes ont été exclus; tout groupe hôte répété a aussi
été exclu. Le lot a été étiqueté à l'aveugle par trois modèles locaux v9.

Résultat : 1 512 consensus (235 positifs, 1 277 négatifs), 23 abstentions et
2 désaccords. Couverture consensus : 98,37 %. Les consensus sont des références
faibles produites automatiquement, pas une vérité terrain humaine.

## Résultats aux seuils gelés

| Modèle | Seuil | TP | FP | FN | TN | Précision | Rappel | F1 | Verdict |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| Production | 0,488787 | 218 | 449 | 17 | 828 | 32,68 % | 92,77 % | 48,34 % | Échec précision |
| Candidat ransomware.live | 0,348493 | 230 | 811 | 5 | 466 | 22,09 % | 97,87 % | 36,05 % | Échec précision |

Critères préenregistrés : au moins 50 consensus de chaque classe, précision
>= 50 %, rappel >= 90 %. L'effectif minimal est atteint. Les deux modèles passent
le critère de rappel et échouent au critère de précision. Le candidat détecte
12 positifs supplémentaires par rapport à la production, mais génère 362 faux
positifs supplémentaires. Il n'est pas promu.

## Décision et limites

- Garder l'artefact et le seuil de production inchangés.
- Ne pas ajuster le seuil ni réentraîner à partir de ce test consommé.
- Le candidat n'est pas qualifié : sa précision est de 22,09 % sur ce lot.
- Les résultats mesurent l'accord avec une majorité d'annotateurs LLM, pas la
  vérité des pages. 25 documents sans consensus sont exclus des métriques.
- Le test est rétrospectif, pas prospectif. L'exclusion exacte des textes déjà
  connus a été faite; les quasi-doublons sémantiques n'ont pas été audités
  séparément.

## Reproductibilité

Tous les artefacts privés et l'objet machine-readable sont dans :
`apps/classifier/data/iteration-2026-10-27-source-refresh/actual-pipeline-retrospective-holdout-v1/`.
Le résultat `comparison-result.json` enregistre empreintes, seuils, métriques,
limites et décision. Les fichiers `production-scores.jsonl` et
`candidate-scores.jsonl` ont été générés sans lire les annotations; le calcul des
métriques a ensuite joint les scores aux consensus v9.
