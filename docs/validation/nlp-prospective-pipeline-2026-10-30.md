# Test prospectif sur la vraie pipeline — tranche 02 — 30 octobre 2026

## Protocole et couverture

La tranche est issue d'un export sans verdict ni score de MongoDB, après la
coupure fixée le 27 octobre 2026 à 19:14:12 UTC. Tous les 722 documents du lot
ont été collectés après la coupure (27 octobre 19:14:36–30 octobre 07:26 UTC)
et sont répartis sur 722 groupes hôtes distincts. Les textes normalisés déjà
présents dans les corpus connus, la tranche prospective initiale et le lot
rétrospectif ont été exclus. Le gel, les empreintes et seuils sont consignés dans
`apps/classifier/data/iteration-2026-10-27-source-refresh/prospective-production-pipeline-comparison-v1/snapshot-02-freeze-manifest.json`.

L'annotation v9 à l'aveugle est achevée : 705 consensus sur 722 documents
(74 positifs, 631 négatifs), 15 abstentions et 2 désaccords. Couverture :
97,65 %. Les étiquettes sont des majorités de trois LLM locaux, pas des labels
vérifiés par humains. Le lot comprend plus de 50 consensus dans chaque classe,
et passe la couverture minimale préenregistrée.

## Résultats aux seuils figés

| Modèle | Seuil | TP | FP | FN | TN | Précision | Rappel | F1 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Production | 0,488787 | 63 | 215 | 11 | 416 | 22,66 % | 85,14 % | 35,80 % |
| Candidat ransomware.live | 0,348493 | 69 | 359 | 5 | 272 | 16,12 % | 93,24 % | 27,49 % |

Critères gelés : précision >=50 %, rappel >=90 %. La production échoue aux deux.
Le candidat respecte le rappel, mais échoue nettement à la précision. Il détecte
6 vrais positifs de plus que la production, au prix de 144 faux positifs
supplémentaires. **Décision : aucune promotion; le modèle et le seuil de
production restent inchangés.** Aucun paramètre ou seuil n'a été ajusté sur ce
test consommé.

## Interprétation et limites

La sur-prédiction positive observée dans le lot rétrospectif se confirme sur un
flux prospectif temporellement postérieur : le problème n'est pas limité à un
seul export. Le corpus est déséquilibré (74 positifs sur 705 consensus), ce qui
rend la précision très sensible aux faux positifs. Les résultats ne sont pas une
vérité terrain humaine et peuvent refléter des erreurs partagées entre les
annotateurs automatiques. L'exclusion textuelle exacte a été appliquée; aucun
audit sémantique indépendant des quasi-doublons n'a été conduit.

## Suite recommandée

Ne pas réutiliser cette tranche pour entraîner ou ajuster le modèle. Construire
une réserve de développement séparée issue d'autres documents postérieurs à ce
gel, avec un échantillonnage indépendant du score. Priorité : couvrir des
négatifs difficiles du style réel (pages commerciales, annuaires, profils,
services génériques) et des positifs explicites, annotés avec la politique gelée.
Comparer ensuite les représentations/caractéristiques en validation groupée sur
cette réserve de développement. Tout candidat devra passer un futur test
prospectif neuf aux seuils définis avant son annotation.

Artefacts de scores, annotations et sortie machine-readable :
`apps/classifier/data/iteration-2026-10-27-source-refresh/prospective-production-pipeline-comparison-v1/`;
résultat complet : `snapshot-02-comparison-result.json`.
