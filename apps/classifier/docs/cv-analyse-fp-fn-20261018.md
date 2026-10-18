# Analyse d'erreurs FP/FN du run de référence : deux leviers lexicaux rejetés, cluster forums non-anglais diagnostiqué

Point de départ : le run de référence `data/cv-reviewed-templates-tfidf-20261016`
(`min-df` 10, graine 20261016, `balanced-only`, familles de plis figées
`data/reviewed-template-fold-families-20261014.json`), TP 126 / FP 41 / TN 299 /
FN 36 au seuil fixe 0,5. Trois pistes explorées sur les 77 erreurs ; aucune
n'a mené à un changement de code conservé.

## 1. Catégorie de crime (carding/fraude financière vs drogue/contrefaçon) : rejetée à l'inspection

Hypothèse initiale : les FN seraient majoritairement du carding/fraude
financière mal reconnu, les FP des pages drogue/contrefaçon au vocabulaire
proche. Lecture manuelle des 77 documents : chevauchement lexical fort des
deux côtés de la frontière, pas de séparation nette par catégorie de crime.
Un contre-exemple direct : une page "DRUGS EMPIRE" est elle-même un FN. Piste
abandonnée avant toute implémentation.

## 2. `s_identity_fraud` (vocabulaire vol d'identité/fullz/blanchiment) : rejetée par validation offline

Piste resserrée sur un vocabulaire plus étroit (identifiants volés,
documents falsifiés) et implémentée comme indicateur structurel dans
`internal/tfidf/structural.go`, sur le même principe que `s_onion_link` ou
`s_cve_mention` :

```
(?i)\b(fake (id|passport)s?|fullz|carded|hacked (wallet|account)s?|
stolen (identity|identities|wallet)|driving licen[cs]e|identity theft|
laundering|launder(ed|ing)? (coins|bitcoin|crypto|money))\b
```

Une CV complète à l'identique du protocole de référence était bloquée :
`data/confirmation-campaign/` (snapshot gelé requis par `cvclassweights`)
est absent de ce worktree, et sa recréation (`prepareconfirmation`) est une
opération de gel explicitement soumise à décision humaine
(`docs/confirmation-nlp.md`). Validation alternative choisie : tester la
regex directement sur le contenu MongoDB intégral (non tronqué) des 41 FP et
36 FN du run de référence, en comptant les occurrences comme indicateur du
sens de l'effet.

| Groupe | Documents touchés | Sens de l'effet |
| --- | ---: | --- |
| FP (41) | 8 | mauvais sens — pousserait des négatifs déjà mal classés encore plus haut |
| FN (36) | 5 | bon sens, mais minoritaire |

Le vocabulaire fullz/fausse pièce d'identité est au moins aussi présent côté
FP que côté FN : des places de marché correctement étiquetées négatives
annoncent elles-mêmes fullz et faux passeports/permis aussi explicitement
que certains FN. La feature a été retirée (`git diff --stat` propre, aucune
trace résiduelle).

## 3. Cluster des 8 FN forums non-anglais : diagnostiqué, deux causes distinctes

Sous-ensemble des FN distingué par la langue et/ou la pauvreté du contenu
extrait : 7haker, probiv.onion, rutorsite, rutordark (russe), xnethacker
(français), Dark Army (anglais), Black Hat Chat ×2 (anglais, pages de
connexion quasi vides). Lecture du contenu intégral de ces 8 documents :
le cluster recouvre en réalité deux problèmes différents.

**a. Écart de vocabulaire multilingue (6/8)** — 7haker, probiv.onion,
rutorsite, rutordark, xnethacker, Dark Army. Le vocabulaire TF-IDF est
dominé par l'anglais ; les lexèmes russes/français informatifs ne passent
pas le seuil `min-df` 10 ou sont écartés du chi-deux plafonné face à leurs
équivalents anglais. Ce n'est pas un effet de dilution par la longueur :
probiv.onion (~40k caractères) et rutorsite/rutordark (~90-100k caractères)
restent sous-notés malgré leur taille.

**b. Extraction pauvre, indépendante de la langue (2/8)** — les deux
Black Hat Chat sont des pages de connexion quasi vides (~1200 caractères,
aucune description de service). Aucun levier lexical ne peut agir : il n'y
a rien à trouver dans le texte extrait. C'est une limite de collecte
(contenu derrière une connexion), pas un problème de modèle.

### Recoupement avec la réserve de vocabulaire déjà testée

`docs/cv-reserve-ecritures-20261015.md` documentait déjà `--script-reserve`
(250 places réservées aux caractéristiques non latines) comme « non
concluant » au niveau agrégé. Relecture des scores OOF de
`data/cv-script-reserve-20261016` et `-20261017` spécifiquement sur ces 8
documents :

| Document | Référence (sans réserve) | Réserve, graine 20261016 | Réserve, graine 20261017 |
| --- | ---: | ---: | ---: |
| 7haker | 0,427 | 0,393 | 0,478 |
| Black Hat Chat #1 | 0,337 | 0,409 | 0,411 |
| Black Hat Chat #2 | 0,356 | 0,430 | 0,443 |
| Dark Army | 0,463 | 0,486 | 0,496 |
| probiv.onion | 0,401 | 0,445 | 0,497 |
| xnethacker | 0,468 | 0,501 | 0,507 |
| rutorsite | 0,381 | 0,433 | 0,430 |
| rutordark | 0,372 | 0,429 | 0,422 |

7 des 8 documents montent sur les deux graines (seul 7haker baisse à la
première graine) ; xnethacker franchit le seuil 0,5 sur les deux graines.
La réserve profite même aux deux pages Black Hat Chat quasi vides, ce qui
nuance la lecture du point (b) ci-dessus : même un contenu très pauvre
contient apparemment quelques jetons non latins suffisants pour profiter
d'un accès au vocabulaire non concurrencé par l'anglais. Mais la matrice de
confusion globale ne bouge quasiment pas entre la référence et la réserve
(41/36 et 37/37 contre 41/36 et 37/37) : le gain sur ce cluster précis est
compensé ailleurs dans les 573 documents d'entraînement. Ceci confirme,
avec un éclairage supplémentaire, la conclusion déjà actée de
`cv-reserve-ecritures-20261015.md` : la réserve cible bien le bon cluster,
mais ne constitue pas un gain net à l'échelle du modèle complet.

## Décision

Aucun changement de code conservé, aucune étiquette modifiée, aucune
campagne de confirmation gelée, aucun modèle exporté. Les deux pistes
lexicales (catégorie de crime, vol d'identité) sont écartées. Le cluster
forums non-anglais est désormais caractérisé en deux sous-causes distinctes
(écart multilingue vs extraction pauvre) ; la seule piste correctrice
identifiée (`--script-reserve` ou un encodeur multilingue, cf.
`docs/cv-embeddinggemma-20261018.md`) était déjà connue et déjà jugée non
concluante au niveau agrégé — ce travail ne change pas cette conclusion,
il en précise la cause.
