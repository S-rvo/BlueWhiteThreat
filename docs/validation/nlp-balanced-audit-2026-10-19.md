# Audit critique du corpus et du candidat — 2026-10-19

## Verdict

Le candidat améliore fortement les métriques globales, mais son évaluation est
dominée par un raccourci de source et de style. Il ne doit pas être promu dans
son état actuel. Le résultat global de 95,3 % de F1 ne mesure pas correctement
la généralisation vers des textes conversationnels décrivant une menace.

## Problèmes critiques

### 1. La source prédit parfaitement la classe

Dans l'entraînement comme dans la validation, aucune source ne contient les deux
classes. Les sources StackExchange et `target_observation` sont uniquement
négatives ; CERT, CISA et GitHub Advisories sont uniquement positives. Un simple
classifieur « majorité par source » obtient donc **100 % sur la validation**.

Les poids confirment ce raccourci : `vulnerability`, `vendor`, `apply` et
`vendor_instructions` dominent les poids positifs ; `how`, `my`, `what`, `ubuntu`
et les marqueurs interrogatifs dominent les poids négatifs. Le modèle apprend en
partie « avis officiel contre question d'utilisateur », plutôt que « menace
actionnable contre contenu hors sujet ».

Conséquence observée : sur le test final, il détecte 50/50 notices KEV mais
seulement **1/6** positifs Security StackExchange.

### 2. Le contrôle SimHash utilisé par le trainer est trop agressif

Le trainer fusionne les documents avec le SimHash Go à distance <= 10. Les 300
documents d'entrée deviennent seulement 230 groupes. Un audit train/test avec
exactement cette implémentation trouve 170 paires à distance <= 10, impliquant
69 documents du test et 72 références d'entraînement. Pourtant, le contrôle
indépendant par shingles de cinq mots à Jaccard >= 0,80 trouve **zéro** paire, et
le texte normalisé trouve également zéro doublon.

Le seuil SimHash regroupe donc de nombreux documents seulement similaires par
genre ou vocabulaire. Il réduit artificiellement la diversité et son résultat ne
doit pas être interprété comme 170 fuites réelles.

### 3. Deux implémentations de « quasi-doublon » divergent

`build_balanced_corpus.py` utilise BLAKE2b, des tokens uniques et un seuil 3.
Le trainer Go utilise FNV-1a, conserve les répétitions et applique le seuil 10.
Le manifeste du corpus ne contrôle donc pas la même notion que l'entraînement.
Le rapport antérieur « zéro quasi-doublon » est vrai seulement pour
l'implémentation Python, pas pour la règle effective du trainer.

### 4. Le test final reste confondu par la source

Les 50 positifs KEV sont des notices très homogènes. Les 100 questions générales
sont presque toutes négatives. Security StackExchange ne fournit que six
positifs consensuels. Le test respecte le nombre global minimal de positifs, mais
pas un croisement classe × style suffisant. Les scores globaux sont donc trop
faciles à satisfaire.

En outre, la borne basse Wilson du rappel global est **80,7 %**, sous l'objectif
de 90 %. Le rappel ponctuel de 91,1 % franchit de peu le seuil, sans preuve assez
forte que le rappel réel le dépasse.

## Problèmes élevés

- **Bruit d'annotation :** cinq consensus négatifs ont dû être retirés après
  revue, car ils décrivaient explicitement malware, injection, exploitation ou
  évasion. Security StackExchange produit 18,75 % de désaccord/abstention dans le
  pool d'entraînement et 20 % dans le test, contre 8 % environ pour les questions
  générales et 0 % pour KEV.
- **Validation trop petite et non croisée par domaine :** 100 documents seulement,
  avec le même raccourci source/classe que l'entraînement. Le score parfait est un
  signal d'alerte, pas une preuve de robustesse.
- **Calibration réutilisée :** le même jeu choisit la régularisation, le seuil et
  la calibration de Platt. Une calibration distincte ou imbriquée manque.
- **Traçabilité imparfaite du test :** la première création du reçu a échoué avant
  le scoring à cause d'un chemin relatif erroné. L'incident est conservé et les
  empreintes ont été prises immédiatement, mais cette campagne ne constitue pas
  une exécution strictement conforme à un reçu antérieur aux scores.
