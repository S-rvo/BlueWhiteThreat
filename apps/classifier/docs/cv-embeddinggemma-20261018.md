# Encodeur multilingue figé : troisième graine indépendante

`docs/cv-embeddinggemma-20261015.md` comparait EmbeddingGemma à Nomic sur deux
graines (20261016, 20261017) : le gain à rappel élevé était présent aux deux
graines, mais deux répétitions ne suffisaient pas à écarter le bruit d'échantillonnage.
Une troisième graine indépendante (20261018) a été ajoutée avec le même protocole :
cache d'encodeur figé, `min-df` 10, régression équilibrée (`balanced-only`),
mêmes groupes et familles de plis figés (`data/reviewed-template-fold-families-20261014.json`).

| Graine | Encodeur | TP / FP / TN / FN | Précision | Rappel | AP |
| --- | --- | --- | ---: | ---: | ---: |
| 20261016 | Nomic | 124 / 71 / 269 / 38 | 0,636 | 0,765 | 0,8081 |
| 20261016 | EmbeddingGemma | 129 / 66 / 274 / 33 | 0,662 | 0,796 | 0,8186 |
| 20261017 | Nomic | 125 / 73 / 267 / 37 | 0,631 | 0,772 | 0,8011 |
| 20261017 | EmbeddingGemma | 129 / 70 / 270 / 33 | 0,648 | 0,796 | 0,8171 |
| 20261018 | Nomic | 125 / 70 / 270 / 37 | 0,641 | 0,772 | 0,805 |
| 20261018 | EmbeddingGemma | 128 / 70 / 270 / 34 | 0,646 | 0,790 | 0,812 |

Comparaison appariée (`data/paired-ranking-embeddinggemma-20261018.json`), minimum
de faux positifs pour atteindre chaque rappel cible :

| Rappel cible | FP Nomic | FP EmbeddingGemma |
| --- | ---: | ---: |
| 0,25 | 1 | 2 |
| 0,50 | 8 | 12 |
| 0,65 | 31 | 18 |
| 0,75 | 56 | 44 |

La troisième graine reproduit exactement le motif des deux premières : aucun
gain (voire une légère perte) aux rappels bas, un gain net et stable aux
rappels élevés (0,65 et 0,75). Sur trois graines indépendantes, la direction
et l'ordre de grandeur du gain à rappel élevé sont constants ; ce n'est plus
compatible avec du bruit d'échantillonnage seul.

Décision : le signal EmbeddingGemma à rappel élevé est confirmé (3/3 graines),
contrairement à l'état précédent où 2 répétitions laissaient planer un doute.
Cela ne change pas la conclusion de `cv-embeddinggemma-20261015.md` sur la
position de ce candidat par rapport aux meilleurs candidats TF-IDF : aucune
comparaison directe encodeur-vs-TF-IDF n'a été faite ici, seulement
encodeur-vs-encodeur (EmbeddingGemma vs Nomic). Rien n'est exporté, aucun
label modifié, aucun seuil de déploiement choisi.

Rapport appairé : `data/paired-ranking-embeddinggemma-20261018.json` ;
runs bruts : `data/cv-nomic-repeat-20261018/`, `data/cv-embeddinggemma-20261018/`.