- **Domaine cible sous-représenté :** les deux positifs `target_observation` ne
  figurent pas dans le corpus final ; le modèle est essentiellement évalué sur
  StackExchange et avis publics.

## Problèmes d'outillage et de rapport

- `prepare_balanced_pool.py` écrit `pool_role=negative_candidate` même pour les
  notices KEV ; cette provenance est incorrecte, même si elle n'entre pas dans le
  prompt d'annotation.
- Les plafonds de source et leurs nombres sont codés en dur dans
  `build_balanced_corpus.py`, ce qui complique leur audit et leur réutilisation.
- `conflicting_groups_excluded` ne décrit pas le nombre de documents fusionnés :
  le passage de 300 à 230 exemples est beaucoup plus important que le compteur
  de six conflits affiché dans la sélection.
- Les artefacts de données sont ignorés par Git ; leurs empreintes existent
  localement, mais le rapport versionné ne suffit pas à les reconstruire sans les
  fichiers privés.

## Plan d'amélioration prioritaire

1. Construire une matrice croisée avec, au minimum, quatre cellules substantielles :
   positifs conversationnels, négatifs conversationnels, positifs officiels et
   négatifs officiels. Viser au moins 100 documents par cellule avant le test.
2. Ajouter des positifs conversationnels décrivant compromission, phishing,
   injection, malware, fuite et mécanismes d'attaque ; ajouter des négatifs de
   style officiel provenant de documentation, notes de version et annonces sans
   menace concrète.
3. Réviser humainement toutes les frontières Security, ainsi qu'un échantillon
   aléatoire stratifié de chaque cellule. Mesurer l'accord et corriger la politique
   avant toute nouvelle annotation massive.
4. Remplacer la fusion SimHash <= 10 par le contrôle shingles existant, ou calibrer
   un seuil SimHash sur des paires dupliquées/non dupliquées vérifiées. Garder les
   groupes événementiels et les segments d'un même parent ensemble.
5. Unifier la déduplication Python/Go dans une seule commande partagée et publier
   séparément doublons exacts, shingles proches et similarité thématique.
6. Faire une validation groupée par source (`leave-one-source-out`) et une
   validation croisée imbriquée. Réserver un jeu distinct à la calibration.
7. Exiger pour la promotion une borne basse de rappel, ainsi qu'un minimum de
   positifs et négatifs dans chaque strate critique. Un exemple raisonnable est
   30 par classe et par style, rappel ponctuel >= 90 %, et borne basse >= 85 %.
8. Refaire ensuite un test vierge avec reçu créé avant tout score. Ne pas réutiliser
   le test final actuel pour sélectionner données, seuil ou hyperparamètres.

## Artefact d'audit

`apps/classifier/experiments/cmd/auditbalanced` applique les implémentations Go réelles de
SimHash et de shingles. Son résultat actuel est conservé dans
`final-test/leakage-audit.json`. Il montre 0 doublon exact, 0 correspondance
Jaccard et 170 paires SimHash <= 10.

## Corrections appliquées après audit

Les correctifs d'outillage suivants sont désormais en place :

- les trainers utilisent `dedup.MergeContentDuplicates`, fondé sur le texte
  normalisé et les shingles de cinq tokens à Jaccard >= 0,80 ; le SimHash
  thématique à distance <= 10 n'est plus utilisé pour réduire l'entraînement ;
- le constructeur Python applique la même règle de shingles et accepte des
  plafonds de sources dans un fichier JSON avec `--source-caps` ;
- les rapports de sélection séparent maintenant documents entrants, groupes
  exacts, groupes de contenu, conflits exacts, conflits proches et fusions de
  même classe ;
- `pool_role` vaut désormais `unlabelled_candidate`, sans préjuger de la classe ;
- le script de split exige quatre cellules `classe × style` et produit leurs
  effectifs dans son manifeste ;
- pour `traincandidate`, 20 % de chaque classe de validation sont réservés de
  façon déterministe à la calibration de Platt. Ils ne participent plus au
  choix de la régularisation ni du seuil.

Tests exécutés : `go test ./...`, compilation de tous les scripts Python et
`git diff --check`. La promotion reste désactivée : le corpus existant ne possède
pas encore assez de positifs conversationnels et de négatifs officiels pour
satisfaire le nouveau split. Un nouveau test vierge restera requis après la
constitution de ces quatre cellules.
